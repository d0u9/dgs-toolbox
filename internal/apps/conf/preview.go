package conf

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
	"dgs-toolbox/internal/conf/render"
	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/conf/target"
)

// renderTarget runs the same rendering the export performs for one target:
// its template and role defaults from the service directory, and its render
// context — node, instance, upstream, principals and own — from the
// inventory's derivation and conf.secrets.
func (m renderer) renderTarget(instance string) ([]byte, error) {
	t, err := m.findTarget(instance)
	if err != nil {
		return nil, err
	}

	// A target is either a deployment, rendered from its service, or a file
	// written out for a person, rendered from its export. The two live in
	// different directories and have different manifests; everything below
	// this point is the same for both.
	var template, defaults, output, dir string
	// What this target needs from its upstream is the consumer's own
	// declaration: a service and an export each say it for themselves.
	var wants confgen.UpstreamDecls
	var fansOut bool
	switch {
	case t.Export != "":
		export, ok := m.l.exports[confgen.ExportKey(t.Service, t.Export)]
		if !ok {
			return nil, fmt.Errorf("%s: export %q is not defined", instance, t.Export)
		}
		if export.Template == "" {
			return nil, fmt.Errorf("%s: export %q writes nothing", instance, t.Export)
		}
		template, defaults, output, wants = export.Template, export.Defaults, export.Output, export.Upstream
		dir = filepath.Join(m.rootPath, m.l.exportDirs[confgen.ExportKey(t.Service, t.Export)])
	default:
		manifest, ok := m.l.manifests[t.Service]
		if !ok {
			return nil, fmt.Errorf("%s: service %q is not defined", instance, t.Service)
		}
		if manifest.Template == "" {
			return nil, fmt.Errorf("%s: service %q renders nothing", instance, t.Service)
		}
		template, defaults, output, wants = manifest.Template, manifest.Defaults, manifest.Output, manifest.Upstream
		fansOut = manifest.FansOut()
		dir = filepath.Join(m.rootPath, m.l.serviceDirs[t.Service])
	}
	_ = output

	templatePath := filepath.Join(dir, template)
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("reading template %s: %w", templatePath, err)
	}

	defaultsPath := filepath.Join(dir, confgen.DefaultsFilename)
	defaultsBytes, err := os.ReadFile(defaultsPath)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading defaults %s: %w", defaultsPath, err)
	}

	instanceMap, nodeMap := m.instanceAndNode(instance, t.Node)

	var own map[string]any
	if m.secretsDir != "" {
		own, err = secretstore.ReadSelf(m.secretsDir, instance)
		if err != nil {
			return nil, err
		}
	}

	principals, err := m.principalsFor(instance)
	if err != nil {
		return nil, err
	}

	// A fan-out instance has one successor per route, so there is no single
	// upstream to resolve: picking one of them would be arbitrary, and the
	// template reads downstreams instead.
	var upstream map[string]any
	downstreams := m.downstreamsFor(instance, fansOut)
	if !fansOut {
		upstream, err = m.upstreamFor(instance, wants)
		if err != nil {
			return nil, err
		}
	}

	return render.Render(render.Input{
		Target:       render.Target{Service: t.Service, Instance: instance},
		Template:     string(templateBytes),
		Defaults:     defaultsBytes,
		DefaultsKind: defaults,
		Instance:     instanceMap,
		Node:         nodeMap,
		Upstream:     upstream,
		Downstreams:  downstreams,
		Published:    m.publishedFor(instance),
		Principals:   principals,
		Self:         own,
	})
}

// findTarget locates instance among the tree's targets.
func (m renderer) findTarget(instance string) (targetRef, error) {
	for _, t := range target.List(m.l.inv, m.l.derived) {
		if t.Instance != instance {
			continue
		}
		if t.Broken != "" {
			return targetRef{}, fmt.Errorf("%s: %s", instance, t.Broken)
		}
		// An unmanaged user has no node, and their bundle is named for
		// them instead — see docs/apps/conf/export.md#what-is-written.
		node := t.Node
		if node == "" {
			node = t.User
		}
		return targetRef{Node: node, Service: t.Service, Export: t.Export}, nil
	}
	return targetRef{}, fmt.Errorf("%s: not found", instance)
}

type targetRef struct {
	Node    string
	Service string
	Export  string
}

