# Fuji live video (LSP) formats

Komitake receives the Fuji kart's live camera stream over the LSP control and
video ports advertised in `connection_info`. This document describes the wire
formats the daemon implements. Semantics that are not needed for demux or
decode are left unnamed.

## Ports (`connection_info`)

`connection_info` is a 16-byte little-endian value written once on the Fuji
control service:

| Offset | Field |
| --- | --- |
| `0x0` | marker (typically `1`) |
| `0x2` | host telemetry UDP port (`5116 + slot`) |
| `0x4` | host LSP control TCP port (`5032 + slot`) |
| `0x6` | host LSP video UDP port (`5016 + slot`) |
| `0x8` | network time (one-second precision) |

The kart initiates TCP to the LSP control port. After accept, the host sends a
single 24-byte `LVNI` greeting; the kart typically replies with no TCP
application data and begins UDP video on the stream port. UDP `5004` (time sync)
is not required for the observed startup path.

### Initial `LVNI` greeting

Little-endian magic `0x494E564C` (`LVNI`), then two zeroed `u64` fields and a
zeroed `u32`:

```text
4c 56 4e 49 00 00 00 00 00 00 00 00
00 00 00 00 00 00 00 00 00 00 00 00
```

Later dynamic `LVNI` records are not sent by Komitake; their semantics are
unresolved.

## UDP datagram layout

Observed video datagrams are 1,416 bytes:

1. 8-byte EC-C1 header
2. 1,400-byte MoLive / `L2` packet
3. 8-byte opaque trailer (monotonic when read big-endian; not fed to `L2`)

### EC-C1 header

| Byte(s) | Meaning |
| --- | --- |
| `0..1` | `(byte0 << 4) \| (byte1 >> 4)` must equal `0xECC`; low nibble of byte `1` must be `1` |
| `2` bit `7` | class flag (clear = source, set = parity) |
| `2..4` | 23-bit packet sequence (after masking the class flag) |
| `5` | generation id (mod 256) |
| `6` | source/data shard count `K` |
| `7` | class-local index (`< K` for source packets) |

Source records are ordered by generation, then source index `0..K-1`. Parity
records are never emitted as media. Komitake currently drops incomplete
generations; parity-class recovery is not exercised in the shipped path.

The FEC codec, when used, is a systematic Vandermonde erasure code over GF(256)
with reduction polynomial `0x11D`.

## MoLive `L2` packet

Each recovered source payload (after EC-C1) begins with a 14-byte header:

| Offset | Field |
| --- | --- |
| `0..1` | ASCII `L2` |
| `2..3` | big-endian checksum |
| `4..11` | big-endian 64-bit anchor; bit 63 is a flag, low 63 bits are the parser base |
| `12..13` | big-endian total length minus one |

Checksum:

```text
checksum = (word0 & 0x7fff) ^ word1 ^ word2 ^ word3 ^ 0xaaaa
```

where `word0..word3` are the big-endian words at offsets `4..11`. Declared total
length is `be16(header[12:14]) + 1` and must be in `16..65535`.

For the packetized UDP path, each feed is one fixed-size MoLive packet
(1,400 bytes in the observed stream). Incomplete headers are discarded rather
than buffered across datagrams.

### Descriptors

After the header, descriptors are base128 tag/length envelopes:

```text
descriptor := base128_tag base128_length payload[length]
```

The fourth length byte terminates structurally. Tag `0` ends the descriptor
phase. Observed stream setup uses tag `1` with a 12-byte payload, then tag `0`.

### Section control and media fragments

One control byte follows the descriptor terminator:

| Bits | Meaning |
| --- | --- |
| `0` | section has stream elements / variable-size mode |
| `1` | big-endian 16-bit serial present |
| `2..7` | six-bit section code |

Stream elements use an MSB-first bit reader: a zero byte ends the section;
width-prefixed stream index; continuation vs completing bit; completing
elements carry metadata and a signed anchor delta; payload length is
`13-bit value + 1` (`1..8192`). Completing elements assemble into logical
records.

### `FRAM` records

Complete stream-zero records begin with ASCII `FRAM`. A big-endian `u32` at
offset `+4` is the total record length. The fixed header is 32 bytes; offsets
`+8`, `+0x10`, and `+0x18` hold three big-endian `u64` values. Encoded media
starts at `+0x20`.

Encoded payloads are Annex-B H.264. The first frame typically begins with SPS
(`7`), PPS (`8`), and IDR (`5`). Later frames begin with AUD (`9`) followed by
non-IDR slices (`1`). Capture-validated frames may also end with a duplicated
trailing AUD; decoder adapters (`ffplay`, WebCodecs) strip only that trailing
AUD before concatenated submission. The daemon's authoritative IPC/WebSocket
payload keeps the unnormalized per-`FRAM` bytes.

Observed stream: H.264 High @ level 3.2, 1280x800, `yuv420p`, 25 fps.

## Delivery in Komitake

- One persistent FFmpeg VAAPI pipeline per kart re-encodes with periodic IDRs
  (10-frame / 400 ms GOP at 25 fps) for safe multiplexing.
- CLI `komitake video`, WebSocket/`KTV1`, and WebRTC/Pion share that transcoder.
- WebRTC skips the cached GOP and starts at the next live IDR.

## Related Fuji formats

Telemetry and drive layouts used alongside video:

- Telemetry UDP: type-prefixed packets (`0x01` status/battery, `0x02` IMU).
- Drive UDP `5102`: 32-byte teleoperation frame at 30 Hz.

See also [switchbrew documentation](https://switchbrew.org/wiki/Mario_Kart_Live:_Home_Circuit)
and the [OpenKart](https://github.com/openkart-sdk/openkart) reference
implementation.
