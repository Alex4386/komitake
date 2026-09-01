//go:build ignore

// Command gennonces fetches the OpenKart lp2p nonce table and writes it as the
// raw binary embedded by the wireless package. The table is pre-encrypted data
// derived from a key OpenKart does not distribute, so it is extracted from
// their published Python source rather than recomputed.
//
// Run via `go generate ./internal/wireless`.
package main

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

const (
	sourceURL   = "https://raw.githubusercontent.com/OpenKart-SDK/openkart/refs/heads/master/openkartd/lp2p_nonces.py"
	outputFile  = "lp2p_nonces.bin"
	nonceCount  = 0x10000
	nonceSize   = 16
	wantBytes   = nonceCount * nonceSize
	beginMarker = "NONCES = \"\"\"\\"
	endMarker   = "\"\"\""
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gennonces:", err)
		os.Exit(1)
	}
}

func run() error {
	src, err := fetch(sourceURL)
	if err != nil {
		return err
	}

	b64, err := extractNonces(src)
	if err != nil {
		return err
	}

	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return fmt.Errorf("decode nonce table: %w", err)
	}
	if len(raw) != wantBytes {
		return fmt.Errorf("nonce table is %d bytes, want %d", len(raw), wantBytes)
	}

	if err := os.WriteFile(outputFile, raw, 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", outputFile, len(raw))
	return nil
}

func fetch(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s: %s", url, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// extractNonces pulls the base64 body of the NONCES triple-quoted string and
// strips its newlines.
func extractNonces(src string) (string, error) {
	start := strings.Index(src, beginMarker)
	if start < 0 {
		return "", fmt.Errorf("%q not found in source", beginMarker)
	}
	start += len(beginMarker)

	rest := src[start:]
	end := strings.Index(rest, endMarker)
	if end < 0 {
		return "", fmt.Errorf("closing %q not found", endMarker)
	}

	body := rest[:end]
	return strings.ReplaceAll(body, "\n", ""), nil
}
