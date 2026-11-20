package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui"
	"filippo.io/age"
)

func TestCredCryptCLI(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	write("keys/key.txt", []byte(id.String()))
	write("recipients/hosts/host-a.json", []byte(`{"keys":[{"public_key":"`+id.Recipient().String()+`","description":"Synthetic"}]}`))
	write("recipients/groups/g-team.json", []byte(`{"hosts":["host-a"]}`))
	settings, err := json.Marshal(map[string]any{"identities": []string{filepath.Join(root, "keys")}, "recipients": filepath.Join(root, "recipients")})
	if err != nil {
		t.Fatal(err)
	}
	write("conf/cred/config.json", settings)
	t.Setenv(config.EnvPath, filepath.Join(root, "conf"))
	run := func(args ...string) (string, error) {
		t.Helper()
		cmd := NewRootCommand(apps.All(), func(tui.Launch) error { t.Fatal("CLI launched TUI"); return nil })
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&out)
		cmd.SetArgs(append([]string{"cred"}, args...))
		err := cmd.Execute()
		return out.String(), err
	}
	source := write("notes.txt", []byte("synthetic secret"))
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"missing source", []string{"e"}, "accepts 1 arg"},
		{"extra source", []string{"e", source, source}, "accepts 1 arg"},
		{"unknown option", []string{"e", source, "--hots", "host-a"}, "unknown flag"},
		{"missing option value", []string{"e", source, "-G"}, "needs an argument"},
		{"empty host", []string{"e", source, "-H", ""}, "nonempty host"},
		{"empty group", []string{"e", source, "-G", ""}, "nonempty group"},
		{"empty output", []string{"e", source, "-Hhost-a", "-o", ""}, "nonempty file path"},
		{"unknown host", []string{"e", source, "-Hhost-b"}, "dgs cred key hosts"},
		{"unknown group", []string{"e", source, "-Gteam"}, "g- prefix"},
		{"no recipients", []string{"e", source}, "select at least one"},
		{"missing file", []string{"d", filepath.Join(root, "missing.age")}, "read source"},
		{"directory decrypt", []string{"d", root}, "regular file"},
		{"ambiguous output", []string{"d", source}, "cannot infer output"},
		{"stdout refused", []string{"d", source, "-o-"}, "stdout output"},
		{"same output", []string{"e", source, "-Hhost-a", "-o", source}, "already exists"},
		{"decrypt recipient flag", []string{"d", source, "-G", "g-team"}, "unknown shorthand flag"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q; got %v (%s)", tc.want, err, out)
			}
			if strings.Contains(out, "Encrypted:") || strings.Contains(out, "Decrypted:") {
				t.Fatal("failure reported success")
			}
			if _, err := os.Stat(source + ".age"); !os.IsNotExist(err) {
				t.Fatal("failed command created output")
			}
		})
	}
	if _, err := run("e", source, "-Hhost-a", "-Gg-team", "-Hhost-a"); err != nil {
		t.Fatal(err)
	}
	// Inferred output must refuse the original plaintext rather than replace it.
	if _, err := run("d", source+".age"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("%v", err)
	}
	restored := filepath.Join(root, "restored.txt")
	if _, err := run("d", "-o", restored, "--", source+".age"); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(restored)
	if err != nil || string(got) != "synthetic secret" {
		t.Fatalf("%q %v", got, err)
	}
	// Default decryption name and long-name aliases use exactly the same implementation.
	alias := write("alias.txt", []byte("alias round trip"))
	if _, err := run("encrypt", alias, "--group", "g-team"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if _, err := run("decrypt", alias+".age"); err != nil {
		t.Fatal(err)
	}
	got, err = os.ReadFile(alias)
	if err != nil || string(got) != "alias round trip" {
		t.Fatalf("%q %v", got, err)
	}
	// A literal dash-prefixed filename is accepted after --.
	t.Chdir(root)
	write("-notes.txt", []byte("dash filename"))
	if _, err := run("e", "-Hhost-a", "--", "-notes.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "-notes.txt.age")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"e", "d", "encrypt", "decrypt"} {
		out, err := run(name, "--help")
		if err != nil || !strings.Contains(out, "Examples:") {
			t.Fatalf("%s: %s %v", name, out, err)
		}
	}
}
