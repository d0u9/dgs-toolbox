package tree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/verifiedcopy"
)

// ErrNoItem is returned for an ID no sidecar has.
var ErrNoItem = errors.New("no such Item")

// FindItem loads the tree and returns the Item with id, and every Item.
func FindItem(root, id string) (Item, []Item, error) {
	items, err := LoadItems(root)
	if err != nil {
		return Item{}, nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, items, nil
		}
	}
	return Item{}, items, fmt.Errorf("%w: %s", ErrNoItem, id)
}

// holder is the Item already keeping digest, if any.
func holder(items []Item, digest string) (Item, bool) {
	for _, item := range items {
		for _, r := range item.Revisions {
			if r.Digest == digest {
				return item, true
			}
		}
	}
	return Item{}, false
}

// AddRevision copies source into a document as its newest revision and moves
// HEAD to it — a renewed licence replaces the old one. The PDF is published
// only after it reads back the same; the sidecar follows. A record has no
// revisions and is refused.
func AddRevision(ctx context.Context, root, id, source string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindDocument {
		return Item{}, fmt.Errorf("%s %s is a record: records have no revisions", item.Type, item.ID)
	}
	digest, err := FileDigest(source)
	if err != nil {
		return Item{}, err
	}
	if other, ok := holder(items, digest); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
	}
	destination := PDFPath(root, id, digest)
	result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: source, Destination: destination})
	if err != nil {
		return Item{}, err
	}
	if result.Digest != digest {
		_ = os.Remove(destination)
		return Item{}, fmt.Errorf("%s changed while it was being imported", source)
	}
	item.Revisions = append(item.Revisions, Revision{
		Digest: digest, Added: now.Format(time.RFC3339), Source: filepath.Base(source),
	})
	item.Head = digest
	if err := WriteItem(root, item); err != nil {
		return Item{}, err
	}
	return item, nil
}

// SetHead points a document's HEAD at one of its revisions, which is how a
// revision added by mistake is stepped back from. Nothing is removed.
func SetHead(root, id, digest string) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, _, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindDocument {
		return Item{}, fmt.Errorf("%s %s is a record: records have no HEAD", item.Type, item.ID)
	}
	for _, r := range item.Revisions {
		if r.Digest == digest {
			item.Head = digest
			return item, WriteItem(root, item)
		}
	}
	return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
}

// SetFields replaces an Item's fields, checked against its Template as import
// checks them: required keys present, and no other document of the type with
// the same distinguishing fields.
func SetFields(root, id string, template Template, given map[string]string) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if template.Type != item.Type {
		return Item{}, fmt.Errorf("%s %s is not a %s", item.Type, item.ID, template.Type)
	}
	fields, err := CleanFields(template, given)
	if err != nil {
		return Item{}, err
	}
	if other, ok := taken(template, items, fields, id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	if err := linked(template, items, fields, id); err != nil {
		return Item{}, err
	}
	item.Fields = fields
	return item, WriteItem(root, item)
}

// taken is the document, other than except, that already has fields'
// distinguishing values.
func taken(t Template, items []Item, fields map[string]string, except string) (Item, bool) {
	if t.Kind != KindDocument {
		return Item{}, false
	}
	for _, item := range items {
		if item.ID != except && item.Kind == KindDocument && item.Type == t.Type && SameDistinguishing(t, item.Fields, fields) {
			return item, true
		}
	}
	return Item{}, false
}

// SetNotes replaces an Item's notes. Surrounding space is trimmed.
func SetNotes(root, id, notes string) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, _, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	item.Notes = strings.TrimSpace(notes)
	return item, WriteItem(root, item)
}
