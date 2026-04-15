// Package phone provides canonical normalization for Indonesian phone numbers.
//
// All inbound phone numbers in the leadflow engine (from LeadSquared,
// Retell webhooks, Gupshup webhooks, 2Chat lookups, manual entry) must
// pass through NormalizeID before being compared or stored. This guarantees
// a single canonical format and eliminates cross-matching failures.
package phone

import (
	"errors"
	"strings"
	"unicode"
)

const (
	// CountryCode is the canonical Indonesian country code prefix (no "+").
	CountryCode = "62"

	// MinDigits is the minimum acceptable total length including country code.
	// Indonesian mobile numbers are typically 10-13 digits total.
	MinDigits = 10

	// MaxDigits is E.164's maximum length.
	MaxDigits = 15
)

var (
	ErrEmpty    = errors.New("phone: empty input")
	ErrTooShort = errors.New("phone: too short")
	ErrTooLong  = errors.New("phone: too long")
)

// NormalizeID canonicalizes an Indonesian phone number to "62xxx" form.
// Accepts any of: "+62 812 3456 7890", "081234567890", "6281234567890",
// "8123456789". Returns the canonical string (no "+", no spaces, no dashes).
func NormalizeID(raw string) (string, error) {
	if raw == "" {
		return "", ErrEmpty
	}

	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "", ErrEmpty
	}

	// Leading "0" → replace with country code.
	if strings.HasPrefix(s, "0") {
		s = CountryCode + s[1:]
	}

	// Add country code if the number doesn't already start with it.
	// A bare "8123..." (starts with 8, the common mobile prefix) without
	// country code gets the prefix added.
	if !strings.HasPrefix(s, CountryCode) {
		s = CountryCode + s
	}

	if len(s) < MinDigits {
		return "", ErrTooShort
	}
	if len(s) > MaxDigits {
		return "", ErrTooLong
	}

	return s, nil
}
