package organizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeWorkflow(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The filename is the workflow, the way a Recipe's filename is its id.
func TestAWorkflowFileIsNamedAfterItsWorkflow(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "quick_note.yaml", "fields:\n  content: payload.text\n  tags: payload.labels\n")

	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	if loaded.Files != 1 {
		t.Fatalf("files = %d, want 1", loaded.Files)
	}
	fields := loaded.Sources["quick_note"]
	if len(fields[FieldContent]) != 1 || fields[FieldContent][0] != "payload.text" {
		t.Fatalf("content = %q", fields[FieldContent])
	}
	if len(fields[FieldTags]) != 1 || fields[FieldTags][0] != "payload.labels" {
		t.Fatalf("tags = %q", fields[FieldTags])
	}
}

// One file answers for a family of workflows, and a field may name several
// keys: a dozen shortcuts that each kept their text somewhere slightly
// different are one file rather than a dozen.
func TestOneFileAnswersForAFamilyOfWorkflows(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "notes.yaml", `
workflows: [quick_note_1, quick_note_2, "*"]
fields:
  content:
    - payload.text
    - payload.body
`)

	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	for _, workflow := range []string{"quick_note_1", "quick_note_2", AnyWorkflow} {
		paths := loaded.Sources[workflow][FieldContent]
		if len(paths) != 2 || paths[0] != "payload.text" {
			t.Fatalf("%s: content = %q", workflow, paths)
		}
	}
}

// Two files naming the same workflow compose field by field, so a file for a
// family and a file for one of its members do not erase each other.
func TestTwoFilesForOneWorkflowComposeByField(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "a_family.yaml", "workflows: [quick_note]\nfields:\n  content: payload.body\n  tags: payload.labels\n")
	writeWorkflow(t, dir, "b_quick_note.yaml", "workflows: [quick_note]\nfields:\n  content: payload.text\n")

	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	fields := loaded.Sources["quick_note"]
	if got := fields[FieldContent]; len(got) != 1 || got[0] != "payload.text" {
		t.Fatalf("content = %q, want the later file to win the field it names", got)
	}
	if got := fields[FieldTags]; len(got) != 1 || got[0] != "payload.labels" {
		t.Fatalf("tags = %q, want the field the later file left alone", got)
	}
}

// A bad file takes only itself out, and says what is wrong with it.
func TestABadWorkflowFileFailsAlone(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"a path outside the payload", "fields:\n  content: text\n", "must name a payload key"},
		{"the payload itself", "fields:\n  content: payload.\n", "must name a payload key"},
		{"no field at all", "workflows: [quick_note]\n", "says nothing"},
		{"a misspelled key", "field:\n  content: payload.text\n", "field not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeWorkflow(t, dir, "good.yaml", "fields:\n  content: payload.text\n")
			writeWorkflow(t, dir, "bad.yaml", tc.body)

			loaded := LoadWorkflows(dir)
			if len(loaded.Failures) != 1 {
				t.Fatalf("failures = %v, want the bad file alone", loaded.Failures)
			}
			if !strings.Contains(loaded.Failures[0].Err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to say %q", loaded.Failures[0].Err, tc.want)
			}
			if _, ok := loaded.Sources["good"]; !ok {
				t.Fatal("the good file was dropped with the bad one")
			}
		})
	}
}

// A missing directory is an installation that has described no workflow, which
// is the ordinary state of one using only the shortcuts this toolbox ships.
func TestLoadWorkflowsTreatsAMissingDirectoryAsNone(t *testing.T) {
	for _, dir := range []string{"", filepath.Join(t.TempDir(), "absent")} {
		loaded := LoadWorkflows(dir)
		if len(loaded.Failures) != 0 || len(loaded.Sources) != 0 {
			t.Fatalf("dir %q: %+v", dir, loaded)
		}
	}
}

// A field kept as several keys is one field by the time an Action asks for it:
// the source composes it, and the Action never learns it was in pieces.
func TestASourceMayComposeSeveralKeys(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "quick_note.yaml", `
fields:
  content: "{{.title}}{{with .body}}\n\n{{.}}{{end}}"
`)
	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	settings := DefaultSettings()
	settings.Sources = loaded.Sources

	capture := quickNote()
	capture.Index.Payload = map[string]any{"title": "Milk", "body": "and the parcel"}
	if got := NewContext(capture, nil).WithSettings(settings).String(FieldContent); got != "Milk\n\nand the parcel" {
		t.Fatalf("content = %q, want both pieces", got)
	}

	// An optional piece guarded with "with" simply is not written.
	capture.Index.Payload = map[string]any{"title": "Milk"}
	if got := NewContext(capture, nil).WithSettings(settings).String(FieldContent); got != "Milk" {
		t.Fatalf("content = %q, want the piece that is there", got)
	}

	// A key named without a guard is the Capture not carrying it, which is
	// ordinary: the field stays missing and Route asks for it.
	capture.Index.Payload = map[string]any{"body": "no title at all"}
	if value, ok := NewContext(capture, nil).WithSettings(settings).Get(FieldContent); ok {
		t.Fatalf("content = %#v, want the field left missing", value)
	}
}

// A template and a payload key can be listed together, first one that resolves
// answering, so a family file can compose for most and point for the rest.
func TestASourceListMixesTemplatesAndKeys(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "notes.yaml", `
workflows: ["*"]
fields:
  content:
    - "{{.title}} — {{.body}}"
    - payload.text
`)
	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	settings := DefaultSettings()
	settings.Sources = loaded.Sources

	capture := quickNote()
	capture.Index.Payload = map[string]any{"text": "just the one key"}
	if got := NewContext(capture, nil).WithSettings(settings).String(FieldContent); got != "just the one key" {
		t.Fatalf("content = %q, want the key the template could not answer for", got)
	}
}

// A template that does not parse is a mistake, and a mistake that shows up as a
// field quietly staying empty is one nobody finds. The file is refused.
func TestAWorkflowFileWithABrokenTemplateIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "broken.yaml", "fields:\n  content: \"{{.title\"\n")
	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 1 || !strings.Contains(loaded.Failures[0].Err.Error(), `field "content"`) {
		t.Fatalf("failures = %v, want the field named", loaded.Failures)
	}
}

// The functions a template may call are the ones every other template in the
// toolbox has, so a source composes the same way a note does.
func TestASourceMayUseTheTemplateFunctions(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "quick_note.yaml", `
fields:
  content: '{{join .tags ", "}} · {{trim .title}}'
`)
	loaded := LoadWorkflows(dir)
	if len(loaded.Failures) != 0 {
		t.Fatalf("failures = %v", loaded.Failures)
	}
	settings := DefaultSettings()
	settings.Sources = loaded.Sources

	capture := quickNote()
	capture.Index.Payload = map[string]any{"title": "  Milk  ", "tags": []string{"errand", "today"}}
	if got := NewContext(capture, nil).WithSettings(settings).String(FieldContent); got != "errand, today · Milk" {
		t.Fatalf("content = %q", got)
	}
}
