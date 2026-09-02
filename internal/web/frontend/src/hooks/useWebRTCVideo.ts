import { useEffect, useRef, useState } from "react";
import i18n from "@/i18n";
import type { VideoStatus } from "@/hooks/useVideoDecoder";

interface SignalDescription { type: RTCSdpType; sdp: string }

interface WebRTCConfigResponse {
  ice_servers?: { urls?: string[] }[];
}

async function fetchICEServers(signal: AbortSignal): Promise<RTCIceServer[]> {
  try {
    const response = await fetch("/v1/webrtc/config", { signal });
    if (!response.ok) return [];
    const config = (await response.json()) as WebRTCConfigResponse;
    return (config.ice_servers ?? [])
      .map((server) => ({ urls: server.urls ?? [] }))
      .filter((server) => server.urls.length > 0);
  } catch {
    return [];
  }
}

type LowLatencyRtpReceiver = RTCRtpReceiver & {
  playoutDelayHint?: number | null;
};

const WEBRTC_JITTER_BUFFER_TARGET_MILLISECONDS = 40;
const WEBRTC_ICE_GATHERING_TIMEOUT_MILLISECONDS = 3000;

function configureLowLatencyReceiver(receiver: RTCRtpReceiver): void {
  if ("jitterBufferTarget" in receiver) {
    receiver.jitterBufferTarget = WEBRTC_JITTER_BUFFER_TARGET_MILLISECONDS;
    return;
  }
  const lowLatencyReceiver = receiver as LowLatencyRtpReceiver;
  if ("playoutDelayHint" in lowLatencyReceiver) {
    lowLatencyReceiver.playoutDelayHint = WEBRTC_JITTER_BUFFER_TARGET_MILLISECONDS / 1000;
  }
}

// The server signals with full (non-trickle) SDP: it gathers all ICE candidates
// before answering. The browser must do the same, otherwise the offer we POST
// carries no candidates and the peer has nothing to pair against. Resolve once
// gathering completes, or after a short cap so a stuck candidate (e.g. a slow
// STUN server) doesn't block host/srflx candidates already gathered.
function waitForIceGathering(connection: RTCPeerConnection, timeoutMs: number): Promise<void> {
  if (connection.iceGatheringState === "complete") return Promise.resolve();
  return new Promise((resolve) => {
    const finish = () => {
      connection.removeEventListener("icegatheringstatechange", onStateChange);
      clearTimeout(timer);
      resolve();
    };
    const onStateChange = () => {
      if (connection.iceGatheringState === "complete") finish();
    };
    const timer = setTimeout(finish, timeoutMs);
    connection.addEventListener("icegatheringstatechange", onStateChange);
  });
}

export function useWebRTCVideo(deviceId: string | null, enabled: boolean) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [status, setStatus] = useState<VideoStatus>(enabled && deviceId ? "waiting" : "idle");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled || !deviceId) {
      setStatus("idle"); setError(null); return;
    }
    let cancelled = false;
    const abortController = new AbortController();
    let peer: RTCPeerConnection | null = null;

    const start = async () => {
      setStatus("waiting"); setError(null);
      const iceServers = await fetchICEServers(abortController.signal);
      if (cancelled) return;

      const connection = new RTCPeerConnection(iceServers.length ? { iceServers } : undefined);
      peer = connection;
      const videoTransceiver = connection.addTransceiver("video", { direction: "recvonly" });
      configureLowLatencyReceiver(videoTransceiver.receiver);
      connection.ontrack = (event) => {
        const stream = event.streams[0] ?? new MediaStream([event.track]);
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          void videoRef.current.play().catch(() => undefined);
        }
        if (!cancelled) { setStatus("playing"); setError(null); }
      };
      connection.onconnectionstatechange = () => {
        if (cancelled) return;
        if (connection.connectionState === "failed") {
          setStatus("error"); setError(i18n.t("errors.webrtcConnectionFailed"));
        } else if (connection.connectionState === "disconnected") {
          setStatus("error"); setError(i18n.t("errors.webrtcConnectionDisconnected"));
        }
      };

      const offer = await connection.createOffer();
      await connection.setLocalDescription(offer);
      // Wait for ICE gathering so the posted offer includes candidates; the
      // server answers with a complete SDP and does not trickle back to us.
      await waitForIceGathering(connection, WEBRTC_ICE_GATHERING_TIMEOUT_MILLISECONDS);
      if (cancelled) return;
      const localOffer = connection.localDescription ?? offer;
      const response = await fetch(`/v1/karts/by-id/${encodeURIComponent(deviceId)}/webrtc`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type: localOffer.type, sdp: localOffer.sdp }),
        signal: abortController.signal,
      });
      if (!response.ok) throw new Error((await response.text()) || response.statusText);
      const answer = await response.json() as SignalDescription;
      if (cancelled) return;
      await connection.setRemoteDescription(answer);
    };
    void start().catch((startError: unknown) => {
      if (cancelled) return;
      setStatus("error"); setError(startError instanceof Error ? startError.message : i18n.t("errors.webrtcSetupFailed"));
    });
    return () => {
      cancelled = true;
      abortController.abort();
      if (peer) peer.close();
      if (videoRef.current) videoRef.current.srcObject = null;
    };
  }, [deviceId, enabled]);

  return { videoRef, status, error };
}
