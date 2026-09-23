package scanread_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"dgs-toolbox/internal/box/scanmeta"
	"dgs-toolbox/internal/box/scanread"
	"dgs-toolbox/internal/box/thumb"
)

func picture(t *testing.T, width, height int) image.Image {
	t.Helper()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			out.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 5), B: 0x30, A: 0xff})
		}
	}
	return out
}

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, picture(t, width, height)); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

func pdfWithImage(t *testing.T, image []byte) []byte {
	t.Helper()
	importer, err := api.Import("pos:c, scale:0.5", types.POINTS)
	if err != nil {
		t.Fatalf("import settings: %v", err)
	}
	var out bytes.Buffer
	if err := api.ImportImages(nil, &out, []io.Reader{bytes.NewReader(image)}, importer, nil); err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	return out.Bytes()
}

// One read has to answer everything, because the next read is a round trip to
// the NAS.
func TestOneReadAnswersEverything(t *testing.T) {
	data := pdfWithImage(t, pngBytes(t, 400, 560))
	result, err := scanread.Read(data)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if result.Size != int64(len(data)) {
		t.Errorf("size: got %d want %d", result.Size, len(data))
	}
	if result.Digest == "" {
		t.Error("no whole-file digest")
	}
	if result.ImageDigest == "" {
		t.Error("no image digest for a scan that holds a picture")
	}
	if result.Digest == result.ImageDigest {
		t.Error("the two digests are the same, so one of them is not being computed")
	}
	if result.Info.Kind != scanmeta.KindPDF || result.Info.Pages != 1 {
		t.Errorf("metadata: %+v", result.Info)
	}
	if result.NeedsRender {
		t.Errorf("needs render: %s", result.RenderError)
	}
	if len(result.Thumbs.Grid) == 0 || len(result.Thumbs.Preview) == 0 {
		t.Fatal("a thumbnail is missing")
	}
	assertJPEGWithin(t, "grid", result.Thumbs.Grid, thumb.GridSize)
	assertJPEGWithin(t, "preview", result.Thumbs.Preview, thumb.PreviewSize)
	if len(result.Thumbs.Grid) >= len(result.Thumbs.Preview) {
		t.Error("the grid thumbnail is not smaller than the preview")
	}
}

func assertJPEGWithin(t *testing.T, name string, data []byte, size int) {
	t.Helper()
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s is not a readable JPEG: %v", name, err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > size || bounds.Dy() > size {
		t.Errorf("%s is %dx%d, over %d", name, bounds.Dx(), bounds.Dy(), size)
	}
}

// The raw stream is what the digest is taken over and the rendered form is what
// the thumbnail is drawn from. A PDF stores a PNG re-compressed, so the two are
// not the same bytes, and confusing them would tie every digest in the Box to a
// library version.
func TestRawAndRenderedAreDifferentThings(t *testing.T) {
	result, err := scanread.Read(pdfWithImage(t, pngBytes(t, 200, 260)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(result.Info.Images) == 0 {
		t.Fatal("no images")
	}
	first := result.Info.Images[0]
	if len(first.Bytes) == 0 {
		t.Error("no raw stream, so there is nothing stable to digest")
	}
	if len(first.Rendered) == 0 {
		t.Error("no rendered form, so there is nothing to draw a thumbnail from")
	}
	if bytes.Equal(first.Bytes, first.Rendered) {
		t.Error("raw and rendered came back identical, so one of them is not what it claims")
	}
}

// A scan is never enlarged: a blurred picture of the same information in a
// bigger file is the wrong answer twice.
func TestSmallScansAreNotEnlarged(t *testing.T) {
	result, err := scanread.Read(pngBytes(t, 120, 90))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if result.NeedsRender {
		t.Fatalf("needs render: %s", result.RenderError)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(result.Thumbs.Preview))
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if decoded.Bounds().Dx() != 120 || decoded.Bounds().Dy() != 90 {
		t.Errorf("a 120x90 scan came back as %v", decoded.Bounds())
	}
}

// A page with nothing to draw is still filed. An inbox that cannot be drained
// is worse than a Box with a missing picture.
func TestNoPictureIsStillAScan(t *testing.T) {
	result, err := scanread.Read([]byte("not a picture and not a pdf"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if result.Digest == "" {
		t.Error("no whole-file digest, so it cannot even be deduplicated")
	}
	if result.ImageDigest != "" {
		t.Error("an image digest was invented for a file with no picture")
	}
	if !result.NeedsRender || result.RenderError == "" {
		t.Errorf("nothing said why there is no picture: %+v", result)
	}
}

func TestReadFileMatchesReadBytes(t *testing.T) {
	data := pdfWithImage(t, pngBytes(t, 150, 200))
	path := filepath.Join(t.TempDir(), "scan-0012.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	fromDisk, err := scanread.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	fromMemory, err := scanread.Read(data)
	if err != nil {
		t.Fatalf("read bytes: %v", err)
	}
	if fromDisk.Digest != fromMemory.Digest || fromDisk.ImageDigest != fromMemory.ImageDigest {
		t.Error("reading from disk and from memory disagreed")
	}
}

// Same file, same answer, every time. A digest that moved between runs would
// make every scan in the Box look new after an upgrade.
func TestDigestsAreStable(t *testing.T) {
	data := pdfWithImage(t, pngBytes(t, 180, 240))
	first, err := scanread.Read(data)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		again, err := scanread.Read(data)
		if err != nil {
			t.Fatal(err)
		}
		if again.Digest != first.Digest || again.ImageDigest != first.ImageDigest {
			t.Fatalf("digests moved: %s/%s then %s/%s", first.Digest, first.ImageDigest, again.Digest, again.ImageDigest)
		}
	}
}
