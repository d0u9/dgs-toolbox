package scanmeta_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"dgs-toolbox/internal/box/scanmeta"
)

// pngBytes builds a small picture: the stand-in for a scanned page.
func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			picture.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return out.Bytes()
}

// pdfWithImage wraps a picture in a one-page A4 PDF, which is what a scanner
// produces and what almost everything in a Box is.
func pdfWithImage(t *testing.T, picture []byte) []byte {
	t.Helper()
	importer, err := api.Import("pos:c, scale:0.5", types.POINTS)
	if err != nil {
		t.Fatalf("import settings: %v", err)
	}
	var out bytes.Buffer
	if err := api.ImportImages(nil, &out, []io.Reader{bytes.NewReader(picture)}, importer, nil); err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	return out.Bytes()
}

func TestReadPDFSaysWhatItIs(t *testing.T) {
	data := pdfWithImage(t, pngBytes(t, 120, 170))
	info, err := scanmeta.Read(data)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if info.Kind != scanmeta.KindPDF {
		t.Errorf("kind: got %q", info.Kind)
	}
	if info.Pages != 1 {
		t.Errorf("pages: got %d", info.Pages)
	}
	if info.PageSize != "A4" {
		t.Errorf("page size: got %q want A4", info.PageSize)
	}
	if info.ImageError != "" {
		t.Errorf("image error: %s", info.ImageError)
	}
	if len(info.Images) != 1 {
		t.Fatalf("images: got %d, want 1", len(info.Images))
	}
	if len(info.Images[0].Bytes) == 0 {
		t.Error("the image stream came back empty, so nothing can be digested or drawn from it")
	}
}

// A scanner rounds its page box, so a page half a millimetre off A4 is A4. A
// Box full of "209x296mm" groups into nothing.
func TestPaperNameToleratesRounding(t *testing.T) {
	const a4Width, a4Height = 595.276, 841.89
	if name, found := scanmeta.PaperName(a4Width, a4Height); !found || name != "A4" {
		t.Errorf("exact A4: got %q %v", name, found)
	}
	if name, found := scanmeta.PaperName(a4Width-1, a4Height+1); !found || name != "A4" {
		t.Errorf("rounded A4: got %q %v", name, found)
	}
	// A page turned on its side is still A4.
	if name, found := scanmeta.PaperName(a4Height, a4Width); !found || name != "A4" {
		t.Errorf("landscape A4: got %q %v", name, found)
	}
	if _, found := scanmeta.PaperName(100, 100); found {
		t.Error("a square page was given a paper name")
	}
}

func TestPaperNameIsStable(t *testing.T) {
	const a4Width, a4Height = 595.276, 841.89
	first, _ := scanmeta.PaperName(a4Width, a4Height)
	for range 20 {
		again, _ := scanmeta.PaperName(a4Width, a4Height)
		if again != first {
			t.Fatalf("the same page got %q and then %q", first, again)
		}
	}
}

func TestReadImageKeepsTheBytes(t *testing.T) {
	data := pngBytes(t, 64, 96)
	info, err := scanmeta.Read(data)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if info.Kind != scanmeta.KindImage {
		t.Errorf("kind: got %q", info.Kind)
	}
	if info.Pages != 1 {
		t.Errorf("pages: got %d", info.Pages)
	}
	if len(info.Images) != 1 {
		t.Fatalf("images: got %d", len(info.Images))
	}
	got := info.Images[0]
	if got.Width != 64 || got.Height != 96 {
		t.Errorf("size: got %dx%d", got.Width, got.Height)
	}
	if !bytes.Equal(got.Bytes, data) {
		t.Error("an image is its own stream and must come back unchanged, not re-encoded")
	}
}

// A file this build cannot decode is still filed: the inbox has to stay
// drainable, and a scan held back for a missing thumbnail is never filed at all.
func TestUndecodableImageIsReportedNotRefused(t *testing.T) {
	info, err := scanmeta.Read([]byte("this is not a picture"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if info.Kind != scanmeta.KindImage || info.Pages != 1 {
		t.Errorf("got %+v", info)
	}
	if info.ImageError == "" {
		t.Error("nothing said why there is no image")
	}
	if len(info.Images) != 0 {
		t.Error("images were invented for a file that has none")
	}
}

func TestColorModeAndDPI(t *testing.T) {
	const a4Width, a4Height = 595.276, 841.89
	mono := scanmeta.Image{Width: 1654, Height: 2339, Bits: 1, ColorSpace: "DeviceGray"}
	if mono.ColorMode() != "mono" {
		t.Errorf("one bit per component is mono, got %q", mono.ColorMode())
	}
	dpi, ok := mono.DPI(a4Width, a4Height)
	if !ok || dpi != 200 {
		t.Errorf("A4 at 1654x2339 is 200 dpi, got %d %v", dpi, ok)
	}
	gray := scanmeta.Image{Bits: 8, ColorSpace: "DeviceGray"}
	if gray.ColorMode() != "gray" {
		t.Errorf("gray: got %q", gray.ColorMode())
	}
	colour := scanmeta.Image{Bits: 8, ColorSpace: "DeviceRGB"}
	if colour.ColorMode() != "colour" {
		t.Errorf("colour: got %q", colour.ColorMode())
	}
	// No page and no pixels is no resolution, never a resolution of zero.
	if _, ok := (scanmeta.Image{}).DPI(a4Width, a4Height); ok {
		t.Error("an image with no pixels reported a resolution")
	}
	if _, ok := mono.DPI(0, 0); ok {
		t.Error("a page with no size reported a resolution")
	}
}

func TestKindComesFromTheBytesNotTheName(t *testing.T) {
	pdf := pdfWithImage(t, pngBytes(t, 40, 60))
	info, err := scanmeta.Read(pdf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if info.Kind != scanmeta.KindPDF {
		t.Errorf("a PDF was read as %q", info.Kind)
	}
	if !strings.HasPrefix(string(pdf[:5]), "%PDF-") {
		t.Fatal("fixture is not a PDF")
	}
}

func TestBrokenPDFIsAnError(t *testing.T) {
	if _, err := scanmeta.Read([]byte("%PDF-1.7\nand then nothing at all")); err == nil {
		t.Fatal("a PDF that cannot be parsed was accepted")
	}
}

// A PDF's own creation date is when it was scanned, and is more reliable than
// the file's modification time, which copying and syncing overwrite. It is
// stored as RFC 3339 with the offset it was written with, never converted.
func TestPDFCreationDateIsRFC3339(t *testing.T) {
	info, err := scanmeta.Read(pdfWithImage(t, pngBytes(t, 40, 60)))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if info.CreatedAt == "" {
		t.Fatal("no creation date")
	}
	if _, err := time.Parse(time.RFC3339, info.CreatedAt); err != nil {
		t.Errorf("creation date %q is not RFC 3339: %v", info.CreatedAt, err)
	}
	if info.Producer == "" {
		t.Error("no producer, which is what separates a Box into scanner eras")
	}
}

func TestNormalRotation(t *testing.T) {
	for in, want := range map[int]int{0: 0, 90: 90, 180: 180, 270: 270, 360: 0, 450: 90, -90: 270, 45: 0} {
		if got := scanmeta.NormalRotation(in); got != want {
			t.Errorf("NormalRotation(%d) = %d, want %d", in, got, want)
		}
	}
}
