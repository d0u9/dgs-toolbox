// Package cases is a doc tree's Cases: a matter being dealt with — renewing a
// passport, applying for a lease — and the Items it has needed so far. A
// Case refers to Items and copies nothing; archiving it records which
// revision of each was the current one, so what was handed over stays known
// after a document is renewed. The rules are in docs/apps/doc/index.md.
package cases

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"

	"gopkg.in/yaml.v3"
)

// Dir is the folder under a tree's root that holds one file per Case.
const Dir = "cases"

// Status is where a Case is in its life.
type Status string

const (
	// Open is a Case still being dealt with: Items come and go.
	Open Status = "open"
	// Archived is a Case that is done. It changes no more until reopened.
	Archived Status = "archived"
)

// DefaultLayout names each PDF of a Case's export by its type, numbering
// two of one type as a View with dedupe: number does.
const DefaultLayout = "{type}.{ext}"

// Entry is one Item a Case uses.
type Entry struct {
	Item  string `yaml:"item" json:"item"`
	Added string `yaml:"added" json:"added"`
	// Note says why it was wanted: "the bank asked on 20 Sep".
	Note string `yaml:"note,omitempty" json:"note,omitempty"`
	// Digest is the revision that was current when the Case was archived.
	Digest string `yaml:"digest,omitempty" json:"digest,omitempty"`
}

// Need is something asked for that is not in the Case yet: "the last three
// electricity bills". It is met by an Item.
type Need struct {
	Text  string `yaml:"text" json:"text"`
	Added string `yaml:"added" json:"added"`
	// Item is the Item that met it, and Met when.
	Item string `yaml:"item,omitempty" json:"item,omitempty"`
	Met  string `yaml:"met,omitempty" json:"met,omitempty"`
}

