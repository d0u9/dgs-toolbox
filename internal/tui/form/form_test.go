package form

import (
	"strings"
	"testing"
)

func TestNavigationAndControlRendering(t *testing.T) {
	model := New(
		Field{ID: "option", Kind: Option, Label: "Option", Value: "Keep"},
		Field{ID: "check", Kind: Checkbox, Label: "Checkbox", Checked: true},
		Field{ID: "text", Kind: Text, Label: "Text", Value: "Demo"},
		Field{ID: "radio", Kind: Radio, Label: "Radio", Value: "Copy", Options: []string{"Copy", "Move"}},
		Field{ID: "button", Kind: Button, Label: "Continue"},
	)
	for _, key := range []string{"down", "j", "right", "l", "tab"} {
		if !model.UpdateNavigation(key) {
			t.Fatalf("%s was not handled", key)
		}
	}
	if model.FocusedID() != "option" {
		t.Fatalf("wrapped focus = %q, want option", model.FocusedID())
	}
	view := model.View([]string{"option", "check", "text", "radio", "button"})
	for _, expected := range []string{"Keep", "[x] Checkbox", "Demo", "(●) Copy", "( ) Move", "[ Continue ]"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("form view does not contain %q:\n%s", expected, view)
		}
	}
}

func TestInteractiveControls(t *testing.T) {
	model := New(
		Field{ID: "option", Kind: Option, Label: "Option", Value: "Skip", Options: []string{"Skip", "Replace"}},
		Field{ID: "check", Kind: Checkbox, Label: "Recursive"},
		Field{ID: "text", Kind: Text, Label: "Name", Value: "Demo"},
		Field{ID: "radio", Kind: Radio, Label: "Transfer", Value: "Copy", Options: []string{"Copy", "Move"}},
	)
	if !model.HandleInteraction("enter") || !model.IsActive() {
		t.Fatal("option did not open")
	}
	model.HandleInteraction("down")
	model.HandleInteraction("enter")
	if got := model.Value("option"); got != "Replace" {
		t.Fatalf("option = %q, want Replace", got)
	}
	model.UpdateNavigation("down")
	model.HandleInteraction(" ")
	if !strings.Contains(model.View([]string{"check"}), "[x]") {
		t.Fatal("checkbox did not toggle")
	}
	model.UpdateNavigation("down")
	model.HandleInteraction("enter")
	model.HandleInteraction("!")
	model.HandleInteraction("enter")
	if got := model.Value("text"); got != "Demo!" {
		t.Fatalf("text = %q, want Demo!", got)
	}
	model.UpdateNavigation("down")
	model.HandleInteraction("right")
	if got := model.Value("radio"); got != "Move" {
		t.Fatalf("radio = %q, want Move", got)
	}
}
