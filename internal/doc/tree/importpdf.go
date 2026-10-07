package tree

import (
	"context"
	"crypto/sha256"
	"dgs-toolbox/internal/doc/country"

	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/tag"
	"dgs-toolbox/internal/verifiedcopy"
)

// ImportRequest is one loose PDF, or images, to take into the tree as a new
// Item.
type ImportRequest struct {
	Source string
	// Folder is the selected in-place output directory for external files or images.
	Folder string
	// Path, when set, is where an external file is copied instead, relative
	// to the root. Nothing may be there yet.
	Path string
	// Reuse confirms the Item whose existing object this new Item will name.
	Reuse    string
	Replaces string
	// Pages are more images after Source, each a page of the PDF made from
	// them; empty for a PDF.
	Pages []string
	// KeepOriginal archives the images beside their PDF.
	KeepOriginal bool
	Template     Template
	Fields       map[string]string
	Notes        string
	Tags         []string
	// RevisionTags are the first revision's own. It starts with the Item's
	// tags, since at first the Item is that one revision; these are added.
	RevisionTags []string
	// SharedWith are the people besides the owner the Item belongs to.
	SharedWith []string
	Now        time.Time
}

// ErrDuplicate is returned when the PDF is already kept in the tree.
var ErrDuplicate = errors.New("already in the tree")

// ErrTaken is returned when a document of the same type and distinguishing
// fields exists: the PDF is a new revision of it, not a new Item.
var ErrTaken = errors.New("a document with these fields exists")

// Import creates an Item from a PDF or images. By Item copies under the digest;
// in place records an internal PDF or copies into the selected folder. Copies
// read back before publication; the sidecar follows. The source stays untouched.
func (st *Store) Import(ctx context.Context, request ImportRequest) (Item, error) {
	if err := st.Require(); err != nil {
		return Item{}, err
	}
	if request.Replaces != "" {
		if err := st.ValidateSupersession(request.Replaces, request.Template.Type, request.Fields); err != nil {
			return Item{}, err
		}
	}
	fields, err := CleanFields(request.Template, request.Fields)
	if err != nil {
		return Item{}, err
	}
	sources := append([]string{request.Source}, request.Pages...)
	if st.InPlace() {
		for _, source := range sources {
			if _, err := st.relativePath(source); err != nil {
				return Item{}, err
			}
		}
	}
	input, err := st.prepareInput(ctx, sources)
	if err != nil {
		return Item{}, err
	}
	defer input.cleanup()
	if !st.InPlace() {
		request.Source = input.pdf
	}
	digest, err := input.digest()
	if err != nil {
		return Item{}, err
	}
	items, err := st.others()
	if err != nil {
		return Item{}, err
	}
	var reused *Revision
	other, duplicate := holder(items, digest)
	if st.InPlace() && request.Reuse != "" {
		other, err = st.GetItem(request.Reuse)
		if err != nil {
			return Item{}, err
		}
		duplicate = true
	}
	if duplicate {
		if !st.InPlace() {
			return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
		}
		file, err := st.ReadPDF(other, digest)
		if err != nil {
			return Item{}, err
		}
		actual, err := FileDigest(file)
		if err != nil {
			return Item{}, err
		}
		if actual != digest {
			return Item{}, fmt.Errorf("existing object no longer matches its recorded digest")
		}
		if request.Reuse == "" {
			return Item{}, &ReuseFile{Item: other.ID, Digest: digest, Path: Placing(other).Paths[digest]}
		}
		for _, r := range other.Revisions {
			if r.Digest == digest {
				reused = &r
				break
			}
		}
	}
	if other, ok := input.heldBy(items); ok && reused == nil {
		return Item{}, fmt.Errorf("%w: %s %s", ErrDuplicate, other.Type, other.ID)
	}
	if other, ok := taken(request.Template, items, fields, request.Replaces); ok {
		return Item{}, fmt.Errorf("%w: %s %s; add the PDF to it as a revision", ErrTaken, other.Type, other.ID)
	}
	if err := st.linked(request.Template, items, fields, ""); err != nil {
		return Item{}, err
	}
	shared, err := sharedList(fields["owner"], request.SharedWith)
	if err != nil {
		return Item{}, err
	}
	id, err := NewID(request.Now)
	if err != nil {
		return Item{}, err
	}
	item := Item{ID: id, Type: request.Template.Type, Kind: request.Template.Kind, Notes: strings.TrimSpace(request.Notes), Tags: tag.List(request.Tags), SharedWith: shared}
	if st.InPlace() {
		item.Locked = request.Template.Locked
	}
	ref := item.saveSnapshot(request.Template.Type, fields, digest, input.name, "", request.Now)
	item.Revisions[len(item.Revisions)-1].Tags = tag.List(append(append([]string(nil), item.Tags...), request.RevisionTags...))
	item.History = []HistoryEvent{{At: request.Now.Format(time.RFC3339), Action: "import", Digest: ref}}
	if st.InPlace() {
		return st.importInPlace(ctx, request, input, item, digest, reused)
	}
	// The Item's folder is new; a failed import leaves nothing behind, not
	// even the folder. Remove takes only an empty directory.
	result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source:      request.Source,
		Destination: st.PDF(Item{ID: id}, digest),
	})
	if err != nil {
		_ = os.Remove(filepath.Dir(st.Sidecar(id)))
		return Item{}, err
	}
	if result.Digest != digest {
		_ = os.Remove(st.PDF(Item{ID: id}, digest))
		_ = os.Remove(filepath.Dir(st.Sidecar(id)))
		return Item{}, fmt.Errorf("%s changed while it was being imported", request.Source)
	}
	input.size = result.Size
	undoOriginal, err := input.archive(ctx, st, item, request.KeepOriginal, nil)
	if err != nil {
		os.Remove(st.PDF(Item{ID: id}, digest))
		os.Remove(filepath.Dir(st.Sidecar(id)))
		return Item{}, err
	}
	input.annotate(&item, ref, request.KeepOriginal)
	if err := st.WriteItem(item); err != nil {
		undoOriginal()
		_ = os.Remove(st.PDF(Item{ID: id}, digest))
		_ = os.Remove(filepath.Dir(st.Sidecar(id)))
		return Item{}, err
	}
	return item, nil
}

