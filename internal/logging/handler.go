package logging

import (
	"fmt"
	"io"
	"log/slog"
)

// Format selects the log encoding.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// ParseFormat maps a format name to a Format.
func ParseFormat(name string) (Format, error) {
	switch normalize(name) {
	case "", "text":
		return FormatText, nil
	case "json":
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unknown log format %q (want text or json)", name)
	}
}

// Options configures a logger.
type Options struct {
	Level  slog.Level
	Format Format

	// AddSource attaches the source file and line to each record. Enabled
	// automatically at trace level, where pinpointing the emitting call site is
	// the whole point.
	AddSource bool
}

// NewLogger builds a logger that understands LevelTrace and renders it as
// "TRACE" rather than "DEBUG-4".
func NewLogger(w io.Writer, opts Options) *Logger {
	if opts.Format == "" {
		opts.Format = FormatText
	}
	addSource := opts.AddSource || opts.Level <= LevelTrace

	handlerOpts := &slog.HandlerOptions{
		Level:       opts.Level,
		AddSource:   addSource,
		ReplaceAttr: replaceAttr,
	}

	var handler slog.Handler
	if opts.Format == FormatJSON {
		handler = slog.NewJSONHandler(w, handlerOpts)
	} else {
		handler = slog.NewTextHandler(w, handlerOpts)
	}
	return &Logger{Logger: slog.New(handler)}
}
