// Package thumb renders a grid thumbnail and a larger preview of a scan.
//
// Both come from the page's embedded image stream, which is what a scanner puts
// there, so no page has to be rasterised and no C library has to be compiled
// in. That is a deliberate trade: a page that is not a single embedded image —
// a vector page, a composite — simply has no thumbnail and is marked
// needs-render. Accepting that gap is what keeps dgs one binary.
//
// Nothing here is truth. Every thumbnail is derivable from the scan's own bytes
// and lives in the discardable cache, so losing one costs a re-read and nothing
// else.
package thumb

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	// Registered so a stream pdfcpu decoded into any of these can be read back.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"dgs-toolbox/internal/imaging"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"dgs-toolbox/internal/doc/scanmeta"
)

// The two sizes, in pixels on the longest side.
const (
	// GridSize is the grid thumbnail. A few KB, and never dropped.
	GridSize = 300
	// PreviewSize is for looking at one scan. Its lifetime follows the caller's cache policy.
	PreviewSize = 1600
)

// Quality is the JPEG quality both sizes are written at. JPEG rather than PNG
// because the content is photographic and a PNG of a scanned page is several
// times larger for no visible gain.
const Quality = 82

// Pair is the two sizes for one scan, both JPEG.
type Pair struct {
	Grid    []byte
	Preview []byte
}

// RenderPicture makes both sizes from one page's picture, as
// scanmeta.Pages.Page returns it, turned as its page says.
func RenderPicture(picture scanmeta.Image) (Pair, error) {
	source, _, err := image.Decode(bytes.NewReader(picture.Rendered))
	if err != nil {
		return Pair{}, fmt.Errorf("decode page: %w", err)
	}
	return both(turning{source, picture.Rotate})
}

// RenderPictureAt draws one page's picture with its longest side at most
// size, turned as its page says. It is for looking closely: a page zoomed
// past the preview's size wants more of the scan's own pixels. A picture
// smaller than size is drawn at its own size, never enlarged.
func RenderPictureAt(picture scanmeta.Image, size int) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(picture.Rendered))
	if err != nil {
		return nil, fmt.Errorf("decode page: %w", err)
	}
	return encode(Turn(fitWith(source, size, draw.ApproxBiLinear), picture.Rotate))
}

// turning is a decoded picture with the turn its page asks for, carried to
// both so the turn is made on the small picture.
type turning struct {
	image.Image
	degrees int
}

// Turn rotates source clockwise by degrees, which is 0, 90, 180 or 270: a
// page's /Rotate, so the picture is drawn the way a PDF reader shows the page.
// Anything else returns source unchanged.
func Turn(source image.Image, degrees int) image.Image {
	switch degrees {
	case 90:
		return imaging.Orient(source, 6)
	case 180:
		return imaging.Orient(source, 3)
	case 270:
		return imaging.Orient(source, 8)
	}
	return source
}

// both encodes one decoded picture at both sizes.
//
// The grid thumbnail is drawn from the preview, not from the scan: a scan is
// several thousand pixels a side, and scaling it twice is most of the cost of
// drawing a page.
func both(source image.Image) (Pair, error) {
	// A page's turn is applied to the preview, not the scan: turning is a pixel
	// at a time, and the preview has a tenth of the scan's pixels.
	degrees := 0
	if turn, ok := source.(turning); ok {
		source, degrees = turn.Image, turn.degrees
	}
	large := Turn(fitWith(source, PreviewSize, draw.ApproxBiLinear), degrees)
	shape := source.Bounds()
	if degrees == 90 || degrees == 270 {
		shape = image.Rect(0, 0, shape.Dy(), shape.Dx())
	}
	preview, err := encode(large)
	if err != nil {
		return Pair{}, err
	}
	grid, err := encode(scaleTo(large, shape, GridSize, draw.CatmullRom))
	if err != nil {
		return Pair{}, err
	}
	return Pair{Grid: grid, Preview: preview}, nil
}

// fitWith is fit with the scaler named. The preview takes the cheaper
// ApproxBiLinear: it shrinks a scan by two or three times, where the cheap
// filter still reads cleanly, and it is several times faster than CatmullRom
// on a picture that size.
func fitWith(source image.Image, size int, scaler draw.Scaler) image.Image {
	return scaleTo(source, source.Bounds(), size, scaler)
}

// scaleTo draws source at the size shape would fit into, so a picture drawn
// from an already shrunk copy keeps the original's proportions exactly.
//
// The grid keeps CatmullRom: a page of text shrunk by a factor of five
// aliases badly enough with the cheap filters that a person cannot tell one
// letter from another in a grid.
func scaleTo(source image.Image, shape image.Rectangle, size int, scaler draw.Scaler) image.Image {
	return imaging.FitShape(source, shape, size, scaler)
}

func encode(picture image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
