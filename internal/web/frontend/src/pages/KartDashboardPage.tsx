import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { AlertCircle } from "lucide-react";
import { ApDialog } from "@/components/ApDialog";
import { GamepadControlsDialog } from "@/components/GamepadControlsDialog";
import { GamepadDrive } from "@/components/GamepadDrive";
import { HelpDialog } from "@/components/HelpDialog";
import { KartLanding } from "@/components/KartLanding";
import { KeyboardDrive } from "@/components/KeyboardDrive";
import { KeyboardControlsDialog } from "@/components/KeyboardControlsDialog";
import { MetricsPanel } from "@/components/MetricsPanel";
import { PairDialog } from "@/components/PairDialog";
import { TopBar } from "@/components/TopBar";
import { TouchDrive } from "@/components/TouchDrive";
import { TouchControlsDialog } from "@/components/TouchControlsDialog";
import { VideoFeed } from "@/components/VideoFeed";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { useDeviceControl, useRealtime } from "@/hooks/useRealtime";
import { useVideoDecoder } from "@/hooks/useVideoDecoder";
import { useWebRTCVideo } from "@/hooks/useWebRTCVideo";
import { api } from "@/lib/api";
import { brandDocumentTitle } from "@/lib/brand";
import { readStoredLimit, resolveKartSlug, kartRouteSlug } from "@/lib/dashboard";
import {
  exitElementFullscreen,
  getFullscreenElement,
  isFullscreenSupported,
  requestElementFullscreen,
  subscribeFullscreenChange,
} from "@/lib/fullscreen";
import { readStoredGamepadSettings, type GamepadControlMode, type GamepadCurveSettings } from "@/lib/gamepad";
import { toast } from "sonner";