// instanceAndNode builds the instance and node datasources for instance,
// whether it is a real, authored instance or one derive.Derive produced.
func (m renderer) instanceAndNode(instance, nodeID string) (instanceMap, nodeMap map[string]any) {
	for _, n := range m.l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == instance && inst.Service != "" {
				return instanceValues(inst), nodeValues(n)
			}
		}
	}
	for _, ci := range m.l.derived.ExportInstances {
		if ci.ID != instance {
			continue
		}
		instanceMap = map[string]any{"id": ci.ID, "export": ci.Export, "bind": ci.Bind}
		if len(ci.Ports) > 0 {
			instanceMap["ports"] = ci.Ports.Numbers()
		}
		if ci.Profile != "" {
			instanceMap["profile"] = ci.Profile
		}
		if len(ci.Values) > 0 {
			instanceMap["values"] = ci.Values
		}
		if ci.Node != "" {
			for _, n := range m.l.inv.Nodes {
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
	v := map[string]any{"id": inst.ID, "service": inst.Service, "bind": inst.Bind}
	if len(inst.Ports) > 0 {
		// A template asks what an instance listens on, not how: the
		// transport belongs to the model, and a role that needs it reads
		// it from its own defaults.
		v["ports"] = inst.Ports.Numbers()
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
func (m renderer) principalsFor(instance string) (map[string][]render.Principal, error) {
	var ports inventory.Ports
	for _, n := range m.l.inv.Nodes {
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
	// A service whose inbound side does not authenticate has no account
	// table and no credential under any of its ports: a grant on one of
	// them implies nothing. Grants still exist — they are who may reach
	// here, which is a separate fact from who holds a secret — so this
	// mirrors the same filter secretstore.ImpliedPaths applies, rather than
	// asking the secrets tree for files nothing ever generates. A web
	// service behind a reverse proxy is the case: the proxy holds a grant
	// on its port and no credential for it.
	authenticates := false
	if t, err := m.findTarget(instance); err == nil {
		if role, ok := m.l.manifests[t.Service]; ok {
			rotatesInPlace = role.Rotation != confgen.RotationDisruptive
			authenticates = role.Auth == confgen.AuthPerPrincipal
		}
	}
	if !authenticates {
		return nil, nil
	}

	out := map[string][]render.Principal{}
	for port := range ports {
		for _, p := range m.l.derived.Principals(instance, port) {
			path := secretstore.Path{Instance: instance, Port: port, Group: p.Group, Name: p.Slot}
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
// found leaving it, or nil for a terminal instance. wants is what the target
// being rendered declared it needs from that hop; anything it did not ask for
// is not read and does not reach its template.
func (m renderer) upstreamFor(instance string, wants confgen.UpstreamDecls) (map[string]any, error) {
	var edge *derive.Edge
	for i := range m.l.derived.Edges {
		e := &m.l.derived.Edges[i]
		if e.FromInstance == instance || e.From.Instance == instance {
			edge = e
			break
		}
	}
	if edge == nil {
		return nil, nil
	}

	var group, slot string
	if e := edge; e.FromInstance != "" {
		// A derived client instance's principal is a device or an unmanaged
		// user — found the same way secretstore paths are.
		group, slot = principalFor(m, instance)
	} else {
		// A relaying instance is its own group, holding one identity there.
		group, slot = instance, inventory.DefaultCredential
	}

	// A hop into a service whose inbound side does not authenticate has no
	// credential to read: a grant on one of its ports implies nothing, and
	// there is no file under it. Asking anyway is how a reverse proxy in
	// front of a web service used to fail — it dials a port that
	// authenticates nobody.
	var authenticates bool
	if to := m.instanceByID(edge.To.Instance); to != nil {
		authenticates = m.l.manifests[to.Service].Auth == confgen.AuthPerPrincipal
	}

	var secret string
	if m.secretsDir != "" && slot != "" && authenticates {
		v, err := secretstore.ReadValue(m.secretsDir, secretstore.Path{
			Instance: edge.To.Instance, Port: edge.To.Port, Group: group, Name: slot,
		})
		if err != nil {
			return nil, err
		}
		secret = v
	}

	// The account name the upstream's own table calls this principal. A
	// protocol whose client sends a user name as well as a secret —
	// Hysteria2's userpass, a share URI's userinfo — needs the name the
	// server will match, not the path segment the secret is filed under.
	account := slot
	for _, g := range m.l.derived.Grants {
		if g.Instance == edge.To.Instance && g.Port == edge.To.Port &&
			g.Principal.Group == group && g.Principal.Slot == slot {
			account = g.Principal.Name
			break
		}
	}

	out := map[string]any{"address": edge.Address, "port": edge.Port, "secret": secret, "account": account}
	// The name the hop's port answers to, when it has one. A client that
	// dials a service by its own name — ss.example.com for one program,
	// hy2.example.com for another on the same node — reads it here; the
	// address stays the node's, for a template that wants that instead.
	//
	// Only for an edge resolved on the universal network, which is where a
	// name is taken to resolve. An edge on loopback or a private network
	// was given the address that network needs, and a template writing
	// `or published address` must fall back to it rather than send a LAN
	// client out to a public name.
	published := ""
	if edge.Network != "" && edge.Network == m.l.inv.Universal {
		published = m.publishedAt(edge.To.Instance, edge.To.Port)
	}
	if published != "" {
		out["published"] = published
	}
	// A secret belongs to the instance it is filed under. It crosses to
	// another only because the program dialling says it needs it, so the
	// question asked here is what this target declared, not what the hop it
	// reaches happens to publish. A reverse proxy in front of a web service
	// declares nothing and is handed nothing.
	if wants.Wants(confgen.UpstreamShared) && m.secretsDir != "" {
		handed := destinationSelf(m, edge.To.Instance, edge.To.Port)
		own, err := secretstore.ReadSelf(m.secretsDir, edge.To.Instance)
		if err != nil {
			return nil, err
		}
		// In the order the port writes them: the protocols that take more
		// than one take them as a sequence, and a different order
		// authenticates nothing.
		values := make([]string, 0, len(handed))
		for _, ref := range handed {
			v, ok := lookupSelf(own, inventory.ParseSelfRef(ref))
			if !ok {
				return nil, fmt.Errorf("%s: %s needs the shared secrets of %s:%s, and %q is missing",
					edge.To.Instance, instance, edge.To.Instance, edge.To.Port, ref)
			}
			values = append(values, v)
		}
		out["shared"] = values
	}
	// The hop's own values, for a target that declared it needs them. A
	// parameter the two ends have to agree on — a Hysteria2 obfuscation
	// mode, the range a client hops ports over — is written on the instance
	// that listens, and reaching it here is what keeps the client's file
	// from holding a second copy of it. Nothing is filtered: these are
	// configuration, and the secrets a client is given arrive as shared.
	if wants.Wants(confgen.UpstreamValues) {
		if to := m.instanceByID(edge.To.Instance); to != nil && len(to.Values) > 0 {
			out["values"] = to.Values
		} else {
			out["values"] = map[string]any{}
		}
	}
	return out, nil
}

// destinationSelf is what instance's port hands to everything granted on
// it, in the order it writes them, or nil when it hands out nothing. See
// docs/apps/conf/inventory.md#a-secret-several-people-hold.
func destinationSelf(m renderer, instance, port string) []string {
	for _, n := range m.l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == instance {
				return inst.Ports[port].Self
			}
		}
	}
	return nil
}

// lookupSelf reads one reference out of the nested self datasource. A
// reference never names a field: half a credential is not a thing to hand
// over, so a name with fields is not found here.
func lookupSelf(self map[string]any, ref inventory.SelfRef) (string, bool) {
	switch v := self[ref.Name].(type) {
	case string:
		return v, ref.Key == ""
	case map[string]any:
		if ref.Key == "" {
			return "", false
		}
		s, ok := v[ref.Key].(string)
		return s, ok
	}
	return "", false
}

// principalFor finds the principal kind and identifier a derived client
// instance grants as, on its own entry edge.
func principalFor(m renderer, instance string) (group, slot string) {
	for _, ci := range m.l.derived.ExportInstances {
		if ci.ID != instance {
			continue
		}
		if ci.Node == "" {
			return ci.User, ci.Credential
		}
		for _, n := range m.l.inv.Nodes {
			if n.ID == ci.Node && n.Broken == "" {
				return n.Owner, ci.Credential
			}
		}
		return "", ""
	}
	return "", ""
}

// renderer is everything rendering one target needs and nothing else: the
// loaded inventory, the root its templates sit under, and the secrets store.
// It is deliberately not a TUI model — the same rendering serves the inspect
// page and `dgs conf export` on the command line, and only one of those has a
// cursor.
type renderer struct {
	l          loaded
	rootPath   string
	secretsDir string
}

// downstreamsFor is the hop that follows this instance in each route through
// it, resolved: what a reverse proxy renders one site block from. It is empty
// unless the service declared that it fans out, so a template that asks and
// was not meant to renders nothing rather than another instance's business.
//
// Ordered by route name, so a rendered file does not change because a route
// was added above another.
func (m renderer) downstreamsFor(instance string, fansOut bool) []render.Downstream {
	if !fansOut {
		return nil
	}
	var out []render.Downstream
	for _, e := range m.l.derived.Edges {
		if e.From.Instance != instance {
			continue
		}
		out = append(out, render.Downstream{
			Route:     e.Route,
			Instance:  e.To.Instance,
			Port:      e.To.Port,
			Published: m.publishedAt(e.To.Instance, e.To.Port),
			Address:   e.Address,
			Number:    e.Port,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Route < out[j].Route })
	return out
}

// publishedFor is this instance's own ports' published names. The service
// behind a proxy renders the same string the proxy matches on — Vaultwarden's
// DOMAIN is the case it exists for — so both sides read one value.
func (m renderer) publishedFor(instance string) map[string]string {
	inst := m.instanceByID(instance)
	if inst == nil {
		return nil
	}
	out := map[string]string{}
	for name, p := range inst.Ports {
		if p.Published != "" {
			out[name] = p.Published
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// publishedAt is one port's published name, or empty. Rule 17 has already
// reported a fan-out downstream that has none, so rendering an empty match
// here does not hide anything.
func (m renderer) publishedAt(instance, port string) string {
	inst := m.instanceByID(instance)
	if inst == nil {
		return ""
	}
	return inst.Ports[port].Published
}

// instanceByID finds a real instance in the inventory.
func (m renderer) instanceByID(instance string) *inventory.Instance {
	for _, n := range m.l.inv.Nodes {
		for i := range n.Instances {
			if n.Instances[i].ID == instance {
				return &n.Instances[i]
			}
		}
	}
	return nil
}
