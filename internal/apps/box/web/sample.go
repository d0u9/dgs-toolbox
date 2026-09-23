package web

import (
	"context"
	"fmt"
	"html"
	"strings"
)

// The stand-in Box. These scans do not exist: they are here so the pages can
// be driven, looked at and argued about before the engine is written, and the
// pages say so in a banner. Everything about them is the kind of thing a
// scanner really produces — a batch scanned in one sitting, a duplicate, a
// multi-document PDF, a page whose image cannot be extracted.

func samplePending() []Scan {
	return []Scan{
		{
			Digest: "a1b2c3d4", Filename: "Scan_0012.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-20T10:02:11+10:00", Colour: "colour", DPI: 600,
			Type: "unsorted",
		},
		{
			Digest: "b2c3d4e5", Filename: "Scan_0013.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-20T10:02:48+10:00", Colour: "colour", DPI: 600,
			Type: "unsorted",
		},
		{
			Digest: "c3d4e5f6", Filename: "Scan_0014.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A5", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-20T10:03:29+10:00", Colour: "mono", DPI: 200,
			Type: "unsorted",
		},
		{
			Digest: "d4e5f6a7", Filename: "Scan_0015.pdf", Kind: "pdf",
			Pages: 5, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-20T10:05:02+10:00", Colour: "mono", DPI: 200,
			Type: "unsorted", NeedsSplit: true,
		},
		{
			Digest: "e5f6a7b8", Filename: "Scan_0016.pdf", Kind: "pdf",
			Pages: 3, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-20T10:07:40+10:00", Colour: "mono", DPI: 300,
			Type: "unsorted",
		},
		{
			Digest: "f6a7b8c9", Filename: "IMG_4821.jpeg", Kind: "image",
			Pages: 1, PageSize: "4032×3024", Producer: "iPhone",
			ScannedAt: "2026-09-20T18:44:03+10:00", Colour: "colour", DPI: 72,
			Type: "unsorted",
		},
		{
			Digest: "07b8c9d0", Filename: "Scan_0017.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-21T09:15:22+10:00", Colour: "colour", DPI: 600,
			Type: "unsorted", DuplicateOf: "11aa22bb",
		},
		{
			Digest: "18c9d0e1", Filename: "Scan_0018.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2026-09-21T09:16:01+10:00", Colour: "colour", DPI: 600,
			Type: "unsorted", DuplicateOf: "33cc44dd", TrashedAt: "2026-03-04T19:20:00+11:00",
		},
		{
			Digest: "29d0e1f2", Filename: "Scan_0019.pdf", Kind: "pdf",
			Pages: 2, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2026-09-21T09:22:10+10:00", Colour: "colour", DPI: 300,
			Type: "unsorted", NeedsRender: true,
		},
		{
			Digest: "3ae1f2a3", Filename: "Scan_0020.pdf", Kind: "pdf",
			Pages: 0, Producer: "", ScannedAt: "2026-09-21T09:30:00+10:00",
			Type: "unsorted", Incomplete: true,
		},
	}
}

