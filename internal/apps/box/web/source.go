package web

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/lifecycle"
	"dgs-toolbox/internal/box/money"
)

// Scan is one scan as the pages see it. Digest identifies it everywhere: the
// page asks for an image, an edit or a discard by digest, never by path, so
// nothing a browser sends can name a file outside the Box.
type Scan struct {
	Digest   string `json:"digest"`
	Filename string `json:"filename"`
	Kind     string `json:"kind"`
	Pages    int    `json:"pages"`
	PageSize string `json:"pageSize"`
	Producer string `json:"producer"`
	// ScannedAt is the scanner's own timestamp, in RFC 3339 with the offset it
	// was read with. Intake is ordered by it: what was scanned consecutively is
	// related, and filename order destroys that.
	ScannedAt string `json:"scannedAt"`
	Colour    string `json:"colour"`
	DPI       int    `json:"dpi"`

	Type          string   `json:"type"`
	TypeKnown     bool     `json:"typeKnown"`
	Reviewed      bool     `json:"reviewed"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags"`
	EventDate     string   `json:"eventDate"`
	EventZone     string   `json:"eventZone"`
	ExpiresAt     string   `json:"expiresAt"`
	ExpiryCleared bool     `json:"expiryCleared"`
	// Total is the amount as money.Amount.String writes it — "AUD 123.50" —
	// or empty when the document has none. Empty is not zero.
	Total string `json:"total"`
	Group string `json:"group"`

	NeedsSplit bool `json:"needsSplit"`
	// Documents is where a split scan's documents begin and end, and what
	// each adds to the file's own fields. Empty is one document, the whole
	// file. Unassigned is computed: pages in no document and not ignored.
	Documents    []SplitDocument `json:"documents"`
	IgnoredPages string          `json:"ignoredPages"`
	Unassigned   string          `json:"unassigned"`
	NeedsRender  bool            `json:"needsRender"`
	// Unfilable marks a filed scan whose inbox file is still there, so it can
	// be taken back to intake and filed again.
	Unfilable  bool `json:"unfilable"`
	Incomplete bool `json:"incomplete"`
	// DuplicateOf is the digest this scan repeats, and Trashed says the match
	// is something already thrown away — so the same judgement is not made a
	// second time.
	DuplicateOf string `json:"duplicateOf"`
	TrashedAt   string `json:"trashedAt"`
	IngestedAt  string `json:"ingestedAt"`

	// Expiry and State are computed, never stored. They are sent so the page
	// does not reimplement the rule.
	Expiry string `json:"expiry"`
	State  string `json:"state"`
}

// Exception is something about the tree that the tool cannot explain: a scan
// with no sidecar, a sidecar with no scan, or bytes that no longer match the
// digest recorded for them.
type Exception struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
	// Detail is what is known: when the digest was recorded, what it was.
	Detail string `json:"detail"`
	// Adoptable marks the one exception with an answer rather than a report —
	// a file put there by hand, which intake can describe where it lies.
	Adoptable bool `json:"adoptable"`
}

// Edit is a change to one scan's metadata. A nil field is left alone, so the
// intake page can set a type without clearing a description.
type Edit struct {
	Digest        string    `json:"digest"`
	Type          *string   `json:"type"`
	Description   *string   `json:"description"`
	EventDate     *string   `json:"eventDate"`
	EventZone     *string   `json:"eventZone"`
	ExpiresAt     *string   `json:"expiresAt"`
	ExpiryCleared *bool     `json:"expiryCleared"`
	Total         *string   `json:"total"`
	Tags          *[]string `json:"tags"`
	Group         *string   `json:"group"`
	Reviewed      *bool     `json:"reviewed"`
	// Documents replaces the whole split: a split is edited as one list, and
	// merging two lists of overlapping ranges has no answer anyone expects.
	Documents    *[]SplitDocument `json:"documents"`
	IgnoredPages *string          `json:"ignoredPages"`
}

