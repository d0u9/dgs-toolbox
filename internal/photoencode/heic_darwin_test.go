//go:build darwin && cgo

package photoencode

import (
	"context"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeHEICRGBFixture(t *testing.T) {
	data, e := os.ReadFile("testdata/rgb.heic")
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultOptions()
	img, m, e := decodeHEIC(data, o)
	if e != nil {
		t.Fatal(e)
	}
	if img.Bounds().Dx() != 32 || img.Bounds().Dy() != 16 || len(m.icc) == 0 {
		t.Fatalf("size/profile: %v %d", img.Bounds(), len(m.icc))
	}
	left := color.NRGBAModel.Convert(img.At(4, 4)).(color.NRGBA)
	right := color.NRGBAModel.Convert(img.At(28, 4)).(color.NRGBA)
	if left.R < 180 || left.B > 70 || right.B < 180 || right.R > 70 {
		t.Fatalf("decoded pixels: %v %v", left, right)
	}
	dest := filepath.Join(t.TempDir(), "out.jpg")
	r := Encode(context.Background(), Job{Source: "testdata/rgb.heic", Destination: dest}, o)
	if !r.Published {
		t.Fatal(r)
	}
	out, e := os.ReadFile(dest)
	if e != nil {
		t.Fatal(e)
	}
	md, e := jpegMetadata(out)
	if e != nil || len(md.icc) == 0 {
		t.Fatalf("output ICC: %d %v", len(md.icc), e)
	}
}
