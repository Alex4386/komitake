import type React from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";

type TouchDriveProps = {
  enabled: boolean;
  steeringLimit: number;
  throttleLimit: number;
  onChange: (steer: number, throttle: number) => void;
};

type Control = "left" | "right" | "brake" | "gas";

export function TouchDrive({ enabled, steeringLimit, throttleLimit, onChange }: TouchDriveProps) {
  const { t } = useTranslation();
  const [active, setActive] = useState<Set<Control>>(() => new Set());
  const onChangeRef = useRef(onChange);
  const steeringLimitRef = useRef(steeringLimit);
  const throttleLimitRef = useRef(throttleLimit);
  onChangeRef.current = onChange;
  steeringLimitRef.current = steeringLimit;
  throttleLimitRef.current = throttleLimit;

  const publish = useCallback((controls: Set<Control>) => {
    let steer = 0;
    let throttle = 0;
    if (controls.has("left")) steer -= 1;
    if (controls.has("right")) steer += 1;
    if (controls.has("gas")) throttle += 1;
    if (controls.has("brake")) throttle -= 1;
    onChangeRef.current(steer * steeringLimitRef.current, throttle * throttleLimitRef.current);
  }, []);

  useEffect(() => {
    if (enabled) return;
    setActive((prev) => (prev.size === 0 ? prev : new Set()));
    onChangeRef.current(0, 0);
  }, [enabled]);

  const press = useCallback((control: Control) => {
    setActive((prev) => {
      if (prev.has(control)) return prev;
      const next = new Set(prev);
      next.add(control);
      publish(next);
      return next;
    });
  }, [publish]);

  const release = useCallback((control: Control) => {
    setActive((prev) => {
      if (!prev.has(control)) return prev;
      const next = new Set(prev);
      next.delete(control);
      publish(next);
      return next;
    });
  }, [publish]);

  if (!enabled) return null;

  return (
    <div className="pointer-events-none absolute inset-x-0 bottom-0 z-20 flex items-end justify-between gap-3 p-4 pb-[max(1rem,env(safe-area-inset-bottom))] sm:p-6">
      <div className="flex gap-3">
        <TouchButton
          label={t("touchInput.steerLeft")}
          onPress={() => press("left")}
          onRelease={() => release("left")}
          activeState={active.has("left")}
        >
          <ChevronLeft className="size-8" />
        </TouchButton>
        <TouchButton
          label={t("touchInput.steerRight")}
          onPress={() => press("right")}
          onRelease={() => release("right")}
          activeState={active.has("right")}
        >
          <ChevronRight className="size-8" />
        </TouchButton>
      </div>
      <div className="flex gap-3">
        <TouchButton
          label={t("touchInput.brake")}
          onPress={() => press("brake")}
          onRelease={() => release("brake")}
          activeState={active.has("brake")}
          variant="brake"
        >
          <span className="text-sm font-semibold uppercase tracking-wide">{t("touchInput.brakeShort")}</span>
        </TouchButton>
        <TouchButton
          label={t("touchInput.gas")}
          onPress={() => press("gas")}
          onRelease={() => release("gas")}
          activeState={active.has("gas")}
          variant="gas"
        >
          <span className="text-sm font-semibold uppercase tracking-wide">{t("touchInput.gasShort")}</span>
        </TouchButton>
      </div>
    </div>
  );
}

type TouchButtonProps = {
  label: string;
  onPress: () => void;
  onRelease: () => void;
  activeState: boolean;
  variant?: "steer" | "brake" | "gas";
  children: React.ReactNode;
};

function TouchButton({ label, onPress, onRelease, activeState, variant = "steer", children }: TouchButtonProps) {
  const pointerRef = useRef<number | null>(null);

  const handleDown = (event: React.PointerEvent<HTMLButtonElement>) => {
    event.preventDefault();
    pointerRef.current = event.pointerId;
    event.currentTarget.setPointerCapture(event.pointerId);
    onPress();
  };
  const handleUp = (event: React.PointerEvent<HTMLButtonElement>) => {
    if (pointerRef.current !== event.pointerId) return;
    pointerRef.current = null;
    onRelease();
  };

  return (
    <button
      type="button"
      aria-label={label}
      aria-pressed={activeState}
      onPointerDown={handleDown}
      onPointerUp={handleUp}
      onPointerCancel={handleUp}
      onContextMenu={(event) => event.preventDefault()}
      className={cn(
        "pointer-events-auto flex size-20 touch-none select-none items-center justify-center rounded-full border shadow-lg backdrop-blur-sm transition-transform active:scale-95 sm:size-24",
        variant === "gas"
          ? "border-primary/50 bg-primary/80 text-primary-foreground"
          : variant === "brake"
            ? "border-destructive/50 bg-destructive/80 text-white"
            : "border-border bg-background/70 text-foreground",
        activeState && "scale-95 ring-4 ring-ring/40",
      )}
    >
      {children}
    </button>
  );
}
