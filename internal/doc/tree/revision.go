package tree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/tag"
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
	Notes *string
	Tags  *[]string
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
		Digest: digest, Added: now.Format(time.RFC3339), Source: filepath.Base(source), Fields: fields,
	})
	item.Head = digest
	event := HistoryEvent{At: now.Format(time.RFC3339), Action: "import_revision", Digest: digest, Changes: map[string][2]string{}}
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
	item.History = append(item.History, event)
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
			if item.Head == digest {
				return item, nil
			}
			item.Head = digest
			item.History = append(item.History, HistoryEvent{At: time.Now().Format(time.RFC3339), Action: "make_head", Digest: digest})
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
	item, _, err := FindItem(root, id)
	if err != nil {
		return Item{}, "", err
	}
	if item.Kind != KindDocument || len(item.Revisions) < 2 {
		return Item{}, "", fmt.Errorf("delete the Item to remove its last revision")
	}
	at := -1
	for i, r := range item.Revisions {
		if r.Digest == digest {
			at = i
			break
		}
	}
	if at < 0 {
		return Item{}, "", fmt.Errorf("%s %s has no revision %s", item.Type, id, digest)
	}
	from := PDFPath(root, id, digest)
	if err := os.MkdirAll(filepath.Join(root, TrashDir), 0o755); err != nil {
		return Item{}, "", err
	}
	to := filepath.Join(root, TrashDir, id+"-"+digest+"-"+now.UTC().Format("20060102T150405Z")+".pdf")
	if _, err := os.Lstat(to); err == nil {
		return Item{}, "", fmt.Errorf("%s is already in the trash", to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Item{}, "", err
	}
	if err := os.Rename(from, to); err != nil {
		return Item{}, "", err
	}
	imported := false
	for _, event := range item.History {
		if (event.Action == "import" || event.Action == "import_revision") && event.Digest == digest {
			imported = true
			break
		}
	}
	if !imported && item.Revisions[at].Added != "" {
		item.History = append(item.History, HistoryEvent{At: item.Revisions[at].Added, Action: "import", Digest: digest})
	}
	item.Revisions = append(item.Revisions[:at], item.Revisions[at+1:]...)
	if item.Head == digest {
		item.Head = item.Revisions[len(item.Revisions)-1].Digest
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "delete_revision", Digest: digest})
	if err := WriteItem(root, item); err != nil {
		if undo := os.Rename(to, from); undo != nil {
			return Item{}, "", fmt.Errorf("write Item: %v; restore PDF: %v", err, undo)
		}
		return Item{}, "", err
	}
	return item, to, nil
}

// SetFields replaces an Item's fields as one revision has them, checked
// against its Template as import checks them: required keys present, and no
// other document of the type with the same distinguishing fields. The
// per_revision values go to the revision with digest — "" is the Current
// one — and the rest to the Item.
func SetFields(root, id, digest string, template Template, given map[string]string) (Item, error) {
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
	if digest == "" {
		digest = item.Current()
	}
	at := -1
	for i, r := range item.Revisions {
		if r.Digest == digest {
			at = i
		}
	}
	if at < 0 {
		return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
	}
	own, perRevision := template.Split(fields)
	previous := item.FieldsAt(digest)
	item.Fields = own
	item.Revisions[at].Fields = perRevision
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
		item.History = append(item.History, HistoryEvent{At: time.Now().Format(time.RFC3339), Action: "edit_fields", Digest: digest, Changes: changes})
	}
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
	old := item.Notes
	item.Notes = strings.TrimSpace(notes)
	if old != item.Notes {
		item.History = append(item.History, HistoryEvent{At: time.Now().Format(time.RFC3339), Action: "edit_notes", Changes: map[string][2]string{"notes": {old, item.Notes}}})
	}
	return item, WriteItem(root, item)
}

// SetTags replaces an Item's tags in their canonical spelling.
func SetTags(root, id string, tags []string) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	item, _, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	old := strings.Join(item.Tags, ", ")
	item.Tags = tag.List(tags)
	if next := strings.Join(item.Tags, ", "); old != next {
		item.History = append(item.History, HistoryEvent{At: time.Now().Format(time.RFC3339), Action: "edit_tags", Changes: map[string][2]string{"tags": {old, next}}})
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
	if _, _, err := FindItem(root, id); err != nil {
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
