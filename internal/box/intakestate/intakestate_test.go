package intakestate_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/intakestate"
	"dgs-toolbox/internal/box/sidecar"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), intakestate.Name)
	draft := sidecar.File{
		Type:        "travel",
		Description: "Haneda to Sydney",
		EventDate:   box.Date{Year: 2019, Month: 3, Day: 11},
		EventZone:   "Asia/Tokyo",
	}
	state := intakestate.File{}
	state.Set(intakestate.Record{Path: "Scan_0012.pdf", Size: 12, ModTime: 34, Digest: "sha256:a", State: intakestate.Classified, Draft: &draft}, time.Now())
	state.Set(intakestate.Record{Path: "Scan_0013.pdf", State: intakestate.Published, PublishedPath: "2026/2026-09-22/Scan_0013-a1b2c3d4.pdf"}, time.Now())
	if err := intakestate.Save(path, state); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded := intakestate.Load(path)
	if len(loaded.Records) != 2 {
		t.Fatalf("got %d records", len(loaded.Records))
	}
	byPath := loaded.Index()
	first := byPath["Scan_0012.pdf"]
	if first.State != intakestate.Classified || first.Draft == nil {
		t.Fatalf("got %+v", first)
	}
	// What was typed has to survive, or coming back means typing it again.
	if first.Draft.Description != "Haneda to Sydney" || first.Draft.EventZone != "Asia/Tokyo" {
		t.Errorf("the draft was lost: %+v", first.Draft)
	}
	if first.Draft.EventDate != draft.EventDate {
		t.Errorf("the event date was lost: %+v", first.Draft.EventDate)
	}
	if first.At == "" {
		t.Error("no timestamp")
	}
}

// Missing, unparseable, truncated, another version — all the same answer, and
// no error. The inbox is still there to be read again.
func TestUnusableStatesStartAgain(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, intakestate.Name)
	for name, content := range map[string]string{
		"missing":     "",
		"not json":    "this is not a state file",
		"truncated":   `{"version":1,"records":[{"path":"a`,
		"old version": `{"version":0,"records":[{"path":"a.pdf","state":"published"}]}`,
		"new version": `{"version":99,"records":[{"path":"a.pdf","state":"published"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			os.Remove(path)
			if content != "" {
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			state := intakestate.Load(path)
			if len(state.Records) != 0 {
				t.Errorf("got %d records from an unusable state", len(state.Records))
			}
		})
	}
}

func TestSettledStates(t *testing.T) {
	settled := []intakestate.State{
		intakestate.Published, intakestate.Duplicate, intakestate.Incomplete, intakestate.Rejected,
	}
	for _, state := range settled {
		if !state.Settled() {
			t.Errorf("%s still needs a person", state)
		}
	}
	for _, state := range []intakestate.State{intakestate.Pending, intakestate.Classified} {
		if state.Settled() {
			t.Errorf("%s was treated as decided", state)
		}
	}
}

func TestRemainingCountsWhatIsLeft(t *testing.T) {
	state := intakestate.File{}
	now := time.Now()
	state.Set(intakestate.Record{Path: "a.pdf", State: intakestate.Published}, now)
	state.Set(intakestate.Record{Path: "b.pdf", State: intakestate.Pending}, now)
	state.Set(intakestate.Record{Path: "c.pdf", State: intakestate.Classified}, now)
	state.Set(intakestate.Record{Path: "d.pdf", State: intakestate.Incomplete}, now)
	if got := state.Remaining(); got != 2 {
		t.Errorf("remaining: got %d want 2", got)
	}
	counts := state.Counts()
	if counts[intakestate.Published] != 1 || counts[intakestate.Pending] != 1 {
		t.Errorf("counts: %+v", counts)
	}
}

// A file whose triple moved is not the file this record is about any more.
func TestStaleOnTheTriple(t *testing.T) {
	record := intakestate.Record{Path: "a.pdf", Size: 100, ModTime: 200}
	if record.Stale(100, 200) {
		t.Error("an unchanged file was called stale")
	}
	if !record.Stale(101, 200) {
		t.Error("a resized file was not called stale")
	}
	if !record.Stale(100, 201) {
		t.Error("a retouched file was not called stale")
	}
}

// A record whose file is gone describes nothing. What was published stays in
// the Box regardless, because the Box is the truth and this file never was.
func TestPruneDropsVanishedFiles(t *testing.T) {
	state := intakestate.File{}
	now := time.Now()
	state.Set(intakestate.Record{Path: "a.pdf", State: intakestate.Published}, now)
	state.Set(intakestate.Record{Path: "b.pdf", State: intakestate.Pending}, now)
	dropped := state.Prune(map[string]bool{"b.pdf": true})
	if dropped != 1 {
		t.Errorf("dropped %d, want 1", dropped)
	}
	if len(state.Records) != 1 || state.Records[0].Path != "b.pdf" {
		t.Errorf("kept %+v", state.Records)
	}
}

func TestSetReplacesRatherThanDuplicates(t *testing.T) {
	state := intakestate.File{}
	now := time.Now()
	state.Set(intakestate.Record{Path: "a.pdf", State: intakestate.Pending}, now)
	state.Set(intakestate.Record{Path: "a.pdf", State: intakestate.Published}, now)
	if len(state.Records) != 1 {
		t.Fatalf("got %d records for one file", len(state.Records))
	}
	if state.Records[0].State != intakestate.Published {
		t.Errorf("state: got %s", state.Records[0].State)
	}
}

// A configured path could point the state at something that is not this inbox.
func TestStateNameMustBeABareFilename(t *testing.T) {
	for _, name := range []string{"sub/state.json", "../state.json", "/etc/state.json"} {
		if _, err := intakestate.PathFor(t.TempDir(), name); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	path, err := intakestate.PathFor("/inbox", "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != intakestate.Name {
		t.Errorf("default: got %q", path)
	}
}

// It lives in the inbox, not in the Box: it is a fact about this pile of files
// and not about the archive.
func TestTheStateLivesInTheInbox(t *testing.T) {
	inbox := t.TempDir()
	path, err := intakestate.PathFor(inbox, "")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != inbox {
		t.Errorf("got %q, want it inside %q", path, inbox)
	}
	// Not hidden: a person looking at their own temporary folder should see
	// what the tool put there.
	if intakestate.Name[0] == '.' {
		t.Errorf("the state file is hidden: %q", intakestate.Name)
	}
}
