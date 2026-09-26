package tree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"time"
)

// SnapshotID hashes canonical field values, type and the attachment digest.
// JSON sorts map keys. Timestamps, source filenames, notes and tags are not
// content: identical snapshots have identical IDs.
func SnapshotID(typ string, fields map[string]string, digest string) string {
	clean := map[string]string{}
	for k, v := range fields {
		if v != "" {
			clean[k] = v
		}
	}
	data, _ := json.Marshal(struct {
		Version int               `json:"version"`
		Type    string            `json:"type"`
		Fields  map[string]string `json:"fields"`
		PDF     string            `json:"pdf"`
	}{1, typ, clean, digest})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// freeze preserves legacy revisions' effective fields before Item fields
// change. Their existing references remain valid for Cases and history.
func (item *Item) freeze() {
	for n, r := range item.Revisions {
		if r.Snapshot {
			continue
		}
		item.Revisions[n].Fields = item.FieldsAt(r.Ref())
		item.Revisions[n].Type = item.Type
		item.Revisions[n].Snapshot = true
	}
}

func (item *Item) saveSnapshot(typ string, fields map[string]string, digest, source string, now time.Time) string {
	item.freeze()
	ref := SnapshotID(typ, fields, digest)
	exists := false
	for _, r := range item.Revisions {
		if r.Ref() == ref {
			exists = true
			break
		}
	}
	if !exists {
		item.Revisions = append(item.Revisions, Revision{ID: ref, Type: typ, Snapshot: true, Digest: digest, Source: source, Fields: maps.Clone(fields), Added: now.Format(time.RFC3339)})
	}
	item.Type = typ
	item.Fields = maps.Clone(fields)
	item.Head = ref
	return ref
}

func (item Item) Revision(ref string) (Revision, bool) {
	for _, r := range item.Revisions {
		if r.Ref() == ref {
			return r, true
		}
	}
	return Revision{}, false
}

// checkSnapshotIdentity applies the existing distinguishing-field rule when
// restoring historical content, just as a field edit does.
func checkSnapshotIdentity(root string, item Item, revision Revision, items []Item) error {
	if !revision.Snapshot {
		return nil
	}
	templates, err := LoadTemplates(root)
	if err != nil {
		return err
	}
	for _, t := range templates {
		if t.Type == revision.Type {
			if other, ok := taken(t, items, revision.Fields, item.ID); ok {
				return fmt.Errorf("%w: %s", ErrTaken, other.ID)
			}
		}
	}
	return nil
}
