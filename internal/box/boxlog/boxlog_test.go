package boxlog_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/box/boxlog"
)

func TestAppendKeepsEveryLineInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), boxlog.Name)
	for _, digest := range []string{"sha256:a", "sha256:b", "sha256:c"} {
		if err := boxlog.Append(path, boxlog.Import(digest, "2026/2026-09-22/scan.pdf")); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	entries, err := boxlog.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries", len(entries))
	}
	for i, want := range []string{"sha256:a", "sha256:b", "sha256:c"} {
		if entries[i].Digest != want {
			t.Errorf("entry %d: got %s want %s", i, entries[i].Digest, want)
		}
	}
}

// The log is what makes a three-hundred-record batch edit recoverable, so an
// edit has to say what changed, from what, and to what.
func TestAnEditRecordsBothSides(t *testing.T) {
	path := filepath.Join(t.TempDir(), boxlog.Name)
	entry := boxlog.Edit("sha256:a", "type", "receipt", "invoice")
	entry.At = time.Date(2026, 9, 22, 14, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if err := boxlog.Append(path, entry); err != nil {
		t.Fatal(err)
	}
	entries, err := boxlog.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	got := entries[0]
	if got.Field != "type" || got.From != "receipt" || got.To != "invoice" {
		t.Errorf("got %+v", got)
	}
	if got.At != "2026-09-22T14:00:00Z" {
		t.Errorf("time: got %q", got.At)
	}
}

// The log is appended to over years by every version of the tool there will
// ever be. One bad line from an interrupted write must not make the rest of
// the history unreadable.
func TestOneBadLineDoesNotHideTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), boxlog.Name)
	if err := boxlog.Append(path, boxlog.Import("sha256:a", "a.pdf")); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	file.WriteString("{\"at\":\"2026 truncated mid-wri\n")
	file.Close()
	if err := boxlog.Append(path, boxlog.Import("sha256:b", "b.pdf")); err != nil {
		t.Fatal(err)
	}
	entries, err := boxlog.Read(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want the two good ones", len(entries))
	}
}

func TestReadingAMissingLogIsNotAnError(t *testing.T) {
	entries, err := boxlog.Read(filepath.Join(t.TempDir(), boxlog.Name))
	if err != nil {
		t.Fatalf("got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries from a log that is not there", len(entries))
	}
}

// Not hidden: a Box whose history is invisible in Finder is a Box nobody knows
// has one.
func TestTheLogIsNotADotfile(t *testing.T) {
	if boxlog.Name[0] == '.' {
		t.Errorf("the log is named %q", boxlog.Name)
	}
}
