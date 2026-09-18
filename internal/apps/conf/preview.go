package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/render"
	"dgs-toolbox/internal/conf/secretstore"
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

// renderTarget runs the same rendering the export performs for one target:
// its template and role defaults from the service directory, and its render
// context — node, instance, upstream, principals and own — from the
// inventory's derivation and conf.secrets.
func (m Model) renderTarget(instance string) ([]byte, error) {
	t, err := m.findTarget(instance)
	if err != nil {
		return nil, err
	}

	manifest, ok := m.manifests[t.Service]
	if !ok {
		return nil, fmt.Errorf("%s: service %q is not defined", instance, t.Service)
	}
	role, ok := manifest.Roles[t.Role]
	if !ok {
		return nil, fmt.Errorf("%s: service %q has no role %q", instance, t.Service, t.Role)
	}
	serviceDir := filepath.Join(m.rootPath, m.serviceDirs[t.Service])

	templatePath := filepath.Join(serviceDir, role.Template)
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", templatePath, err)
	}

	defaultsPath := filepath.Join(serviceDir, t.Role, confgen.DefaultsFilename)
	defaultsBytes, err := os.ReadFile(defaultsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading defaults %s: %w", defaultsPath, err)
	}

	instanceMap, nodeMap := m.instanceAndNode(instance, t.Node)

	var own map[string]string
	if m.secretsDir != "" {
		own, err = secretstore.ReadOwn(m.secretsDir, instance)
		if err != nil {
			return nil, err
		}
	}

	principals, err := m.principalsFor(instance)
	if err != nil {
		return nil, err
	}

	upstream, err := m.upstreamFor(instance)
	if err != nil {
		return nil, err
	}

	return render.Render(render.Input{
		Target:       render.Target{Service: t.Service, Role: t.Role, Instance: instance},
		Template:     string(templateBytes),
		Defaults:     defaultsBytes,
		DefaultsKind: role.Defaults,
		Instance:     instanceMap,
		Node:         nodeMap,
		Upstream:     upstream,
		Principals:   principals,
		Own:          own,
	})
}

// findTarget locates instance among the tree's targets.
func (m Model) findTarget(instance string) (targetRef, error) {
	for _, n := range m.nodes {
		for _, inst := range n.instances {
			if inst.name != instance {
				continue
			}
			if inst.broken != "" {
				return targetRef{}, fmt.Errorf("%s: %s", instance, inst.broken)
			}
			service, role, _ := strings.Cut(inst.detail, " / ")
			return targetRef{Node: n.name, Service: service, Role: role}, nil
		}
	}
	return targetRef{}, fmt.Errorf("%s: not found", instance)
}

type targetRef struct {
	Node    string
	Service string
	Role    string
}

// instanceAndNode builds the instance and node datasources for instance,
// whether it is a real, authored instance or one derive.Derive produced.
func (m Model) instanceAndNode(instance, nodeID string) (instanceMap, nodeMap map[string]any) {
	for _, n := range m.invRoot.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == instance && inst.Service != "" {
				return instanceValues(inst), nodeValues(n)
			}
		}
	}
	for _, ci := range m.derived.ClientInstances {
		if ci.ID != instance {
			continue
		}
		instanceMap = map[string]any{"id": ci.ID, "service": ci.Service, "role": ci.Role, "bind": ci.Bind}
		if len(ci.Ports) > 0 {
			instanceMap["ports"] = ci.Ports
		}
		if len(ci.Values) > 0 {
			instanceMap["values"] = ci.Values
		}
		if ci.Node != "" {
			for _, n := range m.invRoot.Nodes {
				if n.ID == ci.Node && n.Broken == "" {
					nodeMap = nodeValues(n)
				}
			}
		}
		return instanceMap, nodeMap
	}
	return nil, nil
}

func instanceValues(inst inventory.Instance) map[string]any {
	v := map[string]any{"id": inst.ID, "service": inst.Service, "role": inst.Role, "bind": inst.Bind}
	if len(inst.Ports) > 0 {
		v["ports"] = inst.Ports
	}
	if len(inst.Values) > 0 {
		v["values"] = inst.Values
	}
	return v
}

func nodeValues(n inventory.Node) map[string]any {
	networks := map[string]any{}
	for name, addr := range n.Networks {
		networks[name] = addr
	}
	return map[string]any{"id": n.ID, "networks": networks}
}

