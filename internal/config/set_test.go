package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	secretPath := DefaultSecretFile(path)
	if err := os.WriteFile(secretPath, []byte("test-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestApplySettingsChangesWirelessAndWeb(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, `{
  "secret": "ignored-by-secret-file",
  "wireless": {"interface": "wlan0", "address": "192.168.137.1/24"},
  "future": {"enabled": true}
}`)

	webBind := "0.0.0.0:9090"
	iface := "wlan1"
	if err := ApplySettingsChanges(path, SettingsChanges{
		WebBind:           &webBind,
		WirelessInterface: &iface,
	}); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := string(raw)
	for _, wanted := range []string{
		`"bind": "0.0.0.0:9090"`,
		`"interface": "wlan1"`,
		`"future": {`,
		`"address": "192.168.137.1/24"`,
	} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("missing %s in:\n%s", wanted, result)
		}
	}
	if strings.Contains(result, `"listen"`) {
		t.Fatalf("legacy keys remain:\n%s", result)
	}
}

func TestGenerateRootSecret(t *testing.T) {
	t.Parallel()
	first, err := GenerateRootSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateRootSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != rootSecretBytes*2 {
		t.Fatalf("secret length = %d, want %d hex chars", len(first), rootSecretBytes*2)
	}
	if first == second {
		t.Fatal("expected distinct secrets")
	}
}

func TestApplySettingsChangesSecretFile(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, `{}`)
	secret := "stable-root-secret"
	if err := ApplySettingsChanges(path, SettingsChanges{Secret: &secret}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(DefaultSecretFile(path))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != secret {
		t.Fatalf("secret file = %q", string(data))
	}
}

func TestApplySettingsChangesValidation(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, `{}`)
	badBind := "not-a-host-port"
	if err := ApplySettingsChanges(path, SettingsChanges{WebBind: &badBind}); err == nil {
		t.Fatal("expected web.bind validation error")
	}
}

func TestApplySettingsChangesRequiresSSIDAndPSKTogether(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, `{}`)
	ssid := "MyKartNet"
	if err := ApplySettingsChanges(path, SettingsChanges{WirelessSSID: &ssid}); err == nil {
		t.Fatal("expected ssid/psk pairing error")
	}
}

func TestWriteServiceSettingsStillMigratesLegacyKeys(t *testing.T) {
	t.Parallel()
	path := writeTestConfig(t, `{
  "address": "192.168.137.1",
  "listen": "unix:/tmp/legacy.sock",
  "secret": "keep-me",
  "wireless": {"interface": "wlan0", "subnet": "192.168.137.0/24"}
}`)

	settings, err := WriteServiceSettings(
		path,
		WebFile{Bind: "0.0.0.0:8080", TLS: WebTLSFile{Enabled: true, CertFile: "/etc/komitake/web.crt", KeyFile: "/etc/komitake/web.key"}},
		SocketFile{Bind: "unix:/tmp/komitake.sock", Chmod: "0770"},
		VideoFile{},
		WebRTCFile{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Web.Bind != "0.0.0.0:8080" || settings.Socket.Chmod != "0770" {
		t.Fatalf("settings = %+v", settings)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := string(raw)
	for _, legacy := range []string{`"listen"`, `"subnet"`, `"perm"`} {
		if strings.Contains(result, legacy) {
			t.Fatalf("legacy key %s remains in:\n%s", legacy, result)
		}
	}
}
