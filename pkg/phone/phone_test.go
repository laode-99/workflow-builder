package phone

import (
	"errors"
	"testing"
)

func TestNormalizeID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"leading zero", "081234567890", "6281234567890", nil},
		{"plus country code spaces", "+62 812 3456 7890", "6281234567890", nil},
		{"plus country code dashes", "+62-812-3456-7890", "6281234567890", nil},
		{"bare country code", "6281234567890", "6281234567890", nil},
		{"bare mobile without prefix", "8123456789", "628123456789", nil},
		{"mixed whitespace", "  0812 3456 789  ", "628123456789", nil},
		{"empty", "", "", ErrEmpty},
		{"all non-digits", "abc-def", "", ErrEmpty},
		{"too short", "628", "", ErrTooShort},
		{"too long", "6281234567890123456", "", ErrTooLong},
		{"with leading zeros only", "00812", "", ErrTooShort},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeID(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("input %q: expected error %v, got %v (result %q)", tc.input, tc.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("input %q: unexpected error %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("input %q: got %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeIDIdempotent(t *testing.T) {
	inputs := []string{
		"081234567890",
		"+62 812 3456 7890",
		"8123456789",
		"6281234567890",
	}
	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			first, err := NormalizeID(in)
			if err != nil {
				t.Fatalf("first normalize failed: %v", err)
			}
			second, err := NormalizeID(first)
			if err != nil {
				t.Fatalf("second normalize failed: %v", err)
			}
			if first != second {
				t.Errorf("not idempotent: first=%q second=%q", first, second)
			}
		})
	}
}
