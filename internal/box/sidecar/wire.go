package sidecar

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/pagerange"
)

// wire is the sidecar exactly as it appears on disk. It exists so the File a
// caller works with can hold real types — box.Date, money.Amount — while the
// file keeps the flat field names a person edits by hand, and so the domain
// packages stay free of any encoding library.
//
// Every optional field is a pointer or carries omitempty, because absent and
// zero mean different things in this schema and a marshaller that wrote 0 for
// "no total" would invent an amount nobody recorded.
type wire struct {
	Version          int       `yaml:"version"`
	Digest           string    `yaml:"digest"`
	Size             int64     `yaml:"size"`
	Kind             string    `yaml:"kind"`
	OriginalFilename string    `yaml:"original_filename"`
	IngestedAt       Timestamp `yaml:"ingested_at,omitempty"`

	Type        string   `yaml:"type"`
	Reviewed    bool     `yaml:"reviewed"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`

	EventDate     string `yaml:"event_date,omitempty"`
	EventZone     string `yaml:"event_tz,omitempty"`
	ExpiresAt     string `yaml:"expires_at,omitempty"`
	ExpiryCleared bool   `yaml:"expiry_cleared,omitempty"`

	TotalMinor *int64 `yaml:"total_minor,omitempty"`
	Currency   string `yaml:"currency,omitempty"`

	Pages         int       `yaml:"pages,omitempty"`
	PageSize      string    `yaml:"page_size,omitempty"`
	Producer      string    `yaml:"producer,omitempty"`
	ScanCreatedAt Timestamp `yaml:"scan_created_at,omitempty"`

	NeedsSplit bool `yaml:"needs_split,omitempty"`

	Documents    []wireDocument `yaml:"documents,omitempty"`
	IgnoredPages string         `yaml:"ignored_pages,omitempty"`

	TrashedAt   Timestamp `yaml:"trashed_at,omitempty"`
	TrashedFrom string    `yaml:"trashed_from,omitempty"`
	Reason      string    `yaml:"reason,omitempty"`
}

