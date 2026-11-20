package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
	"filippo.io/age"
)

func TestCredShellCompletion(t *testing.T) {
	root := t.TempDir()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	makeConfig := func(name, host, group string) string {
		t.Helper()
		dir := filepath.Join(root, name)
		write := func(name string, data []byte) {
			t.Helper()
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		settings, err := json.Marshal(map[string]any{"recipients": filepath.Join(dir, "recipients"), "identities": []string{filepath.Join(dir, "not-read")}})
		if err != nil {
			t.Fatal(err)
		}
		write("cred/config.json", settings)
		write("recipients/hosts/"+host+".json", []byte(`{"keys":[{"public_key":"`+id.Recipient().String()+`","description":"Synthetic"}]}`))
		write("recipients/groups/"+group+".json", []byte(`{"hosts":["`+host+`"]}`))
		return dir
	}
	first := makeConfig("first", "host-a", "g-team")
	second := makeConfig("second", "host-b", "g-backup")
	t.Setenv(config.EnvPath, first)
	run := func(args ...string) string {
		t.Helper()
		cmd := NewRootCommand(apps.All(), func(tui.Launch) error { t.Fatal("completion launched TUI"); return nil })
		var out, diagnostics bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&diagnostics)
		cmd.SetArgs(append([]string{"__complete"}, args...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err, diagnostics.String())
		}
		return out.String()
	}
	for _, tc := range []struct {
		name                 string
		args                 []string
		candidate, directive string
	}{
		{"host shorthand", []string{"cred", "e", "-H", "host-"}, "host-a", ":4"},
		{"group shorthand", []string{"cred", "e", "-G", "g-"}, "g-team", ":4"},
		{"long alias", []string{"cred", "encrypt", "--group", "g-"}, "g-team", ":4"},
		{"repeated hosts", []string{"cred", "e", "-H", "host-a", "--host", "host-"}, "host-a", ":4"},
		{"prefix miss", []string{"cred", "e", "-H", "missing"}, "", ":4"},
		{"config override", []string{"--config", second, "cred", "e", "-H", "host-"}, "host-b", ":4"},
		{"source paths", []string{"cred", "e", ""}, "", ":0"},
		{"output paths", []string{"cred", "d", "-o", ""}, "", ":0"},
		{"no extra sources", []string{"cred", "e", "notes.txt", ""}, "", ":4"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output := run(tc.args...)
			if !strings.Contains(output, tc.candidate) || !strings.Contains(output, tc.directive+"\n") {
				t.Fatalf("%q", output)
			}
			if tc.candidate == "" && strings.TrimSpace(output) != tc.directive {
				t.Fatalf("unexpected candidates: %q", output)
			}
		})
	}
	// Sharing a public key is a warning, not a reason to suppress completion.
	shared, err := os.ReadFile(filepath.Join(first, "recipients", "hosts", "host-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(first, "recipients", "hosts", "host-b.json"), shared, 0600); err != nil {
		t.Fatal(err)
	}
	if output := run("cred", "e", "-H", "host-"); !strings.Contains(output, "host-a") || !strings.Contains(output, "host-b") || !strings.Contains(output, ":4") {
		t.Fatal(output)
	}
	// Configuration failures must not fall back to filenames or execute actions.
	if err := os.WriteFile(filepath.Join(first, "cred", "config.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if output := run("cred", "e", "-H", ""); strings.TrimSpace(output) != ":5" {
		t.Fatalf("%q", output)
	}
}

func TestShellCompletionScripts(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			cmd := NewRootCommand(apps.All(), func(tui.Launch) error { t.Fatal("script generation launched TUI"); return nil })
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"completion", shell})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "__complete") || !strings.Contains(out.String(), "dgs") {
				t.Fatal("missing completion protocol")
			}
			t.Run("syntax", func(t *testing.T) {
				executable, err := exec.LookPath(shell)
				if err != nil {
					t.Skip("shell not installed")
				}
				check := exec.Command(executable, "-n")
				check.Stdin = bytes.NewReader(out.Bytes())
				if diagnostics, err := check.CombinedOutput(); err != nil {
					t.Fatalf("%v: %s", err, diagnostics)
				}
			})
		})
	}
}
