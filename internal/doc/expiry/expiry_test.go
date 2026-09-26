package expiry

import (
	"testing"
	"time"

	"dgs-toolbox/internal/doc/dates"
)

func TestOf(t *testing.T) {
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		fields map[string]string
		want   Status
	}{
		{map[string]string{"expires": "2036.01.01"}, Status{Date: "2036-01-01", State: Valid, Days: 3385}},
		{map[string]string{"expires": "2026-10-01"}, Status{Date: "2026-10-01", State: Soon, Days: 6}},
		{map[string]string{"expires": "2026-09-25"}, Status{Date: "2026-09-25", State: Soon}},
		{map[string]string{"expires": "2026-09-24"}, Status{Date: "2026-09-24", State: Expired, Days: -1}},
		{map[string]string{"expiry": "长期"}, Status{State: Permanent}},
		{map[string]string{"valid_until": "Permanent"}, Status{State: Permanent}},
		{map[string]string{"expires": "soon-ish"}, Status{State: None}},
		{map[string]string{"owner": "jane"}, Status{State: None}},
	} {
		if got := Of(c.fields, now, DefaultSoon, dates.DMY); got != c.want {
			t.Errorf("%v: got %+v, want %+v", c.fields, got, c.want)
		}
	}
}

func TestMonthExpiry(t *testing.T) {
	for _, c := range []struct {
		month, now, date string
		state            State
	}{
		{"2024-02", "2024-02-29T23:59:59Z", "2024-02-29", Soon},
		{"2024-02", "2024-03-01T00:00:00Z", "2024-02-29", Expired},
		{"2025-02", "2025-02-28T23:59:59Z", "2025-02-28", Soon},
		{"2025-12", "2026-01-01T00:00:00Z", "2025-12-31", Expired},
		{"2025-13", "2025-01-01T00:00:00Z", "", None},
	} {
		now, err := time.Parse(time.RFC3339, c.now)
		if err != nil {
			t.Fatal(err)
		}
		got := Of(map[string]string{"expires": c.month}, now, DefaultSoon, dates.DMY)
		if got.Date != c.date || got.State != c.state {
			t.Errorf("%+v: got %+v", c, got)
		}
	}
}
