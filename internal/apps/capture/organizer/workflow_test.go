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
