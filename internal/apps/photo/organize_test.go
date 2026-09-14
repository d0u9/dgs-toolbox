package photo

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOrganizeFolderAsksBeforeUsingExistingDateFolder(t *testing.T) {
	root := t.TempDir()
	photo := filepath.Join(root, "IMG_0001.JPG")
	if err := os.WriteFile(photo, testJPEG(t, "2026:10:10 11:12:13"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "20261010"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := organizeFolder(strings.NewReader("n\n"), &out, []string{root}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "20261010/") || !strings.Contains(out.String(), "Cancelled") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(photo); err != nil {
		t.Fatalf("cancel moved the photo: %v", err)
	}
	out.Reset()
	if err := organizeFolder(strings.NewReader("y\n"), &out, []string{root}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "20261010", "IMG_0001.JPG")); err != nil {
		t.Fatalf("photo not organized: %v\n%s", err, out.String())
	}
}

func TestOrganizeFolderFlags(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(t.TempDir(), "sorted")
	top := filepath.Join(root, "IMG_0001.JPG")
	nested := filepath.Join(root, "trip", "IMG_0002.JPG")
	hidden := filepath.Join(root, ".cache", "IMG_0003.JPG")
	for _, path := range []string{top, nested, hidden} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, testJPEG(t, "2026:10:10 11:12:13"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dest, "2026", "10", "10"), 0o755); err != nil {
		t.Fatal(err)
	}
	flags := map[string]string{"recursive": "true", "format": "YYYY/MM/DD", "dest": dest, "dry-run": "true", "yes": "true"}
	var out bytes.Buffer
	if err := organizeFolder(strings.NewReader(""), &out, []string{root}, flags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "would organize 2") {
		t.Fatalf("dry run output = %q", out.String())
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("dry run moved a photo: %v", err)
	}
	flags["dry-run"] = "false"
	out.Reset()
	if err := organizeFolder(strings.NewReader(""), &out, []string{root}, flags); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"IMG_0001.JPG", "IMG_0002.JPG"} {
		if _, err := os.Stat(filepath.Join(dest, "2026", "10", "10", name)); err != nil {
			t.Fatalf("%s not organized: %v\n%s", name, err, out.String())
		}
	}
	if _, err := os.Stat(hidden); err != nil {
		t.Fatalf("hidden folder was organized: %v", err)
	}
}

func TestOrganizeFolderRejectsBadFormat(t *testing.T) {
	for _, format := range []string{"../YYYY", "YYYY//MM", "photos", "YYYY MM"} {
		err := organizeFolder(strings.NewReader(""), io.Discard, []string{t.TempDir()}, map[string]string{"format": format})
		if err == nil {
			t.Errorf("format %q accepted", format)
		}
	}
}

// testJPEG builds a minimal JPEG whose EXIF carries only DateTimeOriginal.
func testJPEG(t *testing.T, captured string) []byte {
	t.Helper()
	var tiff bytes.Buffer
	tiff.WriteString("II")
	for _, value := range []any{uint16(42), uint32(8), uint16(1), uint16(0x8769), uint16(4), uint32(1), uint32(26), uint32(0),
		uint16(1), uint16(0x9003), uint16(2), uint32(20), uint32(44), uint32(0)} {
		_ = binary.Write(&tiff, binary.LittleEndian, value)
	}
	tiff.WriteString(captured)
	tiff.WriteByte(0)
	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var data bytes.Buffer
	data.Write([]byte{0xff, 0xd8, 0xff, 0xe1})
	_ = binary.Write(&data, binary.BigEndian, uint16(len(payload)+2))
	data.Write(payload)
	data.Write([]byte{0xff, 0xd9})
	return data.Bytes()
}
