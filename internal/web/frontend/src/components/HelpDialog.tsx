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
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import { Separator } from "@/components/ui/separator";

type HelpDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function HelpDialog({ open, onOpenChange }: HelpDialogProps) {
  const { t } = useTranslation();

  const keyboardControls = [
    { action: t("help.accelerate"), keys: ["W", "↑"] },
    { action: t("help.reverse"), keys: ["S", "↓"] },
    { action: t("help.steerLeft"), keys: ["A", "←"] },
    { action: t("help.steerRight"), keys: ["D", "→"] },
    { action: t("help.resetInput"), keys: ["Space"] },
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("help.title")}</DialogTitle>
          <DialogDescription>{t("help.description")}</DialogDescription>
        </DialogHeader>
        <section aria-labelledby="keyboard-controls-help-title">
          <h3 id="keyboard-controls-help-title" className="mb-2 text-sm font-medium">
            {t("help.keyboardInput")}
          </h3>
          <Separator />
          <div className="flex flex-col">
            {keyboardControls.map((control) => (
              <div
                key={control.action}
                className="flex items-center justify-between gap-4 border-b py-2.5 last:border-b-0"
              >
                <span className="text-sm text-muted-foreground">{control.action}</span>
                <KbdGroup>
                  {control.keys.map((key) => <Kbd key={key}>{key}</Kbd>)}
                </KbdGroup>
              </div>
            ))}
          </div>
        </section>
        <section aria-labelledby="gamepad-controls-help-title">
          <h3 id="gamepad-controls-help-title" className="mb-2 text-sm font-medium">
            {t("help.gamepadInput")}
          </h3>
          <Separator />
          <div className="flex flex-col">
            <div className="flex items-start justify-between gap-4 border-b py-2.5">
              <span className="text-sm font-medium">{t("help.triggerProportional")}</span>
              <span className="max-w-64 text-right text-sm text-muted-foreground">
                {t("help.triggerProportionalDescription")}
              </span>
            </div>
            <div className="flex items-start justify-between gap-4 py-2.5">
              <span className="text-sm font-medium">{t("help.buttonArcade")}</span>
              <span className="max-w-64 text-right text-sm text-muted-foreground">
                {t("help.buttonArcadeDescription")}
              </span>
            </div>
          </div>
        </section>
        <DialogFooter>
          <DialogClose asChild><Button variant="outline">{t("common.close")}</Button></DialogClose>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
