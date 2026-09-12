package organizer

import (
	"testing"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// quickNote is a Capture from a workflow this binary knows nothing about,
// keeping its note under a key of its own.
func quickNote() Capture {
	return Capture{
		Path: "/captures/quick-1",
		Name: "quick-1",
		Index: indexschema.Index{
			Schema:    "v1",
			ID:        "quick-1",
			CreatedAt: "2026-09-12T09:15:00+10:00",
			Source:    indexschema.Source{App: "Shortcut", Workflow: "quick_note"},
			Payload: map[string]any{
				"text":   "buy milk",
				"labels": []any{"errand", "today"},
				"meta":   map[string]any{"heading": "Errands"},
				"count":  float64(2),
			},
		},
	}
}

func sourcedContext(sources map[string]map[FieldID][]string) Context {
	settings := DefaultSettings()
	settings.Sources = sources
	return NewContext(quickNote(), nil).WithSettings(settings)
}

// A workflow says where it keeps a field, and the field resolves from there:
// the Action asks for content and never learns which key answered.
func TestAConfiguredSourceAnswersAField(t *testing.T) {
	ctx := sourcedContext(map[string]map[FieldID][]string{
		"quick_note": {FieldContent: {"payload.text"}, FieldTitle: {"payload.meta.heading"}},
	})
	if got := ctx.String(FieldContent); got != "buy milk" {
		t.Fatalf("content = %q, want the value under the configured key", got)
	}
	if got := ctx.String(FieldTitle); got != "Errands" {
		t.Fatalf("title = %q, want a nested path to resolve", got)
	}
}

// A list is a field like any other, so tags arrive as tags rather than as a
// rendered array.
func TestAConfiguredSourceReadsAListOfStrings(t *testing.T) {
	ctx := sourcedContext(map[string]map[FieldID][]string{"quick_note": {FieldTags: {"payload.labels"}}})
	value, ok := ctx.Get(FieldTags)
	if !ok {
		t.Fatal("tags did not resolve")
	}
	tags, ok := value.([]string)
	if !ok || len(tags) != 2 || tags[0] != "errand" {
		t.Fatalf("tags = %#v, want the strings of the array", value)
	}
}

// A source corrects what the binary assumed as well as naming what it never
// knew: the workflow's own key wins over the compiled-in fallback.
func TestAConfiguredSourceWinsOverTheCompiledInOne(t *testing.T) {
	capture := quickNote()
	capture.Index.Source.Workflow = "been_here"
	capture.Index.Payload["note"] = "the built-in key"
	settings := DefaultSettings()
	settings.Sources = map[string]map[FieldID][]string{"been_here": {FieldContent: {"payload.text"}}}
	ctx := NewContext(capture, nil).WithSettings(settings)

	if got := ctx.String(FieldContent); got != "buy milk" {
		t.Fatalf("content = %q, want the configured key to win", got)
	}
	// Enrichment still wins over both: what the reader typed is the answer.
	ctx.Enrichment = map[FieldID]any{FieldContent: "what I typed"}
	if got := ctx.String(FieldContent); got != "what I typed" {
		t.Fatalf("content = %q, want the typed value", got)
	}
}

// A path that leads nowhere leaves the field missing, which is a question the
// reader is asked rather than an error they have to interpret.
func TestASourceThatDoesNotResolveLeavesTheFieldMissing(t *testing.T) {
	for _, path := range []string{"payload.absent", "payload.meta.absent", "payload.text.deeper", "tags", "payload"} {
		ctx := sourcedContext(map[string]map[FieldID][]string{"quick_note": {FieldContent: {path}}})
		if value, ok := ctx.Get(FieldContent); ok {
			t.Fatalf("path %q resolved to %#v, want the field left missing", path, value)
		}
	}
}

// A workflow with no entry is read the way it always was.
func TestAWorkflowWithNoSourceKeepsTheCompiledInReading(t *testing.T) {
	capture := quickNote()
	capture.Index.Source.Workflow = "been_here"
	capture.Index.Payload["note"] = "from the built-in key"
	ctx := NewContext(capture, nil).WithSettings(DefaultSettings())
	if got := ctx.String(FieldContent); got != "from the built-in key" {
		t.Fatalf("content = %q, want the compiled-in reading", got)
	}
}

// A family of workflows that each keep their text somewhere slightly different
// is one list, not one block each: the paths are tried in order and the first
// that resolves answers.
func TestOneSourceListAnswersForEveryWorkflow(t *testing.T) {
	sources := map[string]map[FieldID][]string{
		AnyWorkflow: {FieldContent: {"payload.text", "payload.body", "payload.note"}},
	}
	for workflow, key := range map[string]string{
		"quick_note_1": "text",
		"quick_note_2": "body",
		"xxx1":         "note",
	} {
		capture := quickNote()
		capture.Index.Source.Workflow = workflow
		capture.Index.Payload = map[string]any{key: "what they wrote"}
		settings := DefaultSettings()
		settings.Sources = sources
		ctx := NewContext(capture, nil).WithSettings(settings)
		if got := ctx.String(FieldContent); got != "what they wrote" {
			t.Fatalf("workflow %s: content = %q, want the first path that resolves", workflow, got)
		}
	}
}

// A workflow that says where it keeps a field has said it: the general list
// does not then read a key it did not name.
func TestAWorkflowWithItsOwnSourceDoesNotFallBackToTheGeneralOne(t *testing.T) {
	capture := quickNote()
	capture.Index.Payload = map[string]any{"body": "the general key"}
	settings := DefaultSettings()
	settings.Sources = map[string]map[FieldID][]string{
		AnyWorkflow:  {FieldContent: {"payload.body"}},
		"quick_note": {FieldContent: {"payload.text"}},
	}
	ctx := NewContext(capture, nil).WithSettings(settings)
	if value, ok := ctx.Get(FieldContent); ok {
		t.Fatalf("content = %#v, want the workflow's own source to be the whole answer", value)
	}
}
