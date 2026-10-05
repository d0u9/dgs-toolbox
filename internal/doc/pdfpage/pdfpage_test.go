package pdfpage

import (
	"bytes"
	"errors"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/pdfpage/pdftest"
)

// onePage writes a computer-made PDF turned by rotate to a temporary file.
func onePage(t *testing.T, rotate int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "made.pdf")
	if err := os.WriteFile(path, pdftest.Made(rotate), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRenderDrawsAComputerMadePage(t *testing.T) {
	if !Available() {
		t.Skip("drawing is not compiled in")
	}
	for _, c := range []struct{ rotate, w, h int }{{0, 565, 800}, {90, 800, 565}} {
		data, err := Render(onePage(t, c.rotate), 1, 800, DefaultQuality)
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != c.w || b.Dy() != c.h {
			t.Fatalf("rotate %d: %dx%d, want %dx%d", c.rotate, b.Dx(), b.Dy(), c.w, c.h)
		}
	}
	if n, err := Count(onePage(t, 0)); err != nil || n != 1 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	if _, err := Render(onePage(t, 0), 2, 800, DefaultQuality); err == nil {
		t.Fatal("page 2 of one drew")
	}
}

func TestRenderRefusesBadArguments(t *testing.T) {
	if _, err := Render("x.pdf", 0, 800, DefaultQuality); err == nil {
		t.Fatal("page 0 accepted")
	}
	if _, err := Render("x.pdf", 1, 0, DefaultQuality); err == nil {
		t.Fatal("size 0 accepted")
	}
	if !Available() {
		if _, err := Render("x.pdf", 1, 800, DefaultQuality); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("want ErrUnavailable, got %v", err)
		}
	}
}

// A filled form keeps what was typed in its fields' appearances, not in the
// page's content; a page drawn without them shows empty boxes.
func TestRenderDrawsFilledFormFields(t *testing.T) {
	if !Available() {
		t.Skip("drawing is not compiled in")
	}
	path := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(path, pdftest.Form(), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := Render(path, 1, 842, DefaultQuality)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	// At 842 on the long side a point is a pixel; image rows run down.
	x, y, w, h := pdftest.FormBox[0], pdftest.FormBox[1], pdftest.FormBox[2], pdftest.FormBox[3]
	r, g, b, _ := img.At(x+w/2, img.Bounds().Dy()-(y+h/2)).RGBA()
	if r > 0x2000 || g > 0x2000 || b > 0x2000 {
		t.Fatalf("field's middle is %04x %04x %04x, want black", r, g, b)
	}
	if r, _, _, _ := img.At(x/2, img.Bounds().Dy()-(y+h/2)).RGBA(); r < 0xe000 {
		t.Fatalf("left of the field is %04x, want white", r)
	}
}
