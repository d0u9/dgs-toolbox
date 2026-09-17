package conf

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// buildRenderableRoot is buildRoot plus the template, defaults and secrets
// files a preview actually needs to render something.
func buildRenderableRoot(t *testing.T) (root, secretsDir string) {
	t.Helper()
	root = t.TempDir()
	writeFile(t, filepath.Join(root, "hysteria2", "confgen.yaml"), `
secrets: hysteria2.yaml
roles:
  server:
    template: templates/server.yaml.tmpl
    defaults: document
    output: config.yaml
`)
	writeFile(t, filepath.Join(root, "hysteria2", "templates", "server.yaml.tmpl"),
		"listen: {{ .listen }}\npassword: {{ (secret .server).password }}\n")
	writeFile(t, filepath.Join(root, "hysteria2", "server", "defaults.yaml"), "listen: :443\n")
	writeFile(t, filepath.Join(root, "hysteria2", "server", "us-sfo.yaml"), "server: sfo-1\n")

	secretsDir = t.TempDir()
	writeFile(t, filepath.Join(secretsDir, "hysteria2.yaml"), "- servers: [sfo-1]\n  password: hunter2\n")
	return root, secretsDir
}

func TestPreview_RendersTheSameWayExportWould(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if m.preview.err != nil {
		t.Fatalf("preview error: %v", m.preview.err)
	}
	got := strings.Join(m.preview.lines, "\n")
	want := "listen: :443\npassword: hunter2"
	if got != want {
		t.Fatalf("preview = %q, want %q", got, want)
	}
}

func TestPreview_MissingSecretShowsTheError(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	// Point the instance's secret_server-equivalent key at nothing on record.
	writeFile(t, filepath.Join(root, "hysteria2", "server", "us-sfo.yaml"), "server: does-not-exist\n")
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil || m.preview.err == nil {
		t.Fatal("want a preview error for a secret nothing matches")
	}
	if !strings.Contains(m.preview.err.Error(), "does-not-exist") {
		t.Fatalf("error = %q, want it to name the missing key", m.preview.err)
	}
}

func TestPreview_NoSecretsConfiguredRefuses(t *testing.T) {
	root, _ := buildRenderableRoot(t)
	m := newModel(root, "") // conf.secrets not configured
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)

	if m.preview == nil || m.preview.err == nil {
		t.Fatal("want a preview error when conf.secrets is not configured")
	}
	if !strings.Contains(m.preview.err.Error(), "conf.secrets") {
		t.Fatalf("error = %q, want it to name conf.secrets", m.preview.err)
	}
}

func TestPreview_EscClosesAndReturnsToTheTree(t *testing.T) {
	root, secretsDir := buildRenderableRoot(t)
	m := newModel(root, secretsDir)
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/us-sfo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	if !m.CapturesShellKey("esc") {
		t.Fatal("CapturesShellKey(esc) = false while previewing, want true")
	}

	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = m2.(Model)
	if m.preview != nil {
		t.Fatal("esc did not close the preview")
	}
	if m.CapturesShellKey("esc") {
		t.Fatal("CapturesShellKey(esc) = true once the preview is closed")
	}
}

func TestPreview_BrokenInstanceCannotBePreviewed(t *testing.T) {
	m := newModel(buildRoot(t), "")
	m.width, m.height = 80, 24
	m.list.SelectID("inst:hysteria2/server/bad")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(Model)
	if m.preview != nil {
		t.Fatal("previewing a broken instance opened a preview")
	}
}
