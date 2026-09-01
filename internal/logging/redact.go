package logging

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
)

// maxDumpBytes caps how much of a payload is rendered. A single RCD message can
// carry 4 KiB and telemetry packets arrive continuously, so unbounded dumps
// would swamp the log and slow the read loop.
const maxDumpBytes = 64

// Secret returns an attribute that describes a sensitive value without
// revealing it. Verbose logging is the usual way keys escape into log files, so
// secret-bearing material must always go through this.
//
// The fingerprint is the first 4 bytes of SHA-256, which is enough to confirm
// two sides derived the same key while being useless to an attacker.
func Secret(key string, value []byte) slog.Attr {
	if len(value) == 0 {
		return slog.String(key, "<empty>")
	}
	sum := sha256.Sum256(value)
	return slog.String(key, fmt.Sprintf("<redacted %d bytes fp=%x>", len(value), sum[:4]))
}

// SecretString is Secret for string-typed secrets.
func SecretString(key, value string) slog.Attr {
	return Secret(key, []byte(value))
}

// Dump renders a byte slice as hex for protocol tracing, truncating beyond
// maxDumpBytes. Use only for payloads known not to carry key material; prefer
// Secret otherwise.
func Dump(key string, data []byte) slog.Attr {
	return slog.String(key, DumpString(data))
}

// DumpString renders data as hex, noting how many bytes were elided.
func DumpString(data []byte) string {
	if len(data) == 0 {
		return "<empty>"
	}
	if len(data) <= maxDumpBytes {
		return hex.EncodeToString(data)
	}
	return fmt.Sprintf("%s...+%d bytes", hex.EncodeToString(data[:maxDumpBytes]), len(data)-maxDumpBytes)
}

// Size reports a payload length without any of its contents. Safe at any
// verbosity, including for secret material.
func Size(key string, data []byte) slog.Attr {
	return slog.Int(key, len(data))
}
