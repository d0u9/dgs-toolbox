package organizer

import (
	"embed"
	"io/fs"
	"sort"
)

// starterFiles are the Recipes and workflow descriptions this toolbox ships.
// They are files rather than Go values, and they are not loaded: both are
// things a reader owns and edits, so what is in effect is only ever what is in
// their own directories. What is compiled in is a copy to start from, written
// out on request.
//
// A definition in Go would be a second way of saying what a file already says,
// and the two would drift — the built-in Recipes named Actions by ids that had
// not existed for some time, and nobody noticed, because nothing read them but
// the compiler.
//
//go:embed starter/recipes/*.yaml starter/workflows/*.yaml
var starterFiles embed.FS

// StarterKind is one of the directories a starter file belongs in, named the
// way it is on disk.
type StarterKind string

const (
	StarterRecipes   StarterKind = "recipes"
	StarterWorkflows StarterKind = "workflows"
)

// StarterKinds are the directories laid down, in the order they are written.
var StarterKinds = []StarterKind{StarterRecipes, StarterWorkflows}

// StarterFile is one shipped file: the directory it belongs in, the filename it
// is written under — which is its id — and its contents.
type StarterFile struct {
	Kind StarterKind
	Name string
	Data []byte
}

// Starters are the shipped files, by kind and then in filename order. A caller
// writes them into a configuration; nothing here reads them back.
func Starters() []StarterFile {
	var starters []StarterFile
	for _, kind := range StarterKinds {
		entries, err := fs.ReadDir(starterFiles, "starter/"+string(kind))
		if err != nil {
			// The files are embedded, so a failure here is a build that went
			// wrong rather than anything a run can do something about.
			continue
		}
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			data, err := starterFiles.ReadFile("starter/" + string(kind) + "/" + name)
			if err != nil {
				continue
			}
			starters = append(starters, StarterFile{Kind: kind, Name: name, Data: data})
		}
	}
	return starters
}

// StartersOf are the shipped files of one kind.
func StartersOf(kind StarterKind) []StarterFile {
	var starters []StarterFile
	for _, starter := range Starters() {
		if starter.Kind == kind {
			starters = append(starters, starter)
		}
	}
	return starters
}
