import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { Minimize2 } from "lucide-react";
import { GamepadDrive } from "@/components/GamepadDrive";
import { KeyboardDrive } from "@/components/KeyboardDrive";
import { TouchDrive } from "@/components/TouchDrive";
import { VideoFeed } from "@/components/VideoFeed";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import { Switch } from "@/components/ui/switch";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useDeviceControl, useRealtime } from "@/hooks/useRealtime";
import { useVideoDecoder } from "@/hooks/useVideoDecoder";
import { useWebRTCVideo } from "@/hooks/useWebRTCVideo";
import { brandDocumentTitle } from "@/lib/brand";
import { readStoredLimit, resolveKartSlug } from "@/lib/dashboard";
import { readStoredGamepadSettings } from "@/lib/gamepad";

// KartFullscreenPage is a chrome-less, full-window driving view intended to be
// opened in its own tab. It exists because iPhone Safari has no element
// Fullscreen API; a dedicated window gives the same immersive experience.
// Control settings are read from localStorage, shared with the dashboard.
export function KartFullscreenPage() {
  const { t } = useTranslation();
  const { slug = "" } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { devices } = useRealtime();
  const selectedKart = resolveKartSlug(slug, devices);
  const selectedId = selectedKart?.ident ?? null;

  const [touchEnabled, setTouchEnabled] = useState(() => localStorage.getItem("komitake-touch-controls") === "true");
  const [videoMode] = useState<"websocket" | "webrtc">(() =>
    localStorage.getItem("komitake-video-mode") === "websocket" ? "websocket" : "webrtc",
  );
  const keyboardSteeringLimit = readStoredLimit("komitake-keyboard-steering-limit");
  const keyboardThrottleLimit = readStoredLimit("komitake-keyboard-throttle-limit");
  const touchSteeringLimit = readStoredLimit("komitake-touch-steering-limit");
  const touchThrottleLimit = readStoredLimit("komitake-touch-throttle-limit");
  const gamepadSettings = readStoredGamepadSettings(localStorage.getItem("komitake-controller-settings"));

  const webSocketVideo = useVideoDecoder(videoMode === "websocket" ? selectedId : null);
  const webRTCVideo = useWebRTCVideo(selectedId, videoMode === "webrtc");
  const {
    telemetry,
    requestedDriveMode,
    sendDrive,
  } = useDeviceControl(
    selectedId,
    videoMode === "websocket" ? webSocketVideo.handlePacket : undefined,
    videoMode === "websocket",
  );
  const videoStatus = videoMode === "websocket" ? webSocketVideo.status : webRTCVideo.status;
  const videoError = videoMode === "websocket" ? webSocketVideo.error : webRTCVideo.error;

  useEffect(() => {
    document.title = brandDocumentTitle(slug);
  }, [slug]);

  const onTouchEnabledChange = useCallback((enabled: boolean) => {
    setTouchEnabled(enabled);
    localStorage.setItem("komitake-touch-controls", String(enabled));
  }, []);

  const onDrive = useCallback((nextSteer: number, nextThrottle: number) => {
    sendDrive(nextSteer, nextThrottle);
  }, [sendDrive]);

  const driveEnabled = Boolean(selectedId) && Boolean(telemetry?.drive_armed) && requestedDriveMode !== false;

  return (
    <TooltipProvider>
      <div className="theme relative h-dvh w-dvw overflow-hidden bg-black text-foreground">
        <VideoFeed
          deviceId={selectedId}
          canvasRef={webSocketVideo.canvasRef}
          videoRef={webRTCVideo.videoRef}
          fullscreenRef={{ current: null }}
          fullscreen
          mode={videoMode}
          status={videoStatus}
          error={videoError}
          errorAtTop={touchEnabled}
          overlay={
            <TouchDrive
              enabled={touchEnabled && driveEnabled}
              steeringLimit={touchSteeringLimit}
              throttleLimit={touchThrottleLimit}
              onChange={onDrive}
            />
          }
        />

        <div className="pointer-events-none absolute inset-x-0 top-0 z-30 flex items-center justify-between gap-2 p-3">
          <Button
            variant="secondary"
            size="sm"
            className="pointer-events-auto cursor-pointer gap-1.5 opacity-80 hover:opacity-100"
            onClick={() => navigate(`/karts/${encodeURIComponent(slug)}`)}
          >
            <Minimize2 className="size-4" />
            {t("fullscreen.exit")}
          </Button>
          <Field orientation="horizontal" className="pointer-events-auto w-auto rounded-md bg-background/70 px-2 py-1 backdrop-blur-sm">
            <FieldLabel htmlFor="fs-touch" className="cursor-pointer text-xs">{t("topBar.touch")}</FieldLabel>
            <Switch
              id="fs-touch"
              size="sm"
              checked={touchEnabled}
              onCheckedChange={onTouchEnabledChange}
              aria-label={t("touchInput.enableAria")}
            />
          </Field>
        </div>

        <KeyboardDrive
          enabled={driveEnabled}
          steeringLimit={keyboardSteeringLimit}
          throttleLimit={keyboardThrottleLimit}
          onChange={onDrive}
        />
        <GamepadDrive
          enabled={driveEnabled}
          mode={gamepadSettings.mode}
          curves={gamepadSettings.profiles[gamepadSettings.mode]}
          rightStickSteering={gamepadSettings.rightStickSteering}
          onChange={onDrive}
          onConnectionChange={() => {}}
        />
        <Toaster />
      </div>
    </TooltipProvider>
  );
}
