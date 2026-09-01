import { useEffect, useRef } from "react";

type KeyboardDriveProps = {
  enabled: boolean;
  steeringLimit: number;
  throttleLimit: number;
  onChange: (steer: number, throttle: number) => void;
};

export function KeyboardDrive({ enabled, steeringLimit, throttleLimit, onChange }: KeyboardDriveProps) {
  const keys = useRef(new Set<string>());
  const onChangeRef = useRef(onChange);
  const steeringLimitRef = useRef(steeringLimit);
  const throttleLimitRef = useRef(throttleLimit);
  onChangeRef.current = onChange;
  steeringLimitRef.current = steeringLimit;
  throttleLimitRef.current = throttleLimit;

  useEffect(() => {
    if (!enabled) {
      keys.current.clear();
      onChangeRef.current(0, 0);
      return;
    }
    const publish = () => {
      const active = keys.current;
      let steer = 0;
      let throttle = 0;
      if (active.has("a") || active.has("arrowleft")) steer -= 1;
      if (active.has("d") || active.has("arrowright")) steer += 1;
      if (active.has("w") || active.has("arrowup")) throttle += 1;
      if (active.has("s") || active.has("arrowdown")) throttle -= 1;
      onChangeRef.current(
        steer * steeringLimitRef.current,
        throttle * throttleLimitRef.current,
      );
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey || event.repeat) return;
      const key = event.key.toLowerCase();
      if (key === " ") {
        event.preventDefault();
        keys.current.clear();
        publish();
        return;
      }
      if (!["w", "a", "s", "d", "arrowup", "arrowdown", "arrowleft", "arrowright"].includes(key)) return;
      event.preventDefault();
      keys.current.add(key);
      publish();
    };
    const onKeyUp = (event: KeyboardEvent) => {
      const key = event.key.toLowerCase();
      if (!keys.current.has(key)) return;
      keys.current.delete(key);
      publish();
    };
    const reset = () => { keys.current.clear(); publish(); };
    window.addEventListener("keydown", onKeyDown);
    window.addEventListener("keyup", onKeyUp);
    window.addEventListener("blur", reset);
    return () => {
      window.removeEventListener("keydown", onKeyDown);
      window.removeEventListener("keyup", onKeyUp);
      window.removeEventListener("blur", reset);
      keys.current.clear();
      onChangeRef.current(0, 0);
    };
  }, [enabled]);

  useEffect(() => {
    if (!enabled || keys.current.size === 0) return;
    let steer = 0;
    let throttle = 0;
    if (keys.current.has("a") || keys.current.has("arrowleft")) steer -= 1;
    if (keys.current.has("d") || keys.current.has("arrowright")) steer += 1;
    if (keys.current.has("w") || keys.current.has("arrowup")) throttle += 1;
    if (keys.current.has("s") || keys.current.has("arrowdown")) throttle -= 1;
    onChangeRef.current(steer * steeringLimit, throttle * throttleLimit);
  }, [enabled, steeringLimit, throttleLimit]);

  return null;
}
