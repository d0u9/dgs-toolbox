package postprocess

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/verifiedcopy"
)

type captureReadFailure struct{ err error }

func (r captureReadFailure) Read([]byte) (int, error) { return 0, r.err }

func TestCaptureDatePreservesReadErrorsWrappedByEXIFDecoder(t *testing.T) {
	failure := errors.New("synthetic read failure")
	input := io.MultiReader(bytes.NewReader([]byte("II*\x00")), captureReadFailure{failure})
	_, err := captureDateReader(input)
	if !errors.Is(err, failure) || errors.Is(err, errNoCaptureDate) {
		t.Fatalf("I/O error classified as missing date: %v", err)
	}
}

func TestUnavailableCaptureDateDiffersFromFileAccessFailure(t *testing.T) {
	_, err := captureDateReader(bytes.NewReader([]byte("unsupported container")))
	if !errors.Is(err, errNoCaptureDate) {
		t.Fatalf("unsupported metadata was treated as I/O failure: %v", err)
	}
	_, err = captureDate(filepath.Join(t.TempDir(), "missing.JPG"))
	if !errors.Is(err, os.ErrNotExist) || errors.Is(err, errNoCaptureDate) {
		t.Fatalf("missing file was treated as missing metadata: %v", err)
	}
}

func existingPhoto(t *testing.T, root, source, path string) ExistingFile {
	t.Helper()
	checked, err := verifiedcopy.Check(context.Background(), root, path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ExistingFile{File: File{Source: source, Path: path}, Digest: checked.Digest, Size: checked.Size}
}

func TestUnorganizedExistingPhotoRetainsVerification(t *testing.T) {
	for _, change := range []string{"unchanged", "before planning", "after planning"} {
		t.Run(change, func(t *testing.T) {
			root := t.TempDir()
			jpg := writePhoto(t, root, "DCIM/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
			raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
			witness := existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)
			files := []File{witness.File, {Source: "card/DCIM/IMG_0001.DNG", Path: raw}}
			changePhoto := func() { writePhoto(t, root, "DCIM/IMG_0001.JPG", jpeg(t, "2026:10:11 11:12:13")) }
			if change == "before planning" {
				changePhoto()
			}
			plan := NewPlanWithExisting(context.Background(), root, files, []ExistingFile{witness})
			if change != "before planning" {
				for _, outcome := range plan.Files {
					if outcome.Status != Moved || outcome.Date != "20261010" {
						t.Fatalf("unchanged shot cannot organize: %#v", outcome)
					}
				}
			}
			if change == "after planning" {
				changePhoto()
			}
			result := Apply(plan)
			for _, outcome := range result.Files {
				if change == "unchanged" {
					if outcome.Status != Moved {
						t.Fatal(outcome)
					}
					if _, err := os.Stat(outcome.Final); err != nil {
						t.Fatal(err)
					}
				} else {
					if outcome.Status != Failed || !strings.Contains(outcome.Error, "counterpart") || outcome.Final != outcome.Original {
						t.Fatalf("changed shot was not refused: %#v", outcome)
					}
					if _, err := os.Stat(outcome.Original); err != nil {
						t.Fatal("changed shot moved", err)
					}
				}
			}
		})
	}
}

func TestIncrementalOrganizationUsesVerifiedExistingJPGWithoutMovingIt(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	witness := existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)
	before, _ := os.Stat(jpg)
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{witness})
	if len(plan.Files) != 1 || plan.Files[0].Date != "20261010" || plan.Files[0].Status != Moved {
		t.Fatalf("plan = %#v", plan.Files)
	}
	result := Apply(plan)
	if result.Files[0].Status != Moved || result.Files[0].DateSource != "sidecar JPG EXIF" {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(filepath.Join(root, "20261010", "IMG_0001.DNG")); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(jpg)
	if !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("existing JPG moved or rewritten")
	}
}

func TestIncrementalOrganizationRefusesUnrelatedShotMember(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	writePhoto(t, root, "20261010/IMG_0001.NEF", []byte("another shot"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	if plan.Files[0].Status != Failed {
		t.Fatal(plan.Files[0])
	}
	if _, err := os.Stat(raw); err != nil {
		t.Fatal(err)
	}
}

func TestIncrementalOrganizationRevalidatesWitnessAfterConfirmation(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:11 11:12:13"))
	if result := Apply(plan); result.Files[0].Status != Failed {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(raw); err != nil {
		t.Fatal("new RAW moved using stale witness")
	}
}

func TestIncrementalOrganizationRefusesRenamedWitness(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/renamed.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	if plan.Files[0].Status != Failed || !strings.Contains(plan.Files[0].Error, "ambiguous") {
		t.Fatal(plan.Files[0])
	}
}

func TestIncrementalOrganizationDoesNotUseGlobalBasenameForPairing(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "other/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/other/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	if plan.Files[0].Status != Moved || plan.Files[0].Date != "20261009" || plan.Files[0].DateSource != "RAW metadata" {
		t.Fatal(plan.Files[0])
	}
}

func TestIncrementalOrganizationKeepsNewMemberWhenDateFoldersDisagree(t *testing.T) {
	root := t.TempDir()
	raw := writePhoto(t, root, "20261009/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	jpg := writePhoto(t, root, "DCIM/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.JPG", Path: jpg}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.DNG", raw)})
	if plan.Files[0].Status != Failed || !strings.Contains(plan.Files[0].Error, "date folder") {
		t.Fatal(plan.Files[0])
	}
	if _, err := os.Stat(jpg); err != nil {
		t.Fatal("new JPG moved away from its verified publication")
	}
}

