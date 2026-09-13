package photo

import (
	"bytes"
	"encoding/binary"
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
	if err := organizeFolder(strings.NewReader("n\n"), &out, []string{root}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "20261010/") || !strings.Contains(out.String(), "Cancelled") {
		t.Fatalf("output = %q", out.String())
	}
	if _, err := os.Stat(photo); err != nil {
		t.Fatalf("cancel moved the photo: %v", err)
	}
	out.Reset()
	if err := organizeFolder(strings.NewReader("y\n"), &out, []string{root}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "20261010", "IMG_0001.JPG")); err != nil {
		t.Fatalf("photo not organized: %v\n%s", err, out.String())
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
