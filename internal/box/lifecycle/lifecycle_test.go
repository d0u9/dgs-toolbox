package lifecycle

import (
	"testing"

	"dgs-toolbox/internal/box"
)

func date(text string) box.Date {
	parsed, err := box.ParseDate(text)
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestExpiryComesFromTheTypeWhenNobodyEnteredOne(t *testing.T) {
	for _, test := range []struct {
		name     string
		scan     Scan
		want     string
		wantSome bool
	}{
		{"a ticket expires on the day", Scan{Type: "ticket", EventDate: date("2019-03-11")}, "2019-03-11", true},
		{"travel keeps for ninety days", Scan{Type: "travel", EventDate: date("2019-03-11")}, "2019-06-09", true},
		{"a policy keeps for a year", Scan{Type: "insurance", EventDate: date("2019-03-11")}, "2020-03-10", true},
		{"a receipt keeps for seven years", Scan{Type: "receipt", EventDate: date("2019-03-11")}, "2026-03-09", true},
		{"a contract has no expiry", Scan{Type: "contract", EventDate: date("2019-03-11")}, "", false},
		{"a letter has no expiry", Scan{Type: "letter", EventDate: date("2019-03-11")}, "", false},
		{"an undated ticket has nothing to compute", Scan{Type: "ticket"}, "", false},
	} {
		got, has := Expiry(test.scan)
		if has != test.wantSome || got.String() != test.want {
			t.Errorf("%s: Expiry = %q, %v; want %q, %v", test.name, got, has, test.want, test.wantSome)
		}
	}
}

func TestAnEnteredExpiryWinsAndAClearedOneMeansPermanent(t *testing.T) {
	entered := Scan{Type: "ticket", EventDate: date("2019-03-11"), ExpiresAt: date("2030-01-01")}
	if got, has := Expiry(entered); !has || got.String() != "2030-01-01" {
		t.Errorf("an entered expiry should win: got %q, %v", got, has)
	}
	// The concert ticket worth keeping: one key clears the expiry, and the
	// type's same-day default must not creep back.
	kept := Scan{Type: "ticket", EventDate: date("2019-03-11"), ExpiryCleared: true}
	if got, has := Expiry(kept); has {
		t.Errorf("a cleared expiry should mean none: got %q", got)
	}
	if state := StateOn(kept, date("2026-09-22")); state != Permanent {
		t.Errorf("a kept ticket is %s, want permanent", state)
	}
}

func TestStateOn(t *testing.T) {
	today := date("2026-09-22")
	for _, test := range []struct {
		name string
		scan Scan
		want State
	}{
		{"expired years ago", Scan{Type: "ticket", EventDate: date("2019-03-11")}, Dead},
		{"a policy from a year ago", Scan{Type: "insurance", EventDate: date("2025-01-01"), ExpiresAt: date("2026-01-01")}, Dead},
		{"a policy still running", Scan{Type: "insurance", EventDate: date("2026-06-01")}, Current},
		{"a receipt still within seven years", Scan{Type: "receipt", EventDate: date("2024-01-01")}, Current},
		{"a contract", Scan{Type: "contract", EventDate: date("2019-03-11")}, Permanent},
		{"a leaflet", Scan{Type: "ephemera"}, Permanent},
		{"not sorted yet", Scan{Type: "unsorted"}, Permanent},
		{"a passport with no date entered", Scan{Type: "identity"}, Undated},
		{"an undated ticket", Scan{Type: "ticket"}, Undated},
		{"a type this build does not know", Scan{Type: "tax-notice", EventDate: date("2019-03-11")}, Permanent},
	} {
		if got := StateOn(test.scan, today); got != test.want {
			t.Errorf("%s: StateOn = %s, want %s", test.name, got, test.want)
		}
	}
}

func TestATicketForTonightHasNotExpiredThisMorning(t *testing.T) {
	tonight := Scan{Type: "ticket", EventDate: date("2026-09-22")}
	if got := StateOn(tonight, date("2026-09-22")); got != Current {
		t.Errorf("a ticket for today is %s, want current", got)
	}
	if got := StateOn(tonight, date("2026-09-23")); got != Dead {
		t.Errorf("a ticket for yesterday is %s, want dead", got)
	}
}

func TestNeedsExpiryListsWhatIsPrintedOnThePaper(t *testing.T) {
	for _, test := range []struct {
		scan Scan
		want bool
	}{
		{Scan{Type: "identity"}, true},
		{Scan{Type: "insurance", EventDate: date("2026-06-01")}, true},
		{Scan{Type: "identity", ExpiresAt: date("2031-01-01")}, false},
		{Scan{Type: "identity", ExpiryCleared: true}, false},
		{Scan{Type: "receipt"}, false},
		{Scan{Type: "letter"}, false},
	} {
		if got := NeedsExpiry(test.scan); got != test.want {
			t.Errorf("NeedsExpiry(%+v) = %v, want %v", test.scan, got, test.want)
		}
	}
}

func TestAnUnknownTypeIsNeverDeclaredDead(t *testing.T) {
	// Reading accepts a type this build does not register. Calling its scans
	// dead would be a build deciding something it cannot know.
	scan := Scan{Type: "warranty", EventDate: date("1999-01-01")}
	if got := StateOn(scan, date("2026-09-22")); got == Dead {
		t.Error("an unknown type was declared dead")
	}
}
