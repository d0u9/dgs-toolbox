package tree

import (
	"context"
	"crypto/sha256"
	"dgs-toolbox/internal/doc/country"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/tag"
	"dgs-toolbox/internal/verifiedcopy"
)

// ImportRequest is one loose PDF to take into the tree as a new Item.
type ImportRequest struct {
	Root     string
	Source   string
	Template Template
	Fields   map[string]string
	Notes    string
	Tags     []string
	Now      time.Time
}

// ErrDuplicate is returned when the PDF is already kept in the tree.
var ErrDuplicate = errors.New("already in the tree")

// ErrTaken is returned when a document of the same type and distinguishing
// fields exists: the PDF is a new revision of it, not a new Item.
var ErrTaken = errors.New("a document with these fields exists")

// Import copies Source into a new Item. The PDF is published under its digest
// only after it reads back the same (verifiedcopy); the sidecar is written
// after that, so a sidecar never names a PDF that is not there. The source is
// never touched.
func Import(ctx context.Context, request ImportRequest) (Item, error) {
	if err := Require(request.Root); err != nil {
		return Item{}, err
	}
	fields, err := CleanFields(request.Template, request.Fields)
	if err != nil {
		return Item{}, err
	}
	digest, err := FileDigest(request.Source)
	if err != nil {
		return Item{}, err
	}
	items, err := LoadItems(request.Root)
	if err != nil {
		return Item{}, err
	}
	if other, ok := holder(items, digest); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
	}
	if other, ok := taken(request.Template, items, fields, ""); ok {
		return Item{}, fmt.Errorf("%w: %s %s; add the PDF to it as a revision", ErrTaken, other.Type, other.ID)
	}
	if err := linked(request.Template, items, fields, ""); err != nil {
		return Item{}, err
	}
	id, err := NewID(request.Now)
	if err != nil {
		return Item{}, err
	}
	result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source:      request.Source,
		Destination: PDFPath(request.Root, id, digest),
	})
	if err != nil {
		return Item{}, err
	}
	if result.Digest != digest {
		_ = os.Remove(PDFPath(request.Root, id, digest))
		return Item{}, fmt.Errorf("%s changed while it was being imported", request.Source)
	}
	own, perRevision := request.Template.Split(fields)
	item := Item{
		ID: id, Type: request.Template.Type, Kind: request.Template.Kind, Fields: own,
		Notes:     strings.TrimSpace(request.Notes),
		Tags:      tag.List(request.Tags),
		Revisions: []Revision{{Digest: digest, Added: request.Now.Format(time.RFC3339), Source: filepath.Base(request.Source), Fields: perRevision}},
	}
	if item.Kind == KindDocument {
		item.Head = digest
	}
	item.History = []HistoryEvent{{At: request.Now.Format(time.RFC3339), Action: "import", Digest: digest}}
	if err := WriteItem(request.Root, item); err != nil {
		return Item{}, err
	}
	return item, nil
}

// CleanFields keeps the Template's keys, trimmed, fills defaults for empty
// ones, and refuses a missing required key. Empty keys are dropped.
func CleanFields(t Template, given map[string]string) (map[string]string, error) {
	fields := map[string]string{}
	var missing []string
	for _, f := range t.Fields {
		value := strings.TrimSpace(given[f.Key])
		if value == "" {
			value = strings.TrimSpace(t.Defaults[f.Key])
		}
		if value == "" {
			if f.Required {
				missing = append(missing, f.Key)
			}
			continue
		}
		kept, err := f.clean(value)
		if err != nil {
			return nil, err
		}
		fields[f.Key] = kept
	}
	for key := range given {
		if !t.has(key) {
			return nil, fmt.Errorf("type %s has no field %s", t.Type, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return fields, nil
}

func (t Template) has(key string) bool {
	for _, f := range t.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// SameDistinguishing reports whether a and b agree, ignoring case, on every
// field t marks distinguishing: whether they name the same document. A
// country agrees however either side writes it: AU and 澳大利亚 are one.
func SameDistinguishing(t Template, a, b map[string]string) bool {
	for _, f := range t.Fields {
		if f.Distinguishing && !sameValue(f, a[f.Key], b[f.Key]) {
			return false
		}
	}
	return true
}

func sameValue(f Field, a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	if f.Type == FieldCountry {
		ca, okA := country.Find(a)
		cb, okB := country.Find(b)
		return okA && okB && ca == cb
	}
	return false
}

// FileDigest is a file's SHA-256 in lowercase hex.
func FileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// linked checks that each item field names an Item in the tree, other than
// the Item itself.
func linked(t Template, items []Item, fields map[string]string, self string) error {
	for _, f := range t.Fields {
		value, ok := fields[f.Key]
		if f.Type != FieldItem || !ok {
			continue
		}
		if value == self {
			return fmt.Errorf("%s: an Item cannot link to itself", f.Key)
		}
		found := false
		for _, item := range items {
			if item.ID == value {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: no Item %s in this tree", f.Key, value)
		}
	}
	return nil
}
