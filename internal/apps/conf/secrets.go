// This file is the Secrets tab of dgs conf inspect: the one view that reads
// the secrets tree itself rather than only the inventory. See
// docs/apps/conf/inspect.md#the-secrets-tab.
package conf

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/tui/scrolllist"
)

// secretState is what one credential path is, comparing what the inventory
// implies against what is on disk. The two directions are the whole point of
// the view: a file the inventory no longer implies and a path with no file
// are both silent everywhere else, and both break something.
type secretState int

const (
	secretPresent secretState = iota
	secretMissing
	secretOrphaned
)

func (s secretState) String() string {
	switch s {
	case secretMissing:
		return "missing — sync would generate it"
	case secretOrphaned:
		return "orphaned — nothing implies it; sync never deletes"
	default:
		return "present"
	}
}

// secretEntry is one credential: a path, what state it is in, and how old
// its rotation leftover is when it has one.
type secretEntry struct {
	path     secretstore.Path
	state    secretState
	previous time.Time // zero when there is no .previous beside it
}

// secretPortGroup is one port of one instance, or its reserved own/ level,
// holding the credentials under it.
type secretPortGroup struct {
	port    string
	entries []secretEntry
}

// secretGroup is one instance's whole subtree of the secrets store.
type secretGroup struct {
	instance string
	expanded bool
	ports    []secretPortGroup
}

// secretsModel is the Secrets tab's own state: the tree, and the error that
// stopped it being read, if any.
type secretsModel struct {
	dir     string
	groups  []*secretGroup
	loadErr error
	// counts summarise the whole tree for the status bar, where the number
	// that matters is how many paths are not in step.
	present, missing, orphaned int
	stale                      int // .previous files past the seven-day limit
}

// staleAfter is validate's rule 16: an overlap is a debt, not a state. See
// docs/apps/conf/inventory.md#rotation.
const staleAfter = 7 * 24 * time.Hour

// buildSecrets compares the paths the inventory implies against the files in
// dir, and arranges both into one tree. It reads the store and writes
// nothing: Sync computes the difference, Generate is what would act on it.
func buildSecrets(l InspectData, dir string) secretsModel {
	m := secretsModel{dir: dir}
	if dir == "" {
		m.loadErr = fmt.Errorf("conf.secrets is not configured")
		return m
	}

	implied := secretstore.ImpliedPaths(l.inv, l.manifests, l.derived)
	res, err := secretstore.Sync(dir, implied)
	if err != nil {
		m.loadErr = err
		return m
	}
	previous, err := secretstore.PreviousModTimes(dir)
	if err != nil {
		m.loadErr = err
		return m
	}

	missing := map[string]bool{}
	for _, p := range res.Missing {
		missing[p.String()] = true
	}

	byInstance := map[string]*secretGroup{}
	var groups []*secretGroup
	add := func(p secretstore.Path, state secretState) {
		g, ok := byInstance[p.Instance]
		if !ok {
			g = &secretGroup{instance: p.Instance, expanded: true}
			byInstance[p.Instance] = g
			groups = append(groups, g)
		}
		var port *secretPortGroup
		for i := range g.ports {
			if g.ports[i].port == p.Port {
				port = &g.ports[i]
			}
		}
		if port == nil {
			g.ports = append(g.ports, secretPortGroup{port: p.Port})
			port = &g.ports[len(g.ports)-1]
		}
		port.entries = append(port.entries, secretEntry{path: p, state: state, previous: previous[p.String()]})
	}

	for _, p := range implied {
		state := secretPresent
		if missing[p.String()] {
			state = secretMissing
		}
		add(p, state)
	}
	for _, p := range res.Orphaned {
		add(p, secretOrphaned)
	}

	for _, g := range groups {
		sort.Slice(g.ports, func(i, j int) bool {
			// own/ last: it is the instance's own material, not one of its
			// listeners, and reading the listeners first matches how the
			// Services tree is laid out.
			if (g.ports[i].port == secretstore.SelfPort) != (g.ports[j].port == secretstore.SelfPort) {
				return g.ports[j].port == secretstore.SelfPort
			}
			return g.ports[i].port < g.ports[j].port
		})
		for i := range g.ports {
			entries := g.ports[i].entries
			sort.Slice(entries, func(a, b int) bool { return entries[a].path.String() < entries[b].path.String() })
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].instance < groups[j].instance })
	m.groups = groups

	for _, g := range groups {
		for _, port := range g.ports {
			for _, e := range port.entries {
				switch e.state {
				case secretMissing:
					m.missing++
				case secretOrphaned:
					m.orphaned++
				default:
					m.present++
				}
				if !e.previous.IsZero() && time.Since(e.previous) > staleAfter {
					m.stale++
				}
			}
		}
	}
	return m
}

// secretTabItems is the Secrets index, laid out as the store itself is:
// instance, then port (or own), then one row per credential. The path on
// screen is the path on disk, so a row is something a reader can go and cat.
func secretTabItems(groups []*secretGroup) []scrolllist.Item {
	var items []scrolllist.Item
	for _, g := range groups {
		count := 0
		for _, port := range g.ports {
			count += len(port.entries)
		}
		items = append(items, scrolllist.Item{
			ID:     "secretinst:" + g.instance,
			Label:  foldArrow(g.expanded, count > 0) + " " + g.instance,
			Detail: plural(count, "credential"),
		})
		if !g.expanded {
			continue
		}
		for i, port := range g.ports {
			branch, cont := branchMid, trunk
			if i == len(g.ports)-1 {
				branch, cont = branchLast, trunkClosed
			}
			items = append(items, scrolllist.Item{
				ID:     "secretport:" + g.instance + "/" + port.port,
				Label:  "  " + branch + port.port,
				Detail: plural(len(port.entries), "credential"),
			})
			for _, e := range port.entries {
				items = append(items, scrolllist.Item{
					ID:     "secret:" + e.path.String(),
					Label:  "  " + cont + trunkClosed + e.leaf(),
					Detail: e.summary(),
				})
			}
		}
	}
	return items
}