// Source is whatever holds the scans. It is the whole boundary between the
// pages and a Box, so the engine replaces Sample without a page changing.
type Source interface {
	// Pending is what intake still has to deal with, in scan-time order.
	Pending() []Scan
	// Scans is everything filed in the Box.
	Scans() []Scan
	// Exceptions is what cannot be explained about the tree.
	Exceptions() []Exception
	// Apply changes one scan's metadata and returns it as it now is.
	Apply(edit Edit) (Scan, error)
	// File takes a pending scan into the Box. It is the only call that would
	// copy bytes, and it is why import and view are separate commands.
	File(digest string, edit Edit) (Scan, error)
	// Trash moves a filed scan into the Box's trash with a reason. Nothing is
	// deleted.
	Trash(digest, reason string) error
	// Image is a scan's thumbnail or preview, as bytes and a media type. page
	// counts from 1 and names which page of a multi-page scan is wanted; page
	// 1 is the picture a grid shows and the only one every scan has.
	Image(digest, size string, page int) ([]byte, string, error)
	// Incomplete is what was read in the inbox and not taken in: truncated
	// files, bad copies, damaged old backups. It is reported rather than
	// skipped, because a skip nobody is told about means believing the inbox
	// is empty when it is not.
	Incomplete() []Exception
	// ApplyMany changes several scans at once. A few hundred scans at one
	// round trip each is the hour of work nobody finishes, so a batch is one
	// request; each record is still written and logged on its own.
	ApplyMany(digests []string, edit Edit) (BatchResult, error)
	// TrashMany discards several scans in one request, which is how a group of
	// duplicates is one decision rather than one per copy.
	TrashMany(digests []string, reason string) (BatchResult, error)
	// Adopt describes a scan that is in the Box with no sidecar, where it lies.
	Adopt(path string, edit Edit) (Scan, error)
	// TrashSummary is what the trash holds, so view can advise on emptying it.
	// Nothing is ever removed here: emptying trash/ is done by hand.
	TrashSummary() TrashSummary
	// Verify reads every byte of every file and reports what no longer matches
	// its recorded digest. It repairs nothing.
	Verify(ctx context.Context) ([]Exception, error)
	// Rejected is what was turned away at intake. Nothing was moved or
	// deleted — the file is still in the inbox — so each can be taken back.
	Rejected() []RejectedScan
	// Restore puts a rejected scan back in the inbox's list, to be decided
	// again. It is named by digest, never by path.
	Restore(digest string) error
	// RejectedFile is a rejected scan's bytes as they sit in the inbox, so it
	// can be looked at again before deciding whether to restore it.
	RejectedFile(digest string) (body []byte, filename, mediaType string, err error)
	// Unfile takes a filed scan back to intake: its Box copy goes to the trash
	// and it is waiting again, with what its sidecar said as the draft.
	Unfile(digest string) error
	// Sample reports whether this Source is made up, so the pages can say so
	// rather than letting someone describe scans that do not exist.
	Sample() bool
}

// RejectedScan is one scan turned away at intake.
type RejectedScan struct {
	Digest   string `json:"digest"`
	Filename string `json:"filename"`
	Reason   string `json:"reason"`
	// At is when it was rejected, RFC 3339.
	At string `json:"at"`
}

// BatchResult is what a batch did. Failures are per record rather than for the
// whole request: one refused date must not undo two hundred correct writes.
type BatchResult struct {
	Changed []Scan        `json:"changed"`
	Failed  []BatchFailed `json:"failed"`
}

// BatchFailed is one record a batch could not change, and why.
type BatchFailed struct {
	Digest string `json:"digest"`
	Error  string `json:"error"`
}

// TrashSummary is what trash/ holds and what box.trash.keep says about it.
type TrashSummary struct {
	Count int `json:"count"`
	// Oldest is the earliest discard date still in the trash, or empty.
	Oldest string `json:"oldest"`
	// KeepDays is box.trash.keep. Zero means no advice is offered.
	KeepDays int `json:"keepDays"`
	// Overdue is how many were discarded longer than KeepDays ago.
	Overdue int `json:"overdue"`
}

// decorate fills in what is computed rather than stored: whether the type is
// one this build knows, the expiry, and the state.
func decorate(scan Scan, today box.Date) Scan {
	_, known := doctype.Lookup(scan.Type)
	scan.TypeKnown = known
	eventDate, _ := box.ParseDate(scan.EventDate)
	expiresAt, _ := box.ParseDate(scan.ExpiresAt)
	subject := lifecycle.Scan{
		Type:          scan.Type,
		EventDate:     eventDate,
		ExpiresAt:     expiresAt,
		ExpiryCleared: scan.ExpiryCleared,
	}
	if expiry, has := lifecycle.Expiry(subject); has {
		scan.Expiry = expiry.String()
	} else {
		scan.Expiry = ""
	}
	scan.State = lifecycle.StateOn(subject, today).String()
	scan.Unassigned = unassigned(scan)
	return scan
}

// Sample is a made-up Box: enough scans, in enough states, to see whether the
// pages are right before the engine exists. It answers every call, so the
// pages can be driven end to end, and it says so through Sample() rather than
// pretending to be a Box.
type Sample struct {
	mutex sync.Mutex
	// currency is what an amount typed with no code is read in, as
	// box.currency says.
	currency string
	pending  []Scan
	filed    []Scan
	rejected []rejectedSample
}

// NewSample builds the stand-in Box. currency is box.currency, so an amount
// typed on the page is read the same way it will be once the engine is real.
func NewSample(currency string) *Sample {
	return &Sample{currency: currency, pending: samplePending(), filed: sampleFiled()}
}

