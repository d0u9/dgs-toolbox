package tree

import (
	"dgs-toolbox/internal/doc/country"
	"fmt"
	"strings"
	"time"
)

// ValidateSupersession checks a manually selected predecessor before import:
// an in-place tree lets any type replace its own kind, another only a visa,
// and the two must share every field the Template's ReplacesKeys name.
func (st *Store) ValidateSupersession(oldID string, typ string, fields map[string]string) error {
	old, err := st.GetItem(oldID)
	if err != nil {
		return err
	}
	if (!st.InPlace() && typ != "visa") || old.Type != typ {
		return fmt.Errorf("only an Item of the same type can be replaced")
	}
	t := Template{Type: typ}
	templates, err := st.LoadTemplates()
	if err != nil {
		return err
	}
	for _, each := range templates {
		if each.Type == typ {
			t = each
		}
	}
	keys := t.ReplacesKeys()
	for _, key := range keys {
		f, ok := t.fieldOK(key)
		if !sameReplacing(fields[key], old.Fields[key], ok && f.Type == FieldCountry) {
			return fmt.Errorf("choose one with the same %s", strings.Join(keys, " and "))
		}
	}
	if old.Retired || old.SupersededBy != "" {
		return fmt.Errorf("the selected Item is no longer in use")
	}
	return nil
}

// sameReplacing says whether two values of a field replacing compares are
// both given and the same. Countries, when place, are compared by their
// code, however written, and one dgs does not know matches nothing.
func sameReplacing(a, b string, place bool) bool {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	if !place {
		return a == b
	}
	ca, okA := country.Normalize(a, country.Format("alpha2"))
	cb, okB := country.Normalize(b, country.Format("alpha2"))
	return okA && okB && ca == cb
}

// Supersede stores the relationship on the predecessor in a single sidecar
// write, together with retirement. The reverse link is derived from Items.
func (st *Store) Supersede(oldID string, newID string, now time.Time) (Item, error) {
	if oldID == newID {
		return Item{}, fmt.Errorf("an Item cannot replace itself")
	}
	next, err := st.GetItem(newID)
	if err != nil {
		return Item{}, err
	}
	if next.Retired {
		return Item{}, fmt.Errorf("the new Item is retired")
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
			return Item{}, fmt.Errorf("this Item already replaces another")
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
