package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

// rsync --link-dest backups hard-link unchanged files. A hard link in one
// target to the copy in another is one copy; Move must keep the source.
func TestHardLinkedCopiesAcrossTargetsCountOnce(t *testing.T) {
	for _, operation := range []Operation{Copy, Move} {
		t.Run(string(operation), func(t *testing.T) {
			plans := multiplePlans(t)
			plans[0].Jobs, plans[1].Jobs = plans[0].Jobs[:1], plans[1].Jobs[:1]
			a := filepath.Join(plans[0].DestinationRoot, "2026", "a.JPG")
			writeFile(t, a, "photo a")
			b := filepath.Join(plans[1].DestinationRoot, "2026", "a.JPG")
			if err := os.MkdirAll(filepath.Dir(b), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(a, b); err != nil {
				t.Fatal(err)
			}
			result, err := RunMultiple(context.Background(), plans, operation, MultipleHooks{})
			if err != nil || result.Files[0].Phase != PhaseFailed || !strings.Contains(result.Files[0].Error, "same file") {
				t.Fatalf("result = %#v, %v", result.Files, err)
			}
			if _, err := os.Stat(plans[0].Jobs[0].Source); err != nil {
				t.Fatal("one physical copy authorized deleting the source", err)
			}
		})
	}
}

func TestMissingTargetIsRefusedNotCreated(t *testing.T) {
	plans := multiplePlans(t)
	mountpoint := filepath.Join(filepath.Dir(plans[1].DestinationRoot), "nas")
	if err := os.Mkdir(mountpoint, 0o755); err != nil {
		t.Fatal(err)
	}
	plans[1].DestinationRoot = filepath.Join(mountpoint, "photos")
	for i := range plans[1].Jobs {
		plans[1].Jobs[i].Destination = filepath.Join(plans[1].DestinationRoot, "DCIM", filepath.Base(plans[1].Jobs[i].Source))
	}
	if _, err := RunMultiple(context.Background(), plans, Copy, MultipleHooks{}); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing target accepted: %v", err)
	}
	if entries, _ := os.ReadDir(mountpoint); len(entries) != 0 {
		t.Fatal("wrote under an empty mount point", entries)
	}
	if entries, _ := os.ReadDir(plans[0].DestinationRoot); len(entries) != 0 {
		t.Fatal("wrote the first target before refusing the second", entries)
	}
}

func TestTargetLostWhileWaitingIsNotRecreated(t *testing.T) {
	for _, replacement := range []bool{false, true} {
		t.Run(fmt.Sprint(replacement), func(t *testing.T) {
			plans := multiplePlans(t)
			result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{Target: func(i int, _ Result) {
				if i == 0 {
					root := plans[1].DestinationRoot
					if err := os.Rename(root, root+"-detached"); err != nil {
						t.Fatal(err)
					}
					if replacement {
						if err := os.Mkdir(root, 0o755); err != nil {
							t.Fatal(err)
						}
					}
				}
			}})
			if err != nil {
				t.Fatal(err)
			}
			if !replacement {
				if _, err := os.Stat(plans[1].DestinationRoot); !os.IsNotExist(err) {
					t.Fatal("disappeared target was recreated", err)
				}
			} else if entries, _ := os.ReadDir(plans[1].DestinationRoot); len(entries) != 0 {
				t.Fatal("replacement target received copies or state", entries)
			}
			for _, file := range result.Files {
				if file.Phase == PhaseComplete {
					t.Fatal("lost target counted as complete", file)
				}
				if _, err := os.Stat(file.Source); err != nil {
					t.Fatal("lost target authorized source deletion", err)
				}
			}
		})
	}
}

