package timeline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// DefinitionsVersion is the version of the timelines file this package reads.
// The file is shared with the Obsidian plugin, so both refuse one they do not
// know rather than guess at it.
const DefinitionsVersion = 1

// SplitYear archives every year that has rolled over: the running note holds
// the current year only. It is the one way to split there is so far.
const SplitYear = "year"

// Definition is one timeline: a note that grows at the top, newest first, and
// the folder its past years go to. Paths are relative to the vault.
type Definition struct {
	// Name is the key the timeline is filed under in the file.
	Name string `json:"name,omitempty"`
	// Note is the running note.
	Note string `json:"note"`
	// Archive is the folder a year that has rolled over is moved into, as
	// <archive>/<year>.md. Empty is the note's own folder.
	Archive string `json:"archive,omitempty"`
	// Split says what stays in the running note. Empty is SplitYear.
	Split string `json:"split,omitempty"`
	// CSSClass is written into a year's archive for the vault's stylesheet.
	CSSClass string `json:"cssclass"`
	// Title names the list in an archive's callout: "2026 年的<Title>".
	Title string `json:"title"`
	// Template is the entry template a program writing Captures uses, by
	// filename in its own template directory. Empty is that program's default.
	Template string `json:"template,omitempty"`
	// ContentRequired says an entry is nothing without its text. A place is
	// worth recording unannotated; an event is not.
	ContentRequired bool `json:"contentRequired,omitempty"`
}

// Header is the introduction of this timeline's archive for a note.
func (d Definition) Header(source string) Header {
	return Header{CSSClass: d.CSSClass, Title: d.Title, Source: source}
}

// definitionName is what a timeline may be called: it becomes part of an
// Action's id and a command's, so it stays plain.
var definitionName = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ValidName reports whether a timeline may be called name.
func ValidName(name string) bool { return definitionName.MatchString(name) }

// ParseDefinitions reads a timelines file:
//
//	{"version": 1, "timelines": {"family": {"note": "…", …}, …}}
//
// It refuses a file of another version, a name that is not lowercase letters,
// digits and hyphens, a timeline without a note, and a split it does not know.
// The timelines come back sorted by name.
func ParseDefinitions(data []byte) ([]Definition, error) {
	var file struct {
		Version   int                   `json:"version"`
		Timelines map[string]Definition `json:"timelines"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("timelines file: %w", err)
	}
	if file.Version != DefinitionsVersion {
		return nil, fmt.Errorf("timelines file: version %d, want %d", file.Version, DefinitionsVersion)
	}
	definitions := make([]Definition, 0, len(file.Timelines))
	for name, definition := range file.Timelines {
		if !definitionName.MatchString(name) {
			return nil, fmt.Errorf("timelines file: %q is not a name: use lowercase letters, digits and hyphens", name)
		}
		if strings.TrimSpace(definition.Note) == "" {
			return nil, fmt.Errorf("timelines file: %s has no note", name)
		}
		if definition.Split == "" {
			definition.Split = SplitYear
		}
		if definition.Split != SplitYear {
			return nil, fmt.Errorf("timelines file: %s splits by %q; only %q is supported", name, definition.Split, SplitYear)
		}
		definition.Name = name
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions, nil
}
