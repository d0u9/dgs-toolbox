package tree

import (
	"fmt"
	"os"
	"slices"
	"time"
)

// TemplatePath is where the Template of a type is kept.
func TemplatePath(root, typ string) string {
	return At(root).DefinitionPath(TemplatesDir, typ)
}

// SaveTemplate writes a Template file's content as it was typed, comments
// included. previous is the type being edited, or "" for a new Template.
//
// A new type must not exist yet. A type Items use cannot be renamed, nor its
// kind changed: every sidecar names its type and kind. A renamed Template's
// old file goes to the trash.
func SaveTemplate(root, previous string, data []byte, now time.Time) (Template, error) {
	if err := Require(root); err != nil {
		return Template{}, err
	}
	t, err := ParseTemplate(data)
	if err != nil {
		return Template{}, err
	}
	raw, err := loadRawTemplates(root)
	if err != nil {
		return Template{}, err
	}
	for _, r := range raw {
		if previous != "" && previous != t.Type && r.Extends == previous {
			return Template{}, fmt.Errorf("type %s extends %s, so it cannot be renamed", r.Type, previous)
		}
	}
	raw = slices.DeleteFunc(raw, func(r Template) bool { return r.Type == previous || r.Type == t.Type })
	resolved, err := Resolve(append(raw, t))
	if err != nil {
		return Template{}, err
	}
	for _, r := range resolved {
		if r.Type == t.Type {
			t = r
		}
	}
	items, err := others(root)
	if err != nil {
		return Template{}, err
	}
	if t.Abstract && previous != "" {
		for _, item := range items {
			if item.Type == previous {
				return Template{}, fmt.Errorf("Items of type %s exist, so it cannot be abstract", previous)
			}
		}
	}
	if previous != t.Type {
		if _, err := os.Lstat(TemplatePath(root, t.Type)); err == nil {
			return Template{}, fmt.Errorf("a Template of type %s already exists", t.Type)
		}
	}
	if previous != "" {
		for _, item := range items {
			if item.Type != previous {
				continue
			}
			if previous != t.Type {
				return Template{}, fmt.Errorf("Items of type %s exist, so it cannot be renamed", previous)
			}
			if item.Kind != t.Kind {
				return Template{}, fmt.Errorf("Items of type %s are %ss, so its kind cannot change", previous, item.Kind)
			}
		}
	}
	// Keep the typed YAML, including comments, while storage owns publication.
	if err := SaveDefinition(At(root), DefinitionKind[Template]{Dir: TemplatesDir, Name: func(t Template) string { return t.Type }, Encode: func(Template) ([]byte, error) { return data, nil }}, previous, t, now); err != nil {
		return t, err
	}

	return t, nil
}

// TrashTemplate moves a Template no Item uses into the trash, and answers
// where it went.
func TrashTemplate(root, typ string, now time.Time) (string, error) {
	if err := Require(root); err != nil {
		return "", err
	}
	items, err := others(root)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		inUse := item.Type == typ
		for _, r := range item.Revisions {
			inUse = inUse || r.Type == typ
		}
		if inUse {
			return "", fmt.Errorf("Items of type %s exist: delete them first", typ)
		}
	}
	raw, err := loadRawTemplates(root)
	if err != nil {
		return "", err
	}
	for _, r := range raw {
		if r.Extends == typ {
			return "", fmt.Errorf("type %s extends %s: change it first", r.Type, typ)
		}
	}
	from := TemplatePath(root, typ)
	if _, err := os.Lstat(from); err != nil {
		return "", fmt.Errorf("no Template of type %s", typ)
	}
	return At(root).TrashDefinition(TemplatesDir, typ, now)
}
