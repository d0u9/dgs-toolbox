package tree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
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

// saveSnapshot keeps a snapshot and moves HEAD to it. paper is the ref of
// the revision it corrects; empty, it begins a revision of its own. Content
// already kept reuses that snapshot, in the revision it is in.
func (item *Item) saveSnapshot(typ string, fields map[string]string, digest, source, paper string, now time.Time) string {
	item.freeze()
	item.settlePapers()
	ref := SnapshotID(typ, fields, digest)
	exists := false
	for _, r := range item.Revisions {
		if r.Ref() == ref {
			exists = true
			break
		}
	}
	if !exists {
		if paper == "" {
			paper = ref
		}
		next := Revision{ID: ref, Paper: paper, Type: typ, Snapshot: true, Digest: digest, Source: source, Fields: maps.Clone(fields), Added: now.Format(time.RFC3339)}
		for _, r := range item.Revisions {
			if r.Digest == digest && digest != "" {
				next.Path = r.Path
				next.Size = r.Size
				next.Lost, next.LossAccepted = r.Lost, r.LossAccepted
				next.SourceDigests = slices.Clone(r.SourceDigests)
				next.Originals = slices.Clone(r.Originals)
				break
			}
		}
		item.Revisions = append(item.Revisions, next)
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
func (st *Store) checkSnapshotIdentity(item Item, revision Revision, items []Item) error {
	if !revision.Snapshot {
		return nil
	}
	templates, err := st.LoadTemplates()
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
