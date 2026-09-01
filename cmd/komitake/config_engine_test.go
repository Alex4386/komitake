package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Alex4386/komitake/internal/config"
	"github.com/Alex4386/komitake/internal/logging"
)

func TestListenAdminAppliesConfiguredSocketMode(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name       string
		mode       os.FileMode
		configured bool
		want       os.FileMode
	}{
		{name: "default", want: config.DefaultSocketPerm},
		{name: "configured", mode: 0o770, configured: true, want: 0o770},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			temporaryFile, err := os.CreateTemp("/tmp", "komitake-*.sock")
			if err != nil {
				t.Fatal(err)
			}
			path := temporaryFile.Name()
			_ = temporaryFile.Close()
			_ = os.Remove(path)
			t.Cleanup(func() { _ = os.Remove(path) })
			listener, cleanup, err := listenAdmin(
				config.IPCAddress{Network: "unix", Address: path},
				testCase.mode,
				testCase.configured,
				logging.NewLogger(io.Discard, logging.Options{}),
			)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			defer listener.Close()
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != testCase.want {
				t.Fatalf("socket mode = %#o, want %#o", info.Mode().Perm(), testCase.want)
			}
		})
	}
}

func TestResolveEndpointUsesNestedSocketBind(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"socket":{"bind":"unix:/tmp/nested.sock"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	endpoint, err := resolveEndpoint(&options{configPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Network != "unix" || endpoint.Address != "/tmp/nested.sock" {
		t.Fatalf("endpoint = %+v", endpoint)
	}
}

func TestResolveWebAddrUsesNestedBind(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"web":{"bind":"0.0.0.0:9090"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := config.ResolveWebAddr(path); got != "0.0.0.0:9090" {
		t.Fatalf("ResolveWebAddr() = %q", got)
	}
}
