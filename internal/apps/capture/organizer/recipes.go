package organizer

import (
	"embed"
	"io/fs"
	"sort"
)

// starterFiles are the Recipes this toolbox ships. They are files rather than
// Go values, and they are not loaded: a Recipe is something a reader owns and
// edits, so the only Recipes in effect are the ones in their Recipe directory.
// What is compiled in is a copy to start from, written out on request.
//
// A definition in Go would be a second way of saying what a file already says,
// and the two would drift — the built-ins named Actions by ids that had not
// existed for some time, and nobody noticed, because nothing read them but the
// compiler.
//
//go:embed starter/*.yaml
var starterFiles embed.FS

// StarterFile is one shipped Recipe: the filename it is written under, which
// is its id, and its contents.
type StarterFile struct {
	Name string
	Data []byte
}

// Starters are the shipped Recipes, in filename order. A caller writes them
// into a Recipe directory; nothing here reads them back.
func Starters() []StarterFile {
	entries, err := fs.ReadDir(starterFiles, "starter")
	if err != nil {
		// The files are embedded, so a failure here is a build that went
		// wrong rather than anything a run can do something about.
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	starters := make([]StarterFile, 0, len(names))
	for _, name := range names {
		data, err := starterFiles.ReadFile("starter/" + name)
		if err != nil {
			continue
		}
		starters = append(starters, StarterFile{Name: name, Data: data})
	}
	return starters
}
