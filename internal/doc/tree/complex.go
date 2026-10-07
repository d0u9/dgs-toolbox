package tree

import (
	"context"
	"dgs-toolbox/internal/tag"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// addInPlaceRevision keeps native imports separate from doc 1's digest folder.
func (st *Store) addInPlaceRevision(ctx context.Context, id, source string, template Template, given map[string]string, m RevisionMetadata, now time.Time) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	item, items, err := st.FindItem(id)
	if err != nil {
		return Item{}, err
	}
	if item.Kind != KindDocument || item.Type != template.Type {
		return Item{}, fmt.Errorf("revision requires a document of the same type")
	}
	fields, err := RevisionFields(template, given)
	if err != nil {
		return Item{}, err
	}
	full, _ := template.Split(item.CurrentFields())
	for k, v := range fields {
		full[k] = v
	}
	if other, ok := taken(template, items, full, id); ok {
		return Item{}, fmt.Errorf("%w: %s", ErrTaken, other.ID)
	}
	sources := append([]string{source}, m.Pages...)
	for _, s := range sources {
		if _, err := st.relativePath(s); err != nil {
			return Item{}, err
		}
	}
	input, err := st.prepareInput(ctx, sources)
	if err != nil {
		return Item{}, err
	}
	defer input.cleanup()
	digest, err := input.digest()
	if err != nil {
		return Item{}, err
	}
	if other, ok := holder(items, digest); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s", ErrDuplicate, other.ID)
	}
	if other, ok := input.heldBy(items); ok && other.ID != id {
		return Item{}, fmt.Errorf("%w: %s", ErrDuplicate, other.ID)
	}
	if digest == item.CurrentDigest() {
		item.saveSnapshot(template.Type, full, digest, input.name, item.PaperOf(item.Current()), now)
		if m.Notes != nil {
			item.Notes = strings.TrimSpace(*m.Notes)
		}
		if m.Tags != nil {
			item.Tags = tag.List(*m.Tags)
		}
		for i := range item.Revisions {
			if item.Revisions[i].Ref() == item.Current() {
				item.Revisions[i].Tags = tag.List(append(item.Revisions[i].Tags, m.RevisionTags...))
			}
		}
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_fields", Digest: item.Current()})
		input.annotate(&item, item.Current(), m.KeepOriginal)
		return item, st.WriteItem(item)
	}
	besides := ""
	if m.Suffix != "" {
		if strings.ContainsAny(m.Suffix, "/\\\x00") {
			return Item{}, fmt.Errorf("suffix cannot contain a path separator")
		}
		prev, _ := item.Revision(item.Current())
		if prev.Path == "" {
			return Item{}, fmt.Errorf("the current revision has no file to sit beside")
		}
		base := strings.TrimSuffix(prev.Path, path.Ext(prev.Path))
		besides = base + m.Suffix + strings.ToLower(filepath.Ext(source))
	}
	item.saveSnapshot(template.Type, full, digest, input.name, "", now)
	if m.Notes != nil {
		item.Notes = strings.TrimSpace(*m.Notes)
	}
	if m.Tags != nil {
		item.Tags = tag.List(*m.Tags)
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "import_revision", Digest: item.Current()})
	for i := range item.Revisions {
		if item.Revisions[i].Ref() == item.Current() {
			item.Revisions[i].Tags = tag.List(m.RevisionTags)
		}
	}
	return st.importInPlace(ctx, ImportRequest{Source: source, Folder: m.Folder, Path: besides, KeepOriginal: m.KeepOriginal, Now: now}, input, item, digest, nil)
}