// leaf is the part of a path below its port: "doug/phone", or a bare name
// for an own secret, which belongs to no group.
func (e secretEntry) leaf() string {
	if e.path.Group == "" {
		return e.path.Name
	}
	return e.path.Group + "/" + e.path.Name
}

// summary is the row's second line: its state, and its rotation leftover
// when it has one, since a .previous past seven days is an error validate
// reports and nothing else surfaces.
func (e secretEntry) summary() string {
	out := e.state.String()
	if e.previous.IsZero() {
		return out
	}
	age := time.Since(e.previous)
	note := fmt.Sprintf("%s · .previous, %d days old", out, int(age.Hours()/24))
	if age > staleAfter {
		note += " — past seven days, rotation unfinished"
	}
	return note
}

// renderSecretDetail is one credential, addressed by its own path. It never
// reads the value: revealing one is its own milestone, and a view that
// printed it as a matter of course would put every credential in the
// terminal's scrollback on the way past.
func renderSecretDetail(m secretsModel, l InspectData, id string) (string, error) {
	for _, g := range m.groups {
		for _, port := range g.ports {
			for _, e := range port.entries {
				if e.path.String() != id {
					continue
				}
				return e.detail(l), nil
			}
		}
	}
	return "", fmt.Errorf("secret %q not found", id)
}

// sharedHolders names everything granted on the ports that hand this self
// secret out, and nothing when no port does. Which ports hand out what is
// the instance's own statement, so two ports of one program can hand out
// two different values — see
// docs/apps/conf/inventory.md#a-secret-several-people-hold.
func sharedHolders(l InspectData, instance, name, key string) []string {
	var handedBy []string
	for _, n := range l.inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.ID == instance {
				handedBy = portsHandingOut(inst, name, key)
			}
		}
	}
	if len(handedBy) == 0 {
		return nil
	}
	on := map[string]bool{}
	for _, port := range handedBy {
		on[port] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, g := range l.derived.Grants {
		if g.Instance != instance || !on[g.Port] || seen[g.Principal.Name] {
			continue
		}
		seen[g.Principal.Name] = true
		out = append(out, g.Principal.Name)
	}
	sort.Strings(out)
	return out
}

func (e secretEntry) detail(l InspectData) string {
	var b strings.Builder
	line(&b, e.path.String(), e.state.String())

	held := "the instance's own"
	account := ""
	if e.path.Port == secretstore.SelfPort {
		// A self secret its role declares `shared` is not the instance's
		// alone: everything granted on one of its ports holds the same
		// value, so rotating it changes what all of them hold. Naming
		// them is the only place that blast radius is visible.
		if who := sharedHolders(l, e.path.Instance, e.path.Name, e.path.Key); len(who) > 0 {
			held = fmt.Sprintf("shared by %s", strings.Join(who, ", "))
		}
	} else {
		held = e.path.Group + "'s " + e.path.Name
		// Who this is, in the inventory's terms rather than the path's.
		// A node principal's ID carries its profile, and a node name may
		// contain a hyphen of its own, so the answer comes from the grant
		// that produced the path rather than from splitting the name.
		for _, g := range l.derived.Grants {
			if g.Instance != e.path.Instance || g.Port != e.path.Port {
				continue
			}
			if g.Principal.Group != e.path.Group || g.Principal.Slot != e.path.Name {
				continue
			}
			account = g.Principal.Name
			break
		}
		if account == "" {
			if u, ok := l.inv.Users[e.path.Group]; ok {
				account = u.Account(e.path.Group, e.path.Name)
			}
		}
	}
	line(&b, field("opens", e.path.Instance), field("held by", held), field("account", account))

	if !e.previous.IsZero() {
		days := int(time.Since(e.previous).Hours() / 24)
		note := fmt.Sprintf("a .previous value sits beside this one, %d days old", days)
		if time.Since(e.previous) > staleAfter {
			note += " — past seven days, validate reports this as an error"
		}
		line(&b, field("rotation", note))
		line(&b, "          delete it once every client renders the current value")
	}

	line(&b, "value not read — see docs/apps/conf/inspect.md#editing-a-secret")
	return b.String()
}

// secretsStatus is the Secrets tab's status-bar summary. The number worth
// putting there is how many paths are not in step, since a tree in step is
// the ordinary case and says nothing.
func (m secretsModel) secretsStatus() string {
	if m.loadErr != nil {
		return "secrets unreadable"
	}
	out := plural(m.present+m.missing+m.orphaned, "credential")
	var notes []string
	if m.missing > 0 {
		notes = append(notes, fmt.Sprintf("%d missing", m.missing))
	}
	if m.orphaned > 0 {
		notes = append(notes, fmt.Sprintf("%d orphaned", m.orphaned))
	}
	if m.stale > 0 {
		notes = append(notes, fmt.Sprintf("%d stale .previous", m.stale))
	}
	if len(notes) == 0 {
		return out + ", in step"
	}
	return out + " — " + strings.Join(notes, ", ")
}
