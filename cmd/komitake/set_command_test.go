package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetCommandUpdatesWirelessInterface(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"wireless":{"interface":"wlan0","address":"192.168.137.1/24"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"set", "--config", path, "--wireless-interface=wlan1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"interface": "wlan1"`) {
		t.Fatalf("config = %s", raw)
	}
}

func TestSetCommandGenerateSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"set", "--config", path, "--generate-secret"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	secret := strings.TrimSpace(string(data))
	if len(secret) != 64 {
		t.Fatalf("secret length = %d, want 64 hex chars", len(secret))
	}
	if strings.Contains(out.String(), secret) {
		t.Fatalf("secret was printed to stdout: %q", out.String())
	}
}

func TestSetCommandGenerateSecretConflictsWithSecret(t *testing.T) {
	cmd := New()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{"set", "--generate-secret", "--secret=manual"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when both secret flags are passed")
	}
}

func TestSetCommandSetsVideoFFmpegProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{"wireless":{"interface":"wlan0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := New()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{"set", "--config", path, "--video-ffmpeg-profile=realtime"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"ffmpeg_profile": "realtime"`) {
		t.Fatalf("config = %s", raw)
	}
}

func TestSetCommandRequiresAFlag(t *testing.T) {
	cmd := New()
	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)
	cmd.SetArgs([]string{"set"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error when no flags are passed")
	}
}
