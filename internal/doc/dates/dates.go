// Package dates finds calendar dates in a document's text, in the forms scans
// carry, and writes each as YYYY-MM-DD.
package dates

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Order is how a date with the day and month both in digits and the year
// last is read: 03/04/2026 is 3 April under DMY and March 4 under MDY.
type Order string

const (
	DMY Order = "DMY"
	MDY Order = "MDY"
)

// DefaultOrder is the order used when none is configured.
const DefaultOrder = DMY

// ParseOrder reads an order, case-insensitively. Empty is DefaultOrder.
func ParseOrder(s string) (Order, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "":
		return DefaultOrder, nil
	case "DMY":
		return DMY, nil
	case "MDY":
		return MDY, nil
	}
	return "", fmt.Errorf("date order %q: use DMY or MDY", s)
}

// Found is one date and where it is: Start and End are byte offsets into the
// text around the date as written.
type Found struct {
	Value string `json:"value"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

var (
	// Year first: 2026-09-25, 2026.09.25, 2026/9/25, 2026年9月25日.
	yearFirst = regexp.MustCompile(`(?:^|[^\d])((\d{4})\s*(?:[-./]|年)\s*(\d{1,2})\s*(?:[-./]|月)\s*(\d{1,2})\s*日?)(?:[^\d]|$)`)
	// Year last: 25/09/2026, 25.09.2026, 25-09-2026.
	yearLast = regexp.MustCompile(`(?:^|[^\d])((\d{1,2})[-./](\d{1,2})[-./](\d{4}))(?:[^\d]|$)`)
)

// Find returns every real calendar date in text, in text order. An impossible
// date — 2026-02-30, or a month past 12 — is not a date and is left out. A
// year-last date whose first number is over 12 is read day first whatever the
// order, and one whose second is over 12 month first.
func Find(text string, order Order) []Found {
	var out []Found
	for _, m := range matches(yearFirst, text) {
		if v, ok := date(m[2].text, m[3].text, m[4].text); ok {
			out = append(out, Found{Value: v, Start: m[1].start, End: m[1].end})
		}
	}
	for _, m := range matches(yearLast, text) {
		day, month := m[2].text, m[3].text
		first, _ := strconv.Atoi(day)
		second, _ := strconv.Atoi(month)
		if second > 12 || (first <= 12 && order == MDY) {
			day, month = month, day
		}
		if v, ok := date(m[4].text, month, day); ok {
			out = append(out, Found{Value: v, Start: m[1].start, End: m[1].end})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Parse reads one value as a date in any form Find knows, and reports whether
// it was one. Surrounding space is ignored; anything else around it is not.
func Parse(s string, order Order) (string, bool) {
	s = strings.TrimSpace(s)
	found := Find(s, order)
	if len(found) == 1 && found[0].Start == 0 && found[0].End == len(s) {
		return found[0].Value, true
	}
	return "", false
}

// Valid reports whether s is a real date written YYYY-MM-DD.
func Valid(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

type span struct {
	text       string
	start, end int
}

// matches returns each match's groups by number, group 1 being the date as
// written. The pattern consumes the non-digit on each side, so the next search
// starts at the end of the date, not past that character.
func matches(pattern *regexp.Regexp, text string) [][]span {
	var out [][]span
	for at := 0; at < len(text); {
		loc := pattern.FindStringSubmatchIndex(text[at:])
		if loc == nil {
			break
		}
		groups := make([]span, len(loc)/2)
		for g := range groups {
			groups[g] = span{text: text[at+loc[2*g] : at+loc[2*g+1]], start: at + loc[2*g], end: at + loc[2*g+1]}
		}
		out = append(out, groups)
		at = groups[1].end
	}
	return out
}

func date(year, month, day string) (string, bool) {
	y, _ := strconv.Atoi(year)
	m, _ := strconv.Atoi(month)
	d, _ := strconv.Atoi(day)
	s := fmt.Sprintf("%04d-%02d-%02d", y, m, d)
	return s, Valid(s)
}
