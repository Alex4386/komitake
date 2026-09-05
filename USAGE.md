# Using Komitake

Komitake connects a Fuji kart to your PC: access point, pairing, drive, telemetry,
and live camera. The CLI runs the daemon, pairs karts, shows status, plays video,
and serves the web UI.

Run `komitake --help` or `komitake <command> --help` for the command reference.

> [!NOTE]  
> This document is written throughly and intended for people trying to integrate komitake to other application.  
> If you want to do some quick racing with your Xbox Controller, See [QUICKSTART.md](QUICKSTART.md)

## Running the daemon

Run it directly:

```sh
komitake daemon
```

`./INSTALL` also ships a systemd unit (see [scripts/install-systemd.sh](scripts/install-systemd.sh)).
If you use it, manage the daemon with your init system, e.g.:

```sh
systemctl enable --now komitake.service
journalctl -u komitake.service -f
```

## Observing the daemon

```sh
komitake status        # current mode, wireless info, pairing state
komitake devices       # connected karts (serial, kind)
komitake devices -l    # include ident, address, MAC, and signal
komitake devices XKW12  # detail for one kart (ident or serial / unique prefix)
```

Both accept `--json` for scripting. Output adapts to its destination: aligned,
colored columns on a terminal; plain tab-separated values when piped, so
`komitake devices | cut -f1` is serial. `NO_COLOR` and `FORCE_COLOR` are
honored.

## Connecting to the daemon

Client commands only need to know where the daemon's admin API is. That address
is given by `--address` in one unified form:

- `unix:/run/komitake.sock`: a local unix socket (this is the default)
- `host:port` or `tcp:host:port`: a daemon reachable over TCP

```sh
komitake status                                  # default local socket
komitake --address unix:/run/komitake.sock status
komitake --address 192.168.1.50:5252 status      # control a remote daemon
```

A Mac controlling a Linux kart host, for example, just points `--address` at the
Linux box. Enable TCP on the daemon side with `socket.bind` (see Configuration); the
admin API is unauthenticated, so only expose it on a trusted network.

Clients do not read `config.json`; `--address` (or the default) is all they
need. `--config` is only consulted to discover `listen` when you pass it
explicitly. `komitake web --config` also uses `web.bind` and enables the Settings dialog.

## Pairing a kart

`komitake pair` puts the running daemon into pairing mode and renders the
pairing QR code in the terminal. Scan it with the kart to join. The daemon
returns to normal mode when pairing completes, is canceled (Ctrl-C), or times
out.

```sh
komitake pair                         # QR shown in the terminal
komitake pair --qr-file /tmp/pair.png # also write a PNG
komitake pair --no-qr                 # skip the terminal render
```

`--qr-file` is deleted when pairing ends unless `--keep-qr-file` is given. The
pairing seed is equivalent to the network key, so it is hidden by default; pass
`--show-secrets` to print it.


## Live video

`komitake video` subscribes to the daemon's existing `AdminService` IPC stream
and pipes complete Annex-B H.264 frames to `ffplay`:

```sh
komitake video                         # the only connected kart
komitake video XKW12                   # ident/serial unique prefix
komitake video --player /usr/bin/ffplay
```

Install FFmpeg/`ffplay` on the machine running the CLI. The daemon sends the
24-byte zeroed `LVNI` greeting immediately after the kart opens TCP `5032`
(see [docs/fuji-video.md](docs/fuji-video.md)). A persistent Intel hardware
FFmpeg pipeline emits periodic IDRs and the daemon retains the latest
IDR-anchored GOP for later subscribers. Multiple CLI/web subscribers share
the daemon's single kart receiver.

