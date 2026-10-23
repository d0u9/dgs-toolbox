package organizer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiscardOnACaptureNobodyOrganized(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 14, 21, 0, 0, 0, time.FixedZone("", 10*3600))
	record, err := SetDiscard(dir, true, at)
	if err != nil || !record.Discarded() || record.Discard.DiscardedAt != "2026-09-14T21:00:00+10:00" {
		t.Fatalf("record = %+v, err = %v", record, err)
	}
	read, found, err := ReadRecord(dir)
	if err != nil || !found || read.Organized() || !read.Discarded() {
		t.Errorf("read = %+v, found = %v: a discard mark is not a run", read, found)
	}
	if _, err := SetDiscard(dir, false, at); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, RecordFilename)); !os.IsNotExist(err) {
		t.Errorf("a record holding only the mark should go with it: %v", err)
	}
}

func TestDiscardKeepsTheRuns(t *testing.T) {
	dir := t.TempDir()
	if _, err := AppendRun(dir, Run{Recipe: "daily", Actions: []RecordedAction{{Action: ActionDailyAppend, Executed: true}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := SetDiscard(dir, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := SetDiscard(dir, false, time.Now()); err != nil {
		t.Fatal(err)
	}
	read, found, err := ReadRecord(dir)
	if err != nil || !found || !read.Organized() || read.Discarded() || len(read.Runs) != 1 {
		t.Errorf("read = %+v, found = %v", read, found)
	}
}

func TestUsesPosition(t *testing.T) {
	recipe := Recipe{Actions: []ActionID{ActionDailyAppend, TimelineAction("locations")}}
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
