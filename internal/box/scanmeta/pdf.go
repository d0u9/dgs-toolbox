package scanmeta

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ReadPDF reads a scan's metadata and its embedded images from rs.
//
// One parse answers everything. The page count, the page size, the Producer,
// the creation date and every image come off a single context, because the
// alternative — a call per question — reparses the file once per answer.
//
// A PDF that cannot be parsed at all is an error: it is not a scan and the
// caller has to say so. Everything softer than that — no Producer, no date, an
// image stream this build cannot pull out — comes back as an empty field or as
// Info.ImageError, because the inbox must stay drainable and a file held back
// for a missing thumbnail is a file nobody ever files.
func ReadPDF(rs io.ReadSeeker) (Info, error) {
	if _, err := rs.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	context, err := api.ReadValidateAndOptimize(rs, pdfConfiguration())
	if err != nil {
		return Info{}, fmt.Errorf("read pdf: %w", err)
	}
	info := Info{
		Kind:      KindPDF,
		Pages:     context.PageCount,
		Producer:  strings.TrimSpace(context.XRefTable.Producer),
		CreatedAt: pdfDate(context.XRefTable.CreationDate),
	}
	// The first page is the one described. A scan whose pages differ in size is
	// rare and is a document about its first page either way.
	if dims, err := context.XRefTable.PageDims(); err == nil && len(dims) > 0 {
		info.Width = dims[0].Width
		info.Height = dims[0].Height
		info.PageSize = describeSize(info.Width, info.Height)
	}
	images, err := readPDFImages(context)
	if err != nil {
		info.ImageError = err.Error()
		return info, nil
	}
	info.Images = images
	return info, nil
}

// pdfConfiguration is a reader's configuration: relaxed validation, and a
// resource it cannot handle is skipped rather than fatal. Scanners and mail
// clients produce PDFs a strict validator refuses, and refusing them here would
// put files a person can open in Preview out of reach of the Box.
func pdfConfiguration() *model.Configuration {
	config := model.NewDefaultConfiguration()
	config.ValidationMode = model.ValidationRelaxed
	config.UnsupportedResourcePolicy = model.UnsupportedResourceSkip
	return config
}

// readPDFImages returns one entry per image object, in page order.
//
// Each entry carries two different sets of bytes, and the difference is the
// whole reason this is not a call to api.ExtractImagesRaw:
//
//   - Bytes is the stream exactly as the file stores it, still encoded. It is
//     what the image digest is taken over. A digest goes into a sidecar and
//     identifies that scan forever, so it may not depend on how some version of
//     some library chose to re-encode a picture; an upgrade would otherwise
//     make every file in the Box look new.
//   - Rendered is what pdfcpu decoded the stream into, which is the only form a
//     thumbnail can be drawn from, and is free to change between versions
//     because nothing stores it.
//
// An object used on several pages is counted once. It is one picture in the
// file and digesting it twice would make a document that repeats a letterhead
// look unlike the same document scanned without one.
func readPDFImages(context *model.Context) ([]Image, error) {
	if context.Optimize == nil {
		return nil, nil
	}
	type placed struct {
		page  int
		objNr int
	}
	var order []placed
	seen := map[int]bool{}
	for page := 1; page <= context.PageCount; page++ {
		for _, objNr := range pdfcpu.ImageObjNrs(context, page) {
			if seen[objNr] {
				continue
			}
			seen[objNr] = true
			order = append(order, placed{page: page, objNr: objNr})
		}
	}
	// ImageObjNrs answers in object-number order within a page, which is an
	// accident of how the file was written. Sorting by page and then by object
	// number makes the sequence the digest is taken over the same on every run.
	sort.Slice(order, func(a, b int) bool {
		if order[a].page != order[b].page {
			return order[a].page < order[b].page
		}
		return order[a].objNr < order[b].objNr
	})

	var images []Image
	for _, entry := range order {
		object := context.Optimize.ImageObjects[entry.objNr]
		if object == nil || object.ImageDict == nil {
			continue
		}
		dictionary := object.ImageDict
		// The raw bytes are copied before anything else touches the dictionary:
		// rendering decodes the stream in place, and a digest taken afterwards
		// would be a digest of whatever that left behind.
		raw := append([]byte(nil), dictionary.Raw...)
		image := Image{
			Page:       entry.page,
			Rotate:     pageRotation(context, entry.page),
			Bits:       intEntry(dictionary, "BitsPerComponent"),
			Width:      intEntry(dictionary, "Width"),
			Height:     intEntry(dictionary, "Height"),
			ColorSpace: nameEntry(dictionary, "ColorSpace"),
			Format:     filterName(dictionary),
			Bytes:      raw,
		}
		if rendered, err := pdfcpu.ExtractImage(context, dictionary, false, "", entry.objNr, false); err == nil && rendered != nil {
			if data, err := io.ReadAll(rendered); err == nil {
				image.Rendered = data
				image.RenderedFormat = strings.ToLower(strings.TrimPrefix(rendered.FileType, "."))
			}
		}
		images = append(images, image)
	}
	return images, nil
}