The implemented parser covers the source-packet path. Parity-class EC-C1
packets are ignored until recovery is validated. One persistent FFmpeg process
per kart uses a persistent hardware FFmpeg pipeline (VAAPI, NVENC, or QSV depending on
`video.hwaccel`, or fully custom args) with one-frame asynchronous
depth and emits a 10-frame (400 ms at 25 fps) GOP with SPS/PPS on every IDR.
The daemon caches the latest IDR-anchored GOP for CLI/WebSocket startup.
WebRTC skips that cache and waits for the next live IDR, avoiding a burst of
old RTP frames. Subscriber queues hold at most four frames (160 ms); overflow
drops stale frames, signals a discontinuity, and resumes at the next IDR
instead of increasing end-to-end delay.

At info level the daemon logs the transcoder contract and lifecycle explicitly:
`hwaccel=<backend>`, `encoder=<ffmpeg encoder>`, optional `render_node` for VAAPI,
`low_power=true` on VAAPI, and `software=true|false`. It then logs the FFmpeg PID and
`video transcoder ready` after the first encoded frame. Verify with:

```sh
journalctl -u komitake -f | grep 'video transcoder'
```

If VAAPI initialization fails, media setup fails and the daemon logs an error; it
does not silently fall back to `libx264`. Use `video.hwaccel=none` for explicit
software encoding, or `custom` with your own ffmpeg args.

### Video transcode config

With `komitake set`:

```sh
komitake set --video-hwaccel=vaapi
komitake set --video-ffmpeg-profile=realtime
komitake set --video-ffmpeg-path=/usr/bin/ffmpeg
```

Restart the daemon after video changes (`sudo systemctl restart komitake.service`).

In `config.json`:

```json
"video": {
  "hwaccel": "auto",
  "ffmpeg_profile": "realtime"
}
```

Optional overrides:

```json
"video": {
  "hwaccel": "custom",
  "ffmpeg_path": "/usr/bin/ffmpeg",
  "ffmpeg_args": {
    "input": ["-hwaccel", "auto", "-f", "h264", "-framerate", "25", "-i", "pipe:0", "-an"],
    "output": ["-c:v", "h264_nvenc", "-f", "h264", "pipe:1"]
  }
}
```

- `hwaccel`: `auto` (default, probes encoders), `vaapi`, `nvenc`, `qsv`, `custom`, or `none` (software/`libx264`).
- `ffmpeg_path`: optional; defaults to `ffmpeg` on `PATH`.
- `ffmpeg_profile`: optional preset tuning. `realtime` lowers buffering and encoder
  latency (e.g. `-fflags nobuffer`, backend-specific low-delay flags). Applied before
  `ffmpeg_args`, so explicit overrides still win.
- `ffmpeg_args`: optional overrides appended to the built-in profile. `input` applies
  before `-i pipe:0`; `output` applies after it. Duplicate flags use ffmpeg last-wins
  semantics (e.g. add `-fflags nobuffer` or override `-qp`). With `custom`, they are
  the full transcode arguments (must include `pipe:0` and `pipe:1`).

## Web UI and REST API

`komitake web` serves a REST API under `/v1` and a web UI, driving the daemon
over its admin API. It dials the daemon with `--address` (same as other client
commands) and serves HTTP or HTTPS on `--web-addr`.

```sh
komitake web                                     # http://127.0.0.1:8080
komitake web --web-addr 0.0.0.0:8080             # expose on the LAN
komitake web --address 192.168.1.50:5252         # control a remote daemon
komitake web --config /etc/komitake/config.json   # apply web.bind and web.tls
```

Set `web.tls.enabled` in the config to serve HTTPS. If `web.tls.cert_file` and
`web.tls.key_file` are both set, they must contain a PEM certificate chain and
matching private key. If both paths are blank, `komitake web` generates an
in-memory self-signed certificate at each startup. Browsers and API clients will
require an explicit trust exception for that certificate. WebSocket/WebCodecs mode requires a secure context: loopback HTTP works in
supported browsers, but LAN WebCodecs must use HTTPS. WebRTC mode also benefits
from HTTPS for normal browser deployment; signaling is same-origin and media uses
ICE/DTLS/SRTP directly between browser and the web host.

