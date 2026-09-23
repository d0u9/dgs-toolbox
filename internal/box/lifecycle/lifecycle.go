// Package lifecycle answers whether a scan is still worth keeping. It is the
// one place the rule lives, so the intake page, the browser, a filter and a
// report cannot disagree about what "expired" means.
//
// Nothing here is stored. A state is computed from the type, the event date and
// an expiry entered by hand, which is why no field can go stale and why nothing
// has to run on a schedule to keep a Box honest.
package lifecycle

import (
	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/doctype"
)

// State is what a scan is now.
type State int

const (
	// Permanent is a scan with no expiry to reach: a contract, a letter, a
	// ticket stub someone chose to keep.
	Permanent State = iota
	// Current is a scan with an expiry still ahead of it.
	Current
	// Dead is a scan whose expiry has passed. Nothing is deleted for it; it is
	// what the filter that fills the trash selects on.
	Dead
	// Undated is a scan of a type that expires, with no date to compute from. It
	// is not dead — nothing is known — and it is the list of scans still worth
	// dating.
	Undated
)

func (s State) String() string {
	switch s {
	case Current:
		return "current"
	case Dead:
		return "dead"
	case Undated:
		return "undated"
	default:
		return "permanent"
	}
}

// Scan is what the rule needs to know about one file: its type name as the
// sidecar spells it, the date on the document, and an expiry someone entered.
type Scan struct {
	Type      string
	EventDate box.Date
	// ExpiresAt is an expiry entered by hand. It always wins over the type's
	// default, and clearing it is how a scan is made permanent — which is how a
	// concert ticket worth keeping escapes the same-day default of its type.
	ExpiresAt box.Date
	// ExpiryCleared records that the expiry was emptied deliberately rather than
	// never filled in. Without it, "permanent because I said so" and "not yet
	// decided" would look identical, and the type default would creep back.
	ExpiryCleared bool
}

// Expiry is the day a scan stops being useful, and whether it has one at all.
//
// The order is: an expiry entered by hand; then an expiry cleared by hand,
// which means none; then the type's default lifetime counted from the event
// date. A type with no default lifetime has no expiry, and an unknown type is
// treated the same way — a build that does not know a type does not get to
// declare its scans dead.
func Expiry(scan Scan) (box.Date, bool) {
	if !scan.ExpiresAt.Zero() {
		return scan.ExpiresAt, true
	}
	if scan.ExpiryCleared {
		return box.Date{}, false
	}
	entry, known := doctype.Lookup(scan.Type)
	if !known || entry.Permanent() {
		return box.Date{}, false
	}
	if scan.EventDate.Zero() {
		return box.Date{}, false
	}
	return scan.EventDate.AddDays(*entry.Lifetime), true
}

// StateOn is what a scan is on a given day. The day is passed in rather than
// read from the clock so that a filter, a test and a report all see one
// "today", and so that today is the caller's zone's today.
//
// A scan whose expiry is today is still current: a ticket for tonight has not
// expired this morning.
func StateOn(scan Scan, today box.Date) State {
	expiry, has := Expiry(scan)
	if !has {
		if wantsADate(scan) {
			return Undated
		}
		return Permanent
	}
	if expiry.Before(today) {
		return Dead
	}
	return Current
}

// wantsADate reports whether the only thing between this scan and an expiry is
// a date nobody has entered.
func wantsADate(scan Scan) bool {
	if scan.ExpiryCleared || !scan.ExpiresAt.Zero() {
		return false
	}
	entry, known := doctype.Lookup(scan.Type)
	if !known {
		return false
	}
	if entry.ExpiryExpected {
		return true
	}
	return !entry.Permanent() && scan.EventDate.Zero()
}

// NeedsExpiry reports whether a scan is of a type whose expiry is printed on
// the document and has not been entered. It is what produces the list worth
// filling in by hand: a passport or a policy whose real date is on the paper.
func NeedsExpiry(scan Scan) bool {
	entry, known := doctype.Lookup(scan.Type)
	if !known || !entry.ExpiryExpected {
		return false
	}
	return scan.ExpiresAt.Zero() && !scan.ExpiryCleared
}
