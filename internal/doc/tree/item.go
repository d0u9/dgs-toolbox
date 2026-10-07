package tree

import (
	"dgs-toolbox/internal/tag"

	"slices"

	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// SidecarName is the file in each Item's folder that holds its metadata.
const SidecarName = "item.dgs-item.yaml"

// Revision is one issue of an Item, optionally with a PDF.
type Revision struct {
	Snapshot bool `yaml:"snapshot,omitempty" json:"snapshot,omitempty"`
	// Paper is the ref of the first snapshot of the revision this snapshot
	// belongs to: its own when it begins one, else it is a correction.
	Paper  string `yaml:"paper,omitempty" json:"paper,omitempty"`
	Type   string `yaml:"type,omitempty" json:"type,omitempty"`
	ID     string `yaml:"id,omitempty" json:"id,omitempty"`
	Digest string `yaml:"digest" json:"digest"`
	Size   *int64 `yaml:"size,omitempty" json:"size,omitempty"`
	Path   string `yaml:"path,omitempty" json:"path,omitempty"`
	Lost   bool   `yaml:"lost,omitempty" json:"lost,omitempty"`
	Added  string `yaml:"added" json:"added"`
	// Source is the name the PDF had when it was imported, for a person
	// wondering where it came from.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	// SourceDigests are the SHA-256 of every image the PDF was made from,
	// so they are known again even when no original was kept.
	SourceDigests []string `yaml:"source_digests,omitempty" json:"source_digests,omitempty"`
	// Originals are those images, kept beside the PDF, in page order.
	Originals []Original `yaml:"originals,omitempty" json:"originals,omitempty"`
	// Fields are this revision's own values of the fields its Template marks
	// per_revision: a renewed card's number and expiry.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty"`
	// Tags are this revision's own, beside the Item's: a reissue, a copy.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
}

// Ref identifies this snapshot; legacy PDF revisions use their digest.
func (r Revision) Ref() string {
	if r.ID != "" {
		return r.ID
	}
	return r.Digest
}

// Item is one sidecar.
type Item struct {
	ID     string            `yaml:"id" json:"id"`
	Type   string            `yaml:"type" json:"type"`
	Kind   Kind              `yaml:"kind" json:"kind"`
	Fields map[string]string `yaml:"fields" json:"fields"`
	Tags   []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	// Frequent marks one of the few Items the owner opens often. Browse puts
	// them first and can show them alone.
	Frequent bool `yaml:"frequent,omitempty" json:"frequent,omitempty"`
	Locked   bool `yaml:"locked,omitempty" json:"locked,omitempty"`
	// Retired marks an Item no longer used — a card for a place or an
	// organisation the owner has left — though it has not expired. Browse
	// shows it faded and last; exports are not affected. RetiredReason is
	// the owner's optional note on why.
	// SharedWith names the people besides the owner the Item belongs to — a
	// letter addressed to two — sorted, never the owner. Outlines select it
	// for them only in a rule that asks.
	SharedWith    []string   `yaml:"shared_with,omitempty" json:"shared_with,omitempty"`
	SupersededBy  string     `yaml:"superseded_by,omitempty" json:"superseded_by,omitempty"`
	Retired       bool       `yaml:"retired,omitempty" json:"retired,omitempty"`
	RetiredReason string     `yaml:"retired_reason,omitempty" json:"retired_reason,omitempty"`
	Head          string     `yaml:"head,omitempty" json:"head,omitempty"`
	Revisions     []Revision `yaml:"revisions" json:"revisions"`
	// Notes is free text the owner writes. It is never a key and never
	// exported.
	Notes   string         `yaml:"notes,omitempty" json:"notes,omitempty"`
	History []HistoryEvent `yaml:"history,omitempty" json:"history,omitempty"`
}

// HistoryEvent records a completed operation on an Item. Older sidecars have
// no history; their revision Added timestamps still identify past imports.
type HistoryEvent struct {
	At                string                       `yaml:"at" json:"at"`
	Action            string                       `yaml:"action" json:"action"`
	Digest            string                       `yaml:"digest,omitempty" json:"digest,omitempty"`
	Target            string                       `yaml:"target,omitempty" json:"target,omitempty"`
	View              string                       `yaml:"view,omitempty" json:"view,omitempty"`
	Changes           map[string][2]string         `yaml:"changes,omitempty" json:"changes,omitempty"`
	FromType          string                       `yaml:"from_type,omitempty" json:"from_type,omitempty"`
	ToType            string                       `yaml:"to_type,omitempty" json:"to_type,omitempty"`
	PreviousFields    map[string]string            `yaml:"previous_fields,omitempty" json:"previous_fields,omitempty"`
	PreviousRevisions map[string]map[string]string `yaml:"previous_revisions,omitempty" json:"previous_revisions,omitempty"`
	NewFields         map[string]string            `yaml:"new_fields,omitempty" json:"new_fields,omitempty"`
	NewRevisions      map[string]map[string]string `yaml:"new_revisions,omitempty" json:"new_revisions,omitempty"`
}

// Current is the revision reference that stands for the Item: HEAD for a
// document, the one revision for a record.
func (i Item) Current() string {
	if i.Head != "" {
		return i.Head
	}
	if len(i.Revisions) > 0 {
		return i.Revisions[len(i.Revisions)-1].Ref()
	}
	return ""
}

// FieldsAt is the Item's fields as one revision has them: the Item's own,
// with the revision's per_revision values over them. A value a sidecar
// keeps at the Item for a per_revision key, from before the key was one,
// stands for every revision that has none of its own.
func (i Item) FieldsAt(digest string) map[string]string {
	for _, r := range i.Revisions {
		if r.Ref() == digest && r.Snapshot {
			out := make(map[string]string, len(r.Fields))
			for k, v := range r.Fields {
				out[k] = v
			}
			return out
		}
	}
	out := make(map[string]string, len(i.Fields))
	for k, v := range i.Fields {
		out[k] = v
	}
	for _, r := range i.Revisions {
		if matchesRef(r, digest) {
			for k, v := range r.Fields {
				out[k] = v
			}
		}
	}
	return out
}

// CurrentFields is FieldsAt the Current revision.
func (i Item) CurrentFields() map[string]string { return i.FieldsAt(i.Current()) }

// LoadItems reads every sidecar the layout lists, sorted by ID. A
// sidecar that cannot be read is an error rather than a gap: a list missing
// an Item looks complete.
func (st *Store) LoadItems() ([]Item, error) {
	names, err := st.Sidecars()
	if err != nil {
		return nil, err
	}
	// Sidecars are read LoadWorkers at a time: on a network share each read
	// waits on the round trip, not the disk.
	found := make([]*Item, len(names))
	errs := make([]error, len(names))
	next := make(chan int)
	var wg sync.WaitGroup
	for range min(LoadWorkers, len(names)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				found[i], errs[i] = st.readItem(names[i])
			}
		}()
	}
	for i := range names {
		next <- i
	}
	close(next)
	wg.Wait()
	var items []Item
	for i := range names {
		if errs[i] != nil {
			return nil, errs[i]
		}
		if found[i] != nil {
			items = append(items, *found[i])
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// LoadWorkers is how many sidecars LoadItems reads at once.
const LoadWorkers = 16

// loadItem reads the Item id's sidecar; none there is no Item.
func (st *Store) loadItem(id string) (*Item, error) { return st.readItem(st.Sidecar(id)) }

// readItem reads the sidecar at path; none there is no Item. The sidecar
// must be where the layout places its ID's.
func (st *Store) readItem(path string) (*Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var item Item
	if err := yaml.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if item.ID == "" || st.Sidecar(item.ID) != path {
		return nil, fmt.Errorf("%s: id %q is not the folder's name", path, item.ID)
	}
	if err := st.ValidateItemPaths(&item); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// A sidecar from before revisions were told from snapshots is read by
	// its history; that is written the next time the sidecar is.
	item.settlePapers()
	return &item, nil
}

// WriteItem replaces an Item's sidecar through a temporary file and a rename,
// so a reader sees the old sidecar or the new one and never half of either.
func (st *Store) WriteItem(item Item) error {
	item.Revisions = slices.Clone(item.Revisions)
	for i := range item.Revisions {
		item.Revisions[i].Originals = slices.Clone(item.Revisions[i].Originals)
	}
	if err := st.ValidateItemPaths(&item); err != nil {
		return err
	}
	item.settlePapers()
	data, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	return WriteFileAtomic(st.Sidecar(item.ID), data)
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// NewID is a ULID: 48 bits of milliseconds and 80 random bits, so IDs made on
// two machines do not collide and sort by when they were made.
func NewID(now time.Time) (string, error) {
	var b [16]byte
	ms := uint64(now.UnixMilli())
	for i := 5; i >= 0; i-- {
		b[i] = byte(ms)
		ms >>= 8
	}
	if _, err := rand.Read(b[6:]); err != nil {
		return "", err
	}
	// 128 bits as 26 base-32 digits, the first carrying only two bits.
	out := make([]byte, 26)
	var acc uint64
	bits := 2 // pad so 130 bits divide into 26 digits of 5
	idx := 0
	acc = 0
	for _, by := range b {
		acc = acc<<8 | uint64(by)
		bits += 8
		for bits >= 5 {
			bits -= 5
			out[idx] = crockford[(acc>>uint(bits))&31]
			idx++
		}
	}
	return string(out), nil
}

// TagsAt is the tags a revision has: the Item's, which hold for every
// revision, and the revision's own, in canonical spelling.
func (item Item) TagsAt(digest string) []string {
	all := append([]string(nil), item.Tags...)
	for _, r := range item.Revisions {
		if matchesRef(r, digest) {
			all = append(all, r.Tags...)
		}
	}
	return tag.List(all)
}

// CurrentDigest returns the current attachment digest, or empty for no PDF.
func (i Item) CurrentDigest() string {
	for _, r := range i.Revisions {
		if r.Ref() == i.Current() {
			return r.Digest
		}
	}
	return ""
}

// matchesRef reports whether r is the revision ref names. A legacy revision
// is also named by its PDF digest; a hashed snapshot only by its ID, since
// several snapshots may share one PDF.
func matchesRef(r Revision, ref string) bool {
	return r.Ref() == ref || (ref != "" && r.ID == "" && r.Digest == ref)
}
