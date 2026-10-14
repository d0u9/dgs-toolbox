package importer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStateWriterPreservesUnrecognizedFiles(t *testing.T) {
	for _, contents := range []string{"synthetic photograph", "invalid", `{"version":99,"hash_algorithm":"SHA-256"}`} {
		t.Run(contents, func(t *testing.T) {
			plans := multiplePlans(t)
			for i := range plans {
				writeFile(t, plans[i].StatePath, contents)
			}
			result, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{})
			if err != nil {
				t.Fatal(err)
			}
			for i, target := range result.Targets {
				if target.StateError == nil {
					t.Fatal("unrecognized state overwritten silently")
				}
				if got, err := os.ReadFile(plans[i].StatePath); err != nil || string(got) != contents {
					t.Fatalf("original state changed: %q %v", got, err)
				}
			}
			for _, file := range result.Files {
				if file.Phase != PhaseComplete {
					t.Fatal("state failure blocked verified copies", file)
				}
			}
		})
	}
}

func TestMultipleCopyRejectsChangedDestinationVersions(t *testing.T) {
	for _, change := range []string{"rewrite", "replace", "symlink", "parent link"} {
		t.Run(change, func(t *testing.T) {
			plans := multiplePlans(t)
			for i := range plans {
				plans[i].Jobs = plans[i].Jobs[:1]
			}
			result, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{Target: func(i int, _ Result) {
				if i != len(plans)-1 {
					return
				}
				path := plans[0].Jobs[0].Destination
				switch change {
				case "rewrite":
					writeFile(t, path, "damaged photo")
				case "replace":
					if err := os.Rename(path, path+".old"); err != nil {
						t.Fatal(err)
					}
					writeFile(t, path, "photo a")
				case "symlink":
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(plans[1].Jobs[0].Destination, path); err != nil {
						t.Fatal(err)
					}
				case "parent link":
					dir := filepath.Dir(path)
					if err := os.Rename(dir, dir+"-old"); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(dir+"-old", dir); err != nil {
						t.Fatal(err)
					}
				}
			}})
			if err != nil || result.Files[0].Phase != PhaseFailed {
				t.Fatalf("changed destination accepted: %#v %v", result.Files, err)
			}
			if _, err := os.Stat(plans[0].Jobs[0].Source); err != nil {
				t.Fatal("source lost", err)
			}
		})
	}
}

func TestNonIncrementalSourceDirectoryLinksAreRefused(t *testing.T) {
	for _, resume := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "resume"}[resume], func(t *testing.T) {
			plans := multiplePlans(t)
			plan := plans[0]
			plan.Incremental, plan.Operation = false, Copy
			if resume {
				if result := Run(context.Background(), plan, nil); result.Files[0].Phase != PhaseComplete {
					t.Fatal(result.Files)
				}
			}
			dir := filepath.Join(plan.SourceRoot, "DCIM")
			outside := filepath.Join(t.TempDir(), "outside")
			writeFile(t, filepath.Join(outside, "a.JPG"), "photo a")
			writeFile(t, filepath.Join(outside, "b.RAW"), "photo b")
			if err := os.Rename(dir, dir+"-original"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, dir); err != nil {
				t.Fatal(err)
			}
			plan.Operation = Move
			result := Run(context.Background(), plan, nil)
			for _, file := range result.Files {
				if file.Phase != PhaseFailed {
					t.Fatal("source link accepted", file)
				}
			}
			for _, name := range []string{"a.JPG", "b.RAW"} {
				if _, err := os.Stat(filepath.Join(outside, name)); err != nil {
					t.Fatal("outside file deleted", err)
				}
			}
		})
	}
}

func TestMoveRechecksSourceParentsImmediatelyBeforeDeletion(t *testing.T) {
	plans := multiplePlans(t)
	plan := plans[0]
	path := plan.Jobs[0].Source
	read, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(path)
	if err := os.Rename(directory, directory+"-original"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(directory+"-original", directory); err != nil {
		t.Fatal(err)
	}
	if err := deletePinnedSource(plan, path, read); err == nil {
		t.Fatal("directory link authorized source deletion")
	}
	if _, err := os.Stat(filepath.Join(directory+"-original", filepath.Base(path))); err != nil {
		t.Fatal("source lost", err)
	}
}
