package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The workspace runs one target without RunMultiple. Its root is pinned too:
// a NAS unmounted mid-run leaves an empty mount point, which must not receive
// the remaining photos while a Move deletes their sources.
func TestRunPinsDestinationRootWithoutMultipleTargets(t *testing.T) {
	for _, replaced := range []bool{false, true} {
		t.Run(map[bool]string{false: "vanished", true: "replaced"}[replaced], func(t *testing.T) {
			base := t.TempDir()
			card, mountpoint := filepath.Join(base, "card"), filepath.Join(base, "nas")
			library := filepath.Join(mountpoint, "photos")
			writeFile(t, filepath.Join(card, "a.JPG"), "photo a")
			writeFile(t, filepath.Join(card, "b.JPG"), "photo b")
			if err := os.MkdirAll(library, 0o755); err != nil {
				t.Fatal(err)
			}
			plan := Plan{Operation: Move, Conflict: Skip, SourceRoot: card, DestinationRoot: library, StatePath: filepath.Join(library, ".dgs-state")}
			for _, name := range []string{"a.JPG", "b.JPG"} {
				plan.Jobs = append(plan.Jobs, Job{Source: filepath.Join(card, name), Destination: filepath.Join(library, name)})
			}
			// The first state write happens after pinning and before any
			// transfer: unmount there, deterministically.
			first := true
			plan.saveState = func(path string, state *State) error {
				if first {
					first = false
					if err := os.Rename(mountpoint, mountpoint+"-unmounted"); err != nil {
						t.Fatal(err)
					}
					if replaced {
						if err := os.Mkdir(mountpoint, 0o755); err != nil {
							t.Fatal(err)
						}
					}
					return nil
				}
				return writeState(path, state)
			}
			result := Run(context.Background(), plan, nil)
			for _, file := range result.Files {
				if file.Phase != PhaseFailed {
					t.Fatalf("lost root accepted: %#v", file)
				}
				if _, err := os.Stat(file.Source); err != nil {
					t.Fatal("source deleted after its destination was lost", err)
				}
			}
			if _, err := os.Stat(library); !os.IsNotExist(err) {
				t.Fatal("destination root recreated under the mount point", err)
			}
		})
	}
}

// A Destination that does not exist yet is created once, then pinned.
func TestRunCreatesMissingRootOnceAndPinsIt(t *testing.T) {
	base := t.TempDir()
	card, library := filepath.Join(base, "card"), filepath.Join(base, "new", "library")
	writeFile(t, filepath.Join(card, "a.JPG"), "photo a")
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: filepath.Join(card, "a.JPG"), Destination: filepath.Join(library, "a.JPG")}},
		Operation: Copy, Conflict: Skip, SourceRoot: card, DestinationRoot: library}, nil)
	if result.Files[0].Phase != PhaseComplete {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(filepath.Join(library, "a.JPG")); err != nil {
		t.Fatal(err)
	}
}

func TestRunRefusesDestinationRootThatIsAFile(t *testing.T) {
	base := t.TempDir()
	card, library := filepath.Join(base, "card"), filepath.Join(base, "library")
	writeFile(t, filepath.Join(card, "a.JPG"), "photo a")
	writeFile(t, library, "not a directory")
	result := Run(context.Background(), Plan{Jobs: []Job{{Source: filepath.Join(card, "a.JPG"), Destination: filepath.Join(library, "a.JPG")}},
		Operation: Move, Conflict: Skip, DestinationRoot: library}, nil)
	if result.Files[0].Phase != PhaseFailed {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(filepath.Join(card, "a.JPG")); err != nil {
		t.Fatal(err)
	}
}

// RunMultiple pins exactly the directories validation checked.
func TestValidateTargetsReturnsTheCheckedIdentities(t *testing.T) {
	plans := multiplePlans(t)
	roots := []string{plans[0].DestinationRoot, plans[1].DestinationRoot}
	identities, err := validateTargets(plans[0].SourceRoot, roots)
	if err != nil || len(identities) != 2 {
		t.Fatalf("identities = %v, %v", identities, err)
	}
	for i, root := range roots {
		info, err := os.Stat(root)
		if err != nil || !os.SameFile(info, identities[i]) {
			t.Fatalf("identity %d does not describe %s", i, root)
		}
	}
}
