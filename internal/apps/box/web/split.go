package web

import (
	"encoding/json"
	"fmt"
	"strings"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/docsplit"
	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/pagerange"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/box/tag"
)

// SplitDocument is one document inside a split scan, as the pages see it:
// what the document itself says, not what it inherits. Pages is the text the
// sidecar stores, "1-3,6".
type SplitDocument struct {
	Pages       string   `json:"pages"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	EventDate   string   `json:"eventDate"`
	EventZone   string   `json:"eventZone"`
	Total       string   `json:"total"`
}

// applySplit folds a new split into scan. Every value is read and written
// back in its canonical form, so what the page shows after a save is what the
// sidecar holds.
func applySplit(scan Scan, documents *[]SplitDocument, ignored *string, defaultCurrency string) (Scan, error) {
	if ignored != nil {
		ranges, err := pagerange.Parse(*ignored)
		if err != nil {
			return Scan{}, err
		}
		scan.IgnoredPages = pagerange.Format(ranges)
	}
	if documents == nil {
		return scan, nil
	}
	out := make([]SplitDocument, 0, len(*documents))
	for position, document := range *documents {
		name := fmt.Sprintf("document %d", position+1)
		ranges, err := pagerange.Parse(document.Pages)
		if err != nil {
			return Scan{}, fmt.Errorf("%s: %w", name, err)
		}
		clean := SplitDocument{
			Pages:       pagerange.Format(ranges),
			Type:        strings.TrimSpace(document.Type),
			Description: strings.TrimSpace(document.Description),
			Tags:        tag.List(document.Tags),
		}
		if clean.Type != "" {
			if _, known := doctype.Lookup(clean.Type); !known {
				return Scan{}, fmt.Errorf("%s: no such type %q", name, clean.Type)
			}
		}
		if trimmed := strings.TrimSpace(document.EventDate); trimmed != "" {
			date, err := box.ParseDate(trimmed)
			if err != nil {
				return Scan{}, fmt.Errorf("%s: %w", name, err)
			}
			clean.EventDate = date.String()
		}
		if trimmed := strings.TrimSpace(document.EventZone); trimmed != "" {
			if _, err := box.LoadZone(trimmed); err != nil {
				return Scan{}, fmt.Errorf("%s: %w", name, err)
			}
			clean.EventZone = trimmed
		}
		if trimmed := strings.TrimSpace(document.Total); trimmed != "" {
			amount, err := money.Parse(trimmed, defaultCurrency)
			if err != nil {
				return Scan{}, fmt.Errorf("%s: %w", name, err)
			}
			clean.Total = amount.String()
		}
		out = append(out, clean)
	}
	scan.Documents = out
	// Saying where the documents are is what needs_split was waiting for.
	if len(out) > 0 {
		scan.NeedsSplit = false
	}
	return scan, nil
}

// checkSplit refuses a split the sidecar could not hold: a page past the end,
// or a document setting what the file already says.
func checkSplit(scan Scan) error {
	record, err := recordFromScan(sidecar.File{Pages: scan.Pages}, scan)
	if err != nil {
		return err
	}
	return docsplit.Check(record)
}

// splitToRecord is the sidecar half of recordFromScan.
func splitToRecord(scan Scan) ([]sidecar.Document, []pagerange.Range, error) {
	ignored, err := pagerange.Parse(scan.IgnoredPages)
	if err != nil {
		return nil, nil, err
	}
	var documents []sidecar.Document
	for _, view := range scan.Documents {
		document := sidecar.Document{
			Type: view.Type, Description: view.Description, Tags: view.Tags, EventZone: view.EventZone,
		}
		if document.Pages, err = pagerange.Parse(view.Pages); err != nil {
			return nil, nil, err
		}
		if document.EventDate, err = parseOptionalDate(view.EventDate); err != nil {
			return nil, nil, err
		}
		if strings.TrimSpace(view.Total) != "" {
			if document.Total, err = money.Parse(view.Total, ""); err != nil {
				return nil, nil, err
			}
		}
		documents = append(documents, document)
	}
	return documents, ignored, nil
}

// splitFromRecord is the page half of scanFromEntry.
func splitFromRecord(record sidecar.File) ([]SplitDocument, string) {
	var out []SplitDocument
	for _, document := range record.Documents {
		view := SplitDocument{
			Pages:       pagerange.Format(document.Pages),
			Type:        document.Type,
			Description: document.Description,
			Tags:        document.Tags,
			EventDate:   document.EventDate.String(),
			EventZone:   document.EventZone,
		}
		if document.Total.Valid() {
			view.Total = document.Total.String()
		}
		out = append(out, view)
	}
	return out, pagerange.Format(record.IgnoredPages)
}

// unassigned is the pages of a split scan that are in no document and not
// ignored, written as ranges. It is computed so the page warns about a
// forgotten page without working it out itself.
func unassigned(scan Scan) string {
	if len(scan.Documents) == 0 || scan.Pages <= 0 {
		return ""
	}
	ignored, err := pagerange.Parse(scan.IgnoredPages)
	if err != nil {
		return ""
	}
	ranges := [][]pagerange.Range{ignored}
	for _, document := range scan.Documents {
		parsed, err := pagerange.Parse(document.Pages)
		if err != nil {
			return ""
		}
		ranges = append(ranges, parsed)
	}
	return pagerange.Format(pagerange.Compact(pagerange.Missing(scan.Pages, ranges...)))
}

// splitText is a split as one line of the log. The whole list is written,
// because a split is edited as one list and a history of it piece by piece
// would not say what it was.
func splitText(documents []SplitDocument) string {
	if len(documents) == 0 {
		return ""
	}
	data, err := json.Marshal(documents)
	if err != nil {
		return ""
	}
	return string(data)
}
