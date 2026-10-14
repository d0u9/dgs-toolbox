package organizer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordStateAndHistoryStaySeparate(t *testing.T) {
	dir := t.TempDir()
	if record, found, err := ReadRecord(dir); err != nil || found || record.Organized() {
		t.Fatalf("missing record: %+v %v %v", record, found, err)
	}
	if _, err := SetFlag(dir, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	failed := Run{Recipe: "daily", Actions: []RecordedAction{{Action: ActionDailyAppend, Error: "write failed"}}}
	if _, err := AppendRun(dir, failed); err != nil {
		t.Fatal(err)
	}
	record, found, err := ReadRecord(dir)
	if err != nil || !found || record.Organized() || !record.Flagged() || len(record.Runs) != 1 {
		t.Fatalf("failed run lost state: %+v %v %v", record, found, err)
	}
	complete := Run{Recipe: "daily", Actions: []RecordedAction{{Action: ActionDailyAppend, Executed: true}}}
	if _, err := AppendRun(dir, complete); err != nil {
		t.Fatal(err)
	}
	record, found, err = ReadRecord(dir)
	if err != nil || !found || !record.Organized() || !record.Flagged() || len(record.Runs) != 2 {
		t.Fatalf("completed run lost history: %+v %v %v", record, found, err)
	}
}

func TestInvalidRecordCannotBeChangedOrDeleted(t *testing.T) {
	for _, body := range []string{`{"schema":"v1","runs":[`, `{"schema":"v2","runs":[]}`, `null`} {
		for _, action := range []string{"flag", "unflag", "append"} {
			t.Run(action+"/"+body, func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, RecordFilename)
				original := []byte(body)
				if err := os.WriteFile(path, original, 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if action == "append" {
					_, err = AppendRun(dir, Run{Recipe: "daily"})
				} else {
					_, err = SetFlag(dir, action == "flag", time.Now())
				}
				if err == nil {
					t.Fatal("invalid record accepted")
				}
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, original) {
					t.Fatalf("record changed: %q %v", after, err)
				}
			})
		}
	}
}
