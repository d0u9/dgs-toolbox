package tree

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

// ImportRequest is one loose PDF to take into the tree as a new Item.
type ImportRequest struct {
	Root     string
	Source   string
	Template Template
	Fields   map[string]string
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
	for _, item := range items {
		for _, revision := range item.Revisions {
			if revision.Digest == digest {
				return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, item.Type, item.ID)
			}
		}
		if request.Template.Kind == KindDocument && item.Kind == KindDocument &&
			item.Type == request.Template.Type && sameDistinguishing(request.Template, item.Fields, fields) {
			return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, item.Type, item.ID)
		}
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
	item := Item{
		ID: id, Type: request.Template.Type, Kind: request.Template.Kind, Fields: fields,
		Revisions: []Revision{{Digest: digest, Added: request.Now.Format(time.RFC3339), Source: filepath.Base(request.Source)}},
	}
	if item.Kind == KindDocument {
		item.Head = digest
	}
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
		fields[f.Key] = value
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

func sameDistinguishing(t Template, a, b map[string]string) bool {
	for _, f := range t.Fields {
		if f.Distinguishing && !strings.EqualFold(a[f.Key], b[f.Key]) {
			return false
		}
	}
	return true
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
