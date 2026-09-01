package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

const rootSecretBytes = 32

// GenerateRootSecret returns a cryptographically random root secret suitable
// for the sibling secret file used by komitake set --generate-secret.
func GenerateRootSecret() (string, error) {
	buf := make([]byte, rootSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate root secret: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
