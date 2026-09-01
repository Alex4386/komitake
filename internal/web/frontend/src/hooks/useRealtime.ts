import { useEffect, useRef, useState } from "react";
import {
  type DriveState,
  type Kart,
  type Status,
  type Telemetry,
  kartWsURL,
  wsURL,
} from "@/lib/api";

export type ConnState = "connecting" | "open" | "closed";

type SnapshotMsg = {
  type: "snapshot";
  status: Status;
  devices: Kart[];
  drive?: DriveState[];
};

type StatusMsg = { type: "status"; status: Status };
type DevicesMsg = { type: "devices"; devices: Kart[] };
type DriveMsg = { type: "drive"; drive: DriveState };
type ErrorMsg = { type: "error"; message: string };

type ServerMsg = SnapshotMsg | StatusMsg | DevicesMsg | DriveMsg | ErrorMsg | { type: string };

export function useRealtime() {
  const [status, setStatus] = useState<Status | null>(null);
  const [devices, setDevices] = useState<Kart[]>([]);
  const [driveById, setDriveById] = useState<Record<string, DriveState>>({});
  const [conn, setConn] = useState<ConnState>("connecting");
  const [error, setError] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    let cancelled = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;

    const connect = () => {
      if (cancelled) return;
      setConn("connecting");
      const ws = new WebSocket(wsURL());
      wsRef.current = ws;

      ws.onopen = () => {
        attempt = 0;
        if (!cancelled) {
          setConn("open");
          setError(null);
        }
      };

      ws.onmessage = (ev) => {
        let msg: ServerMsg;
        try {
          msg = JSON.parse(String(ev.data));
        } catch {
          return;
        }
        switch (msg.type) {
          case "snapshot": {
            const m = msg as SnapshotMsg;
            setStatus(m.status);
            setDevices(m.devices ?? []);
            if (m.drive?.length) {
              const next: Record<string, DriveState> = {};
              for (const d of m.drive) next[d.device_id] = d;
              setDriveById(next);
            }
            break;
          }
          case "status":
            setStatus((msg as StatusMsg).status);
            break;
          case "devices":
            setDevices((msg as DevicesMsg).devices ?? []);
            break;
          case "drive": {
            const d = (msg as DriveMsg).drive;
            if (d?.device_id) {
              setDriveById((prev) => ({ ...prev, [d.device_id]: d }));
            }
            break;
          }
          case "error":
            setError((msg as ErrorMsg).message);
            break;
        }
      };

      ws.onclose = () => {
        if (cancelled) return;
        setConn("closed");
        attempt += 1;
        const delay = Math.min(8000, 500 * 2 ** Math.min(attempt, 4));
        retry = setTimeout(connect, delay);
      };

      ws.onerror = () => {
        ws.close();
      };
    };

    connect();
    return () => {
      cancelled = true;
      if (retry) clearTimeout(retry);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, []);

  const sendDrive = (deviceId: string, steer: number, throttle: number, brake = 0) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(
      JSON.stringify({
        type: "drive",
        device_id: deviceId,
        steer,
        throttle,
        brake,
      }),
    );
  };

  return { status, devices, driveById, conn, error, sendDrive };
}


type DeviceMessage =
  | { type: "drive"; drive: DriveState }
  | { type: "telemetry"; telemetry: Telemetry }
  | { type: "error"; message: string }
  | { type: string };

/** Selected-kart drive and telemetry over /v1/karts/by-id/{id}/ws. */
export function useDeviceControl(
  deviceId: string | null,
  onVideoPacket?: (packet: ArrayBuffer) => void,
  includeVideo = true,
) {
  const [drive, setDrive] = useState<DriveState | null>(null);
  const [telemetry, setTelemetry] = useState<Telemetry | null>(null);
  const [conn, setConn] = useState<ConnState>("closed");
  const [error, setError] = useState<string | null>(null);
  const [requestedDriveMode, setRequestedDriveMode] = useState<boolean | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const videoHandlerRef = useRef(onVideoPacket);
  const requestedDriveModeRef = useRef<boolean | null>(null);

  useEffect(() => {
    videoHandlerRef.current = onVideoPacket;
  }, [onVideoPacket]);

  useEffect(() => {
    if (!deviceId) {
      setDrive(null);
      setTelemetry(null);
      setConn("closed");
      setError(null);
      requestedDriveModeRef.current = null;
      setRequestedDriveMode(null);
      return;
    }
    let cancelled = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    let attempt = 0;
    const connect = () => {
      if (cancelled) return;
      setConn("connecting");
      const socket = new WebSocket(kartWsURL(deviceId, includeVideo));
      socket.binaryType = "arraybuffer";
      wsRef.current = socket;
      socket.onopen = () => {
        attempt = 0;
        if (!cancelled) { setConn("open"); setError(null); }
      };
      socket.onmessage = (event) => {
        if (event.data instanceof ArrayBuffer) {
          videoHandlerRef.current?.(event.data);
          return;
        }
        let message: DeviceMessage;
        try { message = JSON.parse(String(event.data)); } catch { return; }
        switch (message.type) {
          case "drive":
            setDrive((message as { drive: DriveState }).drive);
            break;
          case "telemetry": {
            const nextTelemetry = (message as { telemetry: Telemetry }).telemetry;
            setTelemetry(nextTelemetry);
            if (nextTelemetry.drive_armed === requestedDriveModeRef.current) {
              requestedDriveModeRef.current = null;
              setRequestedDriveMode(null);
            }
            break;
          }
          case "error":
            requestedDriveModeRef.current = null;
            setRequestedDriveMode(null);
            setError((message as { message: string }).message);
            break;
        }
      };
      socket.onclose = () => {
        if (cancelled) return;
        setConn("closed");
        requestedDriveModeRef.current = null;
        setRequestedDriveMode(null);
        attempt += 1;
        retry = setTimeout(connect, Math.min(8000, 500 * 2 ** Math.min(attempt, 4)));
      };
      socket.onerror = () => socket.close();
    };
    connect();
    return () => {
      cancelled = true;
      if (retry) clearTimeout(retry);
      wsRef.current?.close();
      wsRef.current = null;
    };
  }, [deviceId, includeVideo]);

  const sendDrive = (steer: number, throttle: number, brake = 0) => {
    const socket = wsRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN || !deviceId) return;
    socket.send(JSON.stringify({ type: "drive", device_id: deviceId, steer, throttle, brake }));
  };
  const sendDriveMode = (enabled: boolean) => {
    const socket = wsRef.current;
    if (!socket || socket.readyState !== WebSocket.OPEN || !deviceId) return;
    requestedDriveModeRef.current = enabled;
    setRequestedDriveMode(enabled);
    socket.send(JSON.stringify({ type: "drive-mode", enabled }));
  };
  return { drive, telemetry, conn, error, requestedDriveMode, sendDrive, sendDriveMode };
}