func TestIncrementalOrganizationRefusesNewCollisionAfterPlanning(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	writePhoto(t, root, "20261010/IMG_0001.NEF", []byte("other shot"))
	if result := Apply(plan); result.Files[0].Status != Failed {
		t.Fatal(result.Files[0])
	}
	if _, err := os.Stat(raw); err != nil {
		t.Fatal("late collision moved RAW")
	}
}

func TestRunUsesSidecarDateAndRawDate(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		writePhoto(t, root, "camera/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13")),
		writePhoto(t, root, "camera/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03")),
		writePhoto(t, root, "camera/IMG_0002.NEF", tiff(t, "2026:10:11 14:15:16")),
	}
	files := make([]File, len(paths))
	for index, path := range paths {
		files[index] = File{Source: path, Path: path}
	}
	result := Run(root, files)
	wantDates := []string{"20261010", "20261010", "20261011"}
	for index, outcome := range result.Files {
		if outcome.Status != Moved || outcome.Date != wantDates[index] {
			t.Fatalf("outcome %d = %#v", index, outcome)
		}
		if _, err := os.Stat(outcome.Final); err != nil {
			t.Fatalf("final %d: %v", index, err)
		}
		if _, err := os.Stat(paths[index]); !os.IsNotExist(err) {
			t.Fatalf("original %d remains: %v", index, err)
		}
	}
	if result.Files[1].DateSource != "sidecar JPG EXIF" {
		t.Fatalf("RAW date source = %q", result.Files[1].DateSource)
	}
}

func TestRunDoesNotOverwriteExistingTarget(t *testing.T) {
	root := t.TempDir()
	source := writePhoto(t, root, "camera/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	target := writePhoto(t, root, "20261010/IMG_0001.JPG", []byte("existing"))
	result := Run(root, []File{{Source: source, Path: source}})
	if result.Files[0].Status != Failed {
		t.Fatalf("outcome = %#v", result.Files[0])
	}
	if data, _ := os.ReadFile(target); string(data) != "existing" {
		t.Fatalf("existing target changed to %q", data)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source was not retained: %v", err)
	}
}

func writePhoto(t *testing.T, root, relative string, data []byte) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func tiff(t *testing.T, captured string) []byte {
	t.Helper()
	var data bytes.Buffer
	data.WriteString("II")
	_ = binary.Write(&data, binary.LittleEndian, uint16(42))
	_ = binary.Write(&data, binary.LittleEndian, uint32(8))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(0x8769))
	_ = binary.Write(&data, binary.LittleEndian, uint16(4))
	_ = binary.Write(&data, binary.LittleEndian, uint32(1))
	_ = binary.Write(&data, binary.LittleEndian, uint32(26))
	_ = binary.Write(&data, binary.LittleEndian, uint32(0))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(0x9003))
	_ = binary.Write(&data, binary.LittleEndian, uint16(2))
	_ = binary.Write(&data, binary.LittleEndian, uint32(20))
	_ = binary.Write(&data, binary.LittleEndian, uint32(44))
	_ = binary.Write(&data, binary.LittleEndian, uint32(0))
	data.WriteString(captured)
	data.WriteByte(0)
	return data.Bytes()
}

func jpeg(t *testing.T, captured string) []byte {
	t.Helper()
	payload := append([]byte("Exif\x00\x00"), tiff(t, captured)...)
	var data bytes.Buffer
	data.Write([]byte{0xff, 0xd8, 0xff, 0xe1})
	_ = binary.Write(&data, binary.BigEndian, uint16(len(payload)+2))
	data.Write(payload)
	data.Write([]byte{0xff, 0xd9})
	return data.Bytes()
}

