package box

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/config"
)

// TestInitAction_WritesTheMarkerAndNothingElse covers what init is for: the
// folder is a Box afterwards, and the commands that refuse to touch a folder
// without a marker now accept it.
func TestInitAction_WritesTheMarkerAndNothingElse(t *testing.T) {
	root := t.TempDir()
	global := config.Default()

	var out bytes.Buffer
	if err := initAction(nil, &out, []string{root}, nil, global); err != nil {
		t.Fatalf("initAction: %v", err)
	}

	if err := box.RequireBox(root, global.BoxMarker()); err != nil {
		t.Fatalf("RequireBox after init: %v", err)
	}
	marker := filepath.Join(root, global.BoxMarker())
	if !strings.Contains(out.String(), marker) {
		t.Errorf("output names no marker path:\n%s", out.String())
	}
}

// TestInitAction_UsesConfiguredRoot covers the form with no argument, which is
// how init is run once box.root is set.
func TestInitAction_UsesConfiguredRoot(t *testing.T) {
	root := t.TempDir()
	global := config.Default()
	global.Box.Root = root

	if err := initAction(nil, &bytes.Buffer{}, nil, nil, global); err != nil {
		t.Fatalf("initAction: %v", err)
	}
	if err := box.RequireBox(root, global.BoxMarker()); err != nil {
		t.Fatalf("RequireBox after init: %v", err)
	}
}

// TestInitAction_RefusesWithoutARoot covers the case that would otherwise
// create a Box in whatever directory the shell happened to be in.
func TestInitAction_RefusesWithoutARoot(t *testing.T) {
	err := initAction(nil, &bytes.Buffer{}, nil, nil, config.Default())
	if err == nil {
		t.Fatal("initAction with no directory and no box.root: want an error")
	}
	if !strings.Contains(err.Error(), "box.root") {
		t.Errorf("error does not name the setting to fix: %v", err)
	}
}

// TestOnlyTheInteractivePairAreCommands pins the split: import and view open a
// workspace because that is what they are for, and the four that write nothing
// into the Box and ask nothing report to stdout instead.
func TestOnlyTheInteractivePairAreCommands(t *testing.T) {
	app := New()

	var commands, actions []string
	for _, cmd := range app.Commands {
		commands = append(commands, cmd.ID)
	}
	for _, action := range app.Actions {
		actions = append(actions, action.ID)
	}

	wantCommands := []string{"import", "view"}
	wantActions := []string{"init", "index", "verify", "dedupe"}
	if !slices.Equal(commands, wantCommands) {
		t.Errorf("commands = %v, want %v", commands, wantCommands)
	}
	if !slices.Equal(actions, wantActions) {
		t.Errorf("actions = %v, want %v", actions, wantActions)
	}
}