// pageRotation is a page's /Rotate, inherited from the page tree where the
// page does not say, normalised to 0, 90, 180 or 270. A page that cannot be
// read is taken as unturned: the picture is still worth showing.
func pageRotation(context *model.Context, page int) int {
	_, _, inherited, err := context.PageDict(page, false)
	if err != nil || inherited == nil {
		return 0
	}
	return NormalRotation(inherited.Rotate)
}

// NormalRotation folds any multiple of 90 degrees into 0, 90, 180 or 270, and
// anything else to 0, which is what a PDF reader does with it.
func NormalRotation(degrees int) int {
	if degrees%90 != 0 {
		return 0
	}
	return ((degrees % 360) + 360) % 360
}

func intEntry(dictionary *types.StreamDict, key string) int {
	if value := dictionary.IntEntry(key); value != nil {
		return *value
	}
	return 0
}

func nameEntry(dictionary *types.StreamDict, key string) string {
	if value := dictionary.NameEntry(key); value != nil {
		return *value
	}
	return ""
}

func filterName(dictionary *types.StreamDict) string {
	names := make([]string, 0, len(dictionary.FilterPipeline))
	for _, filter := range dictionary.FilterPipeline {
		names = append(names, filter.Name)
	}
	return strings.Join(names, ",")
}

// pdfDate reads a PDF date string — D:20190311203000+09'00' — and writes it
// back as RFC 3339 with the offset it carried. The offset is kept as written
// rather than converted: the instant is the same either way, but the reading a
// person recognises is the local one.
//
// Text that is not a PDF date is returned trimmed rather than dropped. Some
// scanners write a plain ISO date there and it is still the best answer the
// file has.
func pdfDate(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if instant, ok := parsePDFDate(text); ok {
		return instant.Format(time.RFC3339)
	}
	return text
}

func parsePDFDate(text string) (time.Time, bool) {
	body := strings.TrimPrefix(text, "D:")
	// The zone is written as +09'00' or +09'00, and sometimes with no minutes
	// at all. Normalising it first leaves one layout per length to try.
	body = strings.ReplaceAll(body, "'", "")
	body = strings.TrimSuffix(body, "Z")
	layouts := []string{
		"20060102150405-0700",
		"20060102150405-07",
		"20060102150405",
		"200601021504",
		"2006010215",
		"20060102",
	}
	for _, layout := range layouts {
		if instant, err := time.Parse(layout, body); err == nil {
			return instant, true
		}
	}
	// A date already in RFC 3339 needs none of the above.
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if instant, err := time.Parse(layout, strings.TrimSpace(text)); err == nil {
			return instant, true
		}
	}
	return time.Time{}, false
}

// ReadPDFBytes is ReadPDF over bytes already in memory, which is how the one
// read works: the file is fetched from the NAS once and every digest, every
// field and both thumbnails come out of that same copy.
func ReadPDFBytes(data []byte) (Info, error) { return ReadPDF(bytes.NewReader(data)) }
