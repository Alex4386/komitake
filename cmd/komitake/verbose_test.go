package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	adminv1 "github.com/Alex4386/komitake/proto/komitake/admin/v1"
	"github.com/Alex4386/komitake/internal/logging"
)

func TestResolveLevelFromVerbosity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		verbose int
		level   string
		want    slog.Level
	}{
		{name: "default", want: slog.LevelInfo},
		{name: "-v", verbose: 1, want: slog.LevelDebug},
		{name: "-vv", verbose: 2, want: logging.LevelTrace},
		{name: "-vvv saturates", verbose: 3, want: logging.LevelTrace},
		// An explicit level must win, so scripts are unambiguous.
		{name: "explicit beats -vv", verbose: 2, level: "warn", want: slog.LevelWarn},
		{name: "explicit trace", level: "trace", want: logging.LevelTrace},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &options{verbose: tt.verbose, logLevel: tt.level}
			got, err := opts.resolveLevel()
			if err != nil {
				t.Fatalf("resolveLevel() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("level = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResolveLevelRejectsUnknown(t *testing.T) {
	t.Parallel()

	if _, err := (&options{logLevel: "chatty"}).resolveLevel(); err == nil {
		t.Fatal("expected an error for an unknown level")
	}
}

// Logs must go to stderr so stdout stays parseable, including with --json.
func TestVerboseLogsGoToStderrOnly(t *testing.T) {
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return stubAdminClient{}, noopCloser{}, nil
	}
	t.Cleanup(func() { adminDial = dialAdmin })

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var out, errOut bytes.Buffer
	cmd := New()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"-vv", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !strings.Contains(errOut.String(), "connecting to daemon") {
		t.Fatalf("expected debug output on stderr, got %q", errOut.String())
	}
	// stdout must contain only the command result.
	if strings.TrimSpace(out.String()) != "normal" {
		t.Fatalf("stdout = %q, want only the mode name", out.String())
	}
}

func TestVerboseFlagRejectsBadLevel(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--log-level", "chatty", "status"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown log level")
	}
}

func TestVerboseFlagRejectsBadFormat(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	cmd := New()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--log-format", "yaml", "status"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected an error for an unknown log format")
	}
}

// Default verbosity must stay silent so normal output is not polluted.
func TestDefaultVerbosityIsQuiet(t *testing.T) {
	adminDial = func(context.Context, endpoint) (adminv1.AdminServiceClient, adminCloser, error) {
		return stubAdminClient{}, noopCloser{}, nil
	}
	t.Cleanup(func() { adminDial = dialAdmin })

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	var out, errOut bytes.Buffer
	cmd := New()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty at default verbosity", errOut.String())
	}
}
