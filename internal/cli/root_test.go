package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
)

func TestRepeatableActionFlagPreservesEachOccurrence(t *testing.T) {
	var got map[string]string
	app := tui.App{ID: "example", Name: "Example", Actions: []tui.Action{{
		ID: "move", Args: 0,
		Flags: []tui.ActionFlag{{Name: "network", Repeatable: true}},
		Run: func(_ io.Reader, _ io.Writer, _ []string, flags map[string]string) error {
			got = flags
			return nil
		},
	}}}
	command := NewRootCommand([]tui.App{app}, func(tui.Launch) error { return nil })
	command.SetArgs([]string{"example", "move", "--network", "from=network-4,to=network-8,address=10.0.1.8", "--network", "from=tailnet,address=10.0.2.8"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var values []string
	if err := json.Unmarshal([]byte(got["network"]), &values); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(values, []string{"from=network-4,to=network-8,address=10.0.1.8", "from=tailnet,address=10.0.2.8"}) {
		t.Fatalf("network flags = %v", values)
	}
}

func TestPhotoImportWithPathsRunsWithoutTUI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvPath, filepath.Join(root, "config"))
	source := filepath.Join(root, "card")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "a.JPG"), []byte("synthetic photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	targets := []string{filepath.Join(root, "library,a"), filepath.Join(root, "library-b")}
	for _, target := range targets {
		if err := os.Mkdir(target, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		t.Fatal("CLI import launched TUI")
		return nil
	})
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"photo", "import", source, "--dest", targets[0], "--dest", targets[1], "--ext", "JPG"})
	if err := command.Execute(); err != nil {
		t.Fatal(err, output.String())
	}
	for _, target := range targets {
		if data, err := os.ReadFile(filepath.Join(target, "a.JPG")); err != nil || string(data) != "synthetic photo" {
			t.Fatal("destination occurrence was not preserved", err)
		}
	}
}

func TestPhotoImportFlagsWithoutSourceDoNotOpenTUI(t *testing.T) {
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		t.Fatal("invalid CLI invocation launched TUI")
		return nil
	})
	command.SetArgs([]string{"photo", "import", "--dest", "library"})
	if err := command.Execute(); err == nil {
		t.Fatal("missing source accepted")
	}
}

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
	path := filepath.Join(t.TempDir(), "dgs")
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
	shell := filepath.Join(path, "shell", config.PartFile)
	data, err := os.ReadFile(shell)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"disk": true`) || !strings.Contains(output.String(), shell) {
		t.Fatalf("config=%s output=%q", data, output.String())
	}
}

func TestConfigFlagIsPassedToDirectCommandAndOverridesEnvironment(t *testing.T) {
	var got tui.Launch
	command := NewRootCommand(apps.All(), func(launch tui.Launch) error {
		got = launch
		return nil
	})
	command.SetArgs([]string{"photo", "import", "-c", "/tmp/explicit-config"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.ConfigPath != "/tmp/explicit-config" {
		t.Fatalf("config path = %q", got.ConfigPath)
	}
}

// A report answers on stdout instead of opening the TUI, and it is registered
// from the app's own registry entry rather than special-cased by the CLI.
func TestReportFlagWritesToStdoutWithoutStartingTUI(t *testing.T) {
	t.Setenv(config.EnvPath, filepath.Join(t.TempDir(), "dgs"))
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

// Help must be available without running a workspace or action, including direct
// apps (whose internal leaf is not exposed as a subcommand).
func TestEveryRegisteredCommandHasUsageHelp(t *testing.T) {
	paths := [][]string{{}}
	for _, app := range apps.All() {
		paths = append(paths, []string{app.ID})
		if !app.Direct {
			for _, leaf := range app.Commands {
				paths = append(paths, []string{app.ID, leaf.ID})
			}
		}
		for _, action := range app.Actions {
			paths = append(paths, []string{app.ID, action.ID})
		}
	}
	for _, path := range paths {
		t.Run(strings.Join(path, "/"), func(t *testing.T) {
			var output bytes.Buffer
			command := NewRootCommand(apps.All(), func(tui.Launch) error { t.Fatal("help launched a workspace"); return nil })
			command.SetOut(&output)
			command.SetErr(&output)
			command.SetArgs(append(append([]string(nil), path...), "--help"))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			text := output.String()
			if !strings.Contains(strings.ToLower(text), "example") || !strings.Contains(text, "Usage:") {
				t.Fatalf("help lacks usage/examples:\n%s", text)
			}
			if strings.Contains(text, "placeholder") {
				t.Fatalf("stale placeholder help:\n%s", text)
			}
		})
	}
}

func TestActionAndWorkspaceMayNotShareAFlagName(t *testing.T) {
	app := tui.App{ID: "probe", Name: "Probe",
		Commands: []tui.Command{{ID: "run", Name: "Run", Flags: []tui.Flag{{Name: "dest"}}, New: func() tui.CommandModel { return nil }}},
		Actions:  []tui.Action{{ID: "run", Args: 1, Flags: []tui.ActionFlag{{Name: "dest"}}, Run: func(io.Reader, io.Writer, []string, map[string]string) error { return nil }}},
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a shared flag name was accepted silently")
		}
	}()
	NewRootCommand([]tui.App{app}, func(tui.Launch) error { return nil })
}

func TestPhotoImportPreviewAndCleanupFlagsRunWithoutTUI(t *testing.T) {
	root := t.TempDir()
	t.Setenv(config.EnvPath, filepath.Join(root, "config"))
	source, target := filepath.Join(root, "card"), filepath.Join(root, "library")
	for _, directory := range []string{source, target} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "a.JPG"), []byte("synthetic photo"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(extra ...string) string {
		t.Helper()
		command := NewRootCommand(apps.All(), func(tui.Launch) error { t.Fatal("CLI launched TUI"); return nil })
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetArgs(append([]string{"photo", "import", source, "--dest", target}, extra...))
		if err := command.Execute(); err != nil {
			t.Fatal(err, output.String())
		}
		return output.String()
	}
	if output := run("-n", "--move", "--organize", "--delete-state"); !strings.Contains(output, "Dry run") {
		t.Fatal(output)
	}
	if entries, err := os.ReadDir(target); err != nil || len(entries) != 0 {
		t.Fatalf("preview changed target: %v %v", entries, err)
	}
	run("--delete-state")
	if _, err := os.Stat(filepath.Join(target, config.Default().PhotoImportStateFile())); !os.IsNotExist(err) {
		t.Fatal("state not cleaned", err)
	}
	if _, err := os.Stat(filepath.Join(target, "a.JPG")); err != nil {
		t.Fatal(err)
	}
}

func TestBoxCommandIsRemoved(t *testing.T) {
	command := NewRootCommand(apps.All(), func(tui.Launch) error {
		t.Fatal("removed Box command opened a TUI")
		return nil
	})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"box"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("removed Box command: %v", err)
	}
}
