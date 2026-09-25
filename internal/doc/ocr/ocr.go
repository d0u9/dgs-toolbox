// Package ocr reads the text on a PDF's pages. A page with a text layer is
// read from it; a scanned page is rendered and recognised. On macOS this is
// PDFKit and the Vision framework, compiled in through cgo, so dgs stays one
// binary; elsewhere, and without cgo, Recognize refuses with ErrUnavailable.
//
// The text is a reading aid and a source of suggestions. It is never the
// truth about a document: nothing it says is written anywhere without a person
// accepting it.
package ocr

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// DefaultMaxPages is how many pages are read. Identity documents and bills
// say what matters on their first pages, and each recognised page costs about
// a second.
const DefaultMaxPages = 4

// ErrUnavailable is returned where recognition is not compiled in.
var ErrUnavailable = errors.New("text recognition needs macOS and a dgs built with cgo")

// Available reports whether Recognize can work in this build.
func Available() bool { return available }

// Source says where a page's text came from.
type Source string

const (
	// SourceText is the PDF's own text layer.
	SourceText Source = "text"
	// SourceRecognised is text recognised from the rendered page.
	SourceRecognised Source = "recognised"
)

// Box is where a line sits on its page, as fractions of the page's width and
// height, from the top left, the page turned the way it is shown.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// Line is one line of text and where it is.
type Line struct {
	Text string `json:"text"`
	Box  Box    `json:"box"`
}

// Page is one page's lines, in reading order.
type Page struct {
	Lines  []Line `json:"lines"`
	Source Source `json:"source"`
}

// Text is the page's lines, one to a line.
func (p Page) Text() string {
	parts := make([]string, len(p.Lines))
	for i, l := range p.Lines {
		parts[i] = l.Text
	}
	return strings.Join(parts, "\n")
}

// Result is the pages read, in order.
type Result struct {
	Pages []Page `json:"pages"`
}

// pageSeparator is what Text puts between pages.
const pageSeparator = "\n\n"

// Text is every page's text, pages separated by a blank line. Offsets into it
// are what Locate takes.
func (r Result) Text() string {
	parts := make([]string, len(r.Pages))
	for i, p := range r.Pages {
		parts[i] = p.Text()
	}
	return strings.Join(parts, pageSeparator)
}

// Locate is the page and line, counting from 0, that the byte at offset in
// Text falls in. ok is false for an offset on a separator or past the end.
func (r Result) Locate(offset int) (page, line int, ok bool) {
	at := 0
	for p, pg := range r.Pages {
		if p > 0 {
			at += len(pageSeparator)
		}
		for l, ln := range pg.Lines {
			if l > 0 {
				at++
			}
			if offset >= at && offset < at+len(ln.Text) {
				return p, l, true
			}
			at += len(ln.Text)
		}
	}
	return 0, 0, false
}

// Recognize reads up to maxPages pages of the PDF at path; zero or less uses
// DefaultMaxPages.
func Recognize(path string, maxPages int) (Result, error) {
	if maxPages <= 0 {
		maxPages = DefaultMaxPages
	}
	raw, err := recognize(path, maxPages)
	if err != nil {
		return Result{}, err
	}
	return decode(raw)
}

// rawLine is a line as the bridge reports it. Space "display" is Vision's:
// fractions of the page as shown, from the bottom left. Space "page" is the
// PDF's own: fractions of the unturned page, from the bottom left, with the
// page's rotation beside it.
type rawLine struct {
	Text   string  `json:"text"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	W      float64 `json:"w"`
	H      float64 `json:"h"`
	Space  string  `json:"space"`
	Rotate int     `json:"rotate"`
}

// placed turns a bridge line's box into a Box: from the top left of the page
// as shown.
func placed(raw rawLine) Box {
	top := Box{X: raw.X, Y: 1 - raw.Y - raw.H, W: raw.W, H: raw.H}
	if raw.Space != "page" {
		return top
	}
	switch ((raw.Rotate % 360) + 360) % 360 {
	case 90:
		return Box{X: 1 - top.Y - top.H, Y: top.X, W: top.H, H: top.W}
	case 180:
		return Box{X: 1 - top.X - top.W, Y: 1 - top.Y - top.H, W: top.W, H: top.H}
	case 270:
		return Box{X: top.Y, Y: 1 - top.X - top.W, W: top.H, H: top.W}
	}
	return top
}

// decode reads the bridge's one JSON answer: the pages, or an error.
func decode(raw []byte) (Result, error) {
	var answer struct {
		Pages []struct {
			Lines  []rawLine `json:"lines"`
			Source Source    `json:"source"`
		} `json:"pages"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return Result{}, fmt.Errorf("text recognition: %w", err)
	}
	if answer.Error != "" {
		return Result{}, errors.New(answer.Error)
	}
	result := Result{Pages: make([]Page, 0, len(answer.Pages))}
	for _, p := range answer.Pages {
		page := Page{Source: p.Source, Lines: make([]Line, 0, len(p.Lines))}
		for _, l := range p.Lines {
			if text := strings.TrimSpace(l.Text); text != "" {
				page.Lines = append(page.Lines, Line{Text: text, Box: placed(l)})
			}
		}
		result.Pages = append(result.Pages, page)
	}
	return result, nil
}
