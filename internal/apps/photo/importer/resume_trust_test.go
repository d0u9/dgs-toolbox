package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

func digest(t *testing.T, path string) string {
	t.Helper()
	checked, err := verifiedcopy.Check(context.Background(), "", path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return checked.Digest
}

// resumeFixture writes one card photo and a state file holding record, then
// runs a non-incremental Move that would resume from it.
func resumeFixture(t *testing.T, record func(source, library string) FileResult) (string, string, FileResult) {
	t.Helper()
	base := t.TempDir()
	card, library := filepath.Join(base, "card"), filepath.Join(base, "library")
	source := filepath.Join(card, "a.JPG")
	writeFile(t, source, "only copy")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(library, ".dgs-state")
	state := State{Version: StateVersion, HashAlgorithm: "SHA-256", Files: []FileResult{record(source, library)}}
	if err := writeState(statePath, &state); err != nil {
		t.Fatal(err)
	}
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: source, Destination: filepath.Join(library, "a.JPG")}},
		Operation: Move, Conflict: Skip, SourceRoot: card, DestinationRoot: library, StatePath: statePath}, nil)
	return source, library, result.Files[0]
}

// A damaged, edited or foreign state record must never authorise deleting the
// source: each of these once made Move delete the only copy.
func TestResumeDoesNotTrustRecordsThatProveNothing(t *testing.T) {
	cases := map[string]func(t *testing.T) func(source, library string) FileResult{
		"destination is the source": func(t *testing.T) func(string, string) FileResult {
			return func(source, library string) FileResult {
				sum := digest(t, source)
				return FileResult{Source: source, Destination: source, PlannedDestination: filepath.Join(library, "a.JPG"), Size: 9, SourceHash: sum, DestinationHash: sum, Phase: PhaseComplete}
			}
		},
		"destination outside the root": func(t *testing.T) func(string, string) FileResult {
			return func(source, library string) FileResult {
				outside := filepath.Join(filepath.Dir(library), "elsewhere", "a.JPG")
				writeFile(t, outside, "only copy")
				sum := digest(t, source)
				return FileResult{Source: source, Destination: outside, PlannedDestination: filepath.Join(library, "a.JPG"), Size: 9, SourceHash: sum, DestinationHash: sum, Phase: PhaseComplete}
			}
		},
		"destination through a link inside the root": func(t *testing.T) func(string, string) FileResult {
			return func(source, library string) FileResult {
				if err := os.Symlink(filepath.Dir(source), filepath.Join(library, "card-link")); err != nil {
					t.Fatal(err)
				}
				sum := digest(t, source)
				return FileResult{Source: source, Destination: filepath.Join(library, "card-link", "a.JPG"), PlannedDestination: filepath.Join(library, "a.JPG"), Size: 9, SourceHash: sum, DestinationHash: sum, Phase: PhaseComplete}
			}
		},
		"destination hard-linked to the source": func(t *testing.T) func(string, string) FileResult {
			return func(source, library string) FileResult {
				linked := filepath.Join(library, "a.JPG")
				if err := os.Link(source, linked); err != nil {
					t.Fatal(err)
				}
				sum := digest(t, source)
				return FileResult{Source: source, Destination: linked, Size: 9, SourceHash: sum, DestinationHash: sum, Phase: PhaseComplete}
			}
		},
		"digests that disagree": func(t *testing.T) func(string, string) FileResult {
			return func(source, library string) FileResult {
				other := filepath.Join(library, "a.JPG")
				writeFile(t, other, "different")
				return FileResult{Source: source, Destination: other, Size: 9, SourceHash: digest(t, source), DestinationHash: digest(t, other), Phase: PhaseComplete}
			}
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			source, library, result := resumeFixture(t, build(t))
			if result.Existing {
				t.Fatalf("untrustworthy record resumed: %#v", result)
			}
			// The invariant: a Move deletes its source only when a verified
			// copy of it now lies under the destination root. Here the
			// rejected record makes the photo import anew, or keep its source.
			if _, err := os.Stat(source); err == nil {
				return
			}
			copy := filepath.Join(library, "a.JPG")
			data, err := os.ReadFile(copy)
			if err != nil || string(data) != "only copy" || result.Destination != copy || result.Phase != PhaseComplete {
				t.Fatalf("source deleted without a copy under the root: %#v, %q, %v", result, data, err)
			}
		})
	}
}

// A sound record still resumes: the fix must not cost resume itself.
func TestResumeStillTrustsASoundRecord(t *testing.T) {
	source, library, result := resumeFixture(t, func(source, library string) FileResult {
		copy := filepath.Join(library, "a.JPG")
		writeFile(t, copy, "only copy")
		sum := digest(t, source)
		return FileResult{Source: source, Destination: copy, Size: 9, SourceHash: sum, DestinationHash: sum, Phase: PhaseComplete}
	})
	if result.Phase != PhaseComplete || !result.Existing {
		t.Fatalf("sound record not resumed: %#v", result)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("resumed Move kept its source", err)
	}
	if data, err := os.ReadFile(filepath.Join(library, "a.JPG")); err != nil || string(data) != "only copy" {
		t.Fatal(err)
	}
}

// Every Move deletion asks whether the pinned root is still there first.
func TestSourceIsKeptWhenPinnedRootIsGoneBeforeDeletion(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		base := t.TempDir()
		root := filepath.Join(base, "library")
		source := filepath.Join(base, "card", "a.JPG")
		writeFile(t, source, "photo")
		writeFile(t, filepath.Join(root, "a.JPG"), "photo")
		identity, err := os.Stat(root)
		if err != nil {
			t.Fatal(err)
		}
		plan := Plan{DestinationRoot: root, expectedRoot: identity}
		read, err := os.Lstat(source)
		if err != nil {
			t.Fatal(err)
		}
		checked, err := verifiedcopy.Check(context.Background(), "", source, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(root, root+"-unmounted"); err != nil {
			t.Fatal(err)
		}
		if replaced {
			writeFile(t, filepath.Join(root, "a.JPG"), "photo")
		}
		if err := deletePinnedSource(plan, source, read); err == nil {
			t.Fatalf("replaced=%v: deletion allowed without the pinned root", replaced)
		}
		if err := deleteMatchedSource(context.Background(), source, filepath.Join(root, "a.JPG"), plan, checked); err == nil {
			t.Fatalf("replaced=%v: matched deletion allowed without the pinned root", replaced)
		}
		if _, err := os.Stat(source); err != nil {
			t.Fatal("source deleted", err)
		}
	}
}