export function KartDashboardPage() {
  const { t } = useTranslation();
  const { slug = "" } = useParams<{ slug: string }>();
  const navigate = useNavigate();
  const { status, devices, conn: realtimeConn, error } = useRealtime();
  const selectedKart = resolveKartSlug(slug, devices);
  const selectedId = selectedKart?.ident ?? null;
  const [steer, setSteer] = useState(0);
  const [throttle, setThrottle] = useState(0);
  const [pairOpen, setPairOpen] = useState(false);
  const [keyboardControlsOpen, setKeyboardControlsOpen] = useState(false);
  const [gamepadControlsOpen, setGamepadControlsOpen] = useState(false);
  const [gamepadConnected, setGamepadConnected] = useState(false);
  const [touchControlsOpen, setTouchControlsOpen] = useState(false);
  const [touchEnabled, setTouchEnabled] = useState(() => localStorage.getItem("komitake-touch-controls") === "true");
  const [helpOpen, setHelpOpen] = useState(false);
  const [apOpen, setApOpen] = useState(false);
  const [metricsOpen, setMetricsOpen] = useState(false);
  const [videoFullscreen, setVideoFullscreen] = useState(false);
  const [fullscreenTabPromptOpen, setFullscreenTabPromptOpen] = useState(false);
  const videoFullscreenRef = useRef<HTMLDivElement | null>(null);
  const elementFullscreenSupported = isFullscreenSupported();
  const [keyboardSteeringLimit, setKeyboardSteeringLimit] = useState(() => readStoredLimit("komitake-keyboard-steering-limit"));
  const [keyboardThrottleLimit, setKeyboardThrottleLimit] = useState(() => readStoredLimit("komitake-keyboard-throttle-limit"));
  const [touchSteeringLimit, setTouchSteeringLimit] = useState(() => readStoredLimit("komitake-touch-steering-limit"));
  const [touchThrottleLimit, setTouchThrottleLimit] = useState(() => readStoredLimit("komitake-touch-throttle-limit"));
  const [gamepadSettings, setGamepadSettings] = useState(() => readStoredGamepadSettings(localStorage.getItem("komitake-controller-settings")));
  const [videoMode, setVideoMode] = useState<"websocket" | "webrtc">(() => {
    return localStorage.getItem("komitake-video-mode") === "websocket" ? "websocket" : "webrtc";
  });
  const webSocketVideo = useVideoDecoder(videoMode === "websocket" ? selectedId : null);
  const webRTCVideo = useWebRTCVideo(selectedId, videoMode === "webrtc");
  const {
    drive,
    telemetry,
    conn: deviceConn,
    error: deviceError,
    requestedDriveMode,
    sendDrive,
    sendDriveMode,
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

  useEffect(() => {
    if (!drive) return;
    setSteer(drive.steer);
    setThrottle(drive.throttle);
  }, [drive]);

  useEffect(() => {
    if (telemetry?.drive_armed === false) {
      setSteer(0);
      setThrottle(0);
    }
  }, [telemetry?.drive_armed]);

  useEffect(() => {
    return subscribeFullscreenChange(() => {
      setVideoFullscreen(getFullscreenElement() === videoFullscreenRef.current);
    });
  }, []);

  const onVideoModeChange = useCallback((mode: "websocket" | "webrtc") => {
    setVideoMode(mode);
    localStorage.setItem("komitake-video-mode", mode);
  }, []);

  const onKeyboardSteeringLimitChange = useCallback((limit: number) => {
    setKeyboardSteeringLimit(limit);
    localStorage.setItem("komitake-keyboard-steering-limit", String(limit));
  }, []);

  const onKeyboardThrottleLimitChange = useCallback((limit: number) => {
    setKeyboardThrottleLimit(limit);
    localStorage.setItem("komitake-keyboard-throttle-limit", String(limit));
  }, []);

  const onTouchEnabledChange = useCallback((enabled: boolean) => {
    setTouchEnabled(enabled);
    localStorage.setItem("komitake-touch-controls", String(enabled));
  }, []);

  const onTouchSteeringLimitChange = useCallback((limit: number) => {
    setTouchSteeringLimit(limit);
    localStorage.setItem("komitake-touch-steering-limit", String(limit));
  }, []);

  const onTouchThrottleLimitChange = useCallback((limit: number) => {
    setTouchThrottleLimit(limit);
    localStorage.setItem("komitake-touch-throttle-limit", String(limit));
  }, []);

  const persistGamepadSettings = useCallback((settings: typeof gamepadSettings) => {
    setGamepadSettings(settings);
    localStorage.setItem("komitake-controller-settings", JSON.stringify(settings));
  }, []);

  const onGamepadModeChange = useCallback((mode: GamepadControlMode) => {
    persistGamepadSettings({ ...gamepadSettings, mode });
  }, [gamepadSettings, persistGamepadSettings]);

  const onGamepadRightStickSteeringChange = useCallback((rightStickSteering: boolean) => {
    persistGamepadSettings({ ...gamepadSettings, rightStickSteering });
  }, [gamepadSettings, persistGamepadSettings]);

  const onGamepadCurveChange = useCallback((
    mode: GamepadControlMode,
    axis: keyof GamepadCurveSettings,
    power: number,
  ) => {
    persistGamepadSettings({
      ...gamepadSettings,
      profiles: {
        ...gamepadSettings.profiles,
        [mode]: { ...gamepadSettings.profiles[mode], [axis]: power },
      },
    });
  }, [gamepadSettings, persistGamepadSettings]);

  const onDrive = useCallback((nextSteer: number, nextThrottle: number) => {
    setSteer(nextSteer);
    setThrottle(nextThrottle);
    sendDrive(nextSteer, nextThrottle);
  }, [sendDrive]);

  const onDriveModeChange = useCallback((enabled: boolean) => {
    if (!enabled) {
      setSteer(0);
      setThrottle(0);
    }
    sendDriveMode(enabled);
  }, [sendDriveMode]);

  const onVideoFullscreenChange = useCallback(() => {
    const videoSurface = videoFullscreenRef.current;
    if (!videoSurface) {
      toast.error(t("errors.videoSurfaceNotReady"));
      return;
    }
    if (getFullscreenElement() === videoSurface) {
      void exitElementFullscreen().catch((fullscreenError: unknown) => {
        toast.error(fullscreenError instanceof Error ? fullscreenError.message : t("errors.unableToExitFullscreen"));
      });
      return;
    }
    if (elementFullscreenSupported) {
      void requestElementFullscreen(videoSurface).catch((fullscreenError: unknown) => {
        toast.error(fullscreenError instanceof Error ? fullscreenError.message : t("errors.unableToEnterFullscreen"));
      });
      return;
    }
    // No element Fullscreen API (iPhone Safari). Offer a dedicated fullscreen
    // tab, which reproduces the immersive view including touch controls — the
    // native <video> fullscreen path cannot host our DOM overlay.
    setFullscreenTabPromptOpen(true);
  }, [t, elementFullscreenSupported]);

  const openFullscreenTab = useCallback(() => {
    setFullscreenTabPromptOpen(false);
    if (!selectedKart) return;
    window.open(`/ui/karts/${encodeURIComponent(kartRouteSlug(selectedKart))}/fullscreen`, "_blank", "noopener");
  }, [selectedKart]);

  const stopPairing = async () => {
    try {
      await api.stopPairing();
      setPairOpen(false);
      toast.message(t("pair.stopped"));
    } catch (stopError) {
      toast.error((stopError as Error).message);
    }
  };


  return (
    <TooltipProvider>
      <div className="theme flex h-dvh flex-col overflow-hidden bg-background text-foreground">
        <TopBar
          status={status}
          devices={devices}
          telemetry={telemetry}
          selectedId={selectedId}
          onSelect={(kartId) => {
            const kart = devices.find((candidate) => candidate.ident === kartId);
            if (!kart) return;
            navigate(`/karts/${encodeURIComponent(kartRouteSlug(kart))}`);
          }}
          onPair={() => setPairOpen(true)}
          onStopPair={() => void stopPairing()}
          videoMode={videoMode}
          onVideoModeChange={onVideoModeChange}
          onKeyboardControls={() => setKeyboardControlsOpen(true)}
          onGamepadControls={() => setGamepadControlsOpen(true)}
          onTouchControls={() => setTouchControlsOpen(true)}
          touchEnabled={touchEnabled}
          onTouchEnabledChange={onTouchEnabledChange}
          onHelp={() => setHelpOpen(true)}
          onAp={() => setApOpen(true)}
        />

        {(error || deviceError) && (
          <Alert variant="destructive" className="mx-3 mt-2 md:mx-4">
            <AlertCircle />
            <AlertTitle>{t("errors.connectionError")}</AlertTitle>
            <AlertDescription>{error || deviceError}</AlertDescription>
          </Alert>
        )}

        <main className="relative flex min-h-0 flex-1 flex-col">
          {selectedKart ? (
            <>
              <VideoFeed
                deviceId={selectedId}
                canvasRef={webSocketVideo.canvasRef}
                videoRef={webRTCVideo.videoRef}
                fullscreenRef={videoFullscreenRef}
                fullscreen={videoFullscreen}
                mode={videoMode}
                status={videoStatus}
                error={videoError}
                errorAtTop={touchEnabled}
                onSwitchToWebSocket={() => onVideoModeChange("websocket")}
                overlay={
                  <TouchDrive
                    enabled={touchEnabled && Boolean(selectedId) && Boolean(telemetry?.drive_armed) && requestedDriveMode !== false}
                    steeringLimit={touchSteeringLimit}
                    throttleLimit={touchThrottleLimit}
                    onChange={onDrive}
                  />
                }
              />
              <MetricsPanel
                open={metricsOpen}
                onOpenChange={setMetricsOpen}
                steer={steer}
                throttle={throttle}
                drive={drive}
                telemetry={telemetry}
                driveModeEnabled={requestedDriveMode ?? (telemetry?.drive_armed ?? false)}
                driveModePending={requestedDriveMode !== null}
                driveModeAvailable={deviceConn === "open"}
                onDriveModeChange={onDriveModeChange}
                fullscreen={videoFullscreen}
                fullscreenAvailable
                onFullscreenChange={onVideoFullscreenChange}
              />
            </>
          ) : (
            <KartLanding
              unavailableSlug={slug}
              loading={realtimeConn !== "open"}
              onReturn={() => navigate("/", { replace: true })}
            />
          )}
        </main>

        <KeyboardDrive
          enabled={Boolean(selectedId) && Boolean(telemetry?.drive_armed) && requestedDriveMode !== false}
          steeringLimit={keyboardSteeringLimit}
          throttleLimit={keyboardThrottleLimit}
          onChange={onDrive}
        />
        <GamepadDrive
          enabled={Boolean(selectedId) && Boolean(telemetry?.drive_armed) && requestedDriveMode !== false}
          mode={gamepadSettings.mode}
          curves={gamepadSettings.profiles[gamepadSettings.mode]}
          rightStickSteering={gamepadSettings.rightStickSteering}
          onChange={onDrive}
          onConnectionChange={setGamepadConnected}
        />
        <KeyboardControlsDialog
          open={keyboardControlsOpen}
          onOpenChange={setKeyboardControlsOpen}
          steeringLimit={keyboardSteeringLimit}
          throttleLimit={keyboardThrottleLimit}
          onSteeringLimitChange={onKeyboardSteeringLimitChange}
          onThrottleLimitChange={onKeyboardThrottleLimitChange}
        />
        <GamepadControlsDialog
          open={gamepadControlsOpen}
          onOpenChange={setGamepadControlsOpen}
          connected={gamepadConnected}
          settings={gamepadSettings}
          onModeChange={onGamepadModeChange}
          onRightStickSteeringChange={onGamepadRightStickSteeringChange}
          onCurveChange={onGamepadCurveChange}
        />
        <TouchControlsDialog
          open={touchControlsOpen}
          onOpenChange={setTouchControlsOpen}
          touchEnabled={touchEnabled}
          onTouchEnabledChange={onTouchEnabledChange}
          steeringLimit={touchSteeringLimit}
          throttleLimit={touchThrottleLimit}
          onSteeringLimitChange={onTouchSteeringLimitChange}
          onThrottleLimitChange={onTouchThrottleLimitChange}
        />
        <Dialog open={fullscreenTabPromptOpen} onOpenChange={setFullscreenTabPromptOpen}>
          <DialogContent className="sm:max-w-md">
            <DialogHeader>
              <DialogTitle>{t("fullscreen.promptTitle")}</DialogTitle>
              <DialogDescription>{t("fullscreen.promptDescription")}</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setFullscreenTabPromptOpen(false)}>{t("common.no")}</Button>
              <Button onClick={openFullscreenTab}>{t("common.yes")}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        <HelpDialog open={helpOpen} onOpenChange={setHelpOpen} />
        <PairDialog open={pairOpen} onOpenChange={setPairOpen} status={status} />
        <ApDialog open={apOpen} onOpenChange={setApOpen} status={status} />
        <Toaster />
      </div>
    </TooltipProvider>
  );
}

