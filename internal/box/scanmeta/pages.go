package scanmeta

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
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
	for _, objNr := range objects {
		object := p.context.Optimize.ImageObjects[objNr]
		if object == nil || object.ImageDict == nil {
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
