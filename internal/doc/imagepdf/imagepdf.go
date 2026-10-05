// Package imagepdf turns images into one PDF, a page per image, in order.
// It never changes an input, and loses nothing: a JPEG is embedded as its
// own bytes, every other image as its pixels, losslessly compressed. The
// same inputs give the same bytes on the same converter and system, so a
// reimport there finds the same PDF.
package imagepdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // DecodeConfig reads a JPEG's size and colour model
	"strings"

	"dgs-toolbox/internal/doc/docfile"
	"dgs-toolbox/internal/photoencode"
)

// Supported reports the image formats accepted for document import.
func Supported(name string) bool { return docfile.IsImage(name) }

// PixelsPerInch sizes each page: an image's pixels at 300 per inch, with no
// imposed paper size or cropping.
const PixelsPerInch = 300

// Input is one image: the name it is known by, whose extension chooses its
// decoder, and its bytes.
type Input struct {
	Name string
	Data []byte
}

// Convert writes one image as a one-page PDF.
func Convert(ctx context.Context, name string, data []byte) ([]byte, error) {
	return ConvertAll(ctx, []Input{{Name: name, Data: data}})
}

// ConvertAll writes images as one PDF, a page per image in the order given.
// A JPEG whose colour needs no conversion is embedded unchanged, its
// orientation applied by the page drawing it turned; its ICC profile, if
// any, goes with it. Any other image is decoded, oriented, converted to sRGB
// and composited onto white, and its 8-bit pixels kept losslessly. HEIC/HEIF,
// TIFF and colour conversion use macOS ImageIO/ColorSync; other builds
// return the decoder's unsupported error.
func ConvertAll(ctx context.Context, inputs []Input) ([]byte, error) {
	if len(inputs) == 0 {
		return nil, errors.New("no image to convert")
	}
	w := writer{}
	w.add(nil) // 1: catalog, written last
	w.add(nil) // 2: pages, written last
	var kids []string
	for _, in := range inputs {
		if !Supported(in.Name) {
			return nil, fmt.Errorf("%s: unsupported document image format", in.Name)
		}
		p, err := page(ctx, in)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", in.Name, err)
		}
		kids = append(kids, fmt.Sprintf("%d 0 R", w.drawn(p)))
	}
	w.set(1, []byte("<< /Type /Catalog /Pages 2 0 R >>"))
	w.set(2, []byte(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(kids))))
	return w.bytes(), nil
}

// picture is one image ready to draw: its stored size in pixels, its image
// dictionary's entries and stream, an ICC profile stream when it has one,
// and the EXIF orientation the page applies.
type picture struct {
	width, height int
	entries       string
	stream        []byte
	icc           []byte
	components    int
	orientation   int
}

func page(ctx context.Context, in Input) (picture, error) {
	if err := ctx.Err(); err != nil {
		return picture{}, err
	}
	if p, ok := jpegAsIs(in.Data); ok {
		return p, nil
	}
	return pixels(ctx, in)
}

// jpegAsIs is a JPEG's bytes as a DCTDecode image, when they can be shown
// unchanged: a baseline or progressive JPEG in gray or YCbCr, with no
// profile or one of its own colour space. CMYK and anything else is drawn
// from its pixels instead.
func jpegAsIs(data []byte) (picture, bool) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "jpeg" || cfg.Width <= 0 || cfg.Height <= 0 ||
		int64(cfg.Width)*int64(cfg.Height) > photoencode.DefaultMaxPixels {
		return picture{}, false
	}
	components := 0
	switch cfg.ColorModel {
	case color.GrayModel:
		components = 1
	case color.YCbCrModel:
		components = 3
	default:
		return picture{}, false
	}
	orientation, icc, err := photoencode.JPEGLayout(data, components == 1)
	if err != nil || (len(icc) > 0 && iccComponents(icc) != components) {
		return picture{}, false
	}
	return picture{width: cfg.Width, height: cfg.Height, entries: "/BitsPerComponent 8 /Filter /DCTDecode",
		stream: data, icc: icc, components: components, orientation: orientation}, true
}