func (s *Sample) Sample() bool { return true }

func (s *Sample) Pending() []Scan {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	pending := append([]Scan(nil), s.pending...)
	// Scan-time order, because what was scanned consecutively belongs together.
	sort.SliceStable(pending, func(i, j int) bool {
		return pending[i].ScannedAt < pending[j].ScannedAt
	})
	today := box.Today(nil)
	for index := range pending {
		pending[index] = decorate(pending[index], today)
	}
	return pending
}

func (s *Sample) Scans() []Scan {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	filed := append([]Scan(nil), s.filed...)
	today := box.Today(nil)
	for index := range filed {
		filed[index] = decorate(filed[index], today)
		filed[index].Unfilable = true
	}
	sort.SliceStable(filed, func(i, j int) bool {
		left, right := sortKey(filed[i]), sortKey(filed[j])
		if left == right {
			return filed[i].Filename < filed[j].Filename
		}
		return left > right
	})
	return filed
}

// sortKey is the event date where there is one and the scan date otherwise,
// which is what the page says on screen when it has fallen back.
func sortKey(scan Scan) string {
	if scan.EventDate != "" {
		return scan.EventDate
	}
	if len(scan.ScannedAt) >= 10 {
		return scan.ScannedAt[:10]
	}
	return ""
}

func (s *Sample) Exceptions() []Exception {
	return []Exception{
		{
			Kind:      "no sidecar",
			Path:      "2026/2026-09-18/scan-0101-3c7a91ff.pdf",
			Detail:    "Put here by hand, or its sidecar was lost.",
			Adoptable: true,
		},
		{
			Kind:   "no scan",
			Path:   "2026/2026-09-18/9f1e0c22.dgs-doc.yaml",
			Detail: "The sidecar describes a file that is not there. It may have been moved on purpose.",
		},
		{
			Kind:   "digest mismatch",
			Path:   "2024/2024-02-03/scan-0044-77f0e1b9.pdf",
			Detail: "Recorded as sha256:77f0e1b9… on 2024-02-03. The bytes no longer match. Nothing has been rewritten.",
		},
	}
}

func (s *Sample) Apply(edit Edit) (Scan, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if scan, index, found := find(s.filed, edit.Digest); found {
		updated, err := applyEdit(scan, edit, s.currency)
		if err != nil {
			return Scan{}, err
		}
		s.filed[index] = updated
		return decorate(updated, box.Today(nil)), nil
	}
	if scan, index, found := find(s.pending, edit.Digest); found {
		updated, err := applyEdit(scan, edit, s.currency)
		if err != nil {
			return Scan{}, err
		}
		s.pending[index] = updated
		return decorate(updated, box.Today(nil)), nil
	}
	return Scan{}, fmt.Errorf("no scan %q", edit.Digest)
}

func (s *Sample) File(digest string, edit Edit) (Scan, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	scan, index, found := find(s.pending, digest)
	if !found {
		return Scan{}, fmt.Errorf("no pending scan %q", digest)
	}
	edit.Digest = digest
	filed, err := applyEdit(scan, edit, s.currency)
	if err != nil {
		return Scan{}, err
	}
	filed.IngestedAt = time.Now().Format(time.RFC3339)
	s.pending = append(s.pending[:index], s.pending[index+1:]...)
	s.filed = append(s.filed, filed)
	return decorate(filed, box.Today(nil)), nil
}

func (s *Sample) Trash(digest, reason string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if _, index, found := find(s.filed, digest); found {
		s.filed = append(s.filed[:index], s.filed[index+1:]...)
		return nil
	}
	if scan, index, found := find(s.pending, digest); found {
		s.pending = append(s.pending[:index], s.pending[index+1:]...)
		s.rejected = append(s.rejected, rejectedSample{scan: scan, reason: reason, at: time.Now()})
		return nil
	}
	return fmt.Errorf("no scan %q", digest)
}

type rejectedSample struct {
	scan   Scan
	reason string
	at     time.Time
}

// Rejected is what the sample turned away, newest first.
func (s *Sample) Rejected() []RejectedScan {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	out := make([]RejectedScan, 0, len(s.rejected))
	for i := len(s.rejected) - 1; i >= 0; i-- {
		item := s.rejected[i]
		out = append(out, RejectedScan{
			Digest: item.scan.Digest, Filename: item.scan.Filename,
			Reason: item.reason, At: item.at.Format(time.RFC3339),
		})
	}
	return out
}

// Unfile puts a filed sample scan back in the inbox as it was described.
func (s *Sample) Unfile(digest string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	scan, index, found := find(s.filed, digest)
	if !found {
		return fmt.Errorf("no scan %q", digest)
	}
	s.filed = append(s.filed[:index], s.filed[index+1:]...)
	s.pending = append(s.pending, scan)
	sort.SliceStable(s.pending, func(a, b int) bool { return s.pending[a].ScannedAt < s.pending[b].ScannedAt })
	return nil
}

