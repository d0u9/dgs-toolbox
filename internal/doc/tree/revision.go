package tree

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/tag"
	"dgs-toolbox/internal/verifiedcopy"
	"gopkg.in/yaml.v3"
)

// ErrNoItem is returned for an ID no sidecar has.
var ErrNoItem = errors.New("no such Item")

// GetItem reads the one sidecar of the Item id, and nothing else.
func GetItem(root, id string) (Item, error) {
	if id == "" || strings.ContainsAny(id, `/\`) || id == "." || id == ".." {
		return Item{}, fmt.Errorf("%w: %s", ErrNoItem, id)
	}
	item, err := loadItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item == nil {
		return Item{}, fmt.Errorf("%w: %s", ErrNoItem, id)
	}
	return *item, nil
}

// FindItem returns the Item with id, read from its sidecar, and every Item a
// write to it checks against.
func FindItem(root, id string) (Item, []Item, error) {
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, nil, err
	}
	items, err := others(root)
	if err != nil {
		return Item{}, nil, err
	}
	// The Item itself is as its sidecar says now, whatever the others' source.
	for i := range items {
		if items[i].ID == id {
			items[i] = item
		}
	}
	return item, items, nil
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
// HEAD to it — a renewed licence replaces the old one. given holds the new
// revision's values of the Template's per_revision fields. The PDF is
// published only after it reads back the same; the sidecar follows. A record
// has no revisions and is refused.
func AddRevision(ctx context.Context, root, id, source string, template Template, given map[string]string, now time.Time) (Item, error) {
	return AddRevisionWithMetadata(ctx, root, id, source, template, given, RevisionMetadata{}, now)
}

// RevisionMetadata optionally updates the Item's notes and tags when adding a
// revision. Nil fields preserve the values already on the Item.
type RevisionMetadata struct {
	// Pages are more images after the source, each a page of the PDF made
	// from them; KeepOriginal archives the images beside it.
	Pages        []string
	KeepOriginal bool
	Notes        *string
	Tags         *[]string
	// RevisionTags are the new revision's own tags.
	RevisionTags []string
}

// AddRevisionWithMetadata adds a revision and saves any Item metadata edits in
// the same sidecar write.
func AddRevisionWithMetadata(ctx context.Context, root, id, source string, template Template, given map[string]string, metadata RevisionMetadata, now time.Time) (Item, error) {
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
	if template.Type != item.Type {
		return Item{}, fmt.Errorf("%s %s is not a %s", item.Type, item.ID, template.Type)
	}
	fields, err := RevisionFields(template, given)
	if err != nil {
		return Item{}, err
	}
	input, err := prepareInput(ctx, root, append([]string{source}, metadata.Pages...))
	if err != nil {
		return Item{}, err
	}
	defer input.cleanup()
	source = input.pdf
	digest, err := input.digest()
	if err != nil {
		return Item{}, err
	}
	if other, ok := holder(items, digest); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
	}
	if other, ok := input.heldBy(items); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
	}
	full, _ := template.Split(item.CurrentFields())
	for k, v := range fields {
		full[k] = v
	}
	if other, ok := taken(template, items, full, id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	destination := PDFPath(root, id, digest)
	copied := false
	if _, ok := holder(items, digest); ok {
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
		if result.Digest != digest {
			_ = os.Remove(destination)
			return Item{}, fmt.Errorf("%s changed while it was being imported", source)
		}
		copied = true
	}
	targetRef := SnapshotID(template.Type, full, digest)
	if err := input.preserveProvenance(item, targetRef, metadata.KeepOriginal); err != nil {
		if copied {
			os.Remove(destination)
		}
		return Item{}, err
	}
	needsSource, needsArchive := len(input.inputDigests) > 0, metadata.KeepOriginal && len(input.originals) > 0
	if current, ok := item.Revision(item.Current()); ok {
		needsSource = input.needsSource(current)
		needsArchive = needsArchive && len(current.Originals) == 0
	}
	if SnapshotID(item.Type, item.CurrentFields(), item.CurrentDigest()) == SnapshotID(template.Type, full, digest) && metadata.Notes == nil && metadata.Tags == nil && len(metadata.RevisionTags) == 0 && !needsArchive && !needsSource {
		return item, nil
	}
	_, reused := item.Revision(SnapshotID(template.Type, full, digest))
	ref := item.saveSnapshot(template.Type, full, digest, input.name, now)
	// A snapshot returned to keeps its own tags; given ones are added to them.
	for n := range item.Revisions {
		if item.Revisions[n].Ref() != ref {
			continue
		}
		if reused {
			item.Revisions[n].Tags = tag.List(append(append([]string(nil), item.Revisions[n].Tags...), metadata.RevisionTags...))
		} else {
			item.Revisions[n].Tags = tag.List(metadata.RevisionTags)
		}
	}
	event := HistoryEvent{At: now.Format(time.RFC3339), Action: "import_revision", Digest: ref, Changes: map[string][2]string{}}
	if metadata.Notes != nil {
		if next := strings.TrimSpace(*metadata.Notes); next != item.Notes {
			event.Changes["notes"] = [2]string{item.Notes, next}
		}
		item.Notes = strings.TrimSpace(*metadata.Notes)
	}
	if metadata.Tags != nil {
		if next := tag.List(*metadata.Tags); strings.Join(next, ", ") != strings.Join(item.Tags, ", ") {
			event.Changes["tags"] = [2]string{strings.Join(item.Tags, ", "), strings.Join(next, ", ")}
		}
		item.Tags = tag.List(*metadata.Tags)
	}
	if len(event.Changes) == 0 {
		event.Changes = nil
	}
	undoOriginal, err := input.archive(ctx, root, id, metadata.KeepOriginal)
	if err != nil {
		if copied {
			os.Remove(destination)
		}
		return Item{}, err
	}
	input.annotate(&item, ref, metadata.KeepOriginal)
	item.History = append(item.History, event)
	if err := WriteItem(root, item); err != nil {
		undoOriginal()
		if copied {
			os.Remove(destination)
		}
		return Item{}, err
	}
	return item, nil
}

// SetHead points HEAD at one of an Item's revisions, which is how a revision
// added by mistake is stepped back from. Nothing is removed. A document always
// has HEAD; a record has one once it holds a snapshot, and a legacy record
// without one is refused.
func SetHead(root, id, digest string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindDocument && item.Head == "" {
		return Item{}, fmt.Errorf("%s %s is a record: records have no HEAD", item.Type, item.ID)
	}
	for _, r := range item.Revisions {
		if r.Ref() == digest {
			if item.Head == digest {
				return item, nil
			}
			if err := checkSnapshotIdentity(root, item, r, items); err != nil {
				return Item{}, err
			}
			item.Head = digest
			if r.Snapshot {
				item.Type = r.Type
				item.Fields = maps.Clone(r.Fields)
			}
			item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "make_head", Digest: digest})
			return item, WriteItem(root, item)
		}
	}
	return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
}

// TrashRevision removes one document revision and moves its PDF to trash.
// The final revision belongs to the Item and must be deleted with Trash.
func TrashRevision(root, id, digest string, now time.Time) (Item, string, error) {
	if err := Require(root); err != nil {
		return Item{}, "", err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, "", err
	}
	if len(item.Revisions) < 2 {
		return Item{}, "", fmt.Errorf("delete the Item to remove its last revision")
	}
	at := -1
	for i, r := range item.Revisions {
		if r.Ref() == digest {
			at = i
			break
		}
	}
	if at < 0 {
		return Item{}, "", fmt.Errorf("%s %s has no revision %s", item.Type, id, digest)
	}
	if item.Head == digest {
		for n := len(item.Revisions) - 1; n >= 0; n-- {
			if n != at {
				if err := checkSnapshotIdentity(root, item, item.Revisions[n], items); err != nil {
					return Item{}, "", err
				}
				break
			}
		}
	}
	from := PDFPath(root, id, item.Revisions[at].Digest)
	hasPDF := item.Revisions[at].Digest != ""
	for n, r := range item.Revisions {
		if n != at && r.Digest == item.Revisions[at].Digest {
			hasPDF = false
		}
	}
	if err := os.MkdirAll(filepath.Join(root, TrashDir), 0o755); err != nil {
		return Item{}, "", err
	}
	to := filepath.Join(root, TrashDir, id+"-"+digest+"-"+now.UTC().Format("20060102T150405Z")+".pdf")
	if !hasPDF {
		to = strings.TrimSuffix(to, ".pdf") + ".yaml"
	}
	if _, err := os.Lstat(to); err == nil {
		return Item{}, "", fmt.Errorf("%s is already in the trash", to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Item{}, "", err
	}
	// Always retain the original sidecar, including field values. A PDF
	// without its snapshot metadata is not enough to restore a revision.
	metadataPath := strings.TrimSuffix(to, filepath.Ext(to)) + ".yaml"
	data, err := yaml.Marshal(item)
	if err != nil {
		return Item{}, "", err
	}
	file, err := os.OpenFile(metadataPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return Item{}, "", err
	}
	_, err = file.Write(data)
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(metadataPath)
		return Item{}, "", err
	}
	if hasPDF {
		if err := os.Rename(from, to); err != nil {
			_ = os.Remove(metadataPath)
			return Item{}, "", err
		}
	}
	// moved are the originals taken to the trash, to put back if the
	// sidecar cannot be written.
	var moved [][2]string
	putBack := func() error {
		var errs error
		for _, m := range moved {
			if err := os.Rename(m[1], m[0]); err != nil {
				errs = errors.Join(errs, fmt.Errorf("restore original: %w", err))
			}
		}
		if hasPDF {
			if err := os.Rename(to, from); err != nil {
				errs = errors.Join(errs, fmt.Errorf("restore PDF: %w", err))
			}
		}
		return errs
	}
	// An original goes with its revision unless another revision keeps it.
	for n, original := range item.Revisions[at].Originals {
		shared := false
		for i, r := range item.Revisions {
			if i != at && slices.ContainsFunc(r.Originals, func(o Original) bool { return o.Digest == original.Digest }) {
				shared = true
			}
		}
		if shared {
			continue
		}
		path := OriginalPath(root, id, original)
		trashed := fmt.Sprintf("%s-original-%d%s", strings.TrimSuffix(metadataPath, ".yaml"), n+1, original.Extension)
		err := error(nil)
		if path == "" {
			err = fmt.Errorf("invalid original image reference")
		} else if _, statErr := os.Lstat(trashed); !os.IsNotExist(statErr) {
			err = fmt.Errorf("original image trash destination is taken")
		} else {
			err = os.Rename(path, trashed)
		}
		if err != nil {
			if undo := putBack(); undo != nil {
				err = errors.Join(err, undo)
			}
			_ = os.Remove(metadataPath)
			return Item{}, "", err
		}
		moved = append(moved, [2]string{path, trashed})
	}
	imported := false
	for _, event := range item.History {
		if createsRevision(event.Action) && event.Digest == digest {
			imported = true
			break
		}
	}
	if !imported && item.Revisions[at].Added != "" {
		item.History = insertByTime(item.History, HistoryEvent{At: item.Revisions[at].Added, Action: "import", Digest: digest})
	}
	item.Revisions = append(item.Revisions[:at], item.Revisions[at+1:]...)
	if item.Head == digest {
		item.Head = item.Revisions[len(item.Revisions)-1].Ref()
		last := item.Revisions[len(item.Revisions)-1]
		if last.Snapshot {
			item.Fields = maps.Clone(last.Fields)
			item.Type = last.Type
		}
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "delete_revision", Digest: digest})
	if err := WriteItem(root, item); err != nil {
		if undo := putBack(); undo != nil {
			return Item{}, "", errors.Join(err, undo)
		}
		_ = os.Remove(metadataPath)
		return Item{}, "", err
	}
	return item, to, nil
}

// SetFields creates a snapshot from the selected revision with validated
// field values. Unchanged fields do nothing; older snapshots remain intact.
func SetFields(root, id, digest string, template Template, given map[string]string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	baseType := item.Type
	if r, ok := item.Revision(digest); ok && r.Type != "" {
		baseType = r.Type
	}
	if template.Type != baseType {
		return Item{}, fmt.Errorf("%s %s is not a %s", item.Type, item.ID, template.Type)
	}
	fields, err := CleanFields(template, given)
	if err != nil {
		return Item{}, err
	}
	if other, ok := taken(template, items, fields, id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	if err := linked(root, template, items, fields, id); err != nil {
		return Item{}, err
	}
	if digest == "" {
		digest = item.Current()
	}
	at := -1
	for i, r := range item.Revisions {
		if r.Ref() == digest {
			at = i
		}
	}
	if at < 0 {
		return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
	}
	previous := item.FieldsAt(digest)
	if maps.Equal(previous, fields) {
		return item, nil
	}
	base := item.Revisions[at]
	next := item.saveSnapshot(template.Type, fields, base.Digest, base.Source, now)
	changes := map[string][2]string{}
	for key, old := range previous {
		if old != fields[key] {
			changes[key] = [2]string{old, fields[key]}
		}
	}
	for key, value := range fields {
		if _, ok := previous[key]; !ok {
			changes[key] = [2]string{"", value}
		}
	}
	if len(changes) > 0 {
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_fields", Digest: next, Changes: changes})
	}
	return item, WriteItem(root, item)
}

// revisionActions are the history actions whose Digest names the revision
// they created. Any new action that creates a snapshot belongs here.
var revisionActions = map[string]bool{
	"import": true, "import_revision": true, "create_without_pdf": true,
	"create_revision_without_pdf": true, "attach_pdf": true, "edit_fields": true,
	"change_type": true,
}

func createsRevision(action string) bool { return revisionActions[action] }

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
func SetNotes(root, id, notes string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	old := item.Notes
	item.Notes = strings.TrimSpace(notes)
	if old != item.Notes {
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_notes", Changes: map[string][2]string{"notes": {old, item.Notes}}})
	}
	return item, WriteItem(root, item)
}

// SetTags replaces an Item's tags in their canonical spelling.
func SetTags(root, id string, tags []string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	old := strings.Join(item.Tags, ", ")
	item.Tags = tag.List(tags)
	if next := strings.Join(item.Tags, ", "); old != next {
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_tags", Changes: map[string][2]string{"tags": {old, next}}})
	}
	return item, WriteItem(root, item)
}

// TrashDir is where a deleted Item's folder goes. Nothing in the tree is
// erased: an Item deleted by mistake is moved back by hand.
const TrashDir = "trash"

// Trash deletes an Item from the tree by moving its folder — sidecar and
// every revision's PDF — into TrashDir, under its ID and when it was
// deleted. It answers where the folder went.
func Trash(root, id string, now time.Time) (string, error) {
	if err := Require(root); err != nil {
		return "", err
	}
	if _, err := GetItem(root, id); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(root, TrashDir), 0o755); err != nil {
		return "", err
	}
	to := filepath.Join(root, TrashDir, id+"-"+now.UTC().Format("20060102T150405Z"))
	if _, err := os.Lstat(to); err == nil {
		return "", fmt.Errorf("%s is already in the trash", to)
	}
	return to, os.Rename(Dir(root, id), to)
}

// SetFrequent marks or unmarks an Item as one the owner opens often.
func SetFrequent(root, id string, frequent bool, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Frequent == frequent {
		return item, nil
	}
	item.Frequent = frequent
	action := "mark_frequent"
	if !frequent {
		action = "unmark_frequent"
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: action})
	return item, WriteItem(root, item)
}

// SetRetired marks an Item no longer used, with an optional reason, or puts it
// back in use. Retiring an Item already retired changes only its reason.
func SetRetired(root, id string, retired bool, reason string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.SupersededBy != "" {
		return Item{}, fmt.Errorf("undo the replacement before changing retirement")
	}
	reason = strings.TrimSpace(reason)
	if !retired {
		reason = ""
	}
	if item.Retired == retired && item.RetiredReason == reason {
		return item, nil
	}
	event := HistoryEvent{At: now.Format(time.RFC3339), Action: "retire"}
	switch {
	case !retired:
		event.Action = "unretire"
	case item.Retired:
		event.Action = "edit_retired_reason"
	}
	if item.RetiredReason != reason {
		event.Changes = map[string][2]string{"reason": {item.RetiredReason, reason}}
	}
	item.Retired, item.RetiredReason = retired, reason
	item.History = append(item.History, event)
	return item, WriteItem(root, item)
}

// SetSharedWith replaces the people an Item is shared with besides its
// owner, trimmed, without repeats, sorted.
func SetSharedWith(root, id string, people []string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	shared, err := sharedList(item.CurrentFields()["owner"], people)
	if err != nil {
		return Item{}, err
	}
	if slices.Equal(shared, item.SharedWith) {
		return item, nil
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "share",
		Changes: map[string][2]string{"shared_with": {strings.Join(item.SharedWith, ", "), strings.Join(shared, ", ")}}})
	item.SharedWith = shared
	return item, WriteItem(root, item)
}

// sharedList is people trimmed, without repeats, sorted, refusing the
// owner.
func sharedList(owner string, people []string) ([]string, error) {
	owner = strings.TrimSpace(owner)
	var shared []string
	for _, p := range people {
		p = strings.TrimSpace(p)
		if p == "" || slices.Contains(shared, p) {
			continue
		}
		if strings.EqualFold(p, owner) {
			return nil, fmt.Errorf("%s owns this Item: share it with someone else", p)
		}
		shared = append(shared, p)
	}
	sort.Strings(shared)
	return shared, nil
}

// SetRevisionTags replaces one revision's own tags, in canonical spelling.
// The Item's tags are not touched.
func SetRevisionTags(root, id, digest string, tags []string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(root, id)
	if err != nil {
		return Item{}, err
	}
	for i, r := range item.Revisions {
		if r.Ref() != digest {
			continue
		}
		old := strings.Join(r.Tags, ", ")
		item.Revisions[i].Tags = tag.List(tags)
		if next := strings.Join(item.Revisions[i].Tags, ", "); old != next {
			item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_revision_tags", Digest: digest,
				Changes: map[string][2]string{"tags": {old, next}}})
		}
		return item, WriteItem(root, item)
	}
	return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
}
