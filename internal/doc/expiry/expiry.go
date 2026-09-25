// Package expiry reads when a document stops being valid from its fields, and
// says where it stands today: valid, expiring soon, expired, permanent, or
// not known.
package expiry

import (
	"strings"
	"time"

	"dgs-toolbox/internal/doc/dates"
)

// Keys are the field keys that hold an expiry, tried in order.
var Keys = []string{"expires", "expiry", "expires_at", "expiry_date", "valid_until"}

// DefaultSoon is how long before its expiry a document counts as expiring
// soon.
const DefaultSoon = 90 * 24 * time.Hour

// State is where a document stands.
type State string

const (
	Valid     State = "valid"
	Soon      State = "soon"
	Expired   State = "expired"
	Permanent State = "permanent"
	// None is no expiry field, or one that is not a date.
	None State = "none"
)

// permanent are the words a document with no end date writes instead of one.
var permanent = []string{"长期", "永久", "permanent", "indefinite", "no expiry", "none"}

// Status is a document's expiry: the date as YYYY-MM-DD, when there is one,
// and its State. Days is how many days are left; negative once expired.
type Status struct {
	Date  string `json:"date,omitempty"`
	State State  `json:"state"`
	Days  int    `json:"days,omitempty"`
}

// Of reads the expiry from fields and places it against now. A document
// expires at the end of its expiry day. order reads a date whose day and
// month could be either way round.
func Of(fields map[string]string, now time.Time, soon time.Duration, order dates.Order) Status {
	for _, key := range Keys {
		value := strings.TrimSpace(fields[key])
		if value == "" {
			continue
		}
		lower := strings.ToLower(value)
		for _, word := range permanent {
			if lower == word {
				return Status{State: Permanent}
			}
		}
		date, ok := dates.Parse(value, order)
		if !ok {
			return Status{State: None}
		}
		day, err := time.ParseInLocation("2006-01-02", date, now.Location())
		if err != nil {
			return Status{State: None}
		}
		end := day.AddDate(0, 0, 1)
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := int(day.Sub(today).Hours() / 24)
		switch {
		case !now.Before(end):
			return Status{Date: date, State: Expired, Days: days}
		case end.Sub(now) <= soon:
			return Status{Date: date, State: Soon, Days: days}
		}
		return Status{Date: date, State: Valid, Days: days}
	}
	return Status{State: None}
}
