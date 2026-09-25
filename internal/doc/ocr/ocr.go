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

// Page is one page's text, lines in reading order.
type Page struct {
	Text   string `json:"text"`
	Source Source `json:"source"`
}

// Result is the pages read, in order.
type Result struct {
	Pages []Page `json:"pages"`
}

// Text is every page's text, pages separated by a blank line.
func (r Result) Text() string {
	parts := make([]string, 0, len(r.Pages))
	for _, p := range r.Pages {
		if t := strings.TrimSpace(p.Text); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n\n")
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

// decode reads the bridge's one JSON answer: the pages, or an error.
func decode(raw []byte) (Result, error) {
	var answer struct {
		Pages []Page `json:"pages"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return Result{}, fmt.Errorf("text recognition: %w", err)
	}
	if answer.Error != "" {
		return Result{}, errors.New(answer.Error)
	}
	return Result{Pages: answer.Pages}, nil
}
