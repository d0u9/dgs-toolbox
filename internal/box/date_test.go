package box

import (
	"testing"
	"time"
)

func TestParseDateAcceptsOnlyACalendarDate(t *testing.T) {
	got, err := ParseDate(" 2019-03-11 ")
	if err != nil {
		t.Fatalf("ParseDate: %v", err)
	}
	if want := (Date{Year: 2019, Month: 3, Day: 11}); got != want {
		t.Errorf("ParseDate = %+v, want %+v", got, want)
	}
	for _, text := range []string{
		"",
		"2019-3-11",
		"11/03/2019",
		"2019-13-01",
		"2019-02-30",
		"2019-03-11T20:30:00+09:00", // an instant is a different field
		"2019-03-11+09:00",
	} {
		if got, err := ParseDate(text); err == nil {
			t.Errorf("ParseDate(%q) = %+v, want an error", text, got)
		}
	}
}

func TestZeroDateStaysMissing(t *testing.T) {
	var none Date
	if !none.Zero() {
		t.Error("the zero Date should report itself as no date")
	}
	if none.String() != "" {
		t.Errorf("the zero Date prints %q, want empty — a missing date must not become year zero", none.String())
	}
	if got := none.AddDays(90); !got.Zero() {
		t.Errorf("arithmetic on no date produced %q", got)
	}
	some := Date{Year: 2019, Month: 3, Day: 11}
	if none.Before(some) || some.Before(none) {
		t.Error("the zero Date should not compare as a point on the calendar")
	}
}

func TestAddDaysCrossesMonthsYearsAndLeapDays(t *testing.T) {
	for _, test := range []struct {
		from string
		days int
		want string
	}{
		{"2019-03-11", 0, "2019-03-11"},
		{"2019-03-11", 90, "2019-06-09"},
		{"2019-12-31", 1, "2020-01-01"},
		{"2020-02-28", 1, "2020-02-29"},
		{"2019-02-28", 1, "2019-03-01"},
		{"2019-03-11", 365, "2020-03-10"}, // 2020 has a leap day in between
		{"2019-03-11", -1, "2019-03-10"},
	} {
		from, err := ParseDate(test.from)
		if err != nil {
			t.Fatalf("ParseDate(%q): %v", test.from, err)
		}
		if got := from.AddDays(test.days).String(); got != test.want {
			t.Errorf("%s + %d days = %s, want %s", test.from, test.days, got, test.want)
		}
	}
}

func TestBeforeAndAfter(t *testing.T) {
	for _, test := range []struct {
		left, right string
		before      bool
	}{
		{"2019-03-11", "2019-03-12", true},
		{"2019-03-11", "2019-04-01", true},
		{"2019-03-11", "2020-01-01", true},
		{"2019-03-11", "2019-03-11", false},
		{"2019-03-12", "2019-03-11", false},
	} {
		left, right := mustDate(t, test.left), mustDate(t, test.right)
		if got := left.Before(right); got != test.before {
			t.Errorf("%s.Before(%s) = %v, want %v", test.left, test.right, got, test.before)
		}
		if got := right.After(left); got != test.before {
			t.Errorf("%s.After(%s) = %v, want %v", test.right, test.left, got, test.before)
		}
	}
}

func TestDateOfReadsAnInstantInTheZoneItIsAsked(t *testing.T) {
	tokyo, err := LoadZone("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	sydney, err := LoadZone("Australia/Sydney")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	// Late on the 11th in Tokyo is already the 12th in Sydney: a scan's day
	// depends on which place is asked, which is why the zone is stored.
	instant := time.Date(2019, 3, 11, 23, 30, 0, 0, tokyo)
	if got := DateOf(instant, tokyo).String(); got != "2019-03-11" {
		t.Errorf("in Tokyo the instant falls on %s, want 2019-03-11", got)
	}
	if got := DateOf(instant, sydney).String(); got != "2019-03-12" {
		t.Errorf("in Sydney the instant falls on %s, want 2019-03-12", got)
	}
}

func TestTodayUsesTheZoneItIsGiven(t *testing.T) {
	tokyo, err := LoadZone("Asia/Tokyo")
	if err != nil {
		t.Fatalf("LoadZone: %v", err)
	}
	if got := Today(tokyo); got.Zero() {
		t.Error("Today returned no date")
	}
	if got := Today(nil); got.Zero() {
		t.Error("Today(nil) should fall back to the machine's zone")
	}
}

func mustDate(t *testing.T, text string) Date {
	t.Helper()
	parsed, err := ParseDate(text)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", text, err)
	}
	return parsed
}
