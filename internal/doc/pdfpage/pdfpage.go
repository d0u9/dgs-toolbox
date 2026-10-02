// Package pdfpage draws one page of a PDF as a JPEG, as it is shown: text,
// drawings and pictures alike, turned by its /Rotate. It reads nothing from
// the page, so it is how a PDF made on a computer, which has no embedded
// scan to show, is looked at. It is compiled in on macOS with cgo, through
// Core Graphics and ImageIO; elsewhere, and without cgo, Render refuses with
// ErrUnavailable.
package pdfpage

import (
	"errors"
	"fmt"
)

// ErrUnavailable is returned where drawing is not compiled in.
var ErrUnavailable = errors.New("drawing PDF pages needs macOS and a dgs built with cgo")

// DefaultQuality is the JPEG quality pages are written at, 0 to 1.
const DefaultQuality = 0.85

// Available says whether Render can draw here.
func Available() bool { return available }

// Render draws page n of the PDF at path, counting from 1, with its longer
// side longSide pixels, as a JPEG of quality 0 to 1.
func Render(path string, n, longSide int, quality float64) ([]byte, error) {
	if n < 1 {
		return nil, fmt.Errorf("page numbers count from 1, not %d", n)
	}
	if longSide < 1 {
		return nil, fmt.Errorf("a page is at least 1 pixel, not %d", longSide)
	}
	return render(path, n, longSide, quality)
}

// Count is the number of pages of the PDF at path.
func Count(path string) (int, error) { return count(path) }
