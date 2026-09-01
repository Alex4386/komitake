import { useEffect, useRef } from "react";
import {
  readGamepadDriveState,
  type GamepadControlMode,
  type ArcadeLongitudinalState,
  type GamepadCurveSettings,
} from "@/lib/gamepad";

type GamepadDriveProps = {
  enabled: boolean;
  mode: GamepadControlMode;
  curves: GamepadCurveSettings;
  rightStickSteering: boolean;
  onChange: (steer: number, throttle: number) => void;
  onConnectionChange: (connected: boolean) => void;
};

const AXIS_CHANGE_THRESHOLD = 0.002;

export function GamepadDrive({
  enabled,
  mode,
  curves,
  rightStickSteering,
  onChange,
  onConnectionChange,
}: GamepadDriveProps) {
  const onChangeRef = useRef(onChange);
  const onConnectionChangeRef = useRef(onConnectionChange);
  const enabledRef = useRef(enabled);
  const modeRef = useRef(mode);
  const curvesRef = useRef(curves);
  const rightStickSteeringRef = useRef(rightStickSteering);
  onChangeRef.current = onChange;
  onConnectionChangeRef.current = onConnectionChange;
  enabledRef.current = enabled;
  modeRef.current = mode;
  curvesRef.current = curves;
  rightStickSteeringRef.current = rightStickSteering;

  useEffect(() => {
    let animationFrame = 0;
    let connected = false;
    let lastSteer = 0;
    let lastThrottle = 0;
    let lastMode = modeRef.current;
    const arcadeState: ArcadeLongitudinalState = {
      forwardStartedAt: null,
      reverseStartedAt: null,
    };

    const publishConnection = (nextConnected: boolean) => {
      if (connected === nextConnected) return;
      connected = nextConnected;
      onConnectionChangeRef.current(nextConnected);
    };
    const publishDrive = (steer: number, throttle: number) => {
      const steerChanged = Math.abs(steer - lastSteer) >= AXIS_CHANGE_THRESHOLD;
      const throttleChanged = Math.abs(throttle - lastThrottle) >= AXIS_CHANGE_THRESHOLD;
      if (!steerChanged && !throttleChanged) return;
      lastSteer = steer;
      lastThrottle = throttle;
      onChangeRef.current(steer, throttle);
    };
    const poll = () => {
      const gamepad = Array.from(navigator.getGamepads()).find(
        (candidate): candidate is Gamepad => Boolean(candidate?.connected && candidate.mapping === "standard"),
      );
      publishConnection(Boolean(gamepad));
      if (lastMode !== modeRef.current) {
        arcadeState.forwardStartedAt = null;
        arcadeState.reverseStartedAt = null;
        lastMode = modeRef.current;
      }
      if (gamepad && enabledRef.current) {
        const driveState = readGamepadDriveState(
          gamepad,
          modeRef.current,
          curvesRef.current,
          performance.now(),
          arcadeState,
          rightStickSteeringRef.current,
        );
        publishDrive(driveState.steer, driveState.throttle);
      } else {
        arcadeState.forwardStartedAt = null;
        arcadeState.reverseStartedAt = null;
        publishDrive(0, 0);
      }
      animationFrame = requestAnimationFrame(poll);
    };

    if (!("getGamepads" in navigator)) {
      onConnectionChangeRef.current(false);
      return;
    }
    animationFrame = requestAnimationFrame(poll);
    return () => {
      cancelAnimationFrame(animationFrame);
      if (lastSteer !== 0 || lastThrottle !== 0) onChangeRef.current(0, 0);
      onConnectionChangeRef.current(false);
    };
  }, []);

  return null;
}
