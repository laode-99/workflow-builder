package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifyHMAC checks if the given signature matches the HMAC-SHA256 hash
// of the payload using the provided secret.
func VerifyHMAC(payload []byte, secret, signature string) bool {
	if secret == "" || signature == "" {
		return false
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expected := hex.EncodeToString(h.Sum(nil))

	return hmac.Equal([]byte(expected), []byte(signature))
}
