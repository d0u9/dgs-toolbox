package organizer

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadedMappings is the outcome of reading a mapping directory: the tables to
// translate values through, and whatever could not be read.
type LoadedMappings struct {
	Tables   map[string]map[string]string
	Dir      string
	Files    int
	Failures []Failure
}

// LoadMappings reads every mapping file in dir, one table per file named after
// it: country.yaml is the table a template asks for as {{mapped "country" …}}.
//
// A directory rather than a block in the configuration file for the same reason
// the workflows are: a table of country names has no natural end, and a
// configuration file holding one is a file nobody can read for anything else.
func LoadMappings(dir string) LoadedMappings {
	loaded := LoadedMappings{Dir: dir}
	if dir == "" {
		return loaded
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return loaded
	}
	if err != nil {
		loaded.Failures = append(loaded.Failures, Failure{Path: dir, Err: err})
		return loaded
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !isRecipeFile(entry.Name()) {
			continue
		}
		paths = append(paths, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(paths)

	for _, path := range paths {
		loaded.Files++
		table, err := LoadMappingFile(path)
		if err != nil {
			loaded.Failures = append(loaded.Failures, Failure{Path: path, Err: err})
			continue
		}
		if loaded.Tables == nil {
			loaded.Tables = make(map[string]map[string]string)
		}
		loaded.Tables[strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))] = table
	}
	return loaded
}

// LoadMappingFile reads one table: what a Capture records against what this
// vault files it under. The file is the table itself rather than a document
// with a table inside it — there is nothing else to say about a table, and a
// wrapper key would be a line in every file that carries no information.
func LoadMappingFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	table := map[string]string{}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	// A file holding only comments decodes to nothing at all, which is the same
	// mistake as a file holding an empty table and deserves the same sentence.
	if err := decoder.Decode(&table); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if len(table) == 0 {
		return nil, errors.New("a mapping file with no entry translates nothing")
	}
	return table, nil
}
