export interface Kart {
  kind: string;
  ident: string;
  serial?: string;
  address: string;
  mac_address: string;
  signal_dbm?: number;
  /** HUD battery bars from telemetry type 0x01 (0-4). */
  battery?: number;
  cable_connected?: boolean;
  drive_armed?: boolean;
  accel_mps2?: Vec3;
  gyro_dps?: Vec3;
  orientation?: Quat;
  imu_timer_us?: number;
}

export interface Wireless {
  interface?: string;
  address?: string;
  subnet?: string;
  channel?: number;
  ssid?: string;
}

export interface Pairing {
  ssid: string;
  channel: number;
  qr_payload: string; // base64 (huma encodes []byte as base64)
}

export interface Status {
  mode: "stopped" | "normal" | "pairing";
  wireless?: Wireless;
  pairing?: Pairing;
}

export interface DriveState {
  device_id: string;
  steer: number;
  throttle: number;
  brake: number;
  applied: boolean;
  reason?: string;
  updated_at?: string;
}

export interface DriveInput {
  steer: number;
  throttle: number;
  brake?: number;
}


export interface Vec3 {
  x: number;
  y: number;
  z: number;
}

export interface Quat {
  i: number;
  j: number;
  k: number;
  r: number;
}

export interface Telemetry {
  device_id: string;
  /** HUD battery bars from telemetry type 0x01 (0-4). */
  battery?: number;
  cable_connected?: boolean;
  drive_armed?: boolean;
  accel_mps2?: Vec3;
  gyro_dps?: Vec3;
  orientation?: Quat;
  imu_timer_us?: number;
}

export interface WebTLSSettings {
  enabled?: boolean;
  cert_file?: string;
  key_file?: string;
}

export interface WebSettings {
  bind?: string;
  tls?: WebTLSSettings;
}

export interface VideoFFmpegArgsSettings {
  input?: string[];
  output?: string[];
}

export interface VideoSettings {
  hwaccel?: string;
  ffmpeg_path?: string;
  ffmpeg_profile?: string;
  ffmpeg_args?: VideoFFmpegArgsSettings;
}

export interface ServiceSettings {
  web: WebSettings;
  socket: { bind?: string; chmod?: string };
  video?: VideoSettings;
  config_path?: string;
  defaults: {
    web: WebSettings;
    socket: { bind?: string; chmod?: string };
    video?: VideoSettings;
  };
}

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) {
    let detail = res.statusText;
    try {
      const body = await res.json();
      detail = body.detail || body.title || detail;
    } catch {
      // keep statusText
    }
    throw new Error(detail);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const api = {
  status: () => req<Status>("/v1/status"),
  karts: () => req<{ karts: Kart[] }>("/v1/karts").then((r) => r.karts),
  startPairing: () =>
    req<Pairing>("/v1/karts/pair", { method: "POST", body: JSON.stringify({}) }),
  stopPairing: () => req<void>("/v1/karts/pair/stop", { method: "POST" }),
  getDrive: (id: string) => req<DriveState>(`/v1/karts/by-id/${encodeURIComponent(id)}/drive`),
  setDrive: (id: string, body: DriveInput) =>
    req<DriveState>(`/v1/karts/by-id/${encodeURIComponent(id)}/drive`, {
      method: "POST",
      body: JSON.stringify(body),
    }),
  setDriveMode: (id: string, enabled: boolean) =>
    req<Kart>(`/v1/karts/by-id/${encodeURIComponent(id)}/drive-mode`, {
      method: "PUT",
      body: JSON.stringify({ enabled }),
    }),
  shutdownKart: (id: string) =>
    req<Kart>(`/v1/karts/by-id/${encodeURIComponent(id)}/shutdown`, {
      method: "POST",
    }),
  getSettings: () => req<ServiceSettings>("/v1/settings"),
  putSettings: (body: { web: WebSettings; socket: { bind?: string; chmod?: string }; video: VideoSettings }) =>
    req<ServiceSettings>("/v1/settings", { method: "PUT", body: JSON.stringify(body) }),
};

// base64ToBytes decodes huma's base64 []byte encoding into a Uint8Array.
export function base64ToBytes(b64: string): Uint8Array {
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

function wsProtocol(): string {
  return window.location.protocol === "https:" ? "wss:" : "ws:";
}

export function wsURL(): string {
  return `${wsProtocol()}//${window.location.host}/v1/ws`;
}

export function kartWsURL(id: string, includeVideo = true): string {
  const video = includeVideo ? "1" : "0";
  return `${wsProtocol()}//${window.location.host}/v1/karts/by-id/${encodeURIComponent(id)}/ws?video=${video}`;
}
