package tree

import (
	"dgs-toolbox/internal/tag"

	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SidecarName is the file in each Item's folder that holds its metadata.
const SidecarName = "item.dgs-item.yaml"

// Revision is one PDF of an Item.
type Revision struct {
	Digest string `yaml:"digest" json:"digest"`
	Added  string `yaml:"added" json:"added"`
	// Source is the name the PDF had when it was imported, for a person
	// wondering where it came from.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	// Fields are this revision's own values of the fields its Template marks
	// per_revision: a renewed card's number and expiry.
	Fields map[string]string `yaml:"fields,omitempty" json:"fields,omitempty"`
	// Tags are this revision's own, beside the Item's: a reissue, a copy.
	Tags []string `yaml:"tags,omitempty" json:"tags,omitempty"`
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
	// Retired marks an Item no longer used — a card for a place or an
	// organisation the owner has left — though it has not expired. Browse
	// shows it faded and last; exports are not affected. RetiredReason is
	// the owner's optional note on why.
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

// Current is the digest of the PDF that stands for the Item: HEAD for a
// document, the one PDF for a record.
func (i Item) Current() string {
	if i.Head != "" {
		return i.Head
	}
	if len(i.Revisions) > 0 {
		return i.Revisions[len(i.Revisions)-1].Digest
	}
	return ""
}

// FieldsAt is the Item's fields as one revision has them: the Item's own,
// with the revision's per_revision values over them. A value a sidecar
// keeps at the Item for a per_revision key, from before the key was one,
// stands for every revision that has none of its own.
func (i Item) FieldsAt(digest string) map[string]string {
	out := make(map[string]string, len(i.Fields))
	for k, v := range i.Fields {
		out[k] = v
	}
	for _, r := range i.Revisions {
		if r.Digest == digest {
			for k, v := range r.Fields {
				out[k] = v
			}
		}
	}
	return out
}

// CurrentFields is FieldsAt the Current revision.
func (i Item) CurrentFields() map[string]string { return i.FieldsAt(i.Current()) }

// Dir is the Item's folder under root.
func Dir(root, id string) string { return filepath.Join(root, ItemsDir, id) }

// PDFPath is where a revision's PDF is kept.
func PDFPath(root, id, digest string) string {
	return filepath.Join(Dir(root, id), digest+".pdf")
}

// LoadItems reads every sidecar under root's items folder, sorted by ID. A
// sidecar that cannot be read is an error rather than a gap: a list missing
// an Item looks complete.
func LoadItems(root string) ([]Item, error) {
	entries, err := os.ReadDir(filepath.Join(root, ItemsDir))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var items []Item
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(root, ItemsDir, entry.Name(), SidecarName)
		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		var item Item
		if err := yaml.Unmarshal(data, &item); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if item.ID != entry.Name() {
			return nil, fmt.Errorf("%s: id %q is not the folder's name", path, item.ID)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// WriteItem replaces an Item's sidecar through a temporary file and a rename,
// so a reader sees the old sidecar or the new one and never half of either.
func WriteItem(root string, item Item) error {
	data, err := yaml.Marshal(item)
	if err != nil {
		return err
	}
	dir := Dir(root, item.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".item-*.dgs-part")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), filepath.Join(dir, SidecarName))
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
		if r.Digest == digest {
			all = append(all, r.Tags...)
		}
	}
	return tag.List(all)
}
