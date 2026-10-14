package scanmeta

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ErrNoPageImage means the page asked for carries no picture this build can
// decode: a vector page, a blank one, or a picture only shared from another.
var ErrNoPageImage = errors.New("page has no decodable image")

// Pages is a scan opened for paging through: parsed once, with each page's
// picture decoded only when that page is asked for.
//
// Read decodes every image in the file, which is what the digest and the first
// thumbnail need and far more than turning to one page does. A fifty-page
// document opened by Read decodes fifty pictures before the first is shown.
//
// A Pages is not safe for concurrent use: decoding touches the parsed file.
type Pages struct {
	context *model.Context
	// single is the whole picture of a file that is not a PDF: one page.
	single *Image
	count  int
}

// OpenPages parses data for paging. It decodes no PDF image.
func OpenPages(data []byte) (*Pages, error) {
	if !LooksLikePDF(data) {
		info, err := ReadImage(data)
		if err != nil {
			return nil, err
		}
		pages := &Pages{count: 1}
		if len(info.Images) > 0 {
			pages.single = &info.Images[0]
		}
		return pages, nil
	}
	context, err := api.ReadValidateAndOptimize(bytes.NewReader(data), pdfConfiguration())
	if err != nil {
		return nil, fmt.Errorf("read pdf: %w", err)
	}
	return &Pages{context: context, count: context.PageCount}, nil
}

// Count is the number of pages.
func (p *Pages) Count() int { return p.count }

// Page decodes the picture on the named page, counting from 1. Where a page
// holds several, the one with the lowest object number is taken, the same
// order Read lists them in.
func (p *Pages) Page(page int) (Image, error) {
	if page < 1 || page > p.count {
		return Image{}, fmt.Errorf("no page %d", page)
	}
	if p.context == nil {
		if p.single == nil || len(p.single.Rendered) == 0 {
			return Image{}, ErrNoPageImage
		}
		return *p.single, nil
	}
	if p.context.Optimize == nil {
		return Image{}, ErrNoPageImage
	}
	objects := pdfcpu.ImageObjNrs(p.context, page)
	sort.Ints(objects)
	box := pageBox(p.context, page)
	for _, objNr := range objects {
		object := p.context.Optimize.ImageObjects[objNr]
		if object == nil || object.ImageDict == nil || !wholePage(object.ImageDict.Dict, box) {
			continue
		}
		rendered, err := pdfcpu.ExtractImage(p.context, object.ImageDict, false, "", objNr, false)
		if err != nil || rendered == nil {
			continue
		}
		data, err := io.ReadAll(rendered)
		if err != nil || len(data) == 0 {
			continue
		}
		return Image{
			Page:           page,
			Rotate:         pageRotation(p.context, page),
			Rendered:       data,
			RenderedFormat: strings.ToLower(strings.TrimPrefix(rendered.FileType, ".")),
		}, nil
	}
	return Image{}, ErrNoPageImage
}

// pageBox is the page's width and height in points, or zeros when the page
// names none.
func pageBox(context *model.Context, page int) [2]float64 {
	_, _, inherited, err := context.PageDict(page, false)
	if err != nil || inherited == nil {
		return [2]float64{}
	}
	r := inherited.CropBox
	if r == nil {
		r = inherited.MediaBox
	}
	if r == nil {
		return [2]float64{}
	}
	return [2]float64{r.Width(), r.Height()}
}

// wholePage tells a scan of the page from a picture placed on it — a logo on
// a letter made on a computer. A scan has the page's shape, in either
// orientation, and at least ScanMinDPI across. A page with no size passes.
func wholePage(image types.Dict, box [2]float64) bool {
	if box[0] <= 0 || box[1] <= 0 {
		return true
	}
	w, h := image.IntEntry("Width"), image.IntEntry("Height")
	if w == nil || h == nil || *w <= 0 || *h <= 0 {
		return false
	}
	iw, ih := float64(*w), float64(*h)
	near := func(a, b float64) bool { return math.Abs(a-b) <= ScanShapeTolerance*b }
	page := box[0] / box[1]
	switch {
	case near(iw/ih, page):
		return iw >= box[0]/72*ScanMinDPI
	case near(ih/iw, page):
		return ih >= box[0]/72*ScanMinDPI
	}
	return false
}

// ScanShapeTolerance is how far, as a fraction, a picture's shape may be
// from its page's and still be a scan of it.
const ScanShapeTolerance = 0.1

// ScanMinDPI is the fewest pixels per inch across a page a scan of it has.
const ScanMinDPI = 50
