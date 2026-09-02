import {
  Battery,
  BatteryFull,
  BatteryLow,
  BatteryMedium,
  CircleHelp,
  Gamepad2,
  Hand,
  Keyboard,
  Monitor,
  Moon,
  Radio,
  Settings,
  SignalHigh,
  SignalLow,
  SignalMedium,
  SlidersHorizontal,
  Square,
  Sun,
  Video,
  WifiOff,
  Zap,
} from "lucide-react";
import { useTheme } from "next-themes";
import { KartPicker } from "@/components/KartPicker";
import { LanguageSwitcher } from "@/components/LanguageSwitcher";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu, DropdownMenuCheckboxItem, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem,
  DropdownMenuLabel, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuSeparator,
  DropdownMenuSub, DropdownMenuSubContent, DropdownMenuSubTrigger, DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { Kart, Status, Telemetry } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useNavigate, useLocation } from "react-router-dom";
import { useTranslation } from "react-i18next";

type Props = {
  status: Status | null;
  devices: Kart[];
  telemetry: Telemetry | null;
  selectedId: string | null;
  onSelect: (id: string) => void;
  onPair: () => void;
  onStopPair: () => void;
  videoMode: "websocket" | "webrtc";
  onVideoModeChange: (mode: "websocket" | "webrtc") => void;
  onKeyboardControls: () => void;
  onGamepadControls: () => void;
  onTouchControls: () => void;
  touchEnabled: boolean;
  onTouchEnabledChange: (enabled: boolean) => void;
  onHelp: () => void;
  onAp: () => void;
};

