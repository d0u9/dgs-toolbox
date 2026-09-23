package web_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	boxweb "dgs-toolbox/internal/apps/box/web"
)

func pagePNG(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			picture.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x40, A: 0xff})
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// threePages is a PDF of one picture per page. Page 2 is the only landscape
// one, so the shape of what comes back says which page was drawn.
func threePages(t *testing.T) []byte {
	t.Helper()
	importer, err := api.Import("pos:c, scale:0.5", types.POINTS)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	readers := []io.Reader{
		bytes.NewReader(pagePNG(t, 200, 300)),
		bytes.NewReader(pagePNG(t, 300, 200)),
		bytes.NewReader(pagePNG(t, 200, 300)),
	}
	if err := api.ImportImages(nil, &out, readers, importer, nil); err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	return out.Bytes()
}

func landscape(t *testing.T, data []byte) bool {
	t.Helper()
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded.Bounds().Dx() > decoded.Bounds().Dy()
}

func TestEveryPageOfAPDFCanBeLookedAt(t *testing.T) {
	engine, _, inbox := engineBox(t)
	putScan(t, inbox, "Scan_0001.pdf", threePages(t))
	engine.Rescan()
	pending := engine.Pending()
	if len(pending) != 1 || pending[0].Pages != 3 {
		t.Fatalf("pending: %+v", pending)
	}
	digest := pending[0].Digest

	// Pending: drawn from the inbox file.
	second, _, err := engine.Image(digest, "thumb", 2)
	if err != nil {
		t.Fatalf("pending page 2: %v", err)
	}
	if !landscape(t, second) {
		t.Error("pending page 2 is not page 2")
	}
	if _, _, err := engine.Image(digest, "thumb", 4); err == nil {
		t.Error("a fourth page of a three-page scan was drawn")
	}

	// Filed: drawn from the Box and cached.
	filed, err := engine.File(digest, boxweb.Edit{})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	for range 2 {
		second, _, err = engine.Image(filed.Digest, "preview", 2)
		if err != nil {
			t.Fatalf("filed page 2: %v", err)
		}
		if !landscape(t, second) {
			t.Error("filed page 2 is not page 2")
		}
	}
	first, _, err := engine.Image(filed.Digest, "preview", 1)
	if err != nil {
		t.Fatalf("filed page 1: %v", err)
	}
	if landscape(t, first) {
		t.Error("page 1 came back as page 2")
	}
}
