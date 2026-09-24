// Package box holds the small values every part of a Box is written in: the
// date on a document, and the zone that date was read in. The algorithms are
// subpackages — doctype, money, lifecycle — and the commands compose them.
package box

import (
	"fmt"
	"strings"
	"time"
)

// Date is a day on a document, with no time and no zone of its own: the 11th
// of March printed on a ticket is the 11th of March wherever it is later
// sorted. A Date is paired with a Zone when it matters which place's day it
// was, because that is a separate fact and is stored separately.
//
// An event date may be known only to the month or the year: Day, or Month and
// Day, are then 0. Such a date is written as 2019-03 or 2019, sorts before
// every day inside it, and does arithmetic from its last day, so an expiry
// computed from "some time in 2019" is never earlier than the real one.
//
// The zero Date is no date. An event nobody has dated yet has one, and
// arithmetic on it is refused rather than answered from year zero.
type Date struct {
	Year  int
	Month int
	Day   int
}

// Zero reports whether d is no date at all.
func (d Date) Zero() bool { return d == Date{} }

// Partial reports whether d is known only to the month or the year.
func (d Date) Partial() bool { return !d.Zero() && (d.Month == 0 || d.Day == 0) }

// ParseDate reads an ISO 8601 calendar date — 2019-03-11. It refuses anything
// else, including a date with a time or a zone on it: those are instants, which
// are a different field.
func ParseDate(text string) (Date, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Date{}, fmt.Errorf("read date: empty")
	}
	parsed, err := time.Parse("2006-01-02", text)
	if err != nil {
		return Date{}, fmt.Errorf("read date %q: want YYYY-MM-DD", text)
	}
	return Date{Year: parsed.Year(), Month: int(parsed.Month()), Day: parsed.Day()}, nil
}

// ParseEventDate reads a date that may be known only to the month or the
// year: 2019-03-11, 2019-03 or 2019. An event date is the one field where
// "some time in 2019" is an honest answer and a made-up day is not.
func ParseEventDate(text string) (Date, error) {
	text = strings.TrimSpace(text)
	for _, layout := range []struct {
		format string
		keep   func(time.Time) Date
	}{
		{"2006", func(t time.Time) Date { return Date{Year: t.Year()} }},
		{"2006-01", func(t time.Time) Date { return Date{Year: t.Year(), Month: int(t.Month())} }},
	} {
		if len(text) == len(layout.format) {
			parsed, err := time.Parse(layout.format, text)
			if err != nil {
				return Date{}, fmt.Errorf("read date %q: want YYYY, YYYY-MM or YYYY-MM-DD", text)
			}
			return layout.keep(parsed), nil
		}
	}
	date, err := ParseDate(text)
	if err != nil && text != "" {
		return Date{}, fmt.Errorf("read date %q: want YYYY, YYYY-MM or YYYY-MM-DD", text)
	}
	return date, err
}

// String writes the date back as YYYY-MM-DD, or YYYY-MM or YYYY when that is
// all that is known, which is what goes into a sidecar. The zero Date writes
// as empty, so a missing date stays missing rather than becoming a date in
// year zero.
func (d Date) String() string {
	switch {
	case d.Zero():
		return ""
	case d.Month == 0:
		return fmt.Sprintf("%04d", d.Year)
	case d.Day == 0:
		return fmt.Sprintf("%04d-%02d", d.Year, d.Month)
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, d.Month, d.Day)
}

// Last is the last day d can be: d itself when the day is known, else the
// last day of its month or year.
func (d Date) Last() Date {
	switch {
	case d.Zero() || !d.Partial():
		return d
	case d.Month == 0:
		return Date{Year: d.Year, Month: 12, Day: 31}
	}
	last := time.Date(d.Year, time.Month(d.Month)+1, 0, 0, 0, 0, 0, time.UTC)
	return Date{Year: d.Year, Month: d.Month, Day: last.Day()}
}

// AddDays moves a date by whole days. It goes through the calendar rather than
// through 24-hour arithmetic, so a lifetime crossing a daylight-saving change
// still lands on the day it should.
//
// A partial date counts from its last day, so a lifetime never ends before it
// would have from any day the date could be.
func (d Date) AddDays(days int) Date {
	d = d.Last()
	if d.Zero() {
		return Date{}
	}
	moved := d.time(time.UTC).AddDate(0, 0, days)
	return Date{Year: moved.Year(), Month: int(moved.Month()), Day: moved.Day()}
}

// Before reports whether d falls earlier in the calendar than other. The zero
// Date is before nothing: it is not a point on the calendar at all.
func (d Date) Before(other Date) bool {
	if d.Zero() || other.Zero() {
		return false
	}
	if d.Year != other.Year {
		return d.Year < other.Year
	}
	if d.Month != other.Month {
		return d.Month < other.Month
	}
	return d.Day < other.Day
}

// After reports whether d falls later in the calendar than other.
func (d Date) After(other Date) bool { return other.Before(d) }

func (d Date) time(location *time.Location) time.Time {
	return time.Date(d.Year, time.Month(d.Month), d.Day, 0, 0, 0, 0, location)
}

// Today is the calendar date it is now where location is. Which location that
// should be is the caller's decision, because "today" in Sydney and "today" in
// Tokyo are different days for several hours of every day.
func Today(location *time.Location) Date {
	if location == nil {
		location = time.Local
	}
	now := time.Now().In(location)
	return Date{Year: now.Year(), Month: int(now.Month()), Day: now.Day()}
}

// DateOf is the calendar date an instant falls on, read in location.
func DateOf(instant time.Time, location *time.Location) Date {
	if location == nil {
		location = time.Local
	}
	local := instant.In(location)
	return Date{Year: local.Year(), Month: int(local.Month()), Day: local.Day()}
}
