package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSocketPerms(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		input string
		want  os.FileMode
	}{
		{"", DefaultSocketPerm},
		{"0777", 0o777},
		{"777", 0o777},
		{"0o600", 0o600},
		{"0644", 0o644},
	} {
		got, err := ParseSocketPerms(testCase.input)
		if err != nil || got != testCase.want {
			t.Fatalf("ParseSocketPerms(%q) = %#o, %v; want %#o", testCase.input, got, err, testCase.want)
		}
	}
	for _, invalid := range []string{"xyz", "1000"} {
		if _, err := ParseSocketPerms(invalid); err == nil {
			t.Fatalf("ParseSocketPerms(%q) accepted invalid mode", invalid)
		}
	}
}

func TestWriteServiceSettingsPreservesAndMigratesConfig(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "config.json")
	body := `{
  "address": "192.168.137.1",
  "listen": "unix:/tmp/legacy.sock",
  "secret": "keep-me",
  "future": {"enabled": true},
  "web": {"tls": {"future_option": "keep-tls-too"}},
  "wireless": {"interface": "wlan0", "subnet": "192.168.137.0/24"}
}
`
	if err := os.WriteFile(path, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}

	settings, err := WriteServiceSettings(
		path,
		WebFile{Bind: "0.0.0.0:8080", TLS: WebTLSFile{
			Enabled:  true,
			CertFile: "/etc/komitake/web.crt",
			KeyFile:  "/etc/komitake/web.key",
		}},
		SocketFile{Bind: "unix:/tmp/komitake.sock", Chmod: "0770"},
		VideoFile{},
		WebRTCFile{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Web.Bind != "0.0.0.0:8080" || !settings.Web.TLS.Enabled || settings.Web.TLS.CertFile != "/etc/komitake/web.crt" || settings.Web.TLS.KeyFile != "/etc/komitake/web.key" || settings.Socket.Bind != "unix:/tmp/komitake.sock" || settings.Socket.Chmod != "0770" {
		t.Fatalf("settings = %+v", settings)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := string(raw)
	for _, wanted := range []string{
		`"secret": "keep-me"`,
		`"future": {`,
		`"future_option": "keep-tls-too"`,
		`"address": "192.168.137.1/24"`,
		`"bind": "0.0.0.0:8080"`,
		`"enabled": true`,
		`"cert_file": "/etc/komitake/web.crt"`,
		`"key_file": "/etc/komitake/web.key"`,
		`"chmod": "0770"`,
	} {
		if !strings.Contains(result, wanted) {
			t.Fatalf("missing %s in:\n%s", wanted, result)
		}
	}
	for _, legacy := range []string{`"listen"`, `"web_addr"`, `"socket_perms"`, `"subnet"`, `"perm"`} {
		if strings.Contains(result, legacy) {
			t.Fatalf("legacy key %s remains in:\n%s", legacy, result)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("config mode = %#o, want 0640", info.Mode().Perm())
	}
}

func TestWriteServiceSettingsValidation(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, testCase := range map[string]struct {
		web    WebFile
		socket SocketFile
	}{
		"web bind":      {web: WebFile{Bind: "not-a-host-port"}},
		"TLS cert only": {web: WebFile{TLS: WebTLSFile{Enabled: true, CertFile: "server.crt"}}},
		"TLS key only":  {web: WebFile{TLS: WebTLSFile{Enabled: true, KeyFile: "server.key"}}},
		"socket bind":   {socket: SocketFile{Bind: "tcp:not-a-host-port"}},
		"socket chmod":  {socket: SocketFile{Chmod: "0999"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := WriteServiceSettings(path, testCase.web, testCase.socket, VideoFile{}, WebRTCFile{}, nil, nil); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestResolveWebSettingsReadsTLS(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"web":{"bind":" 0.0.0.0:9443 ","tls":{"enabled":true,"cert_file":" server.crt ","key_file":" server.key "}}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	web, err := ResolveWebSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if web.Bind != "0.0.0.0:9443" || !web.TLS.Enabled || web.TLS.CertFile != "server.crt" || web.TLS.KeyFile != "server.key" {
		t.Fatalf("web settings = %+v", web)
	}
}

func TestBuildRuntimeNestedAndLegacyConfig(t *testing.T) {
	t.Parallel()
	runtime, err := BuildRuntime(File{
		Secret:   "secret",
		Socket:   SocketFile{Bind: "unix:/tmp/nested.sock", Chmod: "0770"},
		Web:      WebFile{Bind: "0.0.0.0:9090"},
		Wireless: WirelessFile{Interface: "wlan0", Address: "192.168.50.2/24"},
	}, "/tmp/config.json", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.Listen.Address != "/tmp/nested.sock" || runtime.WebAddr != "0.0.0.0:9090" || runtime.SocketPerm != 0o770 || runtime.Address != "192.168.50.2" || runtime.Subnet.String() != "192.168.50.0/24" {
		t.Fatalf("runtime = %+v", runtime)
	}

	legacyRuntime, err := BuildRuntime(File{
		Secret:   "secret",
		Address:  "10.0.0.1",
		Listen:   "unix:/tmp/legacy.sock",
		Wireless: WirelessFile{Interface: "wlan0", Subnet: "10.0.0.0/24"},
	}, "/tmp/config.json", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if legacyRuntime.Address != "10.0.0.1" || legacyRuntime.Listen.Address != "/tmp/legacy.sock" || legacyRuntime.Subnet.String() != "10.0.0.0/24" {
		t.Fatalf("legacy runtime = %+v", legacyRuntime)
	}
}
