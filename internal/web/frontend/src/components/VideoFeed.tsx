import { Camera, LoaderCircle, ShieldAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import type { VideoStatus } from "@/hooks/useVideoDecoder";
import { cn } from "@/lib/utils";

interface VideoFeedProps {
  deviceId: string | null;
  canvasRef: React.RefObject<HTMLCanvasElement | null>;
  videoRef: React.RefObject<HTMLVideoElement | null>;
  fullscreenRef: React.RefObject<HTMLDivElement | null>;
  fullscreen: boolean;
  mode: "websocket" | "webrtc";
  status: VideoStatus;
  error: string | null;
}

export function VideoFeed({ deviceId, canvasRef, videoRef, fullscreenRef, fullscreen, mode, status, error }: VideoFeedProps) {
  const { t } = useTranslation();

  const mediaClassName = cn(
    status === "playing" ? "object-contain" : "hidden",
    fullscreen
      ? "absolute inset-0 size-full max-h-none max-w-none"
      : "block max-h-full max-w-full",
  );

  return (
    <div
      ref={fullscreenRef}
      className={cn(
        "relative m-3 flex min-h-0 flex-1 items-center justify-center overflow-hidden rounded-xl border shadow-xs md:m-4",
        status === "playing" || fullscreen ? "bg-black" : "bg-card",
        fullscreen && "m-0 size-full max-h-none max-w-none flex-none rounded-none border-0 md:m-0",
        "[:fullscreen]:m-0 [:fullscreen]:size-full [:fullscreen]:max-h-none [:fullscreen]:max-w-none [:fullscreen]:rounded-none [:fullscreen]:border-0",
      )}
    >
      {mode === "websocket" ? (
        <canvas
          ref={canvasRef}
          aria-label={t("video.ariaWebSocket")}
          className={mediaClassName}
        />
      ) : (
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          aria-label={t("video.ariaWebRTC")}
          className={mediaClassName}
        />
      )}
      {status !== "playing" && (
        <Empty className="absolute inset-0 border-0">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              {status === "waiting" ? <LoaderCircle className="animate-spin" /> : status === "unsupported" ? <ShieldAlert /> : <Camera />}
            </EmptyMedia>
            <EmptyTitle>{deviceId ? t("video.loadingVideo") : t("video.selectKart")}</EmptyTitle>
            <EmptyDescription>
              {deviceId
                ? mode === "webrtc"
                  ? t("video.connectingLiveVideo")
                  : t("video.waitingForStream")
                : t("video.chooseKart")}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}
      {error && (
        <Alert variant="destructive" className="absolute inset-x-3 bottom-3">
          <ShieldAlert />
          <AlertTitle>{t("video.cameraUnavailable")}</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
    </div>
  );
}