func TestPlanReportsExistingDatesWithoutMoving(t *testing.T) {
	root := t.TempDir()
	first := writePhoto(t, root, "IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	writePhoto(t, root, "IMG_0002.JPG", jpeg(t, "2026:10:11 11:12:13"))
	writePhoto(t, root, "notes.txt", []byte("text"))
	writePhoto(t, root, "20261011/old.JPG", []byte("old"))
	files, err := FolderFiles(root)
	if err != nil || len(files) != 3 {
		t.Fatalf("files = %#v, %v", files, err)
	}
	plan := NewPlan(root, files)
	if got := plan.Dates(); len(got) != 2 || got[0] != "20261010" || got[1] != "20261011" {
		t.Fatalf("dates = %v", got)
	}
	if got := plan.ExistingDates(); len(got) != 1 || got[0] != "20261011" {
		t.Fatalf("existing = %v", got)
	}
	if _, err := os.Stat(first); err != nil {
		t.Fatalf("plan moved a file: %v", err)
	}
}

// Two folders holding different shots named IMG_0001 on one day must not meet
// in one date folder, where the first one's JPG would be paired with the
// other's RAW.
func TestShotsSharingANameAreNotMixedInOneDateFolder(t *testing.T) {
	root := t.TempDir()
	lone := writePhoto(t, root, "100CANON/IMG_0001.JPG", jpeg(t, "2026:10:10 09:00:00"))
	raw := writePhoto(t, root, "101CANON/IMG_0001.CR2", tiff(t, "2026:10:10 18:00:00"))
	paired := writePhoto(t, root, "101CANON/IMG_0001.JPG", jpeg(t, "2026:10:10 18:00:00"))
	result := Run(root, []File{{Source: lone, Path: lone}, {Source: raw, Path: raw}, {Source: paired, Path: paired}})
	for _, outcome := range result.Files {
		if outcome.Status != Failed || !strings.Contains(outcome.Error, "another shot") {
			t.Fatalf("outcome = %#v", outcome)
		}
	}
	for _, path := range []string{lone, raw, paired} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s moved: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "20261010")); !os.IsNotExist(err) {
		t.Fatalf("date folder created: %v", err)
	}
}

// A RAW whose name is already taken in its date folder by an earlier shot's
// JPG stays where it is.
func TestShotDoesNotJoinAnEarlierShotOfTheSameName(t *testing.T) {
	root := t.TempDir()
	writePhoto(t, root, "20261010/IMG_0001.JPG", []byte("an earlier import"))
	raw := writePhoto(t, root, "camera/IMG_0001.CR2", tiff(t, "2026:10:10 18:00:00"))
	result := Run(root, []File{{Source: raw, Path: raw}})
	if result.Files[0].Status != Failed || !strings.Contains(result.Files[0].Error, "already has IMG_0001.JPG") {
		t.Fatalf("outcome = %#v", result.Files[0])
	}
	if _, err := os.Stat(raw); err != nil {
		t.Fatalf("raw moved: %v", err)
	}
}

func TestKeepBothRenamedShotIsNotOrganizedApart(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "DCIM/IMG_0001-2.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:10 11:12:13"))
	plan := NewPlan(root, []File{{Source: "card/DCIM/IMG_0001.JPG", Path: jpg}, {Source: "card/DCIM/IMG_0001.DNG", Path: raw}})
	for _, outcome := range plan.Files {
		if outcome.Status != Failed || !strings.Contains(outcome.Error, "Keep both") || outcome.Final != outcome.Original {
			t.Fatalf("renamed shot planned apart: %#v", outcome)
		}
	}
}

func TestChangedWitnessReportsWhy(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "20261010/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	raw := writePhoto(t, root, "DCIM/IMG_0001.DNG", tiff(t, "2026:10:09 01:02:03"))
	plan := NewPlanWithExisting(context.Background(), root, []File{{Source: "card/DCIM/IMG_0001.DNG", Path: raw}}, []ExistingFile{existingPhoto(t, root, "card/DCIM/IMG_0001.JPG", jpg)})
	if err := os.WriteFile(jpg, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	result := Apply(plan).Files[0]
	if result.Status != Failed || !strings.Contains(result.Error, "counterpart changed") || result.Final != raw {
		t.Fatalf("result = %#v", result)
	}
}

func TestCancelledOrganizeMovesNothing(t *testing.T) {
	root := t.TempDir()
	jpg := writePhoto(t, root, "DCIM/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	plan := NewPlan(root, []File{{Source: "card/DCIM/IMG_0001.JPG", Path: jpg}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := ApplyContext(ctx, plan).Files[0]
	if result.Status != Failed || !strings.Contains(result.Error, "cancelled") {
		t.Fatal(result)
	}
	if _, err := os.Stat(jpg); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDoesNotFillAReplacedRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "library")
	source := writePhoto(t, root, "camera/IMG_0001.JPG", jpeg(t, "2026:10:10 11:12:13"))
	plan := NewPlan(root, []File{{Source: source, Path: source}})
	if plan.Files[0].Status != Moved {
		t.Fatalf("plan = %#v", plan.Files[0])
	}
	// While the plan waits for confirmation the disk is unmounted, leaving an
	// empty mount point at the same path.
	if err := os.Rename(root, filepath.Join(parent, "unmounted")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	result := Apply(plan)
	if result.Files[0].Status != Failed || result.Files[0].Final != source {
		t.Fatalf("outcome = %#v", result.Files[0])
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatal("organize wrote into the empty mount point", entries)
	}
	if _, err := os.Stat(filepath.Join(parent, "unmounted", "camera", "IMG_0001.JPG")); err != nil {
		t.Fatal(err)
	}
}
