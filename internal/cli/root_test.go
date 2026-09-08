package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

func TestCommandRoutes(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want tui.Launch
	}{
		{name: "toolbox", want: tui.Launch{}},
		{name: "demo", args: []string{"demo"}, want: tui.Launch{App: "demo", Command: "demo"}},
		{name: "photo", args: []string{"photo"}, want: tui.Launch{App: "photo"}},
		{name: "photo import", args: []string{"photo", "import"}, want: tui.Launch{App: "photo", Command: "import"}},
		{name: "photo encode", args: []string{"photo", "encode"}, want: tui.Launch{App: "photo", Command: "encode"}},
		{name: "gpx", args: []string{"gpx"}, want: tui.Launch{App: "gpx"}},
		{name: "gpx import", args: []string{"gpx", "import"}, want: tui.Launch{App: "gpx", Command: "import"}},
		{name: "gpx inspect", args: []string{"gpx", "inspect"}, want: tui.Launch{App: "gpx", Command: "inspect"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got tui.Launch
			command := NewRootCommand(apps.All(), func(launch tui.Launch) error {
				got = launch
				return nil
			})
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("launch = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestExportConfigFlagWritesDefaultsWithoutStartingTUI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dgs", "config.json")
	t.Setenv(config.EnvPath, path)
	called := false
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		called = true
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--export-config", path})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("export started the TUI")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"disk": true`) || !strings.Contains(output.String(), path) {
		t.Fatalf("config=%s output=%q", data, output.String())
	}
}
