package hours

import (
	"testing"
	"time"
)

func TestInBusinessHours(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load jakarta: %v", err)
	}

	w := Window{Start: "07:00", End: "20:00", Timezone: "Asia/Jakarta"}

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{"just before window", time.Date(2026, 4, 15, 6, 59, 0, 0, jakarta), false},
		{"at start", time.Date(2026, 4, 15, 7, 0, 0, 0, jakarta), true},
		{"middle of day", time.Date(2026, 4, 15, 12, 30, 0, 0, jakarta), true},
		{"one minute before end", time.Date(2026, 4, 15, 19, 59, 0, 0, jakarta), true},
		{"at end (exclusive)", time.Date(2026, 4, 15, 20, 0, 0, 0, jakarta), false},
		{"after end", time.Date(2026, 4, 15, 21, 0, 0, 0, jakarta), false},
		{"midnight", time.Date(2026, 4, 15, 0, 0, 0, 0, jakarta), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := w.InBusinessHours(tc.now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("at %s: got %v, want %v", tc.now, got, tc.want)
			}
		})
	}
}

func TestInBusinessHoursUTCInput(t *testing.T) {
	// 03:00 UTC == 10:00 Jakarta (inside window)
	w := Window{Start: "07:00", End: "20:00", Timezone: "Asia/Jakarta"}
	utc := time.Date(2026, 4, 15, 3, 0, 0, 0, time.UTC)
	got, err := w.InBusinessHours(utc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("UTC 03:00 should map to Jakarta 10:00 (inside window)")
	}

	// 14:00 UTC == 21:00 Jakarta (outside window)
	utc2 := time.Date(2026, 4, 15, 14, 0, 0, 0, time.UTC)
	got2, err := w.InBusinessHours(utc2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 {
		t.Errorf("UTC 14:00 should map to Jakarta 21:00 (outside window)")
	}
}

func TestInBusinessHoursInvalidTimezone(t *testing.T) {
	w := Window{Start: "07:00", End: "20:00", Timezone: "Mars/Olympus"}
	_, err := w.InBusinessHours(time.Now())
	if err == nil {
		t.Errorf("expected error for invalid timezone")
	}
}

func TestInBusinessHoursInvalidFormat(t *testing.T) {
	cases := []Window{
		{Start: "7", End: "20:00", Timezone: "Asia/Jakarta"},
		{Start: "07:00", End: "25:00", Timezone: "Asia/Jakarta"},
		{Start: "ab:cd", End: "20:00", Timezone: "Asia/Jakarta"},
	}
	for _, w := range cases {
		_, err := w.InBusinessHours(time.Now())
		if err == nil {
			t.Errorf("expected error for window %+v", w)
		}
	}
}
