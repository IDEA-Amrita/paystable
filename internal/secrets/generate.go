package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateSecureKey returns 32 cryptographically random bytes as 64 hexchars.
func GenerateSecureKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate secure key: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// This func returns local first secrets for a fresh install.
// SECRET_ENCRYPTION_KEY is 64 hex chars (32 bytes) which ParseKey accepts.
func GenerateAll() (map[string]string, error) {
	keys := []string{
		"WEBHOOK_SECRET",
		"MERCHANT_CALLBACK_SECRET",
		"ADMIN_API_KEY",
		"SECRET_ENCRYPTION_KEY",
	}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		val, err := GenerateSecureKey()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = val
	}
	return out, nil
}
