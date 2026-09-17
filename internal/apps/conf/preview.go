package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/render"
)

// previewState holds one target's rendered output, or the error rendering it
// produced, without writing anything. See
// docs/apps/conf/export.md#previewing.
type previewState struct {
	target string
	lines  []string
	err    error
	scroll int
}

// renderTarget runs the same rendering the export performs for one target,
// reading its template, defaults, values and secrets from disk.
func (m Model) renderTarget(service, roleName, instance string) ([]byte, error) {
	svc, role, inst, err := m.lookup(service, roleName, instance)
	if err != nil {
		return nil, err
	}

	serviceDir := filepath.Join(m.rootPath, svc.Dir)

	templatePath := filepath.Join(serviceDir, role.Role.Template)
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", templatePath, err)
	}

	defaultsPath := filepath.Join(serviceDir, role.Name, confgen.DefaultsFilename)
	defaultsBytes, err := os.ReadFile(defaultsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading defaults %s: %w", defaultsPath, err)
	}

	valuesPath := filepath.Join(m.rootPath, inst.Path)
	valuesBytes, err := os.ReadFile(valuesPath)
	if err != nil {
		return nil, fmt.Errorf("reading values %s: %w", valuesPath, err)
	}

	var secretsBytes []byte
	if svc.Manifest.Secrets != "" {
		if m.secretsDir == "" {
			return nil, fmt.Errorf("%s declares a secrets file, but conf.secrets is not configured", service)
		}
		secretsPath := filepath.Join(m.secretsDir, svc.Manifest.Secrets)
		secretsBytes, err = os.ReadFile(secretsPath)
		if err != nil {
			return nil, fmt.Errorf("reading secrets %s: %w", secretsPath, err)
		}
	}

	return render.Render(render.Input{
		Target:       render.Target{Service: service, Role: roleName, Instance: instance},
		Template:     string(templateBytes),
		Defaults:     defaultsBytes,
		DefaultsKind: role.Role.Defaults,
		Values:       valuesBytes,
		Secrets:      secretsBytes,
	})
}

// lookup finds a service, one of its roles and one of the role's instances
// by name, in the root loaded at construction.
func (m Model) lookup(service, roleName, instance string) (confgen.Service, confgen.RoleInstances, confgen.Instance, error) {
	for _, svc := range m.confRoot.Services {
		if svc.Name != service {
			continue
		}
		if svc.Broken != "" {
			return confgen.Service{}, confgen.RoleInstances{}, confgen.Instance{}, fmt.Errorf("%s: manifest is broken: %s", service, svc.Broken)
		}
		for _, role := range svc.Roles {
			if role.Name != roleName {
				continue
			}
			for _, inst := range role.Instances {
				if inst.Name != instance {
					continue
				}
				return svc, role, inst, nil
			}
		}
	}
	return confgen.Service{}, confgen.RoleInstances{}, confgen.Instance{}, fmt.Errorf("%s/%s/%s: not found", service, roleName, instance)
}

// openPreview renders the row under the cursor, when it is a checkable
// instance, and opens the preview.
func (m *Model) openPreview() {
	item, ok := m.list.Selected()
	if !ok {
		return
	}
	var r row
	for _, cand := range m.rows {
		if cand.id == item.ID {
			r = cand
			break
		}
	}
	if r.service != nil || r.role != nil || r.checkbox == "" {
		return // not a checkable instance row
	}
	t := strings.TrimPrefix(r.id, "inst:")
	parts := strings.SplitN(t, "/", 3)
	if len(parts) != 3 {
		return
	}

	out, err := m.renderTarget(parts[0], parts[1], parts[2])
	p := &previewState{target: t, err: err}
	if err == nil {
		p.lines = strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	}
	m.preview = p
}

func (m *Model) closePreview() { m.preview = nil }

// handlePreviewKey handles a key while the preview is open. It always
// reports the key as consumed: the preview owns every key until Esc.
func (m *Model) handlePreviewKey(key string) {
	switch key {
	case "esc":
		m.closePreview()
	case "down", "j":
		m.preview.scroll++
	case "up", "k":
		m.preview.scroll = max(0, m.preview.scroll-1)
	case "pgdown", "ctrl+d":
		m.preview.scroll += max(1, m.height/2)
	case "pgup", "ctrl+u":
		m.preview.scroll = max(0, m.preview.scroll-max(1, m.height/2))
	case "g", "home":
		m.preview.scroll = 0
	case "G", "end":
		m.preview.scroll = max(0, len(m.preview.lines)-1)
	}
}

func (m Model) previewView() string {
	if m.preview.err != nil {
		return m.centered(titleStyle.Render(m.preview.target) + "\n\n" + brokenStyle.Render(m.preview.err.Error()))
	}

	room := max(1, m.height-2)
	scroll := min(m.preview.scroll, max(0, len(m.preview.lines)-1))
	end := min(len(m.preview.lines), scroll+room)
	body := strings.Join(m.preview.lines[scroll:end], "\n")

	header := titleStyle.Render(m.preview.target)
	warning := mutedStyle.Render("plaintext — stays in your terminal's scrollback")
	return header + "\n" + warning + "\n\n" + body
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
