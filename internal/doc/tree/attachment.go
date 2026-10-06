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
func (st *Store) CreateWithoutPDF(request ImportRequest) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	if err := request.Template.Validate(); err != nil {
		return Item{}, err
	}
	fields, err := CleanFields(request.Template, request.Fields)
	if err != nil {
		return Item{}, err
	}
	items, err := st.others()
	if err != nil {
		return Item{}, err
	}
	if other, ok := taken(request.Template, items, fields, ""); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	if err := st.linked(request.Template, items, fields, ""); err != nil {
		return Item{}, err
	}
	shared, err := sharedList(fields["owner"], request.SharedWith)
	if err != nil {
		return Item{}, err
	}
	id, err := NewID(request.Now)
	if err != nil {
		return Item{}, err
	}
	item := Item{ID: id, Type: request.Template.Type, Kind: request.Template.Kind, Notes: strings.TrimSpace(request.Notes), Tags: tag.List(request.Tags), SharedWith: shared}
	ref := item.saveSnapshot(request.Template.Type, fields, "", "", "", request.Now)
	item.History = []HistoryEvent{{At: request.Now.Format(time.RFC3339), Action: "create_without_pdf", Digest: ref}}
	if err := st.WriteItem(item); err != nil {
		_ = os.Remove(filepath.Dir(st.Sidecar(id)))
		return Item{}, err
	}
	return item, nil
}

// AddWithoutPDF records a new issue without requiring a scan.
func (st *Store) AddWithoutPDF(id string, template Template, given map[string]string, metadata RevisionMetadata, now time.Time) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	item, items, err := st.FindItem(id)
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
	if err := st.linked(template, items, fields, id); err != nil {
		return Item{}, err
	}
	full, _ := template.Split(item.CurrentFields())
	for k, v := range fields {
		full[k] = v
	}
	if other, ok := taken(template, items, full, id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	if item.CurrentDigest() == "" && SnapshotID(item.Type, item.CurrentFields(), "") == SnapshotID(item.Type, full, "") && (metadata.Notes == nil || strings.TrimSpace(*metadata.Notes) == item.Notes) && (metadata.Tags == nil || strings.Join(tag.List(*metadata.Tags), ",") == strings.Join(item.Tags, ",")) {
		return item, nil
	}
	ref := item.saveSnapshot(item.Type, full, "", "", metadata.paper(item), now)
	if metadata.Notes != nil {
		item.Notes = strings.TrimSpace(*metadata.Notes)
	}
	if metadata.Tags != nil {
		item.Tags = tag.List(*metadata.Tags)
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "create_revision_without_pdf", Digest: ref})
	return item, st.WriteItem(item)
}

// AttachPDF creates a new snapshot from the selected revision and a PDF.
// The selected revision and its references are never changed.
func (st *Store) AttachPDF(ctx context.Context, id string, ref string, source string, now time.Time) (Item, error) {
	return st.AttachFile(ctx, id, ref, []string{source}, false, now)
}

// AttachFile is AttachPDF for a PDF, or for images put together into one,
// a page each, their originals optionally archived beside it.
func (st *Store) AttachFile(ctx context.Context, id string, ref string, sources []string, keepOriginal bool, now time.Time) (Item, error) {
	return st.AttachFileAs(ctx, id, ref, sources, keepOriginal, false, now)
}

// AttachFileAs is AttachFile; with newRevision the snapshot begins a
// revision rather than correcting the one ref is in.
func (st *Store) AttachFileAs(ctx context.Context, id string, ref string, sources []string, keepOriginal bool, newRevision bool, now time.Time) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	item, items, err := st.FindItem(id)
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
	templates, err := st.LoadTemplates()
	if err != nil {
		return Item{}, err
	}
	for _, t := range templates {
		if t.Type == typ {
			if other, ok := taken(t, items, item.FieldsAt(ref), id); ok {
				return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
			}
		}
	}
	input, err := st.prepareInput(ctx, sources)
	if err != nil {
		return Item{}, err
	}
	defer input.cleanup()
	source := input.pdf
	digest, err := input.digest()
	if err != nil {
		return Item{}, err
	}
	if other, ok := input.heldBy(items); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s", ErrDuplicate, other.ID)
	}
	if digest == base.Digest {
		if err := input.preserveProvenance(item, ref, keepOriginal); err != nil {
			return Item{}, err
		}
		if (!keepOriginal || len(input.originals) == 0 || len(base.Originals) > 0) && !input.needsSource(base) {
			return item, nil
		}
		// Verify the reused PDF before changing only its original metadata.
		actual, err := FileDigest(st.PDF(Item{ID: id}, digest))
		if err != nil {
			return Item{}, err
		}
		if actual != digest {
			return Item{}, fmt.Errorf("stored PDF digest mismatch")
		}
		undo, err := input.archive(ctx, st, id, keepOriginal)
		if err != nil {
			return Item{}, err
		}
		input.annotate(&item, ref, keepOriginal)
		if err := st.WriteItem(item); err != nil {
			undo()
			return Item{}, err
		}
		return item, nil
	}
	if other, ok := holder(items, digest); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s", ErrDuplicate, other.ID)
	}
	destination := st.PDF(Item{ID: id}, digest)
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
	if err := input.preserveProvenance(item, SnapshotID(typ, item.FieldsAt(ref), digest), keepOriginal); err != nil {
		if copied {
			os.Remove(destination)
		}
		return Item{}, err
	}
	next := item.saveSnapshot(typ, item.FieldsAt(ref), digest, input.name, correcting(item, ref, newRevision), now)
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "attach_pdf", Digest: next, Changes: map[string][2]string{"pdf": {base.Digest, digest}}})
	undoOriginal, err := input.archive(ctx, st, id, keepOriginal)
	if err != nil {
		if copied {
			os.Remove(destination)
		}
		return Item{}, err
	}
	input.annotate(&item, next, keepOriginal)
	if err := st.WriteItem(item); err != nil {
		undoOriginal()
		if copied {
			_ = os.Remove(destination)
		}
		return Item{}, err
	}
	return item, nil
}
