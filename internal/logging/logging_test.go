package logging

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	t.Parallel()

	tests := map[string]slog.Level{
		"trace":   LevelTrace,
		"TRACE":   LevelTrace,
		" debug ": slog.LevelDebug,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
		"warn":    slog.LevelWarn,
		"warning": slog.LevelWarn,
		"error":   slog.LevelError,
	}
	for name, want := range tests {
		got, err := ParseLevel(name)
		if err != nil {
			t.Fatalf("ParseLevel(%q) error = %v", name, err)
		}
		if got != want {
			t.Fatalf("ParseLevel(%q) = %v, want %v", name, got, want)
		}
	}

	if _, err := ParseLevel("chatty"); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

// Trace must sort below Debug so -vv is strictly noisier than -v.
func TestTraceIsBelowDebug(t *testing.T) {
	t.Parallel()

	if LevelTrace >= slog.LevelDebug {
		t.Fatalf("LevelTrace = %v, want less than LevelDebug (%v)", LevelTrace, slog.LevelDebug)
	}
}

func TestNewLoggerRendersTraceLevelName(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := NewLogger(&buf, Options{Level: LevelTrace})
	l.Trace("hello")

	out := buf.String()
	// Without ReplaceAttr, slog renders this as DEBUG-4.
	if !strings.Contains(out, "TRACE") {
		t.Fatalf("output = %q, want a TRACE level name", out)
	}
	if strings.Contains(out, "DEBUG-4") {
		t.Fatalf("output = %q, want the custom level name", out)
	}
}

func TestLoggerLevelFiltering(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	l := NewLogger(&buf, Options{Level: slog.LevelDebug})

	l.Trace("trace-line")
	if strings.Contains(buf.String(), "trace-line") {
		t.Fatalf("trace record emitted at debug level: %q", buf.String())
	}

	l.Debug("debug-line")
	if !strings.Contains(buf.String(), "debug-line") {
		t.Fatalf("debug record missing: %q", buf.String())
	}
}

// Trace level implies source attribution, since the point of trace is finding
// which call site produced a record.
func TestTraceLevelAddsSource(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	NewLogger(&buf, Options{Level: LevelTrace}).Trace("x")

	if !strings.Contains(buf.String(), "logging_test.go") {
		t.Fatalf("output = %q, want a source reference", buf.String())
	}
}

func TestNewLoggerJSONFormat(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	NewLogger(&buf, Options{Level: LevelTrace, Format: FormatJSON}).Trace("hello")

	out := buf.String()
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("output = %q, want JSON", out)
	}
	if !strings.Contains(out, `"level":"TRACE"`) {
		t.Fatalf("output = %q, want a TRACE level field", out)
	}
}

func TestParseFormat(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]Format{"": FormatText, "text": FormatText, "JSON": FormatJSON} {
		got, err := ParseFormat(name)
		if err != nil {
			t.Fatalf("ParseFormat(%q) error = %v", name, err)
		}
		if got != want {
			t.Fatalf("ParseFormat(%q) = %v, want %v", name, got, want)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Fatal("expected an error for an unknown format")
	}
}

// The whole point of Secret: the value must never reach the output, at any
// verbosity.
func TestSecretNeverRevealsValue(t *testing.T) {
	t.Parallel()

	secret := []byte("super-secret-psk-material-000000")

	var buf bytes.Buffer
	l := NewLogger(&buf, Options{Level: LevelTrace})
	l.Trace("keys", Secret("psk", secret))

	out := buf.String()
	if strings.Contains(out, string(secret)) {
		t.Fatalf("secret leaked in plaintext: %q", out)
	}
	if strings.Contains(out, hex.EncodeToString(secret)) {
		t.Fatalf("secret leaked as hex: %q", out)
	}
	// The length and a short fingerprint are intentionally retained.
	if !strings.Contains(out, "32 bytes") {
		t.Fatalf("output = %q, want the byte count", out)
	}
	if !strings.Contains(out, "fp=") {
		t.Fatalf("output = %q, want a fingerprint", out)
	}
}

// Equal secrets must fingerprint equally, so operators can confirm both sides
// derived the same key; different secrets must not collide.
func TestSecretFingerprintIsStable(t *testing.T) {
	t.Parallel()

	a := Secret("k", []byte("aaaa")).Value.String()
	b := Secret("k", []byte("aaaa")).Value.String()
	c := Secret("k", []byte("bbbb")).Value.String()

	if a != b {
		t.Fatalf("fingerprints differ for equal input: %q vs %q", a, b)
	}
	if a == c {
		t.Fatal("fingerprints collide for different input")
	}
}

func TestSecretHandlesEmpty(t *testing.T) {
	t.Parallel()

	if got := Secret("k", nil).Value.String(); got != "<empty>" {
		t.Fatalf("Secret(nil) = %q", got)
	}
}

func TestDumpTruncatesLargePayloads(t *testing.T) {
	t.Parallel()

	if got := DumpString(nil); got != "<empty>" {
		t.Fatalf("DumpString(nil) = %q", got)
	}

	small := []byte{1, 2, 3}
	if got := DumpString(small); got != "010203" {
		t.Fatalf("DumpString(small) = %q", got)
	}

	large := bytes.Repeat([]byte{0xab}, 4096)
	got := DumpString(large)
	if len(got) > 2*maxDumpBytes+40 {
		t.Fatalf("dump was not truncated: %d chars", len(got))
	}
	if !strings.Contains(got, "+4032 bytes") {
		t.Fatalf("dump = %q, want an elision count", got)
	}
}

func TestEnabledTracksDefaultLogger(t *testing.T) {
	var buf bytes.Buffer

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	slog.SetDefault(NewLogger(&buf, Options{Level: slog.LevelInfo}).Logger)
	if TraceEnabled() {
		t.Fatal("TraceEnabled() = true at info level")
	}

	slog.SetDefault(NewLogger(&buf, Options{Level: LevelTrace}).Logger)
	if !TraceEnabled() {
		t.Fatal("TraceEnabled() = false at trace level")
	}
}
