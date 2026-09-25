package tree

import (
	"fmt"
	"maps"
	"time"
)

// ChangeType reclassifies an Item using one target Template. The request
// supplies Item fields and each revision's own fields explicitly, so values
// the target does not define are only retained in the history snapshot.
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
			if rev.Digest == digest {
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
		before.PreviousRevisions[rev.Digest] = maps.Clone(rev.Fields)
	}
	next := item
	next.Type = target.Type
	next.Fields = nil
	next.Revisions = append([]Revision(nil), item.Revisions...)
	for i, rev := range item.Revisions {
		given := maps.Clone(fields)
		for key, value := range revisions[rev.Digest] {
			if !target.has(key) || !target.field(key).PerRevision {
				return Item{}, fmt.Errorf("%s is not a revision field of %s", key, target.Type)
			}
			given[key] = value
		}
		clean, err := CleanFields(target, given)
		if err != nil {
			return Item{}, fmt.Errorf("revision %s: %w", rev.Digest, err)
		}
		own, per := target.Split(clean)
		if next.Fields == nil {
			next.Fields = own
		} else if !maps.Equal(next.Fields, own) {
			return Item{}, fmt.Errorf("Item fields differ across revisions")
		}
		next.Revisions[i].Fields = per
		if err := linked(target, items, clean, id); err != nil {
			return Item{}, err
		}
	}
	if other, ok := taken(target, items, next.CurrentFields(), id); ok {
		return Item{}, fmt.Errorf("%w: %s %s", ErrTaken, other.Type, other.ID)
	}
	before.NewFields = maps.Clone(next.Fields)
	before.NewRevisions = map[string]map[string]string{}
	for _, rev := range next.Revisions {
		before.NewRevisions[rev.Digest] = maps.Clone(rev.Fields)
	}
	next.History = append(append([]HistoryEvent(nil), item.History...), before)
	return next, WriteItem(root, next)
}
