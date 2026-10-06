package tree

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefinitionKind describes one YAML definition. Decode and Encode are optional
// for formats with legacy forms or text that must retain comments.
// The name differs from Kind, which is already the public Item-kind enum.
type DefinitionKind[T any] struct {
	Dir      string
	Name     func(T) string
	Validate func(root string, v T) error
	Decode   func(root string, data []byte) (T, error)
	Encode   func(T) ([]byte, error)
}

func definitionName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\") || strings.TrimSpace(name) != name {
		return fmt.Errorf("%q is not a definition name", name)
	}
	return nil
}

// DefinitionPath is owned by the Store; kind-specific validation is performed
// before reading, writing or trashing it.
func (s *Store) DefinitionPath(dir, name string) string {
	return filepath.Join(s.root, dir, name+".yaml")
}
func (s *Store) DefinitionFiles(dir string) ([]string, error) {
	if err := definitionName(dir); err != nil {
		return nil, err
	}
	return filepath.Glob(filepath.Join(s.root, dir, "*.yaml"))
}

// DefinitionExists checks a name without following a symlink, so occupied
// names (including broken links) cannot be silently replaced by a new save.
func (s *Store) DefinitionExists(dir, name string) (bool, error) {
	if err := definitionName(dir); err != nil {
		return false, err
	}
	if err := definitionName(name); err != nil {
		return false, err
	}
	_, err := os.Lstat(s.DefinitionPath(dir, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *Store) DefinitionBytes(dir, name string) ([]byte, error) {
	if err := definitionName(dir); err != nil {
		return nil, err
	}
	if err := definitionName(name); err != nil {
		return nil, err
	}
	return os.ReadFile(s.DefinitionPath(dir, name))
}

func decodeDefinition[T any](s *Store, k DefinitionKind[T], data []byte) (T, error) {
	var v T
	var err error
	if k.Decode != nil {
		v, err = k.Decode(s.root, data)
	} else {
		d := yaml.NewDecoder(bytes.NewReader(data))
		d.KnownFields(true)
		err = d.Decode(&v)
	}
	if err == nil && k.Validate != nil {
		err = k.Validate(s.root, v)
	}
	return v, err
}

// ReadDefinition reads and validates the named definition and its filename.
func ReadDefinition[T any](s *Store, k DefinitionKind[T], name string) (T, error) {
	var zero T
	if err := definitionName(name); err != nil {
		return zero, err
	}
	if err := definitionName(k.Dir); err != nil {
		return zero, err
	}
	return ReadDefinitionFile(s, k, s.DefinitionPath(k.Dir, name))
}

// ReadDefinitionFile is used by diagnostics inspecting an inventoried YAML file.
// It applies the same decoder, validation and filename checks as normal reads.
func ReadDefinitionFile[T any](s *Store, k DefinitionKind[T], path string) (T, error) {
	var zero T
	data, err := os.ReadFile(path)
	if err != nil {
		return zero, err
	}
	v, err := decodeDefinition(s, k, data)
	if err != nil {
		return zero, fmt.Errorf("%s: %w", path, err)
	}
	name := strings.TrimSuffix(filepath.Base(path), ".yaml")
	if k.Name(v) != name {
		return zero, fmt.Errorf("%s: name %q is not the file's name", path, k.Name(v))
	}
	return v, nil
}

// ListDefinitions reads definitions sorted by name. Domain packages may apply
// their own ordering after this (for example open Cases first).
func ListDefinitions[T any](s *Store, k DefinitionKind[T]) ([]T, error) {
	files, err := s.DefinitionFiles(k.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(files))
	for _, p := range files {
		v, err := ReadDefinition(s, k, strings.TrimSuffix(filepath.Base(p), ".yaml"))
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return k.Name(out[i]) < k.Name(out[j]) })
	return out, nil
}

// SaveDefinition writes a new definition or replaces previous. Renames write
// the new file before moving the original bytes to the trash.
func SaveDefinition[T any](s *Store, k DefinitionKind[T], previous string, v T, now time.Time) error {
	name := k.Name(v)
	if err := definitionName(name); err != nil {
		return err
	}
	if previous != "" {
		if err := definitionName(previous); err != nil {
			return err
		}
	}
	if k.Validate != nil {
		if err := k.Validate(s.root, v); err != nil {
			return err
		}
	}
	if err := s.CheckDefinitionSave(k.Dir, previous, name); err != nil {
		return err
	}
	var data []byte
	var err error
	if k.Encode != nil {
		data, err = k.Encode(v)
	} else {
		data, err = yaml.Marshal(v)
	}
	if err != nil {
		return err
	}
	if err := s.WriteDefinition(k.Dir, name, data); err != nil {
		return err
	}
	if previous != "" && previous != name {
		_, err = s.TrashDefinition(k.Dir, previous, now)
	}
	return err
}

// CheckDefinitionSave checks names before domain orchestration writes shared
// dependencies. SaveDefinition repeats it immediately before publication.
func (s *Store) CheckDefinitionSave(dir, previous, name string) error {
	if err := definitionName(dir); err != nil {
		return err
	}
	if err := definitionName(name); err != nil {
		return err
	}
	if previous != "" {
		if err := definitionName(previous); err != nil {
			return err
		}
	}
	if previous != name {
		if _, err := os.Lstat(s.DefinitionPath(dir, name)); err == nil {
			return fmt.Errorf("a %s definition named %s already exists", dir, name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if previous != "" {
		if _, err := os.Lstat(s.DefinitionPath(dir, previous)); err != nil {
			return fmt.Errorf("no %s definition named %s: %w", dir, previous, err)
		}
	}
	return nil
}

// WriteDefinition is the shared atomic writer for definitions with domain
// orchestration (shared rules, inheritance and legacy migration).
func (s *Store) WriteDefinition(dir, name string, data []byte) error {
	if err := definitionName(dir); err != nil {
		return err
	}
	if err := definitionName(name); err != nil {
		return err
	}
	return WriteFileAtomic(s.DefinitionPath(dir, name), data)
}

// TrashDefinition moves original bytes, refusing a timestamp collision.
func (s *Store) TrashDefinition(dir, name string, now time.Time) (string, error) {
	if err := definitionName(dir); err != nil {
		return "", err
	}
	if err := definitionName(name); err != nil {
		return "", err
	}
	from := s.DefinitionPath(dir, name)
	if _, err := os.Lstat(from); err != nil {
		return "", err
	}
	destDir := filepath.Join(s.root, TrashDir, dir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	to := filepath.Join(destDir, name+"-"+now.UTC().Format("20060102T150405Z")+".yaml")
	if _, err := os.Lstat(to); err == nil {
		return "", fmt.Errorf("%s is already in the trash", to)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return to, os.Rename(from, to)
}

// TrashDefinition validates the name and delegates to the Store. Checks for
// references stay in the owning domain package before calling this function.
func TrashDefinition[T any](s *Store, k DefinitionKind[T], name string, now time.Time) (string, error) {
	return s.TrashDefinition(k.Dir, name, now)
}
