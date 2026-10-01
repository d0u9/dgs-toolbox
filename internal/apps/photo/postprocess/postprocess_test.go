package postprocess

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
