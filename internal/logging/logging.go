// Package logging provides the shared slog setup for komitake, including a
// TRACE level below slog's Debug and helpers that keep secret material out of
// log output.
package logging

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

// LevelTrace sits below slog.LevelDebug. Debug is for operational detail an
// administrator might want; Trace is for per-message protocol dumps useful
// when diagnosing a session wire-by-wire.
const LevelTrace = slog.Level(-8)

// levelNames maps custom levels to display strings, since slog only knows its
// four built-ins.
var levelNames = map[slog.Leveler]string{
	LevelTrace: "TRACE",
}

// Enabled reports whether the default logger would emit at the given level.
// Callers use this to skip building expensive attributes, such as hex dumps.
func Enabled(level slog.Level) bool {
	return slog.Default().Enabled(context.Background(), level)
}

// TraceEnabled reports whether trace output is being emitted.
func TraceEnabled() bool {
	return Enabled(LevelTrace)
}

// Trace logs at LevelTrace against the default logger.
func Trace(msg string, args ...any) {
	log(context.Background(), slog.Default(), LevelTrace, msg, args...)
}

// TraceCtx logs at LevelTrace against the default logger, carrying ctx.
func TraceCtx(ctx context.Context, msg string, args ...any) {
	log(ctx, slog.Default(), LevelTrace, msg, args...)
}

// log emits a record whose source location is the caller of the exported
// wrapper, not the wrapper itself. slog.Logger.Log would attribute every record
// to this file, making AddSource useless.
func log(ctx context.Context, l *slog.Logger, level slog.Level, msg string, args ...any) {
	if !l.Enabled(ctx, level) {
		return
	}
	// Skip runtime.Callers, this function, and the exported wrapper.
	var pcs [1]uintptr
	runtime.Callers(3, pcs[:])

	r := slog.NewRecord(time.Now(), level, msg, pcs[0])
	r.Add(args...)
	_ = l.Handler().Handle(ctx, r)
}

// Logger wraps slog.Logger with a Trace method so component loggers built with
// With() can emit trace records too.
type Logger struct {
	*slog.Logger
}

// New wraps an slog.Logger. A nil logger falls back to the default.
func New(l *slog.Logger) *Logger {
	if l == nil {
		l = slog.Default()
	}
	return &Logger{Logger: l}
}

// Trace logs at LevelTrace.
func (l *Logger) Trace(msg string, args ...any) {
	log(context.Background(), l.Logger, LevelTrace, msg, args...)
}

// TraceEnabled reports whether this logger emits trace records.
func (l *Logger) TraceEnabled() bool {
	return l.Logger.Enabled(context.Background(), LevelTrace)
}

// With returns a Logger with the given attributes attached.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...)}
}

// ParseLevel maps a level name to an slog.Level, accepting "trace" in addition
// to slog's built-in names.
func ParseLevel(name string) (slog.Level, error) {
	switch normalize(name) {
	case "trace":
		return LevelTrace, nil
	case "debug":
		return slog.LevelDebug, nil
	case "info", "":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (want trace, debug, info, warn, or error)", name)
	}
}

// LevelNames lists the accepted level names, for shell completion and help.
func LevelNames() []string {
	return []string{"trace", "debug", "info", "warn", "error"}
}

func normalize(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

// replaceAttr renders custom level names and is shared by both handlers.
func replaceAttr(_ []string, a slog.Attr) slog.Attr {
	if a.Key != slog.LevelKey {
		return a
	}
	level, ok := a.Value.Any().(slog.Level)
	if !ok {
		return a
	}
	if name, found := levelNames[level]; found {
		a.Value = slog.StringValue(name)
	}
	return a
}
