// Package scanmeta reads everything derivable from a scan's own bytes: the page
// count, the page size, what produced it, when it was made, and the images
// inside it.
//
// It covers both kinds a Box holds. A scan is a PDF about 99% of the time and a
// camera image the rest, and the two answer the same questions from different
// places — a PDF from its catalogue and its image XObjects, an image from EXIF
// and its own pixels. One package returning one Info is what lets every caller
// above it stop caring which it was handed.
//
// The whole point is that this happens once. Hashing already reads every byte
// of the file over the network, so the metadata, the embedded images and the
// thumbnails all come out of that same read; coming back later for any of it
// means fetching the file from the NAS again.
//
// Nothing here writes a sidecar or decides anything. Values in, values out.
package scanmeta

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Kind is what the bytes turned out to be.
type Kind string

const (
	KindPDF   Kind = "pdf"
	KindImage Kind = "image"
)

// Info is everything a scan's bytes say about themselves.
//
// Every field is best-effort: a scanner that writes no Producer, a PDF with no
// CreationDate and a page of vector drawing with no image in it are all normal,
// and each leaves its field empty rather than failing the read. An empty field
// means "the file does not say", never "zero".
type Info struct {
	Kind Kind
	// Pages is 1 for an image.
	Pages int
	// PageSize is the name of the paper the first page matches — "A4",
	// "Letter" — or its measurements in millimetres when it matches none.
	// It is a prior, not a fact about content: one A4 page is a document, five
	// are a contract, something small is a stub.
	PageSize string
	// Width and Height are the first page in PDF points, which is what the
	// paper name is derived from and what a caller needs to match a size this
	// build has no name for.
	Width, Height float64
	// Producer names the scanner or the software. In practice it partitions a
	// Box into eras: changing scanners once separates thousands of files for
	// free.
	Producer string
	// CreatedAt is when the file says it was made, as it was written, in RFC
	// 3339 with its offset. It is more reliable than the file's modification
	// time, which copying and syncing have long since overwritten.
	CreatedAt string

	// Images are the embedded image streams in page order, already read. They
	// are the input to the second digest — the one that catches the same scan
	// rewrapped by other software — and to both thumbnail sizes.
	Images []Image
	// ImageError records why the images could not be read, for a file whose
	// metadata was fine. It is not a failure: such a scan is filed with
	// needs-render and no thumbnail, because an inbox that cannot be drained
	// is worse than a Box with a missing picture.
	ImageError string
}

// Image is one embedded image stream and what the container says about it.
//
// Resolution and colour mode record a judgement someone already made:
// monochrome at 200 dpi went through the feeder in a batch, colour at 600 dpi
// was placed on the glass deliberately.
type Image struct {
	// Rotate is the page's /Rotate, clockwise degrees: 0, 90, 180 or 270. A
	// scanner that stores a sideways picture and turns the page to show it
	// upright leaves the picture itself sideways, so drawing it means turning
	// it by this much. It says nothing about the bytes and takes no part in any
	// digest.
	Rotate int
	Page   int
	Width  int
	Height int
	// Bits is bits per component, 1 for a bitonal scan.
	Bits int
	// ColorSpace is the PDF name — DeviceGray, DeviceRGB, ICCBased — or the
	// image format's own description.
	ColorSpace string
	// Format is the encoding the bytes are in: "jpeg", "png", "tiff", or the
	// PDF filter when it is not a format an image decoder reads.
	Format string
	// Bytes is the stream exactly as the file stores it, still encoded. This is
	// what the image digest is taken over, and it is deliberately not a decoded
	// picture: a digest goes into a sidecar and names that scan forever, so it
	// may not depend on how one version of one library chose to re-encode
	// something. An upgrade that changed that would make every file in the Box
	// look new.
	Bytes []byte
	// Rendered is the same picture decoded into a form an image library reads,
	// which is the only thing a thumbnail can be drawn from. It is empty when
	// this build cannot decode the stream, which is what needs-render means.
	// Nothing stores it, so it is free to change between versions.
	Rendered []byte
	// RenderedFormat is what Rendered is encoded as — "png", "jpeg", "tiff".
	RenderedFormat string
}

