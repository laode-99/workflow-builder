package statemachine

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseIndoDate converts Indonesian date components to a time.Time.
// tanggal: 1-31
// bulan: Jan/Januari/1 or Me/Mei/5
// jam: HH:mm (24h)
func ParseIndoDate(tanggal, bulan, tahun, jam string) (*time.Time, error) {
	t, _ := strconv.Atoi(tanggal)
	y, _ := strconv.Atoi(tahun)
	if y < 2024 {
		y = time.Now().Year()
	}

	m := parseIndoMonth(bulan)
	if m == 0 {
		return nil, fmt.Errorf("invalid month: %s", bulan)
	}

	timeStr := fmt.Sprintf("%04d-%02d-%02d %s", y, m, t, jam)
	loc, _ := time.LoadLocation("Asia/Jakarta")
	parsed, err := time.ParseInLocation("2006-01-02 15:04", timeStr, loc)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseIndoMonth(s string) time.Month {
	s = strings.ToLower(s)
	months := map[string]time.Month{
		"jan": 1, "januari": 1, "1": 1,
		"feb": 2, "februari": 2, "2": 2,
		"mar": 3, "maret": 3, "3": 3,
		"apr": 4, "april": 4, "4": 4,
		"mei": 5, "5": 5,
		"jun": 6, "juni": 6, "6": 6,
		"jul": 7, "juli": 7, "7": 7,
		"agu": 8, "agustus": 8, "8": 8,
		"sep": 9, "september": 9, "9": 9,
		"okt": 10, "oktober": 10, "10": 10,
		"nov": 11, "november": 11, "11": 11,
		"des": 12, "desember": 12, "12": 12,
	}
	for k, v := range months {
		if strings.HasPrefix(s, k) {
			return v
		}
	}
	return 0
}

// FormatSvsDate returns the M/D/YYYY H:mm:ss AM/PM format required by LeadSquared.
func FormatSvsDate(t time.Time) string {
	return t.Format("1/2/2006 3:04:05 PM")
}

// PatchToMap converts a Patch struct to a map for GORM Updates()
func PatchToMap(p Patch) map[string]interface{} {
	m := make(map[string]interface{})
	if p.Attempt != nil { m["attempt"] = *p.Attempt }
	if p.CallDate != nil { m["call_date"] = *p.CallDate }
	if p.Interest != nil { m["interest"] = *p.Interest }
	if p.Interest2 != nil { m["interest2"] = *p.Interest2 }
	if p.CustomerType != nil { m["customer_type"] = *p.CustomerType }
	if p.DisconnectedReason != nil { m["disconnected_reason"] = *p.DisconnectedReason }
	if p.TerminalInvalid != nil { m["terminal_invalid"] = *p.TerminalInvalid }
	if p.TerminalResponded != nil { m["terminal_responded"] = *p.TerminalResponded }
	if p.TerminalNotInterested != nil { m["terminal_not_interested"] = *p.TerminalNotInterested }
	if p.TerminalSpam != nil { m["terminal_spam"] = *p.TerminalSpam }
	if p.TerminalAgent != nil { m["terminal_agent"] = *p.TerminalAgent }
	if p.TerminalCompleted != nil { m["terminal_completed"] = *p.TerminalCompleted }
	if p.Summary != nil { m["summary"] = *p.Summary }
	if p.WhatsappSentAt != nil { m["whatsapp_sent_at"] = *p.WhatsappSentAt }
	if p.WhatsappReplyAt != nil { m["whatsapp_reply_at"] = *p.WhatsappReplyAt }
	if p.SentToDev != nil { m["sent_to_dev"] = *p.SentToDev }
	if p.SentToWaGroupAt != nil { m["sent_to_wa_group_at"] = *p.SentToWaGroupAt }
	return m
}

// IsWithinBusinessHours checks if the given time is within the project's BH window.
func IsWithinBusinessHours(t time.Time, cfg BusinessHoursCfg) bool {
	loc, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		loc = time.UTC
	}
	nowLocal := t.In(loc)
	current := nowLocal.Format("15:04")
	return current >= cfg.Start && current < cfg.End
}
