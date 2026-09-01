import { useCallback, useEffect, useRef, useState } from "react";
import i18n from "@/i18n";

export type VideoStatus = "idle" | "waiting" | "playing" | "unsupported" | "error";

const HEADER_SIZE = 16;
const MAXIMUM_DECODE_QUEUE = 8;
const FRAME_DURATION_MICROSECONDS = 40_000;

function findNalPayload(data: Uint8Array, wantedType: number): Uint8Array | null {
  for (let index = 0; index + 4 < data.length; index += 1) {
    let header = -1;
    if (data[index] === 0 && data[index + 1] === 0 && data[index + 2] === 1) {
      header = index + 3;
    }
    if (
      index + 4 < data.length &&
      data[index] === 0 &&
      data[index + 1] === 0 &&
      data[index + 2] === 0 &&
      data[index + 3] === 1
    ) {
      header = index + 4;
    }
    if (header >= 0 && (data[header] & 0x1f) === wantedType) {
      return data.subarray(header);
    }
  }
  return null;
}

function codecFromSPS(data: Uint8Array): string | null {
  const sps = findNalPayload(data, 7);
  if (!sps || sps.length < 4) return null;
  return `avc1.${[sps[1], sps[2], sps[3]].map((value) => value.toString(16).padStart(2, "0")).join("")}`;
}


function normalizeAnnexBForDecoder(payload: Uint8Array): Uint8Array {
  const trailingAUD = [0, 0, 0, 1, 9, 0x30];
  if (payload.length < trailingAUD.length) return payload;
  const offset = payload.length - trailingAUD.length;
  for (let index = 0; index < trailingAUD.length; index += 1) {
    if (payload[offset + index] !== trailingAUD[index]) return payload;
  }
  return payload.subarray(0, offset);
}

export function useVideoDecoder(deviceId: string | null) {
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const decoderRef = useRef<VideoDecoder | null>(null);
  const generationRef = useRef(0);
  const waitingForKeyFrameRef = useRef(true);
  const decodeChainRef = useRef<Promise<void>>(Promise.resolve());
  const [status, setStatus] = useState<VideoStatus>(deviceId ? "waiting" : "idle");
  const [error, setError] = useState<string | null>(null);

  const resetDecoder = useCallback((nextStatus: VideoStatus = "waiting", message: string | null = null) => {
    generationRef.current += 1;
    waitingForKeyFrameRef.current = true;
    const decoder = decoderRef.current;
    decoderRef.current = null;
    if (decoder && decoder.state !== "closed") decoder.close();
    setStatus(deviceId ? nextStatus : "idle");
    setError(message);
  }, [deviceId]);

  useEffect(() => {
    resetDecoder(deviceId ? "waiting" : "idle");
    decodeChainRef.current = Promise.resolve();
    return () => {
      generationRef.current += 1;
      waitingForKeyFrameRef.current = true;
      const decoder = decoderRef.current;
      decoderRef.current = null;
      if (decoder && decoder.state !== "closed") decoder.close();
    };
  }, [deviceId, resetDecoder]);

  const configureDecoder = useCallback(async (payload: Uint8Array): Promise<VideoDecoder | null> => {
    if (!("VideoDecoder" in window) || !window.isSecureContext) {
      setStatus("unsupported");
      setError(i18n.t("errors.webCodecsRequired"));
      return null;
    }
    const codec = codecFromSPS(payload);
    if (!codec) {
      setStatus("error");
      setError(i18n.t("errors.invalidVideoFormat"));
      return null;
    }
    const config: VideoDecoderConfig = {
      codec,
      optimizeForLatency: true,
      hardwareAcceleration: "prefer-hardware",
    };
    const support = await VideoDecoder.isConfigSupported(config);
    if (!support.supported) {
      setStatus("unsupported");
      setError(i18n.t("errors.codecUnsupported", { codec }));
      return null;
    }
    const generation = generationRef.current;
    const decoder = new VideoDecoder({
      output: (frame) => {
        try {
          if (generation !== generationRef.current) return;
          const canvas = canvasRef.current;
          if (!canvas) return;
          if (canvas.width !== frame.displayWidth || canvas.height !== frame.displayHeight) {
            canvas.width = frame.displayWidth;
            canvas.height = frame.displayHeight;
          }
          const context = canvas.getContext("2d", { alpha: false });
          context?.drawImage(frame, 0, 0, canvas.width, canvas.height);
          setStatus("playing");
          setError(null);
        } finally {
          frame.close();
        }
      },
      error: (decoderError) => resetDecoder("error", decoderError.message),
    });
    decoder.configure(config);
    decoderRef.current = decoder;
    return decoder;
  }, [resetDecoder]);

  const handlePacket = useCallback((packet: ArrayBuffer) => {
    const bytes = new Uint8Array(packet);
    if (bytes.length < HEADER_SIZE || String.fromCharCode(...bytes.subarray(0, 4)) !== "KTV1") return;
    const flags = bytes[4];
    const keyFrame = (flags & 1) !== 0;
    const discontinuity = (flags & 2) !== 0;
    if (discontinuity) {
      resetDecoder("waiting");
      return;
    }
    if (waitingForKeyFrameRef.current && !keyFrame) return;
    const payload = normalizeAnnexBForDecoder(bytes.subarray(HEADER_SIZE));
    const sequence = new DataView(packet).getBigUint64(8, false);
    decodeChainRef.current = decodeChainRef.current.then(async () => {
      let decoder = decoderRef.current;
      if (!decoder) {
        if (!keyFrame) return;
        decoder = await configureDecoder(payload);
        if (!decoder) return;
        waitingForKeyFrameRef.current = false;
      }
      if (decoder.decodeQueueSize >= MAXIMUM_DECODE_QUEUE) {
        await decoder.flush();
      }
      decoder.decode(new EncodedVideoChunk({
        type: keyFrame ? "key" : "delta",
        timestamp: Number(sequence) * FRAME_DURATION_MICROSECONDS,
        data: payload,
      }));
    }).catch((decodeError: unknown) => {
      const message = decodeError instanceof Error ? decodeError.message : i18n.t("errors.videoPlaybackFailed");
      resetDecoder("error", message);
    });
  }, [configureDecoder, resetDecoder]);

  return { canvasRef, handlePacket, status, error };
}