// ColorMode is a coarse reading of an image: mono, gray or colour.
func (i Image) ColorMode() string {
	switch {
	case i.Bits == 1:
		return "mono"
	case strings.Contains(i.ColorSpace, "Gray"), strings.Contains(i.ColorSpace, "CalGray"):
		return "gray"
	case i.ColorSpace == "":
		return ""
	default:
		return "colour"
	}
}

// DPI is the image's resolution against a page of the given size in points, and
// whether it could be worked out at all. A page of no size, or an image of no
// pixels, has no resolution rather than a resolution of zero.
func (i Image) DPI(pageWidth, pageHeight float64) (int, bool) {
	if pageWidth <= 0 || pageHeight <= 0 || i.Width <= 0 || i.Height <= 0 {
		return 0, false
	}
	// A PDF point is 1/72 inch by definition.
	horizontal := float64(i.Width) / (pageWidth / 72)
	vertical := float64(i.Height) / (pageHeight / 72)
	average := (horizontal + vertical) / 2
	if average <= 0 {
		return 0, false
	}
	return int(average + 0.5), true
}

// paperTolerance is how far a page may sit from a named size and still be
// called by that name, in PDF points — about half a millimetre. Scanners round
// their page boxes, and a Box full of "209.9x296.9mm" instead of "A4" sorts
// into nothing.
const paperTolerance = 1.5

// PaperName is the name of the paper matching a page of width by height points,
// and whether one was found. Both orientations match the same name: a page
// turned on its side is still A4.
func PaperName(width, height float64) (string, bool) {
	names := make([]string, 0, len(types.PaperSize))
	for name := range types.PaperSize {
		names = append(names, name)
	}
	// The table holds several names for one size, so the answer is settled by
	// sorting rather than by map order: the same page must get the same name on
	// every run, or a Box sorted by paper size reshuffles itself.
	sort.Strings(names)
	for _, name := range names {
		dim := types.PaperSize[name]
		if dim == nil {
			continue
		}
		if near(width, dim.Width) && near(height, dim.Height) {
			return name, true
		}
		if near(width, dim.Height) && near(height, dim.Width) {
			return name, true
		}
	}
	return "", false
}

func near(a, b float64) bool {
	difference := a - b
	if difference < 0 {
		difference = -difference
	}
	return difference <= paperTolerance
}

// describeSize names the paper, or measures it in millimetres when no name
// fits. A size with no name is still worth recording: it groups a scanner's
// idiosyncratic output just as well.
func describeSize(width, height float64) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	if name, found := PaperName(width, height); found {
		return name
	}
	const mmPerPoint = 25.4 / 72
	return fmt.Sprintf("%.0fx%.0fmm", width*mmPerPoint, height*mmPerPoint)
}

// Read reads a scan of either kind from bytes already in memory.
//
// The kind is decided by the bytes rather than by the file's extension, because
// a Box takes files from scanners, phones and other people's mail, and the name
// on one of those is not evidence of anything.
func Read(data []byte) (Info, error) {
	if looksLikePDF(data) {
		return ReadPDFBytes(data)
	}
	return ReadImage(data)
}

// LooksLikePDF reports whether data carries the PDF header. It is exported
// because a caller that got an error back needs to know which kind of file it
// failed on: a PDF that will not parse is an incomplete scan, and anything else
// is simply not a scan.
func LooksLikePDF(data []byte) bool { return looksLikePDF(data) }

// Complete reports whether a PDF ends with %%EOF, and is meaningless for
// anything else.
//
// A truncated file, a bad copy and a damaged old backup all fail this, and the
// check is free. It is not a substitute for parsing: a file can end correctly
// and still be unreadable, which is why the caller checks both.
func Complete(data []byte) bool {
	const tail = 2048
	end := data
	if len(end) > tail {
		end = end[len(end)-tail:]
	}
	return bytes.Contains(end, []byte("%%EOF"))
}

// looksLikePDF reports whether data begins with the PDF header. The header is
// allowed a short run-up: files that went through a mail system sometimes carry
// a few bytes before it, and every PDF reader in existence accepts them.
func looksLikePDF(data []byte) bool {
	const runUp = 1024
	head := data
	if len(head) > runUp {
		head = head[:runUp]
	}
	return bytes.Contains(head, []byte("%PDF-"))
}
