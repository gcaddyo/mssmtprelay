package util

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// RandomString returns a URL-safe random string with requested visible length.
func RandomString(length int) (string, error) {
	if length <= 0 {
		return "", fmt.Errorf("invalid length: %d", length)
	}
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate random bytes: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(buf)
	if len(encoded) >= length {
		return encoded[:length], nil
	}
	return encoded, nil
}
