import { useEffect, useRef, useState } from "react";
import i18n from "@/i18n";
import type { VideoStatus } from "@/hooks/useVideoDecoder";

interface SignalDescription { type: RTCSdpType; sdp: string }

type LowLatencyRtpReceiver = RTCRtpReceiver & {
  playoutDelayHint?: number | null;
};

const WEBRTC_JITTER_BUFFER_TARGET_MILLISECONDS = 40;

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

export function useWebRTCVideo(deviceId: string | null, enabled: boolean) {
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const [status, setStatus] = useState<VideoStatus>(enabled && deviceId ? "waiting" : "idle");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled || !deviceId) {
      setStatus("idle"); setError(null); return;
    }
    let cancelled = false;
    const peer = new RTCPeerConnection();
    const videoTransceiver = peer.addTransceiver("video", { direction: "recvonly" });
    configureLowLatencyReceiver(videoTransceiver.receiver);
    peer.ontrack = (event) => {
      const stream = event.streams[0] ?? new MediaStream([event.track]);
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
        void videoRef.current.play().catch(() => undefined);
      }
      if (!cancelled) { setStatus("playing"); setError(null); }
    };
    peer.onconnectionstatechange = () => {
      if (cancelled) return;
      if (peer.connectionState === "failed") {
        setStatus("error"); setError(i18n.t("errors.webrtcConnectionFailed"));
      } else if (peer.connectionState === "disconnected") {
        setStatus("error"); setError(i18n.t("errors.webrtcConnectionDisconnected"));
      }
    };
    const start = async () => {
      setStatus("waiting"); setError(null);
      const offer = await peer.createOffer();
      await peer.setLocalDescription(offer);
      const response = await fetch(`/v1/karts/by-id/${encodeURIComponent(deviceId)}/webrtc`, {
        method: "POST", headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ type: offer.type, sdp: offer.sdp }),
      });
      if (!response.ok) throw new Error((await response.text()) || response.statusText);
      const answer = await response.json() as SignalDescription;
      await peer.setRemoteDescription(answer);
    };
    void start().catch((startError: unknown) => {
      if (cancelled) return;
      setStatus("error"); setError(startError instanceof Error ? startError.message : i18n.t("errors.webrtcSetupFailed"));
    });
    return () => {
      cancelled = true;
      peer.close();
      if (videoRef.current) videoRef.current.srcObject = null;
    };
  }, [deviceId, enabled]);

  return { videoRef, status, error };
}
