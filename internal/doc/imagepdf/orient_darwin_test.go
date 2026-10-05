//go:build darwin && cgo

package imagepdf

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/pdfpage"
	"dgs-toolbox/internal/imaging"
)

// TestEmbeddedJPEGShowsUpright draws each orientation's page and finds each
// quarter's colour where the image turned upright has it.
func TestEmbeddedJPEGShowsUpright(t *testing.T) {
	stored := quadrants(64, 32)
	var in bytes.Buffer
	if err := jpeg.Encode(&in, stored, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	for orientation := 1; orientation <= 8; orientation++ {
		out, err := Convert(context.Background(), "photo.jpg", withOrientation(in.Bytes(), orientation))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "page.pdf")
		os.WriteFile(path, out, 0600)
		picture, err := pdfpage.Render(path, 1, 128, 0.95)
		if err != nil {
			t.Fatal(err)
		}
		drawn, err := jpeg.Decode(bytes.NewReader(picture))
		if err != nil {
			t.Fatal(err)
		}
		want := imaging.Orient(stored, orientation)
		for _, at := range [][2]float64{{.25, .25}, {.75, .25}, {.25, .75}, {.75, .75}} {
			if !near(drawn, want, at) {
				t.Fatalf("orientation %d: quarter %v is not where the upright image has it", orientation, at)
			}
		}
	}
}

func near(a, b image.Image, at [2]float64) bool {
	pick := func(img image.Image) [3]int {
		r := img.Bounds()
		c := img.At(r.Min.X+int(at[0]*float64(r.Dx())), r.Min.Y+int(at[1]*float64(r.Dy())))
		x, y, z, _ := c.RGBA()
		return [3]int{int(x >> 8), int(y >> 8), int(z >> 8)}
	}
	p, q := pick(a), pick(b)
	for i := range p {
		if d := p[i] - q[i]; d > 60 || d < -60 {
			return false
		}
	}
	return true
}
