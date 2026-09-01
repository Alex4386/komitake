import { ChevronDown, Maximize2, Minimize2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Field, FieldLabel } from "@/components/ui/field";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  Table,
  TableBody,
  TableCell,
  TableRow,
} from "@/components/ui/table";
import { Spinner } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import type { DriveState, Quat, Telemetry, Vec3 } from "@/lib/api";
import { cn } from "@/lib/utils";

type MetricsPanelProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  steer: number;
  throttle: number;
  drive: DriveState | null;
  telemetry: Telemetry | null;
  driveModeEnabled: boolean;
  driveModePending: boolean;
  driveModeAvailable: boolean;
  onDriveModeChange: (enabled: boolean) => void;
  fullscreen: boolean;
  fullscreenAvailable: boolean;
  onFullscreenChange: () => void;
};

export function MetricsPanel({
  open,
  onOpenChange,
  steer,
  throttle,
  drive,
  telemetry,
  driveModeEnabled,
  driveModePending,
  driveModeAvailable,
  onDriveModeChange,
  fullscreen,
  fullscreenAvailable,
  onFullscreenChange,
}: MetricsPanelProps) {
  const { t } = useTranslation();

  const driveMetrics = [
    { label: t("metrics.steer"), value: steer.toFixed(2) },
    { label: t("metrics.throttle"), value: throttle.toFixed(2) },
    { label: t("metrics.brake"), value: (drive?.brake ?? 0).toFixed(2) },
  ];
  const imuMetrics = [
    { label: t("metrics.acceleration"), value: formatVector(telemetry?.accel_mps2, "m/s²") },
    { label: t("metrics.angularVelocity"), value: formatVector(telemetry?.gyro_dps, "°/s") },
    { label: t("metrics.yawPitchRoll"), value: formatEuler(telemetry?.orientation) },
  ];

  return (
    <Collapsible
      open={open}
      onOpenChange={onOpenChange}
      className="shrink-0 border-t bg-background"
    >
      <div className="flex items-center justify-between gap-2 px-3 py-2 md:px-4">
        <CollapsibleTrigger asChild>
          <Button variant="ghost" size="sm">
            {t("metrics.title")}
            <ChevronDown className={cn("rotate-180 transition-transform", open && "rotate-0")} />
          </Button>
        </CollapsibleTrigger>
        <div className="flex items-center gap-3">
          <Field orientation="horizontal" className="w-auto" data-disabled={!driveModeAvailable || driveModePending || undefined}>
            <FieldLabel htmlFor="drive-mode" className="cursor-pointer text-sm">
              {t("metrics.driveMode")}
              {driveModePending && <Spinner />}
            </FieldLabel>
            <Switch
              id="drive-mode"
              size="sm"
              checked={driveModeEnabled}
              onCheckedChange={onDriveModeChange}
              disabled={!driveModeAvailable || driveModePending}
              aria-label={t("metrics.driveMode")}
            />
          </Field>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={onFullscreenChange}
            disabled={!fullscreenAvailable}
            aria-label={fullscreen ? t("metrics.exitFullscreen") : t("metrics.showFullscreen")}
          >
            {fullscreen ? <Minimize2 /> : <Maximize2 />}
          </Button>
        </div>
      </div>
      <CollapsibleContent className="max-h-[40vh] overflow-y-auto border-t px-3 py-2 md:px-4">
        <div className="grid gap-x-8 gap-y-4 sm:grid-cols-2">
          <MetricSection
            title={t("metrics.inputsTitle")}
            description={t("metrics.inputsDescription")}
            metrics={driveMetrics}
          />
          <MetricSection
            title={t("metrics.imuTitle")}
            description={t("metrics.imuDescription")}
            metrics={imuMetrics}
          />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

type Metric = {
  label: string;
  value: string;
};

function MetricSection({
  title,
  description,
  metrics,
}: {
  title: string;
  description: string;
  metrics: Metric[];
}) {
  const sectionId = title.toLowerCase().replace(/\s+/g, "-");

  return (
    <section aria-labelledby={`${sectionId}-metrics-title`}>
      <div className="mb-1 px-2">
        <h3 id={`${sectionId}-metrics-title`} className="text-sm font-medium">
          {title}
        </h3>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      <Table>
        <TableBody>
          {metrics.map((metric) => (
            <TableRow key={metric.label}>
              <TableCell className="text-muted-foreground">{metric.label}</TableCell>
              <TableCell className="text-right font-mono text-xs">{metric.value}</TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </section>
  );
}

function formatVector(value: Vec3 | undefined, unit: string): string {
  if (!value) return "—";
  return `${value.x.toFixed(2)} ${value.y.toFixed(2)} ${value.z.toFixed(2)} ${unit}`;
}

function formatEuler(value: Quat | undefined): string {
  if (!value) return "—";
  const length = Math.hypot(value.i, value.j, value.k, value.r);
  if (length < 1e-12) return "—";

  const normalizedI = value.i / length;
  const normalizedJ = value.j / length;
  const normalizedK = value.k / length;
  const normalizedR = value.r / length;
  const roll = Math.atan2(
    2 * (normalizedR * normalizedI + normalizedJ * normalizedK),
    1 - 2 * (normalizedI * normalizedI + normalizedJ * normalizedJ),
  ) * 180 / Math.PI;
  const pitchInput = 2 * (normalizedR * normalizedJ - normalizedK * normalizedI);
  const pitch = Math.abs(pitchInput) >= 1
    ? Math.sign(pitchInput) * 90
    : Math.asin(pitchInput) * 180 / Math.PI;
  const yaw = Math.atan2(
    2 * (normalizedR * normalizedK + normalizedI * normalizedJ),
    1 - 2 * (normalizedJ * normalizedJ + normalizedK * normalizedK),
  ) * 180 / Math.PI;

  return `${yaw.toFixed(0)}° ${pitch.toFixed(0)}° ${roll.toFixed(0)}°`;
}
