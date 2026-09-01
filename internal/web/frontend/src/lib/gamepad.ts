export type GamepadControlMode = "trigger-proportional" | "button-arcade";

export type GamepadCurveSettings = {
  lateralPower: number;
  longitudinalPower: number;
};

export type GamepadControlSettings = {
  mode: GamepadControlMode;
  rightStickSteering: boolean;
  profiles: Record<GamepadControlMode, GamepadCurveSettings>;
};

export type GamepadDriveState = {
  steer: number;
  throttle: number;
};

export type ArcadeLongitudinalState = {
  forwardStartedAt: number | null;
  reverseStartedAt: number | null;
};

export const DEFAULT_GAMEPAD_SETTINGS: GamepadControlSettings = {
  mode: "trigger-proportional",
  rightStickSteering: false,
  profiles: {
    "trigger-proportional": { lateralPower: 1, longitudinalPower: 1 },
    "button-arcade": { lateralPower: 1, longitudinalPower: 1 },
  },
};

const GAMEPAD_DEADZONE = 0.1;
const ARCADE_BUTTON_RAMP_MILLISECONDS = 500;
const MIN_CURVE_POWER = 0.5;
const MAX_CURVE_POWER = 3;

export function readStoredGamepadSettings(rawValue: string | null): GamepadControlSettings {
  if (!rawValue) return DEFAULT_GAMEPAD_SETTINGS;
  try {
    const parsedValue = JSON.parse(rawValue) as Partial<GamepadControlSettings>;
    const mode = parsedValue.mode === "button-arcade" ? "button-arcade" : "trigger-proportional";
    return {
      mode,
      rightStickSteering: Boolean(parsedValue.rightStickSteering),
      profiles: {
        "trigger-proportional": sanitizeCurveSettings(parsedValue.profiles?.["trigger-proportional"]),
        "button-arcade": sanitizeCurveSettings(parsedValue.profiles?.["button-arcade"]),
      },
    };
  } catch {
    return DEFAULT_GAMEPAD_SETTINGS;
  }
}

export function readGamepadDriveState(
  gamepad: Gamepad,
  mode: GamepadControlMode,
  curves: GamepadCurveSettings,
  timestamp: number,
  kartState: ArcadeLongitudinalState,
  rightStickSteering = false,
): GamepadDriveState {
  const leftSteer = applySignedPowerCurve(gamepad.axes[0] ?? 0, curves.lateralPower);
  const rightSteer = rightStickSteering
    ? applySignedPowerCurve(gamepad.axes[2] ?? 0, curves.lateralPower)
    : 0;
  const steer = Math.abs(leftSteer) >= Math.abs(rightSteer) ? leftSteer : rightSteer;
  if (mode === "button-arcade") {
    const forwardPressed = readButtonValue(gamepad.buttons[0]) > GAMEPAD_DEADZONE;
    const reversePressed = readButtonValue(gamepad.buttons[1]) > GAMEPAD_DEADZONE;
    kartState.forwardStartedAt = updateButtonStart(kartState.forwardStartedAt, forwardPressed, timestamp);
    kartState.reverseStartedAt = updateButtonStart(kartState.reverseStartedAt, reversePressed, timestamp);
    const forward = applyArcadeButtonCurve(kartState.forwardStartedAt, timestamp, curves.longitudinalPower);
    const reverse = applyArcadeButtonCurve(kartState.reverseStartedAt, timestamp, curves.longitudinalPower);
    return { steer, throttle: clamp(forward - reverse, -1, 1) };
  }
  kartState.forwardStartedAt = null;
  kartState.reverseStartedAt = null;
  const forward = applyUnsignedPowerCurve(readButtonValue(gamepad.buttons[7]), curves.longitudinalPower);
  const reverse = applyUnsignedPowerCurve(readButtonValue(gamepad.buttons[6]), curves.longitudinalPower);
  return { steer, throttle: clamp(forward - reverse, -1, 1) };
}

function updateButtonStart(startedAt: number | null, pressed: boolean, timestamp: number): number | null {
  if (!pressed) return null;
  return startedAt ?? timestamp;
}

function applyArcadeButtonCurve(startedAt: number | null, timestamp: number, power: number): number {
  if (startedAt === null) return 0;
  const heldRatio = clamp((timestamp - startedAt) / ARCADE_BUTTON_RAMP_MILLISECONDS, 0, 1);
  return heldRatio === 0 ? 0 : heldRatio ** power;
}

function sanitizeCurveSettings(settings: Partial<GamepadCurveSettings> | undefined): GamepadCurveSettings {
  return {
    lateralPower: sanitizeCurvePower(settings?.lateralPower),
    longitudinalPower: sanitizeCurvePower(settings?.longitudinalPower),
  };
}

function sanitizeCurvePower(value: number | undefined): number {
  if (!Number.isFinite(value)) return 1;
  return clamp(value ?? 1, MIN_CURVE_POWER, MAX_CURVE_POWER);
}

function readButtonValue(button: GamepadButton | undefined): number {
  if (!button) return 0;
  return clamp(button.value || (button.pressed ? 1 : 0), 0, 1);
}

function applySignedPowerCurve(value: number, power: number): number {
  const direction = Math.sign(value);
  const magnitude = applyUnsignedPowerCurve(Math.abs(value), power);
  return direction * magnitude;
}

function applyUnsignedPowerCurve(value: number, power: number): number {
  const normalizedValue = clamp((value - GAMEPAD_DEADZONE) / (1 - GAMEPAD_DEADZONE), 0, 1);
  return normalizedValue === 0 ? 0 : normalizedValue ** power;
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.min(maximum, Math.max(minimum, value));
}
