import { useTranslation } from "react-i18next";
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
import { Slider } from "@/components/ui/slider";

type KeyboardControlsDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  steeringLimit: number;
  throttleLimit: number;
  onSteeringLimitChange: (limit: number) => void;
  onThrottleLimitChange: (limit: number) => void;
};

export function KeyboardControlsDialog({
  open,
  onOpenChange,
  steeringLimit,
  throttleLimit,
  onSteeringLimitChange,
  onThrottleLimitChange,
}: KeyboardControlsDialogProps) {
  const { t } = useTranslation();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[calc(100svh-2rem)] overflow-y-auto sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("keyboardInput.title")}</DialogTitle>
          <DialogDescription>
            {t("keyboardInput.description")}
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-5 py-1">
          <Field>
            <div className="flex items-center justify-between gap-4">
              <FieldLabel id="keyboard-steering-limit-label" htmlFor="keyboard-steering-limit">{t("keyboardInput.steeringLimit")}</FieldLabel>
              <span className="font-mono text-xs text-muted-foreground">{Math.round(steeringLimit * 100)}%</span>
            </div>
            <Slider
              id="keyboard-steering-limit"
              min={10}
              max={100}
              step={5}
              value={[steeringLimit * 100]}
              onValueChange={(value) => onSteeringLimitChange((value[0] ?? 100) / 100)}
              aria-label={t("keyboardInput.steeringLimitAria")}
            />
            <FieldDescription>{t("keyboardInput.steeringLimitDescription")}</FieldDescription>
          </Field>
          <Field>
            <div className="flex items-center justify-between gap-4">
              <FieldLabel id="keyboard-throttle-limit-label" htmlFor="keyboard-throttle-limit">{t("keyboardInput.throttleLimit")}</FieldLabel>
              <span className="font-mono text-xs text-muted-foreground">{Math.round(throttleLimit * 100)}%</span>
            </div>
            <Slider
              id="keyboard-throttle-limit"
              min={10}
              max={100}
              step={5}
              value={[throttleLimit * 100]}
              onValueChange={(value) => onThrottleLimitChange((value[0] ?? 100) / 100)}
              aria-label={t("keyboardInput.throttleLimitAria")}
            />
            <FieldDescription>{t("keyboardInput.throttleLimitDescription")}</FieldDescription>
          </Field>
        </div>
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