// wireDocument is one entry of documents. Pages are the text a person types —
// "1-3,6" — rather than a list of numbers, so a hand edit reads like the page.
type wireDocument struct {
	Pages       string   `yaml:"pages"`
	Type        string   `yaml:"type,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
	EventDate   string   `yaml:"event_date,omitempty"`
	EventZone   string   `yaml:"event_tz,omitempty"`
	TotalMinor  *int64   `yaml:"total_minor,omitempty"`
	Currency    string   `yaml:"currency,omitempty"`
}

// Encode writes file as the bytes of a sidecar.
func Encode(file File) ([]byte, error) {
	file.Version = Version
	out := wire{
		Version:          Version,
		Digest:           file.Digest,
		Size:             file.Size,
		Kind:             string(file.Kind),
		OriginalFilename: file.OriginalFilename,
		IngestedAt:       file.IngestedAt,
		Type:             file.Type,
		Reviewed:         file.Reviewed,
		Description:      file.Description,
		Tags:             file.Tags,
		EventDate:        file.EventDate.String(),
		EventZone:        file.EventZone,
		ExpiresAt:        file.ExpiresAt.String(),
		ExpiryCleared:    file.ExpiryCleared,
		Pages:            file.Pages,
		PageSize:         file.PageSize,
		Producer:         file.Producer,
		ScanCreatedAt:    file.ScanCreatedAt,
		NeedsSplit:       file.NeedsSplit,
		TrashedAt:        file.TrashedAt,
		TrashedFrom:      file.TrashedFrom,
		Reason:           file.Reason,
	}
	// A total is written only when there is one. Refusing an unknown currency
	// here rather than on the way in keeps a sidecar from recording an amount
	// whose scale this build cannot state: two decimals is wrong by a hundred
	// for JPY and by a thousand for KWD.
	if file.Total.Currency != "" {
		if !file.Total.Valid() {
			return nil, money.UnknownCurrencyError{Code: file.Total.Currency}
		}
		minor := file.Total.Minor
		out.TotalMinor = &minor
		out.Currency = file.Total.Currency
	}
	out.IgnoredPages = pagerange.Format(file.IgnoredPages)
	for _, document := range file.Documents {
		entry := wireDocument{
			Pages:       pagerange.Format(document.Pages),
			Type:        document.Type,
			Description: document.Description,
			Tags:        document.Tags,
			EventDate:   document.EventDate.String(),
			EventZone:   document.EventZone,
		}
		if entry.Pages == "" {
			return nil, errors.New("a document names no pages")
		}
		if document.Total.Currency != "" {
			if !document.Total.Valid() {
				return nil, money.UnknownCurrencyError{Code: document.Total.Currency}
			}
			minor := document.Total.Minor
			entry.TotalMinor = &minor
			entry.Currency = document.Total.Currency
		}
		out.Documents = append(out.Documents, entry)
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Decode reads the bytes of a sidecar. The name is used only in messages.
func Decode(data []byte, name string) (File, error) {
	// The version is read on its own first. A file from a newer format is
	// likely to hold keys this build has never heard of, and "version 2 is
	// newer than this build reads" is the answer a person can act on, where a
	// complaint about one unknown key is not.
	var header struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &header); err != nil {
		return File{}, fmt.Errorf("%s: %w", name, err)
	}
	if header.Version > Version {
		return File{}, fmt.Errorf("%s: version %d is newer than this build reads", name, header.Version)
	}

	var in wire
	// Unknown keys are refused rather than dropped. A sidecar is edited by hand
	// and is the only copy of what it holds, so reading one, changing a field
	// and writing it back must never be the step that loses a key this build
	// did not recognise — a typo is told about instead. A file from a newer
	// format says so in its version, which is checked below.
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		return File{}, fmt.Errorf("%s: %w", name, err)
	}
	file := File{
		Version:          in.Version,
		Digest:           in.Digest,
		Size:             in.Size,
		Kind:             Kind(in.Kind),
		OriginalFilename: in.OriginalFilename,
		IngestedAt:       in.IngestedAt,
		Type:             in.Type,
		Reviewed:         in.Reviewed,
		Description:      in.Description,
		Tags:             in.Tags,
		EventZone:        strings.TrimSpace(in.EventZone),
		ExpiryCleared:    in.ExpiryCleared,
		Pages:            in.Pages,
		PageSize:         in.PageSize,
		Producer:         in.Producer,
		ScanCreatedAt:    in.ScanCreatedAt,
		NeedsSplit:       in.NeedsSplit,
		TrashedAt:        in.TrashedAt,
		TrashedFrom:      in.TrashedFrom,
		Reason:           in.Reason,
	}
	var err error
	if file.EventDate, err = readDate(in.EventDate, name, "event_date"); err != nil {
		return File{}, err
	}
	if file.IgnoredPages, err = pagerange.Parse(in.IgnoredPages); err != nil {
		return File{}, fmt.Errorf("%s: ignored_pages: %w", name, err)
	}
	for position, entry := range in.Documents {
		field := fmt.Sprintf("documents[%d]", position)
		document := Document{
			Type: entry.Type, Description: entry.Description, Tags: entry.Tags,
			EventZone: strings.TrimSpace(entry.EventZone),
		}
		if document.EventZone != "" {
			if _, err := box.LoadZone(document.EventZone); err != nil {
				return File{}, fmt.Errorf("%s: %s.event_tz: %w", name, field, err)
			}
		}
		if document.Pages, err = pagerange.Parse(entry.Pages); err != nil {
			return File{}, fmt.Errorf("%s: %s.pages: %w", name, field, err)
		}
		if len(document.Pages) == 0 {
			return File{}, fmt.Errorf("%s: %s names no pages", name, field)
		}
		if document.EventDate, err = readDate(entry.EventDate, name, field+".event_date"); err != nil {
			return File{}, err
		}
		if document.Total, err = readAmount(entry.TotalMinor, entry.Currency, name+": "+field); err != nil {
			return File{}, err
		}
		file.Documents = append(file.Documents, document)
	}
	if file.ExpiresAt, err = readDate(in.ExpiresAt, name, "expires_at"); err != nil {
		return File{}, err
	}
	// A zone is checked on the way in so an offset never reaches the rest of
	// the program, where it would be right today and wrong after a daylight
	// saving change.
	if file.EventZone != "" {
		if _, err := box.LoadZone(file.EventZone); err != nil {
			return File{}, fmt.Errorf("%s: event_tz: %w", name, err)
		}
	}
	// An amount needs both halves. A number with no currency has no scale and a
	// currency with no number is not an amount, and guessing either one writes a
	// figure nobody entered.
	if file.Total, err = readAmount(in.TotalMinor, in.Currency, name); err != nil {
		return File{}, err
	}
	return file, nil
}

func readAmount(minor *int64, currency, name string) (money.Amount, error) {
	switch {
	case minor != nil && currency == "":
		return money.Amount{}, fmt.Errorf("%s: total_minor without currency", name)
	case minor == nil && currency != "":
		return money.Amount{}, fmt.Errorf("%s: currency without total_minor", name)
	case minor != nil:
		amount := money.Amount{Minor: *minor, Currency: strings.ToUpper(strings.TrimSpace(currency))}
		if !amount.Valid() {
			return money.Amount{}, fmt.Errorf("%s: %w", name, money.UnknownCurrencyError{Code: amount.Currency})
		}
		return amount, nil
	}
	return money.Amount{}, nil
}

func readDate(text, name, field string) (box.Date, error) {
	if strings.TrimSpace(text) == "" {
		return box.Date{}, nil
	}
	date, err := box.ParseDate(text)
	if err != nil {
		return box.Date{}, fmt.Errorf("%s: %s: %w", name, field, err)
	}
	return date, nil
}