// Case is one file under cases/.
type Case struct {
	Name   string `yaml:"name" json:"name"`
	Title  string `yaml:"title,omitempty" json:"title,omitempty"`
	Status Status `yaml:"status" json:"status"`
	Opened string `yaml:"opened" json:"opened"`
	// Archived is when it was last archived.
	Archived string `yaml:"archived,omitempty" json:"archived,omitempty"`
	Notes    string `yaml:"notes,omitempty" json:"notes,omitempty"`
	// Layout names the PDFs of an export, as a View's does. Empty is
	// DefaultLayout.
	Layout  string  `yaml:"layout,omitempty" json:"layout,omitempty"`
	Entries []Entry `yaml:"entries" json:"entries"`
	Needs   []Need  `yaml:"needs" json:"needs"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with c.
func (c Case) Validate() error {
	if !namePattern.MatchString(c.Name) {
		return fmt.Errorf("case %q: use lowercase letters, digits, _ and -", c.Name)
	}
	if c.Status != Open && c.Status != Archived {
		return fmt.Errorf("case %s: status %q is neither open nor archived", c.Name, c.Status)
	}
	if c.Layout != "" {
		if _, err := view.Parse(c.Layout); err != nil {
			return fmt.Errorf("case %s: %w", c.Name, err)
		}
	}
	seen := map[string]bool{}
	for _, e := range c.Entries {
		if e.Item == "" {
			return fmt.Errorf("case %s: an entry names no Item", c.Name)
		}
		if seen[e.Item] {
			return fmt.Errorf("case %s: Item %s is in it twice", c.Name, e.Item)
		}
		seen[e.Item] = true
	}
	for _, n := range c.Needs {
		if strings.TrimSpace(n.Text) == "" {
			return fmt.Errorf("case %s: a need says nothing", c.Name)
		}
	}
	return nil
}

// New is an open Case with nothing in it yet.
func New(name, title string, now time.Time) (Case, error) {
	c := Case{Name: name, Title: strings.TrimSpace(title), Status: Open, Opened: stamp(now), Entries: []Entry{}, Needs: []Need{}}
	return c, c.Validate()
}

func stamp(now time.Time) string { return now.Format(time.RFC3339) }

var errArchived = errors.New("the Case is archived: reopen it to change it")

// Has reports whether the Case uses item.
func (c Case) Has(item string) bool {
	for _, e := range c.Entries {
		if e.Item == item {
			return true
		}
	}
	return false
}

// Add puts an Item in the Case.
func (c *Case) Add(item, note string, now time.Time) error {
	if c.Status == Archived {
		return errArchived
	}
	if c.Has(item) {
		return fmt.Errorf("Item %s is already in the Case", item)
	}
	c.Entries = append(c.Entries, Entry{Item: item, Added: stamp(now), Note: strings.TrimSpace(note)})
	return nil
}

// Remove takes an Item out of the Case, and unmeets any need it met.
func (c *Case) Remove(item string) error {
	if c.Status == Archived {
		return errArchived
	}
	if !c.Has(item) {
		return fmt.Errorf("Item %s is not in the Case", item)
	}
	kept := c.Entries[:0]
	for _, e := range c.Entries {
		if e.Item != item {
			kept = append(kept, e)
		}
	}
	c.Entries = kept
	for i := range c.Needs {
		if c.Needs[i].Item == item {
			c.Needs[i].Item, c.Needs[i].Met = "", ""
		}
	}
	return nil
}

// AddNeed records something asked for that the Case does not have yet.
func (c *Case) AddNeed(text string, now time.Time) error {
	if c.Status == Archived {
		return errArchived
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("say what is needed")
	}
	c.Needs = append(c.Needs, Need{Text: text, Added: stamp(now)})
	return nil
}

// Meet marks need i met by item, putting item in the Case with the need's
// text as its note when it is not there already.
func (c *Case) Meet(i int, item string, now time.Time) error {
	if c.Status == Archived {
		return errArchived
	}
	if i < 0 || i >= len(c.Needs) {
		return fmt.Errorf("no need %d", i)
	}
	if !c.Has(item) {
		if err := c.Add(item, c.Needs[i].Text, now); err != nil {
			return err
		}
	}
	c.Needs[i].Item, c.Needs[i].Met = item, stamp(now)
	return nil
}

// DropNeed forgets need i: it is no longer asked for.
func (c *Case) DropNeed(i int) error {
	if c.Status == Archived {
		return errArchived
	}
	if i < 0 || i >= len(c.Needs) {
		return fmt.Errorf("no need %d", i)
	}
	c.Needs = append(c.Needs[:i], c.Needs[i+1:]...)
	return nil
}

// Archive closes the Case, recording for each entry the revision current
// now. An entry whose Item is gone keeps the revision it had, if any.
func (c *Case) Archive(items []tree.Item, now time.Time) error {
	if c.Status == Archived {
		return errors.New("the Case is already archived")
	}
	current := map[string]string{}
	for _, item := range items {
		current[item.ID] = item.Current()
	}
	for i := range c.Entries {
		if d, ok := current[c.Entries[i].Item]; ok {
			c.Entries[i].Digest = d
		}
	}
	c.Status, c.Archived = Archived, stamp(now)
	return nil
}

// Reopen makes an archived Case open again. The revisions recorded stay
// until it is archived again.
func (c *Case) Reopen() error {
	if c.Status != Archived {
		return errors.New("the Case is open")
	}
	c.Status = Open
	return nil
}

// Items is the Items of the Case as an export takes them, in the Case's
// order: an open Case's at their current revision, an archived Case's at
// the revision recorded when it was archived. Missing are the entries whose
// Item, or recorded revision, the tree no longer has.
func (c Case) Items(items []tree.Item) (found []tree.Item, missing []string) {
	byID := map[string]tree.Item{}
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, e := range c.Entries {
		item, ok := byID[e.Item]
		if !ok {
			missing = append(missing, e.Item)
			continue
		}
		if c.Status == Archived && e.Digest != "" {
			has := false
			for _, r := range item.Revisions {
				has = has || r.Ref() == e.Digest
			}
			if !has {
				missing = append(missing, e.Item)
				continue
			}
			item.Head = e.Digest
			if r, ok := item.Revision(e.Digest); ok && r.Snapshot {
				item.Type = r.Type
				item.Fields = item.FieldsAt(e.Digest)
			}
		}
		found = append(found, item)
	}
	return found, missing
}

// View is the View a Case is exported by: its layout over its Items, one
// revision each, two landing on one name numbered. Its name, case-<name>,
// is what the export's manifest records.
func (c Case) View() view.View {
	layout := c.Layout
	if layout == "" {
		layout = DefaultLayout
	}
	return view.View{Name: "case-" + c.Name, Node: view.Node{Selection: view.Head, File: layout}, Dedupe: view.DedupeNumber}
}

// Path is where a Case's file is.
func Path(root, name string) string { return tree.At(root).DefinitionPath(Dir, name) }

// Load reads every cases/*.yaml under root, open ones first, then by name.
func Load(root string) ([]Case, error) {
	out, err := tree.ListDefinitions(tree.At(root), definitionKind())
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status == Open
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ReadFile reads one Case file: it must parse, be valid, and be named for
// its Case.
func ReadFile(path string) (Case, error) {
	return tree.ReadDefinitionFile(tree.At(""), definitionKind(), path)
}

// Find is the Case of that name.
func Find(root, name string) (Case, error) {
	all, err := Load(root)
	if err != nil {
		return Case{}, err
	}
	for _, c := range all {
		if c.Name == name {
			return c, nil
		}
	}
	return Case{}, fmt.Errorf("no Case named %s", name)
}

// Save writes a Case through a temporary file and a rename. A new Case
// refuses a name another has.
func Save(root string, c Case, create bool) error {
	if err := tree.Require(root); err != nil {
		return err
	}
	previous := c.Name
	if create {
		previous = ""
	}
	return tree.SaveDefinition(tree.At(root), definitionKind(), previous, c, time.Time{})
}

func definitionKind() tree.DefinitionKind[Case] {
	return tree.DefinitionKind[Case]{Dir: Dir, Name: func(c Case) string { return c.Name }, Validate: func(_ string, c Case) error { return c.Validate() }, Decode: func(_ string, data []byte) (Case, error) {
		var c Case
		d := yaml.NewDecoder(bytes.NewReader(data))
		d.KnownFields(true)
		if err := d.Decode(&c); err != nil {
			return c, err
		}
		if c.Entries == nil {
			c.Entries = []Entry{}
		}
		if c.Needs == nil {
			c.Needs = []Need{}
		}
		return c, nil
	}}
}

// SaveAs saves a draft Case, including renames. Closing and membership changes
// use the Case's domain operations before saving.
func SaveAs(root, previous string, c Case, now time.Time) error {
	if err := tree.Require(root); err != nil {
		return err
	}
	return tree.SaveDefinition(tree.At(root), definitionKind(), previous, c, now)
}

// Trash moves a Case's file into the tree's trash, and answers where it went.
// Its Items stay where they are.
func Trash(root, name string, now time.Time) (string, error) {
	if err := tree.Require(root); err != nil {
		return "", err
	}
	exists, err := tree.At(root).DefinitionExists(Dir, name)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", fmt.Errorf("no Case named %s", name)
	}
	return tree.TrashDefinition(tree.At(root), definitionKind(), name, now)
}
