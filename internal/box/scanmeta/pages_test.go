package scanmeta_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"dgs-toolbox/internal/box/scanmeta"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Paging decodes the page asked for and nothing else, and names each page's
// picture by the page it sits on.
func TestPagesDecodeOnePageAtATime(t *testing.T) {
	importer, err := api.Import("pos:c, scale:0.5", types.POINTS)
	if err != nil {
		t.Fatalf("import settings: %v", err)
	}
	var out bytes.Buffer
	pictures := []io.Reader{bytes.NewReader(pngBytes(t, 40, 60)), bytes.NewReader(pngBytes(t, 60, 40))}
	if err := api.ImportImages(nil, &out, pictures, importer, nil); err != nil {
		t.Fatalf("build pdf: %v", err)
	}
	pages, err := scanmeta.OpenPages(out.Bytes())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if pages.Count() != 2 {
		t.Fatalf("count: got %d, want 2", pages.Count())
	}
	for page := 1; page <= 2; page++ {
		picture, err := pages.Page(page)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		if picture.Page != page || len(picture.Rendered) == 0 {
			t.Errorf("page %d: %+v", page, picture.Page)
		}
	}
	if _, err := pages.Page(3); err == nil {
		t.Error("page 3 of 2 was answered")
	}
}

// A picture file is one page, and that page is the picture.
func TestPagesOfAPictureFile(t *testing.T) {
	pages, err := scanmeta.OpenPages(pngBytes(t, 20, 30))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if pages.Count() != 1 {
		t.Fatalf("count: got %d", pages.Count())
	}
	if picture, err := pages.Page(1); err != nil || len(picture.Rendered) == 0 {
		t.Fatalf("page 1: %v", err)
	}
	if _, err := pages.Page(2); errors.Is(err, scanmeta.ErrNoPageImage) {
		t.Error("page 2 reported as blank rather than missing")
	}
}