// principalsFor reads every per-principal port's accounts and secrets for
// instance.
func (m Model) principalsFor(instance string) (map[string][]render.Principal, error) {
	var ports map[string]int
	for _, n := range m.invRoot.Nodes {
		for _, inst := range n.Instances {
			if inst.ID == instance {
				ports = inst.Ports
			}
		}
	}
	if len(ports) == 0 {
		return nil, nil
	}

	// A role whose template cannot emit two accounts for one principal
	// declares rotation: disruptive — then a .previous value never becomes
	// a second account. See docs/apps/conf/inventory.md#rotation.
	rotatesInPlace := true
	if t, err := m.findTarget(instance); err == nil {
		if role, ok := m.manifests[t.Service].Roles[t.Role]; ok {
			rotatesInPlace = role.Rotation != confgen.RotationDisruptive
		}
	}

	out := map[string][]render.Principal{}
	for port := range ports {
		for _, p := range m.derived.Principals(instance, port) {
			path := secretstore.Path{Instance: instance, Port: port, Kind: string(p.Kind), Name: p.ID}
			principal := render.Principal{Name: p.Name}
			if m.secretsDir != "" {
				v, err := secretstore.ReadValue(m.secretsDir, path)
				if err != nil {
					return nil, err
				}
				principal.Secret = v
			}
			out[port] = append(out[port], principal)

			if m.secretsDir != "" && rotatesInPlace {
				// A role rendering an account table emits both the current
				// and the previous value as two accounts, so the old
				// credential keeps working until the rotation finishes.
				if v, ok, err := secretstore.ReadPrevious(m.secretsDir, path); err != nil {
					return nil, err
				} else if ok {
					out[port] = append(out[port], render.Principal{Name: p.Name, Secret: v})
				}
			}
		}
	}
	return out, nil
}

// upstreamFor resolves instance's own upstream, the one edge derive.Derive
// found leaving it, or nil for a terminal instance.
func (m Model) upstreamFor(instance string) (map[string]any, error) {
	var edge *derive.Edge
	for i := range m.derived.Edges {
		e := &m.derived.Edges[i]
		if e.FromInstance == instance || e.From.Instance == instance {
			edge = e
			break
		}
	}
	if edge == nil {
		return nil, nil
	}

	var kind, id string
	if e := edge; e.FromInstance != "" {
		// A derived client instance's principal is a node or a user,
		// whichever produced it — found the same way secretstore paths are.
		kind, id = principalFor(m, instance)
	} else {
		kind, id = string(derive.PrincipalInstance), instance
	}

	var secret string
	if m.secretsDir != "" && kind != "" {
		v, err := secretstore.ReadValue(m.secretsDir, secretstore.Path{
			Instance: edge.To.Instance, Port: edge.To.Port, Kind: kind, Name: id,
		})
		if err != nil {
			return nil, err
		}
		secret = v
	}

	out := map[string]any{"address": edge.Address, "port": edge.Port, "secret": secret}
	if combineOwn := destinationCombineOwn(m, edge.To.Instance); combineOwn != "" && m.secretsDir != "" {
		own, err := secretstore.ReadOwn(m.secretsDir, edge.To.Instance)
		if err != nil {
			return nil, err
		}
		v, ok := own[combineOwn]
		if !ok {
			return nil, fmt.Errorf("%s: missing own secret %q that %s/%s combines with each principal's own",
				edge.To.Instance, combineOwn, edge.To.Instance, edge.To.Port)
		}
		out["own"] = v
	}
	return out, nil
}

// destinationCombineOwn is instance's role's combine_own, or empty when it
// has none — the own secret every client reaching instance also needs
// alongside its own principal secret. See
// docs/apps/conf/inventory.md#a-shared-identity-alongside-a-principals-own.
func destinationCombineOwn(m Model, instance string) string {
	for _, n := range m.invRoot.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == instance {
				return m.manifests[inst.Service].Roles[inst.Role].CombineOwn
			}
		}
	}
	return ""
}

// principalFor finds the principal kind and identifier a derived client
// instance grants as, on its own entry edge.
func principalFor(m Model, instance string) (kind, id string) {
	for _, ci := range m.derived.ClientInstances {
		if ci.ID != instance {
			continue
		}
		if ci.Node != "" {
			return string(derive.PrincipalNode), ci.Node
		}
		return string(derive.PrincipalUser), ci.User
	}
	return "", ""
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
	if r.node != nil || r.checkbox == "" {
		return // not a checkable instance row
	}
	instance := r.keys[0]

	out, err := m.renderTarget(instance)
	p := &previewState{target: instance, err: err}
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
