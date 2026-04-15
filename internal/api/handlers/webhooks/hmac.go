// Package webhooks contains the HTTP handlers that receive external events
// (Retell call outcomes, forwarded Gupshup chat messages) and translate them
// into state machine events + Asynq tasks.
//
// Every handler verifies an HMAC-SHA256 signature against a per-project
// shared secret stored as a credential with integration = "webhook_secret".
// Bad signature → 401, logged, nothing else.
package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/gofiber/fiber/v2"
)

// ErrBadSignature is returned by VerifyHMAC when the header signature does
// not match the body's computed signature.
var ErrBadSignature = errors.New("webhook: bad HMAC signature")

// VerifyHMAC computes HMAC-SHA256(body, secret) and compares it to the
// hex-encoded header value. Returns nil on match, ErrBadSignature on
// mismatch, or nil if the secret is empty (HMAC disabled for this project).
func VerifyHMAC(secret string, body []byte, headerValue string) error {
	if secret == "" {
		return nil // HMAC disabled
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(headerValue)) {
		return ErrBadSignature
	}
	return nil
}

// applyHMAC is a Fiber-friendly wrapper. Returns a 401 JSON response if the
// signature is wrong; otherwise passes through.
func applyHMAC(c *fiber.Ctx, secret string) error {
	if err := VerifyHMAC(secret, c.Body(), c.Get("X-Signature")); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "bad signature"})
	}
	return nil
}
