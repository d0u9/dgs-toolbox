package imaging

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"golang.org/x/image/draw"
)

func jpegBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			picture.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, nil); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// withOrientation splices an APP1 EXIF segment carrying one Orientation entry
// in after the start-of-image marker, in the byte order given.
func withOrientation(data []byte, order binary.ByteOrder, orientation uint16) []byte {
	tiff := make([]byte, 8+2+12+4)
	if order == binary.LittleEndian {
		copy(tiff, "II")
	} else {
		copy(tiff, "MM")
	}
	order.PutUint16(tiff[2:], 42)
	order.PutUint32(tiff[4:], 8)
	order.PutUint16(tiff[8:], 1)
	order.PutUint16(tiff[10:], 0x0112)
	order.PutUint16(tiff[12:], 3)
	order.PutUint32(tiff[14:], 1)
	order.PutUint16(tiff[18:], orientation)
	segment := append([]byte("Exif\x00\x00"), tiff...)
	header := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(header[2:], uint16(len(segment)+2))
	out := append([]byte{}, data[:2]...)
	out = append(out, header...)
	out = append(out, segment...)
	return append(out, data[2:]...)
}

func decodedSize(t *testing.T, data []byte) (int, int) {
	t.Helper()
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if format != "jpeg" {
		t.Fatalf("format = %s", format)
	}
	return config.Width, config.Height
}

func TestOrientationIsReadInEitherByteOrder(t *testing.T) {
	plain := jpegBytes(t, 8, 4)
	if got := JPEGOrientation(plain); got != 1 {
		t.Fatalf("no EXIF read as %d", got)
	}
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		if got := JPEGOrientation(withOrientation(plain, order, 6)); got != 6 {
			t.Fatalf("%v: orientation = %d, want 6", order, got)
		}
	}
	if got := JPEGOrientation(withOrientation(plain, binary.BigEndian, 9)); got != 1 {
		t.Fatalf("out of range read as %d", got)
	}
}

func TestOrientMovesEachCornerWhereItBelongs(t *testing.T) {
	// A 3x2 picture with its top-left pixel marked: where that pixel lands says
	// which transform was drawn.
	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	mark := color.RGBA{255, 0, 0, 255}
	source.Set(0, 0, mark)
	cases := map[int]image.Point{
		1: {0, 0}, 2: {2, 0}, 3: {2, 1}, 4: {0, 1},
		5: {0, 0}, 6: {1, 0}, 7: {1, 2}, 8: {0, 2},
	}
	for orientation, want := range cases {
		turned := Orient(source, orientation)
		size := turned.Bounds().Size()
		if orientation >= 5 && size != (image.Point{2, 3}) {
			t.Fatalf("%d: size = %v, want 2x3", orientation, size)
		}
		if got := color.RGBAModel.Convert(turned.At(want.X, want.Y)); got != mark {
			t.Fatalf("%d: mark not at %v", orientation, want)
		}
	}
}

func TestFitShrinksTheLongSideAndNeverEnlarges(t *testing.T) {
	wide := image.NewRGBA(image.Rect(0, 0, 400, 100))
	if got := Fit(wide, 200, draw.ApproxBiLinear).Bounds().Size(); got != (image.Point{200, 50}) {
		t.Fatalf("wide = %v", got)
	}
	tall := image.NewRGBA(image.Rect(0, 0, 100, 400))
	if got := Fit(tall, 200, draw.ApproxBiLinear).Bounds().Size(); got != (image.Point{50, 200}) {
		t.Fatalf("tall = %v", got)
	}
	if got := Fit(wide, 1000, draw.ApproxBiLinear); got != image.Image(wide) {
		t.Fatal("a small picture was redrawn")
	}
}

func TestTranscodeShrinksAndTurnsUpright(t *testing.T) {
	data := withOrientation(jpegBytes(t, 400, 200), binary.LittleEndian, 6)
	out, err := TranscodeJPEG(data, JPEGOptions{MaxSide: 100, Quality: 70})
	if err != nil {
		t.Fatal(err)
	}
	if width, height := decodedSize(t, out); width != 50 || height != 100 {
		t.Fatalf("size = %dx%d, want 50x100", width, height)
	}
	if JPEGOrientation(out) != 1 {
		t.Fatal("the result still asks to be turned")
	}
}

func TestTranscodeRefusesWhatIsNotAJPEG(t *testing.T) {
	var out bytes.Buffer
	if err := png.Encode(&out, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	if _, err := TranscodeJPEG(out.Bytes(), JPEGOptions{}); !errors.Is(err, ErrNotJPEG) {
		t.Fatalf("err = %v, want ErrNotJPEG", err)
	}
	if _, err := TranscodeJPEG([]byte("nothing"), JPEGOptions{}); !errors.Is(err, ErrNotJPEG) {
		t.Fatalf("err = %v, want ErrNotJPEG", err)
	}
}
