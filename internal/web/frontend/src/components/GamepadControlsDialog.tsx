import { CircleAlert, Gamepad2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/components/ui/field";
import { Separator } from "@/components/ui/separator";
import { Slider } from "@/components/ui/slider";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import type {
  GamepadControlMode,
  GamepadControlSettings,
  GamepadCurveSettings,
} from "@/lib/gamepad";

type GamepadControlsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  connected: boolean;
  settings: GamepadControlSettings;
  onModeChange: (mode: GamepadControlMode) => void;
  onRightStickSteeringChange: (enabled: boolean) => void;
  onCurveChange: (
    mode: GamepadControlMode,
    axis: keyof GamepadCurveSettings,
    power: number,
  ) => void;
};

export function GamepadControlsDialog({
  open,
  onOpenChange,
  connected,
  settings,
  onModeChange,
  onRightStickSteeringChange,
  onCurveChange,
}: GamepadControlsDialogProps) {
  const { t } = useTranslation();

  const steeringMapping = settings.rightStickSteering
    ? t("gamepadInput.bothSticksSteer")
    : t("gamepadInput.leftStickSteers");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("gamepadInput.title")}</DialogTitle>
          <DialogDescription>
            {t("gamepadInput.description")}
          </DialogDescription>
        </DialogHeader>
        <Alert>
          {connected ? <Gamepad2 /> : <CircleAlert />}
          <AlertTitle>{connected ? t("gamepadInput.connected") : t("gamepadInput.notConnected")}</AlertTitle>
          <AlertDescription>
            {connected
              ? t("gamepadInput.connectedDescription")
              : t("gamepadInput.notConnectedDescription")}
          </AlertDescription>
        </Alert>
        <Field orientation="horizontal" className="rounded-lg border p-3">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <FieldLabel htmlFor="right-stick-steering">{t("gamepadInput.rightStickSteering")}</FieldLabel>
            <FieldDescription>
              {t("gamepadInput.rightStickSteeringDescription")}
            </FieldDescription>
          </div>
          <Switch
            id="right-stick-steering"
            checked={settings.rightStickSteering}
            onCheckedChange={onRightStickSteeringChange}
            aria-label={t("gamepadInput.rightStickSteeringAria")}
          />
        </Field>
        <Separator />
        <Tabs value={settings.mode} onValueChange={(value) => {
          if (value === "trigger-proportional" || value === "button-arcade") onModeChange(value);
        }}>
          <TabsList className="w-full">
            <TabsTrigger value="trigger-proportional">{t("gamepadInput.triggerProportional")}</TabsTrigger>
            <TabsTrigger value="button-arcade">{t("gamepadInput.buttonArcade")}</TabsTrigger>
          </TabsList>
          <TabsContent value="trigger-proportional">
            <GamepadProfileSettings
              mode="trigger-proportional"
              mapping={t("gamepadInput.triggerMapping", { steering: steeringMapping })}
              curves={settings.profiles["trigger-proportional"]}
              onCurveChange={onCurveChange}
            />
          </TabsContent>
          <TabsContent value="button-arcade">
            <GamepadProfileSettings
              mode="button-arcade"
              mapping={t("gamepadInput.buttonMapping", { steering: steeringMapping })}
              curves={settings.profiles["button-arcade"]}
              onCurveChange={onCurveChange}
            />
          </TabsContent>
        </Tabs>
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type GamepadProfileSettingsProps = {
  mode: GamepadControlMode;
  mapping: string;
  curves: GamepadCurveSettings;
  onCurveChange: GamepadControlsDialogProps["onCurveChange"];
};

function GamepadProfileSettings({
  mode,
  mapping,
  curves,
  onCurveChange,
}: GamepadProfileSettingsProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-5 pt-2">
      <p className="text-sm text-muted-foreground">{mapping}</p>
      <div className="grid grid-cols-2 gap-4">
        <CurveField
          id={`${mode}-lateral-power`}
          label={t("gamepadInput.steeringSensitivityCurve")}
          description={t("gamepadInput.steeringSensitivityCurveDescription")}
          value={curves.lateralPower}
          onChange={(power) => onCurveChange(mode, "lateralPower", power)}
        />
        <CurveField
          id={`${mode}-longitudinal-power`}
          label={t("gamepadInput.throttleSensitivityCurve")}
          description={mode === "trigger-proportional"
            ? t("gamepadInput.throttleSensitivityTriggerDescription")
            : t("gamepadInput.throttleSensitivityButtonDescription")}
          value={curves.longitudinalPower}
          onChange={(power) => onCurveChange(mode, "longitudinalPower", power)}
        />
      </div>
      <p className="text-xs text-muted-foreground">
        {t("gamepadInput.curveHint")}
      </p>
    </div>
  );
}

type CurveFieldProps = {
  id: string;
  label: string;
  description: string;
  value: number;
  onChange: (value: number) => void;
};

function CurvePlot({ power, label }: { power: number; label: string }) {
  const { t } = useTranslation();
  const curvePoints = Array.from({ length: 41 }, (_, index) => {
    const input = index / 40;
    const output = input ** power;
    return `${(input * 100).toFixed(2)},${((1 - output) * 100).toFixed(2)}`;
  }).join(" ");

  return (
    <svg
      viewBox="0 0 100 100"
      role="img"
      aria-label={t("gamepadInput.responsePlot", { label, power: power.toFixed(1) })}
      className="aspect-square w-full rounded-md bg-muted/40"
    >
      {[25, 50, 75].map((position) => (
        <g key={position} className="stroke-border" strokeWidth="0.75">
          <line x1={position} y1="0" x2={position} y2="100" />
          <line x1="0" y1={position} x2="100" y2={position} />
        </g>
      ))}
      <line
        x1="0"
        y1="100"
        x2="100"
        y2="0"
        className="stroke-muted-foreground/40"
        strokeWidth="1"
        strokeDasharray="3 3"
      />
      <polyline
        points={curvePoints}
        fill="none"
        className="stroke-primary"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}

function CurveField({ id, label, description, value, onChange }: CurveFieldProps) {
  return (
    <Field className="rounded-lg border p-3">
      <div className="flex items-center justify-between gap-4">
        <FieldLabel htmlFor={id}>{label}</FieldLabel>
        <span className="font-mono text-xs text-muted-foreground">{value.toFixed(1)}</span>
      </div>
      <CurvePlot power={value} label={label} />
      <Slider
        id={id}
        min={0.5}
        max={3}
        step={0.1}
        value={[value]}
        onValueChange={(nextValue) => onChange(nextValue[0] ?? 1)}
        aria-label={label}
      />
      <FieldDescription>{description}</FieldDescription>
    </Field>
  );
}
