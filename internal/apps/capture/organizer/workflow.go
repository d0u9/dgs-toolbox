package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadedWorkflows is the outcome of reading a workflow directory: where each
// workflow keeps its fields, and whatever could not be read. As with Recipes, a
// bad file takes only itself out.
type LoadedWorkflows struct {
	Sources  map[string]map[FieldID][]string
	Dir      string
	Files    int
	Failures []Failure
}

// LoadWorkflows reads every workflow file in dir. One file describes one
// workflow — or a family of them that answer alike — rather than all of them
// describing each other in one document: workflows arrive one at a time, and a
// list that grows without bound in the configuration file is a list nobody
// reads.
//
// A missing directory is not an error: the workflows this toolbox ships
// shortcuts for are read without being described, and anything else is simply
// asked for in FIELDS until a file says where it lives.
func LoadWorkflows(dir string) LoadedWorkflows {
	loaded := LoadedWorkflows{Dir: dir}
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
		workflows, fields, err := LoadWorkflowFile(path)
		if err != nil {
			loaded.Failures = append(loaded.Failures, Failure{Path: path, Err: err})
			continue
		}
		if loaded.Sources == nil {
			loaded.Sources = make(map[string]map[FieldID][]string)
		}
		for _, workflow := range workflows {
			// A workflow named by two files takes both, field by field: the
			// later file wins the fields it names and leaves the rest alone,
			// so a file covering a family and a file covering one of them
			// compose rather than one erasing the other.
			if loaded.Sources[workflow] == nil {
				loaded.Sources[workflow] = make(map[FieldID][]string, len(fields))
			}
			for field, sources := range fields {
				loaded.Sources[workflow][field] = sources
			}
		}
	}
	return loaded
}

// workflowDocument is the on-disk shape. Fields are paths into the payload,
// one or several, because the same logical field is kept under a different key
// by every workflow and sometimes under one of several by a family of them.
type workflowDocument struct {
	Workflows []string                 `yaml:"workflows"`
	Fields    map[string]workflowPaths `yaml:"fields"`
}

// workflowPaths is one source or a list of them. Both spellings are honest: a
// field kept in one place is a string, and a field a family keeps in slightly
// different places is the list of those places. A source is a payload key, or a
// template over the payload when the field is several keys put together.
type workflowPaths []string

func (p *workflowPaths) UnmarshalYAML(node *yaml.Node) error {
	var single string
	if err := node.Decode(&single); err == nil {
		*p = workflowPaths{single}
		return nil
	}
	var several []string
	if err := node.Decode(&several); err != nil {
		return errors.New("a field is a payload key or a list of them")
	}
	*p = several
	return nil
}

// LoadWorkflowFile reads one workflow file: which workflows it answers for, and
// where each field lives. The filename is the workflow when the file does not
// say otherwise, the way a Recipe's filename is its id.
func LoadWorkflowFile(path string) (workflows []string, fields map[FieldID][]string, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var document workflowDocument
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return nil, nil, fmt.Errorf("decode: %w", err)
	}

	workflows = document.Workflows
	if len(workflows) == 0 {
		workflows = []string{strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))}
	}
	for _, workflow := range workflows {
		if strings.TrimSpace(workflow) == "" {
			return nil, nil, errors.New("a workflow needs a name")
		}
	}
	if len(document.Fields) == 0 {
		return nil, nil, errors.New("a workflow file with no field says nothing")
	}

	fields = make(map[FieldID][]string, len(document.Fields))
	for field, paths := range document.Fields {
		if len(paths) == 0 {
			return nil, nil, fmt.Errorf("field %q names no payload key", field)
		}
		for _, source := range paths {
			if IsTemplateSource(source) {
				// A template is checked here rather than when it is rendered:
				// one that does not parse is a mistake, and a mistake that
				// only shows up as a field quietly staying empty is one nobody
				// finds. What it names is not checked — that is the Capture's
				// business, and a key it does not carry is ordinary.
				if _, err := ParseSource(Settings{}, source); err != nil {
					return nil, nil, fmt.Errorf("field %q: %w", field, err)
				}
				continue
			}
			// A path that starts anywhere else resolves to nothing, and a
			// field that silently stays missing is the confusion these files
			// exist to end.
			if !strings.HasPrefix(source, PayloadRoot+".") || strings.TrimSpace(source) == PayloadRoot+"." {
				return nil, nil, fmt.Errorf("field %q must name a payload key or a template over the payload, got %q", field, source)
			}
		}
		fields[FieldID(field)] = paths
	}
	return workflows, fields, nil
}
