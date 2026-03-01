// Package hmac provides HMAC-SHA256 signing and verification for Glawmail messages.
package hmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// Sign creates an HMAC-SHA256 signature for the given payload.
// Returns "sha256=<hex_digest>".
func Sign(secret, payload string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// Verify checks that the signature matches the payload.
// Uses constant-time comparison to prevent timing attacks.
func Verify(secret, payload, signature string) bool {
	expected := Sign(secret, payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}
