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

	"dgs-toolbox/internal/box/scanmeta"
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
	path, err := Cached(context.Background(), source, dir)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("broken"), 0600)
	rebuilt, err := Cached(context.Background(), source, dir)
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
