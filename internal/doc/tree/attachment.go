package tree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/tag"
	"dgs-toolbox/internal/verifiedcopy"
)

// CreateWithoutPDF creates an Item with a metadata-only first revision.
func CreateWithoutPDF(request ImportRequest) (Item, error) {
	if err := Require(request.Root); err != nil {
		return Item{}, err
	}
	if err := request.Template.Validate(); err != nil {
		return Item{}, err
	}
	fields, err := CleanFields(request.Template, request.Fields)
	if err != nil {
		return Item{}, err
	}
	items, err := LoadItems(request.Root)
	if err != nil {
		return Item{}, err
	}
	if other, ok := taken(request.Template, items, fields, ""); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	if err := linked(request.Template, items, fields, ""); err != nil {
		return Item{}, err
	}
	id, err := NewID(request.Now)
	if err != nil {
		return Item{}, err
	}
	item := Item{ID: id, Type: request.Template.Type, Kind: request.Template.Kind, Notes: strings.TrimSpace(request.Notes), Tags: tag.List(request.Tags)}
	ref := item.saveSnapshot(request.Template.Type, fields, "", "", request.Now)
	item.History = []HistoryEvent{{At: request.Now.Format(time.RFC3339), Action: "create_without_pdf", Digest: ref}}
	if err := WriteItem(request.Root, item); err != nil {
		_ = os.Remove(Dir(request.Root, id))
		return Item{}, err
	}
	return item, nil
}

// AddWithoutPDF records a new issue without requiring a scan.
func AddWithoutPDF(root, id string, template Template, given map[string]string, metadata RevisionMetadata, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindDocument || template.Type != item.Type {
		return Item{}, fmt.Errorf("new revisions require a matching document Template")
	}
	fields, err := RevisionFields(template, given)
	if err != nil {
		return Item{}, err
	}
	if err := linked(template, items, fields, id); err != nil {
		return Item{}, err
	}
	full, _ := template.Split(item.CurrentFields())
	for k, v := range fields {
		full[k] = v
	}
	if item.CurrentDigest() == "" && SnapshotID(item.Type, item.CurrentFields(), "") == SnapshotID(item.Type, full, "") && (metadata.Notes == nil || strings.TrimSpace(*metadata.Notes) == item.Notes) && (metadata.Tags == nil || strings.Join(tag.List(*metadata.Tags), ",") == strings.Join(item.Tags, ",")) {
		return item, nil
	}
	ref := item.saveSnapshot(item.Type, full, "", "", now)
	if metadata.Notes != nil {
		item.Notes = strings.TrimSpace(*metadata.Notes)
	}
	if metadata.Tags != nil {
		item.Tags = tag.List(*metadata.Tags)
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "create_revision_without_pdf", Digest: ref})
	return item, WriteItem(root, item)
}

// AttachPDF creates a new snapshot from the selected revision and a PDF.
// The selected revision and its references are never changed.
func AttachPDF(ctx context.Context, root, id, ref, source string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	base, ok := item.Revision(ref)
	if !ok {
		return Item{}, fmt.Errorf("no such revision")
	}
	typ := base.Type
	if typ == "" {
		typ = item.Type
	}
	templates, err := LoadTemplates(root)
	if err != nil {
		return Item{}, err
	}
	for _, t := range templates {
		if t.Type == typ {
			if other, ok := taken(t, items, item.FieldsAt(ref), id); ok {
				return Item{}, fmt.Errorf("%w: %s", ErrTaken, other.ID)
			}
		}
	}
	digest, err := FileDigest(source)
	if err != nil {
		return Item{}, err
	}
	if digest == base.Digest {
		return item, nil
	}
	if other, ok := holder(items, digest); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s", ErrDuplicate, other.ID)
	}
	destination := PDFPath(root, id, digest)
	copied := false
	// A different snapshot may already own the same blob. Verify it before
	// reusing it; never overwrite a corrupt or missing attachment silently.
	if other, ok := holder(items, digest); ok && other.ID == id {
		actual, err := FileDigest(destination)
		if err != nil {
			return Item{}, err
		}
		if actual != digest {
			return Item{}, fmt.Errorf("stored PDF digest mismatch")
		}
	} else {
		result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{Source: source, Destination: destination})
		if err != nil {
			return Item{}, err
		}
		copied = true
		if result.Digest != digest {
			_ = os.Remove(destination)
			return Item{}, fmt.Errorf("source changed while attaching PDF")
		}
	}
	next := item.saveSnapshot(typ, item.FieldsAt(ref), digest, filepath.Base(source), now)
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "attach_pdf", Digest: next, Changes: map[string][2]string{"pdf": {base.Digest, digest}}})
	if err := WriteItem(root, item); err != nil {
		if copied {
			_ = os.Remove(destination)
		}
		return Item{}, err
	}
	return item, nil
}