The API is unauthenticated, like the admin API it fronts; only expose
`--web-addr` on a trusted network. Endpoints:

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/v1/status` | daemon mode, wireless info, pairing session |
| GET | `/v1/karts/by-id/{id}` | one kart by RCD ident (prefix ok) |
| GET | `/v1/karts/by-serial/{serial}` | 307 → `/v1/karts/by-id/{id}` (prefix ok) |
| GET | `/v1/karts/by-serial/{serial}/…` | 307 → matching `/by-id/{id}/…` path |
| GET | `/v1/karts` | connected karts |
| POST | `/v1/karts/pair` | enter pairing mode, returns the QR payload (optional `wait_seconds`) |
| POST | `/v1/karts/pair/stop` | leave pairing mode |
| GET/POST | `/v1/karts/by-id/{id}/drive` | live teleop (throttle/steering/brake light) over physical Fuji UDP 5102 |
| PUT | `/v1/karts/by-id/{id}/drive-mode` | enable or disable teleoperation; transitions reset axes to neutral |
| GET | `/v1/ws` | WebSocket: live status, devices, drive events |
| GET | `/v1/karts/by-id/{id}/ws` | WebSocket: telemetry/drive JSON plus optional binary H.264 video |
| POST | `/v1/karts/by-id/{id}/webrtc` | WebRTC SDP offer/answer for the shared H.264 stream |
| GET/PUT | `/v1/settings` | Read or update `web.bind`, `web.tls`, `socket.bind`, and `socket.chmod` |

Fuji **control** (product_code / GetParam) is on the daemon admin IPC, not the
CLI: `GetProductCode` and `GetDeviceParam` over the unix/TCP admin API (see
`pkg/komitake`). On connect the daemon fetches `product_code` and fills
`DeviceSummary.serial` for `ListDevices` / the web UI.

OpenAPI is served at `/openapi.json`. The web UI is a Vite/React + shadcn/ui
(+ ReUI primitives) app under [internal/web/frontend](internal/web/frontend);
build with `npm run build` there before embedding. Without a build, `komitake web`
still serves the API and shows a placeholder page.

Messages on `/v1/ws` are JSON. The server pushes `snapshot`, `status`, `devices`,
and `drive` events. The selected-device WebSocket keeps drive/telemetry as text JSON. In WebSocket
video mode it also sends versioned `KTV1` binary envelopes containing sequence,
keyframe/discontinuity flags, and one complete Annex-B frame. In WebRTC mode the
control WebSocket sets `video=0`; the browser posts one non-trickle SDP offer to
the WebRTC endpoint and receives Pion-packetized H.264 RTP. The browser requests
a 40 ms jitter-buffer target (with the legacy playout-delay hint fallback), and
the server starts that stream at the next live IDR. A persisted UI toggle
switches modes. Both modes share the same kart receiver and hardware transcoder;
neither changes the kart session nor starts another transcoder. Clients may send `{ "type":"drive", "device_id":"…",
"steer":0, "throttle":0, "brake":0 }` or `{ "type":"drive-mode", "enabled":false }` on the selected-device WebSocket. Drive-mode transitions reset all axes to neutral; disabling disarms UDP output, and drive commands do not implicitly re-enable it. After `connection_info` and
`SetState(1)`, the daemon sends the physical 32-byte Fuji teleoperation frame to
kart UDP 5102 at 30 Hz. Throttle and steering are signed bytes, brake controls the
independent brake-light byte, and a little-endian counter begins at offset 4.

`connection_info` carries telemetry UDP `5116 + slot`, LSP control TCP
`5032 + slot`, and LSP video UDP `5016 + slot`. Komitake reserves all three
atomically before advertising the slot. The accepted kart connection receives
the zeroed `LVNI` greeting; EC-C1, 1,400-byte MoLive/L2 packets, stream-zero
media fragments, and `FRAM` records are assembled daemon-side (see
[docs/fuji-video.md](docs/fuji-video.md)).

Telemetry datagrams are direct type-prefixed packets, not `RL` records: type
`0x01` supplies the Switchbrew-documented cable-connected bit and battery HUD
bars (0-4), and type `0x02` supplies timer, quaternion, and raw IMU samples.
`drive_armed` is derived from the daemon's local control-session state rather
than a telemetry bit.


## Configuration

Use [config.example.json](./config.example.json) as a starting point. When
`--config` is not given, the daemon's lookup order is:

1. `./config.json`
2. `/etc/komitake/config.json`

```sh
komitake daemon --config /path/to/config.json
komitake web --config /path/to/config.json
```

The configuration is hierarchical:

```json
{
  "socket": {
    "bind": "unix:/run/komitake.sock",
    "chmod": "0600"
  },
  "web": {
    "bind": "127.0.0.1:8080",
    "tls": {
      "enabled": true,
      "cert_file": "/etc/komitake/web.crt",
      "key_file": "/etc/komitake/web.key"
    }
  },
  "wireless": {
    "interface": "wlan0",
    "address": "192.168.137.1/24"
  }
}
```

- `socket.bind` selects the admin API (`unix:/path` or trusted-network TCP).
- `socket.chmod` sets the Unix socket mode; it defaults to owner-only `0600`.
- `web.bind` is used by `komitake web` when `--web-addr` is omitted.
- `web.tls.enabled` switches the web listener to HTTPS.
- `web.tls.cert_file` and `web.tls.key_file` select a PEM certificate/key pair; omit both to generate an in-memory self-signed certificate.
- `wireless.address` is the AP host address and prefix, from which subnet and gateway are derived.

The daemon `--listen` flag and web `--web-addr` flag override config values.
Legacy `address`, `listen`, `web_addr`, `socket_perms`, `socket.perm`, and
`wireless.subnet` keys remain readable and are migrated when Settings saves the
file. Settings writes are atomic and preserve secrets, unknown fields, and file
permissions.

Generated game-network credentials are stored in `state.json` next to the
active config file; pairing-session metadata in `pairing.json`.

The root `secret` may live in a sibling `secret` file (mode `0600`). When
present it overrides any `"secret"` field in `config.json`, so `config.json`
can stay world-readable for operators. `./INSTALL` sets it up that way under
`/etc/komitake/`.

## Logging

Logging is controlled by global flags and always goes to stderr, so `--json`
output on stdout stays parseable.

```sh
komitake daemon -v               # debug: config, connections, handshake steps
komitake daemon -vv              # trace: every RCD message with hex payloads
komitake daemon -vv --log-format json | jq .
```

| Level | What it adds |
| --- | --- |
| `info` (default) | state changes, device connect/disconnect, AP lifecycle |
| `debug` (`-v`) | effective config, handshake steps, port registration |
| `trace` (`-vv`) | every RCD message with hex payloads, transcript digests, source location |

`--log-level` sets the level explicitly and overrides `-v`. Trace payload dumps
are capped at 64 bytes per record.

**Secrets are never logged at any verbosity.** PSKs, master keys, pairing IDs,
derived secret keys, and the config `secret` are reported as
`<redacted N bytes fp=xxxxxxxx>`. The fingerprint is a SHA-256 prefix, enough to
confirm two sides derived the same key without disclosing it.

## Global flags

| Flag | Description |
| --- | --- |
| `--address` | daemon admin API address: `unix:/path` or `host:port` (default `unix:/run/komitake.sock`) |
| `--config` | path to `config.json` (daemon and web; clients use it only for address discovery) |
| `--json` | emit JSON instead of formatted text |
| `--no-color` | disable colored output |
| `-v`, `-vv` | increase log verbosity |
| `--log-level` | explicit level; overrides `-v` |
| `--log-format` | `text` or `json` |

The daemon additionally accepts `--listen`, `--interface`, and `--pairing-file`.

## Shell completion

```sh
komitake completion bash | sudo tee /etc/bash_completion.d/komitake
komitake completion zsh > "${fpath[1]}/_komitake"
komitake completion fish > ~/.config/fish/completions/komitake.fish
```
