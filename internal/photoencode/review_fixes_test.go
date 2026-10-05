package photoencode

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"
)

func TestPreservedEXIFDropsThumbnailBytes(t *testing.T) {
	o := binary.LittleEndian
	ex := make([]byte, 8)
	copy(ex, "II")
	o.PutUint16(ex[2:], 42)
	o.PutUint32(ex[4:], 8)
	entry := func(tag, typ uint16, count, value uint32) {
		ex = o.AppendUint16(ex, tag)
		ex = o.AppendUint16(ex, typ)
		ex = o.AppendUint32(ex, count)
		ex = o.AppendUint32(ex, value)
	}
	// IFD0 at 8: one entry, next IFD1 at 26.
	ex = o.AppendUint16(ex, 1)
	entry(0x112, 3, 1, 6)
	ex = o.AppendUint32(ex, 26)
	// IFD1 at 26: thumbnail offset and length, then the thumbnail at 56.
	ex = o.AppendUint16(ex, 2)
	entry(0x201, 4, 1, 56)
	entry(0x202, 4, 1, 14)
	ex = o.AppendUint32(ex, 0)
	ex = append(ex, "OLD THUMBNAIL!"...)
	m := metadata{exif: ex}
	if err := m.readExif(); err != nil {
		t.Fatal(err)
	}
	kept, err := m.outputExif(true, 10, 10, 240)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(kept, []byte("THUMBNAIL")) || len(kept)-6 != 26 {
		t.Fatalf("thumbnail kept: %d bytes %q", len(kept)-6, kept)
	}
	if result := (metadata{exif: kept[6:]}); result.readExif() != nil || result.orientation != 1 {
		t.Fatal("kept EXIF unreadable")
	}
}

func TestValidateNamesTheOption(t *testing.T) {
	o := DefaultOptions()
	o.MaxPixels = 0
	var optionErr *OptionError
	if err := o.Validate(); !errors.As(err, &optionErr) || optionErr.Option != "max_pixels" {
		t.Fatal(err)
	}
	o = DefaultOptions()
	o.Quality = 0
	if err := o.Validate(); !errors.As(err, &optionErr) || optionErr.Option != "quality" {
		t.Fatal(err)
	}
}

func TestExistingOutputIsSkippedNotFailed(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "a.png"), image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	out := filepath.Join(root, "out")
	o := DefaultOptions()
	jobs, err := Plan(root, out, o)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if r := Encode(context.Background(), jobs[0], o); !r.Published {
		t.Fatal(r)
	}
	jobs, err = Plan(root, out, o)
	if err != nil || len(jobs) != 1 || !jobs[0].Exists {
		t.Fatal(jobs, err)
	}
	if r := Encode(context.Background(), jobs[0], o); r.Published || !r.Skipped || r.Cancelled {
		t.Fatal(r)
	}
}

func TestEarlierOutputsAreNotInputs(t *testing.T) {
	root := t.TempDir()
	writePNG(t, filepath.Join(root, "a.png"), image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	o := DefaultOptions()
	o.Recursive = true
	jobs, err := Plan(root, root, o)
	if err != nil || len(jobs) != 1 {
		t.Fatal(jobs, err)
	}
	if r := Encode(context.Background(), jobs[0], o); !r.Published {
		t.Fatal(r)
	}
	jobs, err = Plan(root, root, o)
	if err != nil || len(jobs) != 1 || filepath.Base(jobs[0].Source) != "a.png" {
		t.Fatalf("earlier output planned as input: %+v %v", jobs, err)
	}
}

func TestCancelledJobIsMarkedCancelled(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.png")
	writePNG(t, src, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if r := Encode(ctx, Job{Source: src, Destination: filepath.Join(root, "out.jpg")}, DefaultOptions()); r.Published || !r.Cancelled {
		t.Fatal(r)
	}
}

func TestEncodeRefusesLinkInsideDestination(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "a.png")
	writePNG(t, src, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	out, elsewhere := filepath.Join(root, "out"), filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(out, "nested")); err != nil {
		t.Fatal(err)
	}
	r := Encode(context.Background(), Job{Source: src, Destination: filepath.Join(out, "nested", "a.jpg"), Root: out}, DefaultOptions())
	if r.Published {
		t.Fatal("wrote through a link inside the destination")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatal(entries)
	}
}