// RejectedFile has no file to give: a sample scan is only a picture, so the
// picture is what is shown.
func (s *Sample) RejectedFile(digest string) ([]byte, string, string, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for _, item := range s.rejected {
		if item.scan.Digest == digest {
			return samplePage(item.scan, "preview", 1), item.scan.Filename, "image/svg+xml", nil
		}
	}
	return nil, "", "", fmt.Errorf("no rejected scan %q", digest)
}

// Restore puts a rejected sample scan back where it was in scan-time order.
func (s *Sample) Restore(digest string) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for i, item := range s.rejected {
		if item.scan.Digest != digest {
			continue
		}
		s.rejected = append(s.rejected[:i], s.rejected[i+1:]...)
		s.pending = append(s.pending, item.scan)
		sort.SliceStable(s.pending, func(a, b int) bool { return s.pending[a].ScannedAt < s.pending[b].ScannedAt })
		return nil
	}
	return fmt.Errorf("no rejected scan %q", digest)
}

func (s *Sample) Image(digest, size string, page int) ([]byte, string, error) {
	scan, _, found := find(s.pending, digest)
	if !found {
		scan, _, found = find(s.filed, digest)
	}
	if !found {
		return nil, "", fmt.Errorf("no scan %q", digest)
	}
	if page < 1 {
		page = 1
	}
	if page > 1 && page > scan.Pages {
		return nil, "", fmt.Errorf("%s has no page %d", digest, page)
	}
	return samplePage(scan, size, page), "image/svg+xml", nil
}

func find(scans []Scan, digest string) (Scan, int, bool) {
	for index, scan := range scans {
		if scan.Digest == digest {
			return scan, index, true
		}
	}
	return Scan{}, 0, false
}

// applyEdit is the part of an edit that is the same for a stand-in and for a
// Box: what a field means, and what is refused. Setting a type to one this
// build does not register is refused on the way in; a type already in a
// sidecar is kept as it is on the way out.
func applyEdit(scan Scan, edit Edit, defaultCurrency string) (Scan, error) {
	if edit.Type != nil {
		if _, known := doctype.Lookup(*edit.Type); !known {
			return Scan{}, fmt.Errorf("no such type %q: one of %s", *edit.Type, strings.Join(doctype.Names(), ", "))
		}
		if scan.Type != *edit.Type {
			scan.Reviewed = false
		}
		scan.Type = *edit.Type
	}
	if edit.Description != nil {
		scan.Description = strings.TrimSpace(*edit.Description)
	}
	if edit.EventDate != nil {
		if trimmed := strings.TrimSpace(*edit.EventDate); trimmed == "" {
			scan.EventDate = ""
		} else {
			parsed, err := box.ParseDate(trimmed)
			if err != nil {
				return Scan{}, err
			}
			scan.EventDate = parsed.String()
		}
	}
	if edit.EventZone != nil {
		if trimmed := strings.TrimSpace(*edit.EventZone); trimmed != "" {
			if _, err := box.LoadZone(trimmed); err != nil {
				return Scan{}, err
			}
		}
		scan.EventZone = strings.TrimSpace(*edit.EventZone)
	}
	if edit.ExpiresAt != nil {
		if trimmed := strings.TrimSpace(*edit.ExpiresAt); trimmed == "" {
			scan.ExpiresAt = ""
		} else {
			parsed, err := box.ParseDate(trimmed)
			if err != nil {
				return Scan{}, err
			}
			scan.ExpiresAt = parsed.String()
			scan.ExpiryCleared = false
		}
	}
	if edit.ExpiryCleared != nil {
		scan.ExpiryCleared = *edit.ExpiryCleared
		if *edit.ExpiryCleared {
			scan.ExpiresAt = ""
		}
	}
	if edit.Total != nil {
		trimmed := strings.TrimSpace(*edit.Total)
		if trimmed == "" {
			scan.Total = ""
		} else {
			amount, err := money.Parse(trimmed, defaultCurrency)
			if err != nil {
				return Scan{}, err
			}
			scan.Total = amount.String()
		}
	}
	if edit.Tags != nil {
		scan.Tags = normalizeTags(*edit.Tags)
	}
	if edit.Group != nil {
		scan.Group = strings.TrimSpace(*edit.Group)
	}
	if edit.Reviewed != nil {
		scan.Reviewed = *edit.Reviewed
	}
	scan, err := applySplit(scan, edit.Documents, edit.IgnoredPages, defaultCurrency)
	if err != nil {
		return Scan{}, err
	}
	if err := checkSplit(scan); err != nil {
		return Scan{}, err
	}
	return scan, nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, duplicate := seen[tag]; duplicate {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}
