package tree

import (
	"dgs-toolbox/internal/tag"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Metadata sets only fields whose pointers are present. Empty values clear a
// field. All changes validate in memory before one sidecar publication.
type Metadata struct {
	Notes        *string   `json:"notes,omitempty"`
	Tags         *[]string `json:"tags,omitempty"`
	SharedWith   *[]string `json:"shared_with,omitempty"`
	Retired      *bool     `json:"retired,omitempty"`
	Reason       *string   `json:"reason,omitempty"`
	Frequent     *bool     `json:"frequent,omitempty"`
	RevisionTags *[]string `json:"revision_tags,omitempty"`
}

func (s *Store) SetMetadata(id, digest string, m Metadata, now time.Time) (Item, error) {
	if err := Require(s.root); err != nil {
		return Item{}, err
	}
	item, err := GetItem(s.root, id)
	if err != nil {
		return Item{}, err
	}
	if m.RevisionTags != nil && digest == "" {
		return Item{}, fmt.Errorf("revision_tags requires digest")
	}
	if m.Notes != nil {
		item, err = applyNotes(item, *m.Notes, now)
		if err != nil {
			return Item{}, err
		}
	}
	if m.Tags != nil {
		item, err = applyTags(item, *m.Tags, now)
		if err != nil {
			return Item{}, err
		}
	}
	if m.SharedWith != nil {
		item, err = applySharedWith(item, *m.SharedWith, now)
		if err != nil {
			return Item{}, err
		}
	}
	if m.Retired != nil || m.Reason != nil {
		retired, reason := item.Retired, item.RetiredReason
		if m.Retired != nil {
			retired = *m.Retired
		}
		if m.Reason != nil {
			reason = *m.Reason
		}
		item, err = applyRetired(item, retired, reason, now)
		if err != nil {
			return Item{}, err
		}
	}
	if m.Frequent != nil {
		item, err = applyFrequent(item, *m.Frequent, now)
		if err != nil {
			return Item{}, err
		}
	}
	if m.RevisionTags != nil {
		item, err = applyRevisionTags(item, digest, *m.RevisionTags, now)
		if err != nil {
			return Item{}, err
		}
	}
	return item, s.WriteItem(item)
}

func applyNotes(item Item, notes string, now time.Time) (Item, error) {
	old := item.Notes
	item.Notes = strings.TrimSpace(notes)
	if old != item.Notes {
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_notes", Changes: map[string][2]string{"notes": {old, item.Notes}}})
	}
	return item, nil
}

func applyTags(item Item, tags []string, now time.Time) (Item, error) {
	old := strings.Join(item.Tags, ", ")
	item.Tags = tag.List(tags)
	if next := strings.Join(item.Tags, ", "); old != next {
		item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "edit_tags", Changes: map[string][2]string{"tags": {old, next}}})
	}
	return item, nil
}

func applyFrequent(item Item, frequent bool, now time.Time) (Item, error) {
	if item.Frequent == frequent {
		return item, nil
	}
	item.Frequent = frequent
	action := "mark_frequent"
	if !frequent {
		action = "unmark_frequent"
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: action})
	return item, nil
}

func applyRetired(item Item, retired bool, reason string, now time.Time) (Item, error) {
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
	return item, nil
}

func applySharedWith(item Item, people []string, now time.Time) (Item, error) {
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
	return item, nil
}

func applyRevisionTags(item Item, digest string, tags []string, now time.Time) (Item, error) {
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
		return item, nil
	}
	return Item{}, fmt.Errorf("%s %s has no revision %s", item.Type, item.ID, digest)
}
