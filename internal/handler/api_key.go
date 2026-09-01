package handler

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const apiKeyBytes = 32

func generateAPIKey() (string, error) {
	randomBytes := make([]byte, apiKeyBytes)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}

	return "na_" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}
