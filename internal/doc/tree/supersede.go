package tree

import (
	"dgs-toolbox/internal/doc/country"
	"fmt"
	"strings"
	"time"
)

// ValidateSupersession checks a manually selected predecessor before import.
func ValidateSupersession(root, oldID, typ string, fields map[string]string) error {
	old, _, err := FindItem(root, oldID)
	if err != nil {
		return err
	}
	if typ != "visa" || old.Type != typ {
		return fmt.Errorf("only visas can replace visas")
	}
	a, okA := country.Normalize(fields["country"], country.Format("alpha2"))
	b, okB := country.Normalize(old.Fields["country"], country.Format("alpha2"))
	if strings.TrimSpace(fields["owner"]) == "" || fields["owner"] != old.Fields["owner"] || !okA || !okB || a != b {
		return fmt.Errorf("choose a visa for the same owner and country")
	}
	if old.Retired || old.SupersededBy != "" {
		return fmt.Errorf("the selected visa is no longer in use")
	}
	return nil
}

// Supersede stores the relationship on the predecessor in a single sidecar
// write, together with retirement. The reverse link is derived from Items.
func Supersede(root, oldID, newID string, now time.Time) (Item, error) {
	if oldID == newID {
		return Item{}, fmt.Errorf("a visa cannot replace itself")
	}
	next, _, err := FindItem(root, newID)
	if err != nil {
		return Item{}, err
	}
	if next.Retired {
		return Item{}, fmt.Errorf("the new visa is retired")
	}
	if err := ValidateSupersession(root, oldID, next.Type, next.Fields); err != nil {
		return Item{}, err
	}
	items, err := LoadItems(root)
	if err != nil {
		return Item{}, err
	}
	for _, it := range items {
		if it.SupersededBy == newID {
			return Item{}, fmt.Errorf("this visa already replaces another visa")
		}
	}
	old, _, err := FindItem(root, oldID)
	if err != nil {
		return Item{}, err
	}
	old.SupersededBy = newID
	old.Retired = true
	old.RetiredReason = "Superseded by " + newID
	old.History = append(old.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "supersede", Target: newID})
	return old, WriteItem(root, old)
}

// UndoSupersession restores the predecessor to use without changing snapshots.
func UndoSupersession(root, id string, now time.Time) (Item, error) {
	old, _, err := FindItem(root, id)
	if err != nil {
		return Item{}, err
	}
	if old.SupersededBy == "" {
		return Item{}, fmt.Errorf("this item has no replacement")
	}
	old.History = append(old.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "undo_supersede", Target: old.SupersededBy})
	old.SupersededBy = ""
	old.Retired = false
	old.RetiredReason = ""
	return old, WriteItem(root, old)
}
