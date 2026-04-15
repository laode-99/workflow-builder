// Package hours provides project-aware business-hours checks for the leadflow engine.
//
// Every outbound dispatch (Retell calls, Gupshup bridging, Gupshup final) is
// gated by InBusinessHours using the project's configured start/end/timezone.
// Webhooks always process regardless of business hours — only *new dispatches*
// respect the window.
package hours

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is a business-hours window for a single project.
type Window struct {
	Start    string // "HH:MM" local
	End      string // "HH:MM" local, exclusive
	Timezone string // IANA name, e.g. "Asia/Jakarta"
}

// InBusinessHours reports whether `now` falls within [start, end) in the window's timezone.
func (w Window) InBusinessHours(now time.Time) (bool, error) {
	loc, err := time.LoadLocation(w.Timezone)
	if err != nil {
		return false, fmt.Errorf("load timezone %q: %w", w.Timezone, err)
	}
	sh, sm, err := parseHM(w.Start)
	if err != nil {
		return false, fmt.Errorf("parse start %q: %w", w.Start, err)
	}
	eh, em, err := parseHM(w.End)
	if err != nil {
		return false, fmt.Errorf("parse end %q: %w", w.End, err)
	}

	local := now.In(loc)
	start := time.Date(local.Year(), local.Month(), local.Day(), sh, sm, 0, 0, loc)
	end := time.Date(local.Year(), local.Month(), local.Day(), eh, em, 0, 0, loc)

	return !local.Before(start) && local.Before(end), nil
}

func parseHM(hm string) (int, int, error) {
	parts := strings.Split(strings.TrimSpace(hm), ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected HH:MM, got %q", hm)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("parse hour: %w", err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse minute: %w", err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("out of range %q", hm)
	}
	return h, m, nil
}
