package tree

import (
	"fmt"
	"maps"
	"time"
)

// ChangeType creates a new snapshot of HEAD using the target Template.
// Earlier revisions retain their original type and complete field values.
func ChangeType(root, id string, target Template, fields map[string]string, revisions map[string]map[string]string, now time.Time) (Item, error) {
	if err := Require(root); err != nil {
		return Item{}, err
	}
	if err := target.Validate(); err != nil {
		return Item{}, err
	}
	item, items, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if item.Type == target.Type {
		return Item{}, fmt.Errorf("Item already has type %s", target.Type)
	}
	if item.Kind != target.Kind {
		return Item{}, fmt.Errorf("cannot change kind from %s to %s", item.Kind, target.Kind)
	}
	for key := range fields {
		if !target.has(key) || target.field(key).PerRevision {
			return Item{}, fmt.Errorf("%s is not an Item field of %s", key, target.Type)
		}
	}
	for digest := range revisions {
		found := false
		for _, rev := range item.Revisions {
			if rev.Ref() == digest {
				found = true
				break
			}
		}
		if !found {
			return Item{}, fmt.Errorf("unknown revision %s", digest)
		}
	}
	before := HistoryEvent{At: now.Format(time.RFC3339), Action: "change_type", FromType: item.Type, ToType: target.Type,
		PreviousFields: maps.Clone(item.Fields), PreviousRevisions: map[string]map[string]string{}}
	for _, rev := range item.Revisions {
		before.PreviousRevisions[rev.Ref()] = maps.Clone(rev.Fields)
	}
	given := maps.Clone(fields)
	if given == nil {
		given = map[string]string{}
	}
	for key, value := range revisions[item.Current()] {
		if !target.has(key) || !target.field(key).PerRevision {
			return Item{}, fmt.Errorf("%s is not a revision field of %s", key, target.Type)
		}
		given[key] = value
	}
	clean, err := CleanFields(target, given)
	if err != nil {
		return Item{}, err
	}
	if err := linked(target, items, clean, id); err != nil {
		return Item{}, err
	}
	if other, ok := taken(target, items, clean, id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	base, ok := item.Revision(item.Current())
	if !ok {
		return Item{}, fmt.Errorf("no current revision")
	}
	next := item
	before.Digest = next.saveSnapshot(target.Type, clean, base.Digest, base.Source, now)
	before.NewFields = maps.Clone(next.Fields)
	before.NewRevisions = map[string]map[string]string{next.Current(): maps.Clone(clean)}
	next.History = append(append([]HistoryEvent(nil), item.History...), before)
	return next, WriteItem(root, next)
}