func sampleFiled() []Scan {
	return []Scan{
		{
			Digest: "11aa22bb", Filename: "scan-0004-11aa22bb.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2019-03-11T20:31:02+09:00", IngestedAt: "2019-03-14T19:04:00+11:00",
			Colour: "colour", DPI: 600,
			Type: "travel", Reviewed: true, Description: "Haneda → Sydney boarding pass",
			EventDate: "2019-03-11", EventZone: "Asia/Tokyo", Tags: []string{"japan-2019"},
		},
		{
			Digest: "22bb33cc", Filename: "scan-0005-22bb33cc.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A5", Producer: "ScanSnap Manager",
			ScannedAt: "2019-03-11T20:33:40+09:00", IngestedAt: "2019-03-14T19:04:02+11:00",
			Colour: "colour", DPI: 600,
			Type: "ticket", Reviewed: true, Description: "Ghibli Museum",
			EventDate: "2019-03-09", EventZone: "Asia/Tokyo", Total: "JPY 1000",
			ExpiryCleared: true, Tags: []string{"japan-2019", "keep"},
		},
		{
			Digest: "33cc44dd", Filename: "scan-0006-33cc44dd.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "ScanSnap Manager",
			ScannedAt: "2024-02-03T11:12:00+11:00", IngestedAt: "2024-02-03T11:30:00+11:00",
			Colour: "mono", DPI: 200,
			Type: "receipt", Reviewed: true, Description: "Laptop stand",
			EventDate: "2024-02-01", EventZone: "Australia/Sydney", Total: "AUD 51.25",
		},
		{
			Digest: "44dd55ee", Filename: "scan-0007-44dd55ee.pdf", Kind: "pdf",
			Pages: 6, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2025-06-30T15:02:00+10:00", IngestedAt: "2025-06-30T15:40:00+10:00",
			Colour: "mono", DPI: 300,
			Type: "insurance", Reviewed: true, Description: "Car — comprehensive",
			EventDate: "2025-07-01", EventZone: "Australia/Sydney", Total: "AUD 1240.50",
		},
		{
			Digest: "55ee66ff", Filename: "scan-0008-55ee66ff.pdf", Kind: "pdf",
			Pages: 2, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2021-08-14T09:00:00+10:00", IngestedAt: "2021-08-14T09:20:00+10:00",
			Colour: "colour", DPI: 600,
			Type: "identity", Reviewed: true, Description: "Passport — main pages",
		},
		{
			Digest: "66ff7700", Filename: "scan-0009-66ff7700.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A6", Producer: "ScanSnap Manager",
			ScannedAt: "2023-11-02T20:00:00+11:00", IngestedAt: "2023-11-03T08:00:00+11:00",
			Colour: "colour", DPI: 600,
			Type: "invite", Reviewed: true, Description: "Wedding — Anna and Tom",
			EventDate: "2023-12-16", EventZone: "Australia/Sydney",
		},
		{
			Digest: "778800aa", Filename: "scan-0010-778800aa.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A5", Producer: "ScanSnap Manager",
			ScannedAt: "2019-03-12T21:10:00+09:00", IngestedAt: "2019-03-14T19:05:00+11:00",
			Colour: "colour", DPI: 600,
			Type: "ephemera", Reviewed: true, Description: "Shinkansen leaflet",
			Tags: []string{"japan-2019"},
		},
		{
			Digest: "8800aabb", Filename: "scan-0011-8800aabb.pdf", Kind: "pdf",
			Pages: 4, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2020-05-05T14:00:00+10:00", IngestedAt: "2020-05-05T14:30:00+10:00",
			Colour: "mono", DPI: 300,
			Type: "contract", Reviewed: true, Description: "Lease — Erskineville",
			EventDate: "2020-05-01", EventZone: "Australia/Sydney",
		},
		{
			Digest: "99aabbcc", Filename: "scan-0012-99aabbcc.pdf", Kind: "pdf",
			Pages: 2, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2020-05-05T14:03:00+10:00", IngestedAt: "2020-05-05T14:30:02+10:00",
			Colour: "mono", DPI: 300,
			Type: "contract", Reviewed: true, Description: "Lease — Erskineville (pages 5–6)",
			EventDate: "2020-05-01", EventZone: "Australia/Sydney",
		},
		{
			Digest: "aabbccdd", Filename: "scan-0013-aabbccdd.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "HP ScanJet",
			ScannedAt: "2016-01-09T10:00:00+11:00", IngestedAt: "2016-01-09T10:30:00+11:00",
			Colour: "mono", DPI: 200,
			Type: "unsorted",
		},
		{
			Digest: "bbccddee", Filename: "scan-0014-bbccddee.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "HP ScanJet",
			ScannedAt: "2016-01-09T10:01:00+11:00", IngestedAt: "2016-01-09T10:30:02+11:00",
			Colour: "mono", DPI: 200,
			Type: "statement", Description: "Electricity — Q4",
			EventDate: "2015-12-28", EventZone: "Australia/Sydney", Total: "AUD 214.80",
		},
		{
			Digest: "ccddeeff", Filename: "scan-0015-ccddeeff.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A5", Producer: "iPhone",
			ScannedAt: "2026-09-01T12:00:00+10:00", IngestedAt: "2026-09-01T12:10:00+10:00",
			Colour: "colour", DPI: 72,
			Type: "ticket", Description: "Cinema — Dune Part Three",
			EventDate: "2026-09-19", EventZone: "Australia/Sydney", Total: "AUD 24.00",
		},
		{
			Digest: "ddeeff00", Filename: "scan-0016-ddeeff00.jpeg", Kind: "image",
			Pages: 1, PageSize: "3024×4032", Producer: "iPhone",
			ScannedAt: "2022-04-18T16:20:00+10:00", IngestedAt: "2022-04-18T16:40:00+10:00",
			Colour: "colour", DPI: 72,
			Type: "object", Reviewed: true, Description: "Grandfather's pocket watch, back plate",
		},
		{
			Digest: "eeff0011", Filename: "scan-0017-eeff0011.pdf", Kind: "pdf",
			Pages: 2, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2018-07-07T19:00:00+10:00", IngestedAt: "2018-07-08T09:00:00+10:00",
			Colour: "colour", DPI: 600,
			Type: "letter", Reviewed: true, Description: "Letter from Dad",
			EventDate: "1998-06-02", EventZone: "Asia/Shanghai",
		},
		{
			Digest: "ff001122", Filename: "scan-0018-ff001122.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2026-06-02T09:00:00+10:00", IngestedAt: "2026-06-02T09:20:00+10:00",
			Colour: "mono", DPI: 300,
			Type: "medical", Reviewed: true, Description: "Blood results",
			EventDate: "2026-05-28", EventZone: "Australia/Sydney",
		},
		{
			Digest: "00112233", Filename: "scan-0019-00112233.pdf", Kind: "pdf",
			Pages: 1, PageSize: "A4", Producer: "Canon LiDE 400",
			ScannedAt: "2026-08-11T09:00:00+10:00", IngestedAt: "2026-08-11T09:20:00+10:00",
			Colour: "mono", DPI: 300,
			Type: "warranty", Description: "Fridge — five years",
			EventDate: "2026-08-01", EventZone: "Australia/Sydney", Total: "AUD 1899.00",
		},
	}
}

