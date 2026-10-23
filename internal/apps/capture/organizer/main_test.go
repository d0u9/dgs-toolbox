package organizer

import (
	"os"
	"testing"
)

// testYear is the year the tests' running notes are for. The test Captures are
// from 2026, and a test about this year and the last has to mean the same
// thing whenever it runs, so the clock's year is named rather than read.
const testYear = "2026"

func TestMain(m *testing.M) {
	currentYear = func() string { return testYear }
	os.Exit(m.Run())
}
