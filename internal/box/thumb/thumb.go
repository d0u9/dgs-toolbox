// Package thumb makes the two pictures a Box keeps beside its index: a small
// one for grids and a larger one for looking at a single scan.
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
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	// Registered so a stream pdfcpu decoded into any of these can be read back.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"dgs-toolbox/internal/box/scanmeta"
)

// The two sizes, in pixels on the longest side.
const (
	// GridSize is the grid thumbnail. A few KB, and never dropped.
	GridSize = 300
	// PreviewSize is for looking at one scan. It is dropped after
	// box.preview.keep days without use.
	PreviewSize = 1600
)

// Quality is the JPEG quality both sizes are written at. JPEG rather than PNG
// because the content is photographic and a PNG of a scanned page is several
// times larger for no visible gain.
const Quality = 82

// ErrNoImage is returned for a scan with no embedded image to draw from. It is
// what needs-render means, and it is an outcome rather than a failure: the scan
// is still filed, still described and still deduplicated, and only the picture
// is missing.
var ErrNoImage = errors.New("no embedded image to render")

// Pair is the two sizes for one scan, both JPEG.
type Pair struct {
	Grid    []byte
	Preview []byte
}

// Render makes both sizes from a scan's first page.
//
// The first page is the one shown, because that is the page a person recognises
// a document by. Rendering every page would multiply the cache by the page
// count to answer a question nobody asks of a grid.
func Render(info scanmeta.Info) (Pair, error) {
	source, err := firstPicture(info)
	if err != nil {
		return Pair{}, err
	}
	return both(source)
}

// firstPicture decodes the picture a document is recognised by.
//
// It is the first page's image where there is one, and otherwise the only
// image the file has, wherever it sits: a PDF whose single picture is on page
// three is still a scan with a thumbnail, and calling it needs-render would put
// a file a person can open in Preview out of reach of the grid.
func firstPicture(info scanmeta.Info) (image.Image, error) {
	picture, err := pagePicture(info, 1)
	if err == nil || !errors.Is(err, ErrNoImage) {
		return picture, err
	}
	if len(info.Images) != 1 || len(info.Images[0].Rendered) == 0 {
		return nil, ErrNoImage
	}
	only := info.Images[0]
	decoded, _, decodeErr := image.Decode(bytes.NewReader(only.Rendered))
	if decodeErr != nil {
		return nil, fmt.Errorf("decode page %d: %w", only.Page, decodeErr)
	}
	return decoded, nil
}

// RenderPage makes both sizes from one named page, counting from 1.
//
// It exists for the page view of a multi-page scan, which is asked for a page
// at a time and only for the scan being looked at. A page is drawn when it is
// asked for rather than with the rest: a fifty-page document costs fifty
// thumbnails nobody has looked at, and the first page is the only one the grid
// ever wants.
func RenderPage(info scanmeta.Info, page int) (Pair, error) {
	if page < 1 {
		page = 1
	}
	source, err := pagePicture(info, page)
	if err != nil {
		return Pair{}, err
	}
	return both(source)
}

// both encodes one decoded picture at both sizes.
func both(source image.Image) (Pair, error) {
	grid, err := encode(fit(source, GridSize))
	if err != nil {
		return Pair{}, err
	}
	preview, err := encode(fit(source, PreviewSize))
	if err != nil {
		return Pair{}, err
	}
	return Pair{Grid: grid, Preview: preview}, nil
}

// pagePicture decodes the named page's image. It reads the rendered form, not
// the raw stream: the raw bytes are what the digest is taken over and are often
// in an encoding no image library reads.
//
// A plain picture file carries no page number at all, so page 1 of one is the
// picture itself and any other page of it does not exist.
func pagePicture(info scanmeta.Info, page int) (image.Image, error) {
	for _, candidate := range info.Images {
		if len(candidate.Rendered) == 0 {
			continue
		}
		if candidate.Page != page && !(page == 1 && candidate.Page == 0) {
			continue
		}
		decoded, _, err := image.Decode(bytes.NewReader(candidate.Rendered))
		if err != nil {
			return nil, fmt.Errorf("decode page %d: %w", page, err)
		}
		return decoded, nil
	}
	return nil, ErrNoImage
}

// fit scales source so its longest side is at most size.
//
// An image already smaller than that is returned untouched. Enlarging a small
// scan produces a blurred picture of exactly the same information and a file
// several times the size, which is the wrong answer twice.
func fit(source image.Image, size int) image.Image {
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return source
	}
	if width <= size && height <= size {
		return source
	}
	if width >= height {
		height = height * size / width
		width = size
	} else {
		width = width * size / height
		height = size
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	// CatmullRom rather than a nearest-neighbour or bilinear scale: a page of
	// text shrunk by a factor of five aliases badly enough with the cheap
	// filters that a person cannot tell one letter from another in a grid.
	draw.CatmullRom.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

func encode(picture image.Image) ([]byte, error) {
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
