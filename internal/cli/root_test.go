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
		{name: "capture", args: []string{"capture"}, want: tui.Launch{App: "capture", Command: "scan"}},
		{name: "photo", args: []string{"photo"}, want: tui.Launch{App: "photo"}},
		{name: "photo import", args: []string{"photo", "import"}, want: tui.Launch{App: "photo", Command: "import"}},
		{name: "photo encode", args: []string{"photo", "encode"}, want: tui.Launch{App: "photo", Command: "encode"}},
		{name: "geo", args: []string{"geo"}, want: tui.Launch{App: "geo"}},
		{name: "geo gpx", args: []string{"geo", "gpx"}, want: tui.Launch{App: "geo", Command: "gpx"}},
		{name: "cred", args: []string{"cred"}, want: tui.Launch{App: "cred"}},
		{name: "cred keys", args: []string{"cred", "keys"}, want: tui.Launch{App: "cred", Command: "keys"}},
		{name: "cred vault", args: []string{"cred", "vault"}, want: tui.Launch{App: "cred", Command: "vault"}},
		{name: "geo gpx listen", args: []string{"geo", "gpx", "--host", "0.0.0.0", "--port", "9000"}, want: tui.Launch{App: "geo", Command: "gpx", Flags: map[string]string{"host": "0.0.0.0", "port": "9000"}}},
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

func TestVersionFlagDoesNotStartTUI(t *testing.T) {
	called := false
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		called = true
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--version"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("version started the TUI")
	}
	if got := output.String(); !strings.Contains(got, "dgs version dev") {
		t.Fatalf("output = %q", got)
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

func TestConfigFlagIsPassedToDirectCommandAndOverridesEnvironment(t *testing.T) {
	var got tui.Launch
	command := NewRootCommand(apps.All(), func(launch tui.Launch) error {
		got = launch
		return nil
	})
	command.SetArgs([]string{"photo", "import", "-c", "/tmp/explicit-config.json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != "/tmp/explicit-config.json" {
		t.Fatalf("config path = %q", got.ConfigPath)
	}
}

// A report answers on stdout instead of opening the TUI, and it is registered
// from the app's own registry entry rather than special-cased by the CLI.
func TestReportFlagWritesToStdoutWithoutStartingTUI(t *testing.T) {
	t.Setenv(config.EnvPath, filepath.Join(t.TempDir(), "dgs", "config.json"))
	called := false
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		called = true
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	// The recipes are files, so an installation that has not been given them
	// has none: they are laid down first, by the flag that does that.
	command.SetArgs([]string{"capture", "--init"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	command.SetArgs([]string{"capture", "--recipes"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("a report flag started the TUI")
	}
	if !strings.Contains(output.String(), "RECIPES") || !strings.Contains(output.String(), "obsidian_location_daily") {
		t.Fatalf("output = %q", output.String())
	}
}

// Without the flag the same command still launches its session.
func TestAppCommandStillLaunchesWhenNoReportIsRequested(t *testing.T) {
	var got tui.Launch
	command := NewRootCommand(apps.All(), func(launch tui.Launch) error {
		got = launch
		return nil
	})
	command.SetArgs([]string{"capture"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, tui.Launch{App: "capture", Command: "scan"}) {
		t.Fatalf("launch = %#v", got)
	}
}
