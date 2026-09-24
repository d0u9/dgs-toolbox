// Package sidecar reads and writes the YAML file kept beside every scan in a
// Box. It is the only truth about a scan: everything the owner has said about
// the file and everything derived from its bytes. The index is a cache built
// from these files and nothing may exist only there, so this package is the
// input to every other part of box.
//
// The file is named from the digest alone, never from the scan's filename,
// because macOS hands out names in NFD while most software writes NFC and a
// pairing that depended on those bytes would report real sidecars as orphans.
//
// Values in and values out: this package holds the schema and the file
// operations, and computes nothing. Expiry and state come from
// internal/box/lifecycle, amounts from internal/box/money.
package sidecar

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/lifecycle"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/pagerange"
)

// Suffix is what a sidecar's name ends in. It carries no leading dot: a Box
// whose metadata is invisible in Finder looks like a folder of PDFs with
// nothing beside them, which is the opposite of the point.
const Suffix = ".dgs-doc.yaml"

// Version is the format this build writes.
const Version = 1

// DefaultPrefix is how many hex digits of the digest name a sidecar, matching
// the digits appended to the scan's own filename. A Box that had to extend the
// prefix to break a collision inside one directory passes the longer length.
const DefaultPrefix = 8

// partSuffix names the temporary file a sidecar is written under. It is the
// one name box writes that begins with a dot: it exists for a moment and is
// not something anyone should open. Photo Import writes the same name for the
// same mechanism.
const partSuffix = ".dgs-part"

// Kind is what the bytes are. Almost every scan is a PDF; the rest are images
// straight from a phone camera.
type Kind string

const (
	KindPDF   Kind = "pdf"
	KindImage Kind = "image"
)

// File is a sidecar's content, in the order it is written.
//
// Absent and zero are different answers throughout. A document with no total
// has no Total at all rather than a total of zero, and an undated event has the
// zero box.Date rather than a day in year zero.
type File struct {
	Version          int
	Digest           string // "sha256:" followed by lowercase hex
	Size             int64
	Kind             Kind
	OriginalFilename string    // the stem as it arrived, bytes unchanged
	IngestedAt       Timestamp // a real instant, written in UTC; older sidecars carry a local offset
	// EditedAt is when dgs last changed what the sidecar says, empty for one
	// never edited since it was filed. It is a key rather than the file's
	// modification time because copying a Box to another disk rewrites that.
	EditedAt Timestamp

	// Type is a doctype name. Reviewed is false while it is still the guess
	// intake made, and true once a human has confirmed it in view: without the
	// distinction a guess and a confirmation are indistinguishable and the whole
	// metadata set is equally untrustworthy.
	Type        string
	Reviewed    bool
	Description string
	Tags        []string

	// EventDate is the day on the document, read in EventZone. It is a plain
	// date plus an IANA zone name and never an offset, because an offset
	// recorded now is wrong for the same date in another month.
	EventDate box.Date
	EventZone string
	// ExpiresAt is an expiry entered by hand, which always wins over the type's
	// default lifetime. ExpiryCleared records that one was emptied deliberately
	// rather than never filled in; without it "permanent because I said so" and
	// "not yet decided" look identical and the type default creeps back.
	ExpiresAt     box.Date
	ExpiryCleared bool

	// Total is the amount on the document, absent when there is none. It is
	// stored as a minor-unit integer plus an ISO 4217 code, never as a float and
	// never converted to another currency.
	Total money.Amount

	// Derived from the bytes, and rewritten whenever they are read again.
	Pages         int
	PageSize      string
	Producer      string
	ScanCreatedAt Timestamp

	// NeedsSplit marks one file holding several documents before anyone has
	// said where the documents begin and end, and keeps the inbox drainable
	// when that is not worth doing at intake.
	NeedsSplit bool

	// Documents is the other direction said in full: which pages of this one
	// file are which document. The file itself is never cut. Empty means the
	// whole file is one document, which is almost every scan. IgnoredPages are
	// pages that are deliberately no document — a blank, a separator sheet —
	// so that a page nobody put anywhere can be told apart from one that was
	// forgotten. Inheritance is internal/box/docsplit's.
	Documents    []Document
	IgnoredPages []pagerange.Range

	// Trash is set only on a sidecar that travelled into trash/ with its file.
	// Nothing in a Box is deleted, so a discarded scan keeps its metadata and
	// keeps taking part in deduplication.
	TrashedAt   Timestamp
	TrashedFrom string
	Reason      string
}

// Document is one document inside a split file. Its fields only add to the
// file's: a field the file already has is the document's too and cannot be
// set again here, and Tags are added to the file's tags, never removed from
// them. The file-level fields are what the whole batch shares — where it came
// from, when it was scanned — and a document says what is its own.
type Document struct {
	Pages       []pagerange.Range
	Type        string
	Description string
	Tags        []string
	EventDate   box.Date
	EventZone   string
	Total       money.Amount
}

// Timestamp is an instant written as RFC 3339 with the offset it was read at,
// kept as text rather than parsed into a time.Time so a round trip returns the
// bytes the file held. Intake and a PDF's own creation date are instants;
// an event's day is not, and is a box.Date instead.
type Timestamp string

// Zero reports whether t records no instant.
func (t Timestamp) Zero() bool { return strings.TrimSpace(string(t)) == "" }

// Scan is what lifecycle needs to compute this scan's expiry and state. The
// answer is never stored: a policy scanned a year ago is expired because its
// type has a one-year lifetime, not because anything was recalculated.
func (f File) Scan() lifecycle.Scan {
	return lifecycle.Scan{
		Type:          f.Type,
		EventDate:     f.EventDate,
		ExpiresAt:     f.ExpiresAt,
		ExpiryCleared: f.ExpiryCleared,
	}
}

// Zone loads the event's location. A sidecar with no zone gets the local one,
// which is what a scan filed without saying where it happened means.
func (f File) Zone() (*time.Location, error) {
	if strings.TrimSpace(f.EventZone) == "" {
		return time.Local, nil
	}
	return box.LoadZone(f.EventZone)
}

// ShortDigest returns the first length hex digits of the digest, which is what
// names this sidecar and what is appended to the scan's own filename.
func ShortDigest(digest string, length int) string {
	hex := strings.TrimPrefix(digest, "sha256:")
	if length <= 0 || length > len(hex) {
		length = len(hex)
	}
	return hex[:length]
}

// NameFor is the file name a sidecar for digest takes.
func NameFor(digest string, length int) string {
	return ShortDigest(digest, length) + Suffix
}

// PathFor is the sidecar beside a scan in dir.
func PathFor(dir, digest string, length int) string {
	return filepath.Join(dir, NameFor(digest, length))
}

// ErrNotFound is returned by Load when no sidecar is there. A scan without one
// is an orphan, which is a report rather than a failure.
var ErrNotFound = errors.New("no sidecar")

// Load reads the sidecar at path.
//
// A file written by a newer build is refused rather than read as far as this
// one understands it: dropping the fields it does not know and writing the
// result back would silently discard metadata that has no other copy.
func Load(path string) (File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, fmt.Errorf("%s: %w", path, ErrNotFound)
		}
		return File{}, err
	}
	return Decode(data, path)
}

// Save writes file to path, replacing it in one step so a crash never leaves a
// sidecar half written. The temporary file is created in the same directory, so
// the rename stays within one filesystem and is atomic.
func Save(path string, file File) error {
	data, err := Encode(file)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*"+partSuffix)
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	// Flush before the rename: a rename that lands before the bytes do leaves a
	// sidecar of the right name holding nothing, and the sidecar is the only
	// copy of what a person typed.
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
