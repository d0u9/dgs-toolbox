package imagepdf

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"dgs-toolbox/internal/doc/scanmeta"
)

func TestImagePageIsDeterministicAndFullSize(t *testing.T) {
	for _, format := range []string{"png", "jpg"} {
		t.Run(format, func(t *testing.T) {
			img := image.NewNRGBA(image.Rect(0, 0, 40, 60))
			img.Set(4, 5, color.NRGBA{R: 255, A: 255})
			var input bytes.Buffer
			if format == "png" {
				if err := png.Encode(&input, img); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := jpeg.Encode(&input, img, nil); err != nil {
					t.Fatal(err)
				}
			}
			a, err := Convert(context.Background(), "scan."+format, input.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			b, err := Convert(context.Background(), "scan."+format, input.Bytes())
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(a, b) {
				t.Fatal("PDF changed for identical input")
			}
			info, err := scanmeta.Read(a)
			if err != nil {
				t.Fatal(err)
			}
			if len(info.Images) != 1 || info.Images[0].Width != 40 || info.Images[0].Height != 60 {
				t.Fatalf("images: %+v", info.Images)
			}
		})
	}
}

func TestBrokenImageAndCanceledConversion(t *testing.T) {
	if _, err := Convert(context.Background(), "scan.png", []byte("broken")); err == nil {
		t.Fatal("accepted broken image")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var in bytes.Buffer
	png.Encode(&in, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	if _, err := Convert(ctx, "scan.png", in.Bytes()); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestCachedRebuildsCorruptionAndPrepareLeavesSource(t *testing.T) {
	var in bytes.Buffer
	png.Encode(&in, image.NewNRGBA(image.Rect(0, 0, 3, 4)))
	source := filepath.Join(t.TempDir(), "scan.png")
	os.WriteFile(source, in.Bytes(), 0600)
	dir := t.TempDir()
	path, err := Cached(context.Background(), []string{source}, dir)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("broken"), 0600)
	rebuilt, err := Cached(context.Background(), []string{source}, dir)
	if err != nil {
		t.Fatal(err)
	}
	actual, _ := os.ReadFile(rebuilt)
	if !bytes.Equal(actual, expected) {
		t.Fatal("cache rebuild differs")
	}
	original, _ := os.ReadFile(source)
	if !bytes.Equal(original, in.Bytes()) {
		t.Fatal("source changed")
	}
}

// quadrants is an image of four colours, one per quarter, so its turn and
// flip can be told apart.
func quadrants(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	colours := []color.NRGBA{{R: 255, A: 255}, {G: 255, A: 255}, {B: 255, A: 255}, {R: 255, G: 255, A: 255}}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, colours[boolInt(x >= w/2)+2*boolInt(y >= h/2)])
		}
	}
	return img
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// withOrientation puts an EXIF block naming orientation after a JPEG's
// start marker.
func withOrientation(jpg []byte, orientation int) []byte {
	tiff := []byte{'M', 'M', 0, 42, 0, 0, 0, 8, 0, 1, 0x01, 0x12, 0, 3, 0, 0, 0, 1, 0, byte(orientation), 0, 0, 0, 0, 0, 0}
	body := append([]byte("Exif\x00\x00"), tiff...)
	segment := append([]byte{0xff, 0xe1, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
	return append(append(append([]byte{}, jpg[:2]...), segment...), jpg[2:]...)
}

func TestJPEGIsEmbeddedAsItsOwnBytes(t *testing.T) {
	var in bytes.Buffer
	if err := jpeg.Encode(&in, quadrants(64, 32), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	for orientation := 1; orientation <= 8; orientation++ {
		data := withOrientation(in.Bytes(), orientation)
		out, err := Convert(context.Background(), "photo.JPG", data)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(out, data) || !bytes.Contains(out, []byte("/DCTDecode")) {
			t.Fatalf("orientation %d: JPEG bytes not embedded as they are", orientation)
		}
		if len(out) > len(data)+2048 {
			t.Fatalf("orientation %d: %d bytes for a %d-byte JPEG", orientation, len(out), len(data))
		}
		pw, ph, _ := placement(64, 32, orientation)
		if (orientation >= 5) != (pw < ph) {
			t.Fatalf("orientation %d: page %vx%v", orientation, pw, ph)
		}
	}
}

func TestImagesBecomePagesInOrder(t *testing.T) {
	var a, b bytes.Buffer
	jpeg.Encode(&a, quadrants(30, 20), nil)
	png.Encode(&b, quadrants(10, 40))
	out, err := ConvertAll(context.Background(), []Input{{"a.jpg", a.Bytes()}, {"b.png", b.Bytes()}})
	if err != nil {
		t.Fatal(err)
	}
	info, err := scanmeta.Read(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Pages != 2 || len(info.Images) != 2 || info.Images[0].Width != 30 || info.Images[1].Width != 10 {
		t.Fatalf("pages %d images %+v", info.Pages, info.Images)
	}
	if _, err := ConvertAll(context.Background(), []Input{{"a.jpg", a.Bytes()}, {"b.pdf", []byte("%PDF-")}}); err == nil {
		t.Fatal("accepted a PDF as a page")
	}
}
