// Package target is a doc tree's Targets: the places its Views are exported
// to, by name, each with what it is for and the folder it goes to unless
// another is chosen when exporting. They live in the tree, in targets.yaml,
// so the tree says where its Views go; the folder is only a default, since a
// folder belongs to one machine. The rules are in docs/apps/doc/index.md.
package target

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// File is the file under a tree's root that holds its Targets.
const File = "targets.yaml"

// Target is one place Views are exported to.
type Target struct {
	Name string `yaml:"-" json:"name"`
	// About says what it is for: "read on the phone".
	About string `yaml:"about,omitempty" json:"about,omitempty"`
	// Folder is where it goes unless another is chosen. A leading ~ is the
	// home directory of the machine exporting. Empty asks every time.
	Folder string `yaml:"folder,omitempty" json:"folder,omitempty"`
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// Validate reports the first thing wrong with t.
func (t Target) Validate() error {
	if !namePattern.MatchString(t.Name) {
		return fmt.Errorf("target %q: use lowercase letters, digits, _ and -", t.Name)
	}
	if t.Folder != "" && !strings.HasPrefix(t.Folder, "~") && !filepath.IsAbs(t.Folder) {
		return fmt.Errorf("target %s: the folder %q is neither absolute nor under ~", t.Name, t.Folder)
	}
	return nil
}

// Expand is folder with a leading ~ made home.
func Expand(folder, home string) string {
	if folder == "~" {
		return home
	}
	if strings.HasPrefix(folder, "~/") {
		return filepath.Join(home, folder[2:])
	}
	return folder
}

// Load reads root's Targets, sorted by name. A tree without the file has
// none.
func Load(root string) ([]Target, error) {
	path := filepath.Join(root, File)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Target{}, nil
	}
	if err != nil {
		return nil, err
	}
	var byName map[string]Target
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&byName); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make([]Target, 0, len(byName))
	for name, t := range byName {
		t.Name = name
		if err := t.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save replaces root's Targets through a temporary file and a rename.
func Save(root string, targets []Target) error {
	byName := map[string]Target{}
	for _, t := range targets {
		if err := t.Validate(); err != nil {
			return err
		}
		if _, ok := byName[t.Name]; ok {
			return fmt.Errorf("two Targets are named %s", t.Name)
		}
		byName[t.Name] = t
	}
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(byName); err != nil {
		return err
	}
	data := buf.Bytes()
	part, err := os.CreateTemp(root, ".dgs-part-*")
	if err != nil {
		return err
	}
	defer os.Remove(part.Name())
	if _, err := part.Write(data); err != nil {
		part.Close()
		return err
	}
	if err := part.Sync(); err != nil {
		part.Close()
		return err
	}
	if err := part.Close(); err != nil {
		return err
	}
	if err := os.Chmod(part.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(part.Name(), filepath.Join(root, File))
}

// Folders is where each Target goes this time: the folder chosen for it, or
// its own folder with ~ made home. A Target with neither maps to "".
func Folders(targets []Target, chosen map[string]string, home string) map[string]string {
	out := map[string]string{}
	for _, t := range targets {
		if f := chosen[t.Name]; f != "" {
			out[t.Name] = filepath.Clean(f)
		} else if t.Folder != "" {
			out[t.Name] = filepath.Clean(Expand(t.Folder, home))
		} else {
			out[t.Name] = ""
		}
	}
	return out
}