export function TopBar({ status, devices, telemetry, selectedId, onSelect, onPair, onStopPair, videoMode, onVideoModeChange, onKeyboardControls, onGamepadControls, onTouchControls, touchEnabled, onTouchEnabledChange, onHelp, onAp }: Props) {
  const { t } = useTranslation();
  const location = useLocation();
  const selected = devices.find((device) => device.ident === selectedId) ?? null;
  const battery = telemetry?.battery ?? selected?.battery;
  const cableConnected = telemetry?.cable_connected ?? selected?.cable_connected;
  const signalStrength = selected?.signal_dbm;
  const pairing = status?.mode === "pairing";
  const { theme = "dark", resolvedTheme, setTheme } = useTheme();
  const AppearanceIcon = resolvedTheme === "light" ? Sun : Moon;
  const navigate = useNavigate();

  const BatteryIcon = battery === undefined
    ? Battery
    : battery <= 1
      ? BatteryLow
      : battery === 2
        ? BatteryMedium
        : BatteryFull;
  const signalBad = signalStrength !== undefined && signalStrength < -75;
  const SignalIcon = signalStrength === undefined
    ? WifiOff
    : signalStrength >= -60
      ? SignalHigh
      : signalStrength >= -75
        ? SignalMedium
        : SignalLow;
  const batteryLabel = battery === undefined
    ? t("topBar.batteryUnavailable")
    : t("topBar.batteryLevel", { level: battery }) + (cableConnected ? t("topBar.batteryCharging") : "");
  const signalLabel = signalStrength === undefined
    ? t("topBar.signalUnavailable")
    : t("topBar.signalLevel", { level: signalStrength });

  return (
    <header className="flex min-h-14 shrink-0 items-center gap-2 border-b bg-background px-3 md:px-4">
      <div className="flex min-w-0 items-center gap-2">
        <Button variant="ghost" onClick={() => navigate('/')}>
          <h1 className="font-heading hidden shrink-0 text-base font-semibold tracking-tight sm:block">{t("app.title")}</h1>
        </Button>
        <KartPicker devices={devices} selectedId={selectedId} onSelect={onSelect} onPair={onPair} />
      </div>
      <div className="ml-auto flex min-w-0 items-center gap-1.5 overflow-x-auto py-2">
        <div className="flex items-center gap-2 text-muted-foreground">
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                role="img"
                tabIndex={0}
                aria-label={signalLabel}
                className={cn(
                  "inline-flex items-center gap-1.5",
                  signalBad && "text-destructive",
                )}
              >
                <SignalIcon className="size-5" />
                {signalStrength !== undefined && (
                  <span className="hidden font-mono text-xs tabular-nums md:inline">
                    {signalStrength} dBm
                  </span>
                )}
              </span>
            </TooltipTrigger>
            <TooltipContent>{signalLabel}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <span
                role="img"
                tabIndex={0}
                aria-label={batteryLabel}
                className={cn(
                  "relative inline-flex",
                  battery === 1 && "text-yellow-500",
                )}
              >
                <BatteryIcon className="size-5" />
                {cableConnected && (
                  <Zap className="absolute -top-1 -right-1 size-3 rounded-full bg-background p-0.5 text-muted-foreground" />
                )}
              </span>
            </TooltipTrigger>
            <TooltipContent>{batteryLabel}</TooltipContent>
          </Tooltip>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="icon-sm" aria-label={t("topBar.settingsMenu")}><Settings /></Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-56">
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t("topBar.webUiSettings")}</DropdownMenuLabel>
              <DropdownMenuSub>
                <DropdownMenuSubTrigger><Video /> {t("topBar.videoReceiveMethod")}</DropdownMenuSubTrigger>
                <DropdownMenuSubContent>
                  <DropdownMenuRadioGroup value={videoMode} onValueChange={(value) => {
                    if (value === "websocket" || value === "webrtc") onVideoModeChange(value);
                  }}>
                    <DropdownMenuRadioItem value="webrtc">WebRTC</DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="websocket">WebSocket</DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuSub>
                <DropdownMenuSubTrigger><AppearanceIcon /> {t("topBar.theme")}</DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-40">
                  <DropdownMenuRadioGroup value={theme} onValueChange={setTheme}>
                    <DropdownMenuRadioItem value="system"><Monitor /> {t("topBar.themeSystem")}</DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="light"><Sun /> {t("topBar.light")}</DropdownMenuRadioItem>
                    <DropdownMenuRadioItem value="dark"><Moon /> {t("topBar.dark")}</DropdownMenuRadioItem>
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <LanguageSwitcher />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger><Gamepad2 /> {t("topBar.input")}</DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="w-48">
                  <DropdownMenuGroup>
                    <DropdownMenuItem onSelect={onKeyboardControls}><Keyboard /> {t("topBar.keyboard")}</DropdownMenuItem>
                    <DropdownMenuItem onSelect={onGamepadControls}><Gamepad2 /> {t("topBar.gamepad")}</DropdownMenuItem>
                    <DropdownMenuItem onSelect={onTouchControls}><Hand /> {t("topBar.touch")}</DropdownMenuItem>
                    <DropdownMenuCheckboxItem
                      checked={touchEnabled}
                      onCheckedChange={onTouchEnabledChange}
                      onSelect={(event) => event.preventDefault()}
                    >
                      {t("topBar.touchControls")}
                    </DropdownMenuCheckboxItem>
                  </DropdownMenuGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
              <DropdownMenuItem onSelect={onHelp}><CircleHelp /> {t("topBar.help")}</DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuGroup>
              <DropdownMenuLabel>{t("topBar.systemSection")}</DropdownMenuLabel>
              <DropdownMenuItem onSelect={() => navigate("/settings", { state: { from: location.pathname } })}><SlidersHorizontal /> {t("topBar.daemonSettings")}</DropdownMenuItem>
              <DropdownMenuItem onSelect={onAp}><Radio /> {t("topBar.accessPoint")}</DropdownMenuItem>
            </DropdownMenuGroup>
            {pairing && (
              <>
                <DropdownMenuSeparator />
                <DropdownMenuGroup>
                  <DropdownMenuItem onSelect={onStopPair}><Square /> {t("topBar.stopPairing")}</DropdownMenuItem>
                </DropdownMenuGroup>
              </>
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
