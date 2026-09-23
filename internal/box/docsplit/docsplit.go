// Package docsplit says what the documents inside one split scan are.
//
// A scan holding several documents is never cut: its sidecar lists which
// pages are which document, and this package turns that list into what each
// document is — the file's own fields plus whatever the document adds.
//
// Inheritance only adds. A field the file already has is every document's,
// and a document setting it again is refused rather than allowed to win,
// because two answers to one question in one sidecar is exactly the metadata
// nobody can trust. Tags are added to the file's, never removed from them.
// The file's type counts as unset while it is unsorted, which is what
// unsorted means.
package docsplit

import (
	"fmt"
	"strings"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/pagerange"
	"dgs-toolbox/internal/box/sidecar"
)

// Document is one document as it reads once inherited.
type Document struct {
	Pages       []pagerange.Range
	Type        string
	Description string
	Tags        []string
	EventDate   box.Date
	EventZone   string
	Total       money.Amount
}

// Coverage is how the file's pages are accounted for.
type Coverage struct {
	// Unassigned is every page in no document and not ignored. It is what a
	// split is warned about: a page nobody put anywhere is a page that
	// disappears from every filter.
	Unassigned []int
}

// Check reports the first thing wrong with a file's split: a range past the
// last page, or a document setting a field the file already has. A file with
// no documents has nothing to check. A file whose page count is not known —
// zero — skips the bounds check.
func Check(file sidecar.File) error {
	if len(file.Documents) == 0 {
		if len(file.IgnoredPages) > 0 {
			return fmt.Errorf("ignored pages without documents: the whole file is one document")
		}
		return nil
	}
	if file.Pages > 0 {
		if err := pagerange.Within(file.IgnoredPages, file.Pages); err != nil {
			return fmt.Errorf("ignored pages: %w", err)
		}
	}
	ignored := map[int]struct{}{}
	for _, page := range pagerange.Pages(file.IgnoredPages) {
		ignored[page] = struct{}{}
	}
	for position, document := range file.Documents {
		name := fmt.Sprintf("document %d", position+1)
		if len(document.Pages) == 0 {
			return fmt.Errorf("%s names no pages", name)
		}
		if file.Pages > 0 {
			if err := pagerange.Within(document.Pages, file.Pages); err != nil {
				return fmt.Errorf("%s: %w", name, err)
			}
		}
		// Ignored means no document's. A page that is both is two answers.
		for _, page := range pagerange.Pages(document.Pages) {
			if _, found := ignored[page]; found {
				return fmt.Errorf("%s includes page %d, which is ignored", name, page)
			}
		}
		if document.Type != "" {
			if _, known := doctype.Lookup(document.Type); !known {
				return fmt.Errorf("%s: no such type %q", name, document.Type)
			}
			if typed(file.Type) {
				return fmt.Errorf("%s sets a type, but the file is already %s", name, file.Type)
			}
		}
		if document.Description != "" && file.Description != "" {
			return fmt.Errorf("%s sets a description, but the file already has one", name)
		}
		if !document.EventDate.Zero() && !file.EventDate.Zero() {
			return fmt.Errorf("%s sets an event date, but the file already has one", name)
		}
		if document.EventZone != "" && file.EventZone != "" {
			return fmt.Errorf("%s sets a zone, but the file already has one", name)
		}
		if document.Total.Currency != "" && file.Total.Currency != "" {
			return fmt.Errorf("%s sets a total, but the file already has one", name)
		}
	}
	return nil
}

// Resolve lists the documents in a file. A file with no split is one
// document covering every page.
func Resolve(file sidecar.File) []Document {
	if len(file.Documents) == 0 {
		whole := Document{
			Type: file.Type, Description: file.Description, Tags: file.Tags,
			EventDate: file.EventDate, EventZone: file.EventZone, Total: file.Total,
		}
		if file.Pages > 0 {
			whole.Pages = []pagerange.Range{{From: 1, To: file.Pages}}
		}
		return []Document{whole}
	}
	out := make([]Document, 0, len(file.Documents))
	for _, own := range file.Documents {
		resolved := Document{
			Pages:       own.Pages,
			Type:        file.Type,
			Description: first(file.Description, own.Description),
			Tags:        addTags(file.Tags, own.Tags),
			EventDate:   file.EventDate,
			EventZone:   first(file.EventZone, own.EventZone),
			Total:       file.Total,
		}
		if !typed(file.Type) && own.Type != "" {
			resolved.Type = own.Type
		}
		if resolved.Type == "" {
			resolved.Type = doctype.Unsorted
		}
		if resolved.EventDate.Zero() {
			resolved.EventDate = own.EventDate
		}
		if resolved.Total.Currency == "" {
			resolved.Total = own.Total
		}
		out = append(out, resolved)
	}
	return out
}

// Cover says which pages of a split file are accounted for.
func Cover(file sidecar.File) Coverage {
	if len(file.Documents) == 0 || file.Pages <= 0 {
		return Coverage{}
	}
	ranges := [][]pagerange.Range{file.IgnoredPages}
	for _, document := range file.Documents {
		ranges = append(ranges, document.Pages)
	}
	return Coverage{Unassigned: pagerange.Missing(file.Pages, ranges...)}
}

func typed(name string) bool {
	name = strings.TrimSpace(name)
	return name != "" && name != doctype.Unsorted
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func addTags(inherited, own []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, list := range [][]string{inherited, own} {
		for _, tag := range list {
			if _, found := seen[tag]; found {
				continue
			}
			seen[tag] = struct{}{}
			out = append(out, tag)
		}
	}
	return out
}