func TestReplacedTargetWithSameCopyInodesCannotAuthorizeMove(t *testing.T) {
	plans := multiplePlans(t)
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{Target: func(i int, _ Result) {
		if i != len(plans)-1 {
			return
		}
		root := plans[0].DestinationRoot
		if err := os.Rename(root, root+"-detached"); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "DCIM"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, job := range plans[0].Jobs {
			old := filepath.Join(root+"-detached", "DCIM", filepath.Base(job.Destination))
			if err := os.Link(old, job.Destination); err != nil {
				t.Fatal(err)
			}
		}
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.Files {
		if file.Phase == PhaseComplete {
			t.Fatal("copy identity hid changed root identity", file)
		}
		if _, err := os.Stat(file.Source); err != nil {
			t.Fatal("replacement mount authorized deletion", err)
		}
	}
}

// Only existing directories compare by identity; a volume that ignores case
// or Unicode normalization makes two spellings one directory.
func TestTargetSpellingsOfOneDirectoryAreRefused(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	writeFile(t, filepath.Join(source, "a.JPG"), "photo")
	for _, pair := range [][2]string{{"Library", "library"}, {norm.NFC.String("Café"), norm.NFD.String("Café")}} {
		first, second := filepath.Join(root, pair[0]), filepath.Join(root, pair[1])
		if err := os.Mkdir(first, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(second); err != nil {
			// A volume that distinguishes the spellings: the second is
			// missing and refused for that reason instead.
			if err := ValidateTargets(source, []string{first, second}); err == nil {
				t.Fatalf("%q accepted", pair)
			}
			continue
		}
		if err := ValidateTargets(source, []string{first, second}); err == nil || !strings.Contains(err.Error(), "alias") {
			t.Fatalf("%q: %v", pair, err)
		}
	}
}

func TestSharedFilesystemsAreReported(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, dir := range []string{a, b} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	pairs := SharedFilesystems([]string{a, b})
	if len(pairs) != 1 || pairs[0] != [2]int{0, 1} {
		t.Fatalf("pairs = %v", pairs)
	}
}

func TestCancelledRunStartsNoTarget(t *testing.T) {
	plans := multiplePlans(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := RunMultiple(ctx, plans, Copy, MultipleHooks{})
	if err != nil || result.Started != 0 {
		t.Fatalf("started = %d, %v", result.Started, err)
	}
	for _, target := range result.Targets {
		for _, file := range target.Files {
			if file.Phase != PhaseFailed || !strings.HasPrefix(file.Error, "not started") {
				t.Fatal(file)
			}
		}
	}
	for _, plan := range plans {
		if entries, _ := os.ReadDir(plan.DestinationRoot); len(entries) != 0 {
			t.Fatal("cancelled run wrote a target", entries)
		}
	}
	for _, file := range result.Files {
		if file.Phase != PhaseFailed {
			t.Fatal(file)
		}
	}
}

func TestMoveRemovalUsesWorkersAndReportsEachFile(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "card")
	var plans []Plan
	for _, name := range []string{"a", "b"} {
		destination := filepath.Join(root, name)
		if err := os.Mkdir(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		plans = append(plans, Plan{SourceRoot: source, DestinationRoot: destination, Conflict: Skip, Workers: 4, Incremental: true})
	}
	for i := range 12 {
		name := fmt.Sprintf("IMG_%04d.JPG", i)
		writeFile(t, filepath.Join(source, name), name)
		for j := range plans {
			plans[j].Jobs = append(plans[j].Jobs, Job{Source: filepath.Join(source, name), Destination: filepath.Join(plans[j].DestinationRoot, name)})
		}
	}
	reported := map[string]int{}
	finals := 0
	result, err := RunMultiple(context.Background(), plans, Move, MultipleHooks{
		File: func(file FileResult) { reported[file.Source]++ },
		Event: func(_ int, event Event) {
			if event.Phase == PhaseComplete {
				finals++
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(reported) != 12 || finals != 24 {
		t.Fatalf("reported %d files, %d final events", len(reported), finals)
	}
	for source, count := range reported {
		if count != 1 {
			t.Fatalf("%s reported %d times", source, count)
		}
		if _, err := os.Stat(source); !os.IsNotExist(err) {
			t.Fatal("verified Move kept source", err)
		}
	}
	for _, file := range result.Files {
		if file.Phase != PhaseComplete {
			t.Fatal(file)
		}
	}
}
