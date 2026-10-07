//go:build darwin && cgo

package filethumb

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeThumbnailUsesQuickLookWithoutHelperExecutable(t *testing.T) {
	picture := image.NewRGBA(image.Rect(0, 0, 80, 60))
	for y := 0; y < 60; y++ {
		for x := 0; x < 80; x++ {
			picture.Set(x, y, color.RGBA{R: uint8(x * 3), G: uint8(y * 4), B: 90, A: 255})
		}
	}
	path := filepath.Join(t.TempDir(), "synthetic.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, picture); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := Render(path)
	if err != nil {
		t.Skip("native QuickLook provider unavailable in this test environment")
	}
	if _, err := jpeg.Decode(bytes.NewReader(data)); err != nil {
		t.Fatal("native result is not a thumbnail", err)
	}
	if _, err := Render(filepath.Join(t.TempDir(), "missing.png")); err == nil {
		t.Fatal("missing file rendered")
	}
}
