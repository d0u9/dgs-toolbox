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
