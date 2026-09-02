import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { Slider } from "@/components/ui/slider";

type TouchControlsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  touchEnabled: boolean;
  onTouchEnabledChange: (enabled: boolean) => void;
  steeringLimit: number;
  throttleLimit: number;
  onSteeringLimitChange: (limit: number) => void;
  onThrottleLimitChange: (limit: number) => void;
};

export function TouchControlsDialog({
  open,
  onOpenChange,
  touchEnabled,
  onTouchEnabledChange,
  steeringLimit,
  throttleLimit,
  onSteeringLimitChange,
  onThrottleLimitChange,
}: TouchControlsDialogProps) {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("touchInput.title")}</DialogTitle>
          <DialogDescription>{t("touchInput.description")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-5 py-1">
          <Field orientation="horizontal">
            <Checkbox
              id="touch-enabled"
              checked={touchEnabled}
              onCheckedChange={(value) => onTouchEnabledChange(value === true)}
              aria-label={t("touchInput.enableAria")}
            />
            <div className="grid gap-0.5">
              <FieldLabel htmlFor="touch-enabled" className="cursor-pointer">{t("touchInput.enable")}</FieldLabel>
              <FieldDescription>{t("touchInput.enableDescription")}</FieldDescription>
            </div>
          </Field>
          <Field data-disabled={!touchEnabled || undefined}>
            <div className="flex items-center justify-between gap-4">
              <FieldLabel htmlFor="touch-steering-limit">{t("touchInput.steeringLimit")}</FieldLabel>
              <span className="font-mono text-xs text-muted-foreground">{Math.round(steeringLimit * 100)}%</span>
            </div>
            <Slider
              id="touch-steering-limit"
              min={10}
              max={100}
              step={5}
              disabled={!touchEnabled}
              value={[steeringLimit * 100]}
              onValueChange={(value) => onSteeringLimitChange((value[0] ?? 100) / 100)}
              aria-label={t("touchInput.steeringLimitAria")}
            />
            <FieldDescription>{t("touchInput.steeringLimitDescription")}</FieldDescription>
          </Field>
          <Field data-disabled={!touchEnabled || undefined}>
            <div className="flex items-center justify-between gap-4">
              <FieldLabel htmlFor="touch-throttle-limit">{t("touchInput.throttleLimit")}</FieldLabel>
              <span className="font-mono text-xs text-muted-foreground">{Math.round(throttleLimit * 100)}%</span>
            </div>
            <Slider
              id="touch-throttle-limit"
              min={10}
              max={100}
              step={5}
              disabled={!touchEnabled}
              value={[throttleLimit * 100]}
              onValueChange={(value) => onThrottleLimitChange((value[0] ?? 100) / 100)}
              aria-label={t("touchInput.throttleLimitAria")}
            />
            <FieldDescription>{t("touchInput.throttleLimitDescription")}</FieldDescription>
          </Field>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
