//go:build darwin && cgo

package photoencode

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/tiff"
)

func TestNativeHEICRGBFixture(t *testing.T) {
	data, e := os.ReadFile("testdata/rgb.heic")
	if e != nil {
		t.Fatal(e)
	}
	o := DefaultOptions()
	img, m, e := decodeNative("HEIC", data, o)
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

func writeTIFF(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if e := tiff.Encode(&buf, img, nil); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(path, buf.Bytes(), 0600); e != nil {
		t.Fatal(e)
	}
}

func TestNativeTIFF(t *testing.T) {
	root := t.TempDir()
	rgb16 := image.NewRGBA64(image.Rect(0, 0, 64, 32))
	gray := image.NewGray(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			if x < 32 {
				rgb16.SetRGBA64(x, y, color.RGBA64{R: 0xffff, A: 0xffff})
			} else {
				rgb16.SetRGBA64(x, y, color.RGBA64{B: 0xffff, A: 0xffff})
			}
			gray.SetGray(x, y, color.Gray{Y: uint8(x * 4)})
		}
	}
	writeTIFF(t, filepath.Join(root, "colour.tif"), rgb16)
	writeTIFF(t, filepath.Join(root, "gray.tiff"), gray)
	o := DefaultOptions()
	o.Size = 16
	jobs, e := Plan(root, filepath.Join(root, "out"), o)
	if e != nil || len(jobs) != 2 {
		t.Fatal(jobs, e)
	}
	for _, j := range jobs {
		r := Encode(context.Background(), j, o)
		if !r.Published || r.Width != 16 || r.Height != 8 {
			t.Fatalf("%+v", r)
		}
		data, _ := os.ReadFile(j.Destination)
		img, e := jpeg.Decode(bytes.NewReader(data))
		if e != nil {
			t.Fatal(e)
		}
		if filepath.Base(j.Source) == "gray.tiff" {
			c := color.NRGBAModel.Convert(img.At(15, 4)).(color.NRGBA)
			if c.R != c.G || c.G != c.B || c.R < 150 {
				t.Fatal("gray TIFF pixels", c)
			}
			continue
		}
		left := color.NRGBAModel.Convert(img.At(2, 4)).(color.NRGBA)
		right := color.NRGBAModel.Convert(img.At(14, 4)).(color.NRGBA)
		if left.R < 180 || left.B > 70 || right.B < 180 || right.R > 70 {
			t.Fatalf("16-bit TIFF pixels: %v %v", left, right)
		}
	}
}

func systemProfile(t *testing.T, name string) []byte {
	t.Helper()
	icc, e := os.ReadFile(filepath.Join("/System/Library/ColorSync/Profiles", name))
	if e != nil {
		t.Skip(e)
	}
	return icc
}

func TestExportsAreSRGB(t *testing.T) {
	_, srgb, e := toSRGB(image.NewNRGBA(image.Rect(0, 0, 1, 1)), nil)
	if e != nil || len(srgb) < 128 {
		t.Fatal(e)
	}
	root := t.TempDir()
	// Pure Display P3 red lies outside sRGB: converted, red stays saturated
	// while green and blue move off zero or red clips.
	p3 := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for i := 0; i < len(p3.Pix); i += 4 {
		copy(p3.Pix[i:], []byte{255, 0, 0, 255})
	}
	converted, profile, e := toSRGB(p3, systemProfile(t, "Display P3.icc"))
	if e != nil || !bytes.Equal(profile, srgb) {
		t.Fatal(e)
	}
	if c := converted.NRGBAAt(0, 0); c.R != 255 || c.A != 255 {
		t.Fatal("P3 red", c)
	}
	identity, _, e := toSRGB(p3, srgb)
	if e != nil || identity.NRGBAAt(0, 0) != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatal("sRGB to sRGB changed pixels", identity.NRGBAAt(0, 0), e)
	}
	adobe := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	copy(adobe.Pix, []byte{0, 128, 0, 255})
	moved, _, e := toSRGB(adobe, systemProfile(t, "AdobeRGB1998.icc"))
	if e != nil || moved.NRGBAAt(0, 0) == adobe.NRGBAAt(0, 0) {
		t.Fatal("Adobe RGB was not converted", e)
	}
	in := filepath.Join(root, "gray.jpg")
	grayJPEG(t, in, systemProfile(t, "Generic Gray Gamma 2.2 Profile.icc"))
	out := filepath.Join(root, "gray-out.jpg")
	if r := Encode(context.Background(), Job{Source: in, Destination: out}, DefaultOptions()); !r.Published {
		t.Fatal(r)
	}
	data, _ := os.ReadFile(out)
	if m, e := jpegMetadata(data); e != nil || !bytes.Equal(m.icc, srgb) {
		t.Fatal("output is not tagged sRGB", e)
	}
	untagged := filepath.Join(root, "plain.png")
	writePNG(t, untagged, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	if r := Encode(context.Background(), Job{Source: untagged, Destination: filepath.Join(root, "plain.jpg")}, DefaultOptions()); !r.Published {
		t.Fatal(r)
	}
	data, _ = os.ReadFile(filepath.Join(root, "plain.jpg"))
	if m, _ := jpegMetadata(data); !bytes.Equal(m.icc, srgb) {
		t.Fatal("untagged output is not tagged sRGB")
	}
}
