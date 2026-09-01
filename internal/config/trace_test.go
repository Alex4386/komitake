package config

import (
	"bytes"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Alex4386/komitake/internal/logging"
)

// Load logs the effective configuration, which includes secret-bearing fields.
// At trace level none of that material may reach the output.
func TestLoadNeverLogsSecrets(t *testing.T) {
	var buf bytes.Buffer

	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(logging.NewLogger(&buf, logging.Options{Level: logging.LevelTrace}).Logger)

	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")

	// Distinctive values so any leak is unambiguous.
	psk := strings.Repeat("ab", 32)
	masterKey := strings.Repeat("cd", 64)
	ident := strings.Repeat("ef", 16)

	body := `{
  "address": "192.168.137.1",
  "secret": "TOP-SECRET-CONFIG-VALUE",
  "wireless": {
    "interface": "wlan0",
    "subnet": "192.168.137.0/24",
    "channel": 6,
    "ssid": "Gtesting",
    "psk": "` + psk + `"
  },
  "rcd": {
    "name": "Komitake",
    "ident": "` + ident + `",
    "master_key": "` + masterKey + `"
  }
}`
	if err := os.WriteFile(configPath, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	rt, err := Load(configPath, Options{})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(rt.GroupInfo.PSK) != 32 {
		t.Fatalf("psk is %d bytes, want 32", len(rt.GroupInfo.PSK))
	}

	out := buf.String()
	if out == "" {
		t.Fatal("no log output was captured")
	}

	for name, needle := range map[string]string{
		"wireless psk":  psk,
		"rcd master":    masterKey,
		"config secret": "TOP-SECRET-CONFIG-VALUE",
	} {
		if strings.Contains(out, needle) {
			t.Fatalf("%s leaked into log output:\n%s", name, out)
		}
	}

	// Also confirm the derived runtime values are absent.
	if strings.Contains(out, hex.EncodeToString(rt.ServerInfo.MasterKey)) {
		t.Fatalf("derived master key leaked:\n%s", out)
	}

	// Non-secret operational fields should still be present, since that is the
	// point of logging the effective configuration.
	for _, needle := range []string{"wlan0", "Gtesting", "192.168.137.1"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("expected %q in the effective config log:\n%s", needle, out)
		}
	}
}
