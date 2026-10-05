// Package imagepdf converts one image into one lossless PDF page. It never
// changes the input. The bytes are deterministic, so reimport finds the same PDF.
package imagepdf

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/photoencode"
)

// Supported reports the image formats accepted for document import.
func Supported(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".heic", ".heif", ".tif", ".tiff":
		return true
	}
	return false
}

// Convert retains the full pixel dimensions, applies orientation and composites
// transparency onto white. Native colour conversion and HEIC/TIFF decoding use
// macOS ImageIO/ColorSync; other builds return the decoder's unsupported error.
func Convert(ctx context.Context, name string, data []byte) ([]byte, error) {
	if !Supported(name) {
		return nil, fmt.Errorf("unsupported document image format")
	}
	o := photoencode.DefaultOptions()
	o.Size = 65535
	img, err := photoencode.DecodeImage(ctx, name, data, o)
	if err != nil {
		return nil, err
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	var pixels bytes.Buffer
	z := zlib.NewWriter(&pixels)
	row := make([]byte, w*3)
	for y := 0; y < h; y++ {
		if err := ctx.Err(); err != nil {
			z.Close()
			return nil, err
		}
		for x := 0; x < w; x++ {
			copy(row[x*3:x*3+3], img.Pix[y*img.Stride+x*4:y*img.Stride+x*4+3])
		}
		if _, err := z.Write(row); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	// 300 pixels per inch, with no imposed paper size or cropping.
	pw, ph := float64(w)*72/300, float64(h)*72/300
	content := fmt.Sprintf("q %.6f 0 0 %.6f 0 0 cm /Im0 Do Q", pw, ph)
	objs := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Kids [3 0 R] /Count 1 >>"),
		[]byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.6f %.6f] /Contents 4 0 R /Resources << /XObject << /Im0 5 0 R >> >> >>", pw, ph)),
		[]byte(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)),
		append([]byte(fmt.Sprintf("<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", w, h, pixels.Len())), append(pixels.Bytes(), []byte("\nendstream")...)...),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs))
	for i, obj := range objs {
		offsets[i] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n", i+1)
		out.Write(obj)
		out.WriteString("\nendobj\n")
	}
	at := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, offset := range offsets {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, at)
	return out.Bytes(), nil
}
