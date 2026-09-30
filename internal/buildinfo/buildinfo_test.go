package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestFillTakesTheStampedCommit(t *testing.T) {
	Commit, Date = "unknown", "unknown"
	fill([]debug.BuildSetting{
		{Key: "vcs.revision", Value: "0123456789abcdef0123"},
		{Key: "vcs.time", Value: "2026-10-01T00:00:00Z"},
		{Key: "vcs.modified", Value: "true"},
	})
	if got, want := String(), "dev (commit 0123456789ab-dirty, 2026-10-01T00:00:00Z)"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestFillKeepsLdflags(t *testing.T) {
	Commit, Date = "abc", "then"
	fill([]debug.BuildSetting{{Key: "vcs.revision", Value: "def"}})
	if Commit != "abc" || Date != "then" {
		t.Fatalf("ldflags overwritten: %s %s", Commit, Date)
	}
}