// CleanFields keeps the Template's keys, trimmed, fills defaults for empty
// ones, and refuses a missing required key. Empty keys are dropped.
func CleanFields(t Template, given map[string]string) (map[string]string, error) {
	fields := map[string]string{}
	var missing []string
	for _, f := range t.Fields {
		value := strings.TrimSpace(given[f.Key])
		if value == "" {
			value = strings.TrimSpace(t.Defaults[f.Key])
		}
		if value == "" {
			if f.Required {
				missing = append(missing, f.Key)
			}
			continue
		}
		kept, err := f.clean(value)
		if err != nil {
			return nil, err
		}
		fields[f.Key] = kept
	}
	for key := range given {
		if !t.has(key) {
			return nil, fmt.Errorf("type %s has no field %s", t.Type, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("missing %s", strings.Join(missing, ", "))
	}
	return fields, nil
}

func (t Template) has(key string) bool {
	for _, f := range t.Fields {
		if f.Key == key {
			return true
		}
	}
	return false
}

// SameDistinguishing reports whether a and b agree, ignoring case, on every
// field t marks distinguishing: whether they name the same document. A
// country agrees however either side writes it: AU and 澳大利亚 are one.
func SameDistinguishing(t Template, a, b map[string]string) bool {
	for _, f := range t.Fields {
		if f.Distinguishing && !sameValue(f, a[f.Key], b[f.Key]) {
			return false
		}
	}
	return true
}

func sameValue(f Field, a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	if f.Type == FieldCountry {
		ca, okA := country.Find(a)
		cb, okB := country.Find(b)
		return okA && okB && ca == cb
	}
	return false
}

// FileDigest is a file's SHA-256 in lowercase hex.
func FileDigest(path string) (string, error) {
	digest, _, err := fileDigest(path)
	return digest, err
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

// linked checks that each item field names an Item in the tree, and each
// revision field one of its revisions, other than the Item itself.
func (st *Store) linked(t Template, items []Item, fields map[string]string, self string) error {
	var anchors map[string]bool
	for _, f := range t.Fields {
		value, ok := fields[f.Key]
		if !ok || value == "" || (f.Type != FieldItem && f.Type != FieldRevision) {
			continue
		}
		id, ref := value, ""
		if f.Type == FieldRevision {
			id, ref, _ = SplitRevisionLink(value)
		}
		if id == self {
			return fmt.Errorf("%s: an Item cannot link to itself", f.Key)
		}
		var found *Item
		for i := range items {
			if items[i].ID == id {
				found = &items[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("%s: no Item %s in this tree", f.Key, id)
		}
		if ref != "" && !found.hasRevision(ref) {
			return fmt.Errorf("%s: Item %s has no revision %s", f.Key, id, ref)
		}
		for theirs, ours := range f.Match {
			want := MatchValue(ours, fields)
			if theirs == AnchorKey && anchors == nil {
				templates, err := st.LoadTemplates()
				if err != nil {
					return err
				}
				anchors = AnchorTypes(templates)
			}
			got := Theirs(theirs, *found, anchors)
			if want != "" && got != want {
				return fmt.Errorf("%s: Item %s has %s %q, not the %q %s says", f.Key, id, theirs, got, want, ours)
			}
		}
	}
	return nil
}

func (i Item) hasRevision(ref string) bool {
	for _, r := range i.Revisions {
		if matchesRef(r, ref) {
			return true
		}
	}
	return false
}
