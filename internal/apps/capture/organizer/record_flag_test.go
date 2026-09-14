package organizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlagOnACaptureNobodyOrganized(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 14, 21, 0, 0, 0, time.FixedZone("", 10*3600))
	record, err := SetFlag(dir, true, at)
	if err != nil || !record.Flagged() || record.Flag.FlaggedAt != "2026-09-14T21:00:00+10:00" {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
	read, organized := ReadRecord(dir)
	if organized || !read.Flagged() {
		t.Errorf("read = %+v, organized = %v: a flag is not a run", read, organized)
	}
	if _, err := SetFlag(dir, false, at); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, RecordFilename)); !os.IsNotExist(err) {
		t.Errorf("a record holding only the flag should go with it: %v", err)
	}
}

func TestFlagKeepsTheRuns(t *testing.T) {
	dir := t.TempDir()
	if _, err := AppendRun(dir, Run{Recipe: "daily", Actions: []RecordedAction{{Action: ActionDailyAppend, Executed: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetFlag(dir, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := SetFlag(dir, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	read, organized := ReadRecord(dir)
	if !organized || read.Flagged() || len(read.Runs) != 1 {
		t.Errorf("read = %+v, organized = %v", read, organized)
	}
}

func TestUsesPosition(t *testing.T) {
	recipe := Recipe{Actions: []ActionID{ActionDailyAppend, ActionLocationAppend}}
	if !UsesPosition(recipe, recipe.Actions) {
		t.Error("the location note writes the position")
	}
	if UsesPosition(recipe, []ActionID{ActionDailyAppend}) {
		t.Error("a disabled Action does not count")
	}
	if !UsesPosition(Recipe{Actions: []ActionID{ActionReminderAtPlace}}, []ActionID{ActionReminderAtPlace}) {
		t.Error("a reminder at a place uses the position")
	}
}
