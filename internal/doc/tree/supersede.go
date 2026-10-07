package tree

import (
	"dgs-toolbox/internal/doc/country"
	"fmt"
	"strings"
	"time"
)

// ValidateSupersession checks a manually selected predecessor before import.
func (st *Store) ValidateSupersession(oldID string, typ string, fields map[string]string) error {
	old, err := st.GetItem(oldID)
	if err != nil {
		return err
	}
	if (!st.InPlace() && typ != "visa") || old.Type != typ {
		return fmt.Errorf("only visas can replace visas")
	}
	a, okA := country.Normalize(fields["country"], country.Format("alpha2"))
	b, okB := country.Normalize(old.Fields["country"], country.Format("alpha2"))
	if owner := strings.TrimSpace(fields["owner"]); owner == "" || owner != strings.TrimSpace(old.Fields["owner"]) || !okA || !okB || a != b {
		return fmt.Errorf("choose a visa for the same owner and country")
	}
	if old.Retired || old.SupersededBy != "" {
		return fmt.Errorf("the selected visa is no longer in use")
	}
	return nil
}

// Supersede stores the relationship on the predecessor in a single sidecar
// write, together with retirement. The reverse link is derived from Items.
func (st *Store) Supersede(oldID string, newID string, now time.Time) (Item, error) {
	if oldID == newID {
		return Item{}, fmt.Errorf("a visa cannot replace itself")
	}
	next, err := st.GetItem(newID)
	if err != nil {
		return Item{}, err
	}
	if next.Retired {
		return Item{}, fmt.Errorf("the new visa is retired")
	}
	if err := st.ValidateSupersession(oldID, next.Type, next.Fields); err != nil {
		return Item{}, err
	}
	items, err := st.others()
	if err != nil {
		return Item{}, err
	}
	for _, it := range items {
		if it.SupersededBy == newID {
			return Item{}, fmt.Errorf("this visa already replaces another visa")
		}
	}
	old, err := st.GetItem(oldID)
	if err != nil {
		return Item{}, err
	}
	old.SupersededBy = newID
	old.Retired = true
	old.RetiredReason = "Superseded by " + newID
	old.History = append(old.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "supersede", Target: newID})
	return old, st.WriteItem(old)
}

// UndoSupersession restores the predecessor to use without changing snapshots.
func (st *Store) UndoSupersession(id string, now time.Time) (Item, error) {
	old, err := st.GetItem(id)
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
	return old, st.WriteItem(old)
}