// samplePage draws a scan instead of extracting one: a sheet with a few ruled
// lines, at the right aspect ratio, so the pages can be judged on layout
// without a PDF anywhere. SVG because it costs no dependency and scales to
// either size. The colours are the tokens of docs/web.md written out, because
// an SVG served as its own document cannot see the page's custom properties.
func samplePage(scan Scan, size string, number int) []byte {
	width, height := 420, 594
	if strings.Contains(scan.PageSize, "×") {
		width, height = 480, 640
	}
	if scan.PageSize == "A5" || scan.PageSize == "A6" {
		height = 420
	}
	title := scan.Description
	if title == "" {
		title = scan.Filename
	}
	lines := strings.Builder{}
	for row := range 9 {
		y := 150 + row*38
		length := width - 140
		if row%3 == 2 {
			length = length / 2
		}
		fmt.Fprintf(&lines, `<rect x="70" y="%d" width="%d" height="10" rx="5" fill="#e8e8e8"/>`, y, length)
	}
	if scan.NeedsRender {
		lines.Reset()
		fmt.Fprintf(&lines, `<text x="%d" y="%d" text-anchor="middle" font-size="18" fill="#636363">no image to extract</text>`, width/2, height/2)
	}
	stamp := scan.PageSize
	if scan.Pages > 1 {
		stamp = fmt.Sprintf("%s · page %d of %d", scan.PageSize, number, scan.Pages)
	}
	page := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d" role="img" aria-label="%s">
<rect width="%d" height="%d" fill="#ffffff"/>
<rect x="0.5" y="0.5" width="%d" height="%d" fill="none" stroke="#c2c2c2"/>
<text x="70" y="90" font-size="26" fill="#1a1a1a">%s</text>
<text x="70" y="118" font-size="16" fill="#636363">%s</text>
%s
<text x="70" y="%d" font-size="14" fill="#c2c2c2">sample page · not a real scan</text>
</svg>`,
		width, height, width, height, html.EscapeString(title),
		width, height, width-1, height-1,
		html.EscapeString(truncate(title, 26)), html.EscapeString(stamp),
		lines.String(), height-40)
	return []byte(page)
}

func truncate(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}

// Incomplete is one truncated file, so the intake page can be seen reporting
// what it held back.
func (s *Sample) Incomplete() []Exception {
	return []Exception{{
		Kind:   "incomplete",
		Path:   "scan-0117.pdf",
		Detail: "No %%EOF: the file is truncated. It was not taken in.",
	}}
}

func (s *Sample) ApplyMany(digests []string, edit Edit) (BatchResult, error) {
	var result BatchResult
	for _, digest := range digests {
		edit.Digest = digest
		scan, err := s.Apply(edit)
		if err != nil {
			result.Failed = append(result.Failed, BatchFailed{Digest: digest, Error: err.Error()})
			continue
		}
		result.Changed = append(result.Changed, scan)
	}
	return result, nil
}

func (s *Sample) TrashMany(digests []string, reason string) (BatchResult, error) {
	var result BatchResult
	for _, digest := range digests {
		if err := s.Trash(digest, reason); err != nil {
			result.Failed = append(result.Failed, BatchFailed{Digest: digest, Error: err.Error()})
		}
	}
	return result, nil
}

// Adopt is refused by the stand-in: there is no tree to find a file in, and
// pretending otherwise would describe a scan that does not exist.
func (s *Sample) Adopt(path string, _ Edit) (Scan, error) {
	return Scan{}, fmt.Errorf("these scans are made up: %s is not a file", path)
}

func (s *Sample) TrashSummary() TrashSummary {
	return TrashSummary{}
}

// Redraw has no files to draw from.
func (s *Sample) Redraw(context.Context, string) (RedrawResult, error) {
	return RedrawResult{}, fmt.Errorf("these scans are made up: there are no pictures to redraw")
}

// Verify has nothing to read, so it finds nothing. It does not pretend the
// bytes were checked.
func (s *Sample) Verify(context.Context) ([]Exception, error) {
	return nil, fmt.Errorf("these scans are made up: there are no bytes to verify")
}
