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
	for _, expected := range []string{"Keep", "[x] Checkbox", "Demo", "(●) Copy", "( ) Move", "Continue"} {
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

func TestMouseClickOperatesControls(t *testing.T) {
	model := New(
		Field{ID: "radio", Kind: Radio, Label: "Mode", Value: "Copy", Options: []string{"Copy", "Move"}},
		Field{ID: "option", Kind: Option, Label: "Theme", Value: "Light", Options: []string{"Light", "Dark"}},
		Field{ID: "check", Kind: Checkbox, Label: "Hidden"},
	)
	if id, ok := model.Click([]string{"radio", "option", "check"}, 30, 0); !ok || id != "radio" || model.Value("radio") != "Move" {
		t.Fatalf("radio click: id=%q value=%q", id, model.Value("radio"))
	}
	model.Click([]string{"radio", "option", "check"}, 3, 1)
	if !model.IsActive() {
		t.Fatal("option click did not open choices")
	}
	model.Click([]string{"radio", "option", "check"}, 3, 3)
	if model.Value("option") != "Dark" || model.IsActive() {
		t.Fatal("option choice was not applied")
	}
	model.Click([]string{"radio", "option", "check"}, 3, 2)
	if !model.Checked("check") {
		t.Fatal("checkbox click did not toggle")
	}
}

func TestMultiCheckboxSupportsDynamicOptions(t *testing.T) {
	model := New(Field{ID: "extensions", Kind: MultiCheckbox, Label: "Extensions", SelectAll: true})
	model.SetOptions("extensions", []string{"DNG", "JPG", "PNG"}, true)
	model.SetFocusID("extensions")
	model.HandleInteraction("right")
	model.HandleInteraction("down")
	model.HandleInteraction("down")
	model.HandleInteraction(" ")
	if got := model.Values("extensions"); len(got) != 2 || got[0] != "DNG" || got[1] != "PNG" {
		t.Fatalf("selected = %#v", got)
	}
	if !strings.Contains(model.View([]string{"extensions"}), "[ ] JPG") {
		t.Fatal("unchecked dynamic extension is not rendered")
	}
	model.Click([]string{"extensions"}, 20, 0)
	if got := model.Values("extensions"); len(got) != 3 {
		t.Fatalf("All selected %#v", got)
	}
}

func TestClickingMultiCheckboxClosesAnotherOptionWithoutSharingCursor(t *testing.T) {
	model := New(
		Field{ID: "extensions", Kind: MultiCheckbox, Label: "Extensions", Options: []string{"DNG", "JPG"}, Selected: []string{"DNG", "JPG"}, SelectAll: true},
		Field{ID: "duplicates", Kind: Option, Label: "Duplicates", Value: "Skip", Options: []string{"Skip", "Replace", "Keep both"}},
	)
	model.SetFocusID("duplicates")
	model.HandleInteraction("enter")
	model.HandleInteraction("down")
	model.Click([]string{"extensions", "duplicates"}, 20, 1)
	if model.activeID != "extensions" || model.Value("duplicates") != "Skip" {
		t.Fatal("clicking Extensions did not cancel the unconfirmed option")
	}
	if got := model.Values("extensions"); len(got) != 1 || got[0] != "JPG" {
		t.Fatalf("extensions = %#v", got)
	}
}

func TestSpaceOpensOptionAndMultiCheckboxUsesVerticalSubnavigation(t *testing.T) {
	model := New(
		Field{ID: "duplicates", Kind: Option, Label: "Duplicates", Value: "Skip", Options: []string{"Skip", "Replace"}},
		Field{ID: "extensions", Kind: MultiCheckbox, Label: "Extensions", Options: []string{"DNG", "JPG"}, Selected: []string{"DNG", "JPG"}, SelectAll: true},
	)
	model.SetFocusID("duplicates")
	if !model.HandleInteraction(" ") || !model.IsActive() {
		t.Fatal("Space did not open Option")
	}
	model.HandleInteraction("esc")
	model.SetFocusID("extensions")
	model.HandleInteraction("right")
	model.HandleInteraction("down")
	model.HandleInteraction("down")
	model.HandleInteraction(" ")
	if got := model.Values("extensions"); len(got) != 1 || got[0] != "DNG" {
		t.Fatalf("vertical subnavigation selected %#v", got)
	}
	model.HandleInteraction("left")
	if model.IsActive() {
		t.Fatal("Left did not leave multi-checkbox subitems")
	}
}

func TestNumberControlIsBounded(t *testing.T) {
	model := New(Field{ID: "workers", Kind: Number, Label: "Workers", Value: "1", Min: 1, Max: 3, Step: 1})
	model.HandleInteraction("right")
	model.HandleInteraction("right")
	model.HandleInteraction("right")
	if got := model.IntValue("workers"); got != 3 {
		t.Fatalf("workers=%d", got)
	}
	model.HandleInteraction("left")
	if got := model.IntValue("workers"); got != 2 {
		t.Fatalf("workers=%d", got)
	}
}

func TestPasteIntoText(t *testing.T) {
	m := New(Field{ID: "name", Kind: Text, Label: "Name", Value: "a-"}, Field{ID: "on", Kind: Checkbox, Label: "On"})
	if !m.Paste("laptop-mac-01 \n") || m.Value("name") != "a-laptop-mac-01" || !m.CapturesText() {
		t.Fatalf("paste: %q active %v", m.Value("name"), m.CapturesText())
	}
	m.HandleInteraction("enter")
	m.UpdateNavigation("down")
	if m.Paste("x") {
		t.Error("pasted into a checkbox")
	}
}

func TestTextEditingMovesTheCursor(t *testing.T) {
	m := New(Field{ID: "host", Kind: Text, Label: "Host", Value: "mbp-mac"})
	m.HandleInteraction("enter")
	for _, key := range []string{"left", "left", "left", "2", "0", "1", "8", "ctrl+a", "x", "ctrl+e", "!"} {
		m.HandleInteraction(key)
	}
	if got := m.Value("host"); got != "xmbp-2018mac!" {
		t.Fatalf("value %q", got)
	}
	m.HandleInteraction("ctrl+a")
	m.HandleInteraction("ctrl+k")
	if got := m.Value("host"); got != "" {
		t.Fatalf("ctrl+k left %q", got)
	}
	for _, key := range []string{"a", "space", "b", "alt+backspace"} {
		m.HandleInteraction(key)
	}
	if got := m.Value("host"); got != "a " {
		t.Fatalf("word delete left %q", got)
	}
	// Paste goes in at the cursor, and Esc restores the value before editing.
	m.HandleInteraction("ctrl+a")
	m.Paste("pre")
	if got := m.Value("host"); got != "prea " {
		t.Fatalf("paste at cursor %q", got)
	}
	m.HandleInteraction("esc")
	if got := m.Value("host"); got != "mbp-mac" {
		t.Fatalf("esc restored %q", got)
	}
}