// iccComponents is how many colour components a profile describes: 1 for
// GRAY, 3 for RGB, 0 for anything else.
func iccComponents(icc []byte) int {
	if len(icc) < 20 {
		return 0
	}
	switch string(icc[16:20]) {
	case "GRAY":
		return 1
	case "RGB ":
		return 3
	}
	return 0
}

// pixels decodes an image, oriented, in sRGB on white, into a lossless
// FlateDecode image.
func pixels(ctx context.Context, in Input) (picture, error) {
	o := photoencode.DefaultOptions()
	o.Size = 65535
	img, err := photoencode.DecodeImage(ctx, in.Name, in.Data, o)
	if err != nil {
		return picture{}, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var out bytes.Buffer
	z := zlib.NewWriter(&out)
	row := make([]byte, w*3)
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			z.Close()
			return picture{}, err
		}
		for x := 0; x < w; x++ {
			copy(row[x*3:x*3+3], img.Pix[y*img.Stride+x*4:y*img.Stride+x*4+3])
		}
		if _, err := z.Write(row); err != nil {
			return picture{}, err
		}
	}
	if err := z.Close(); err != nil {
		return picture{}, err
	}
	return picture{width: w, height: h, entries: "/BitsPerComponent 8 /Filter /FlateDecode",
		stream: out.Bytes(), components: 3, orientation: 1}, nil
}

// placement is the page size, in points, and the matrix drawing the image's
// unit square on it, so its EXIF orientation shows upright. a and b are the
// image's stored width and height in points; orientations 5–8 turn it a
// quarter, swapping the page's sides.
func placement(width, height, orientation int) (pw, ph float64, m [6]float64) {
	a, b := float64(width)*72/PixelsPerInch, float64(height)*72/PixelsPerInch
	switch orientation {
	case 2:
		return a, b, [6]float64{-a, 0, 0, b, a, 0}
	case 3:
		return a, b, [6]float64{-a, 0, 0, -b, a, b}
	case 4:
		return a, b, [6]float64{a, 0, 0, -b, 0, b}
	case 5:
		return b, a, [6]float64{0, -a, -b, 0, b, a}
	case 6:
		return b, a, [6]float64{0, -a, b, 0, 0, a}
	case 7:
		return b, a, [6]float64{0, a, b, 0, 0, 0}
	case 8:
		return b, a, [6]float64{0, a, -b, 0, b, 0}
	}
	return a, b, [6]float64{a, 0, 0, b, 0, 0}
}

// writer collects numbered objects and writes them as one PDF with its
// cross-reference table.
type writer struct{ objs [][]byte }

func (w *writer) add(obj []byte) int    { w.objs = append(w.objs, obj); return len(w.objs) }
func (w *writer) set(n int, obj []byte) { w.objs[n-1] = obj }

func stream(dict string, data []byte) []byte {
	head := fmt.Sprintf("<< %s /Length %d >>\nstream\n", dict, len(data))
	return append(append([]byte(head), data...), "\nendstream"...)
}

// drawn adds a picture's image, its profile, and a page drawing it; it
// returns the page's object number.
func (w *writer) drawn(p picture) int {
	space := map[int]string{1: "/DeviceGray", 3: "/DeviceRGB"}[p.components]
	if len(p.icc) > 0 {
		space = fmt.Sprintf("[/ICCBased %d 0 R]", w.add(stream(fmt.Sprintf("/N %d /Alternate %s", p.components, space), p.icc)))
	}
	img := w.add(stream(fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s %s", p.width, p.height, space, p.entries), p.stream))
	pw, ph, m := placement(p.width, p.height, p.orientation)
	content := w.add(stream("", []byte(fmt.Sprintf("q %s cm /Im0 Do Q", numbers(m[:])))))
	return w.add([]byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %s] /Contents %d 0 R /Resources << /XObject << /Im0 %d 0 R >> >> >>",
		numbers([]float64{pw, ph}), content, img)))
}

func numbers(values []float64) string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = fmt.Sprintf("%.6f", v)
	}
	return strings.Join(out, " ")
}

func (w *writer) bytes() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(w.objs))
	for i, obj := range w.objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(obj)
		out.WriteString("\nendobj\n")
	}
	at := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(w.objs)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(w.objs)+1, at)
	return out.Bytes()
}
