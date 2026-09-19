// Package inventory reads a generator root's inventory: nodes/*.yaml,
// users.yaml, routes.yaml and networks.yaml, parsed into one model. It only
// parses — no derivation and no validation. A file that will not parse is
// listed with its error rather than dropped, as confgen already does for a
// broken manifest.
//
// The model is docs/apps/conf/inventory.md.
package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// NodesDir is the generator root's subdirectory holding one file per node.
const NodesDir = "nodes"

// UsersFilename, RoutesFilename and NetworksFilename name the generator
// root's other inventory files.
const (
	UsersFilename    = "users.yaml"
	RoutesFilename   = "routes.yaml"
	NetworksFilename = "networks.yaml"
)

// DevicesNone is the one value a user's `devices` key may take: this person
// has no node file in this inventory, and that is deliberate. It says only
// that, so it exists to tell a person with no devices apart from one whose
// device files were misplaced, which otherwise look identical. It decides
// nothing: a credential no device names is carried by the person whether or
// not this is written, and writing it beside a device file is a contradiction
// validate reports.
const DevicesNone = "none"

// DefaultCredential is the credential a person has when they declare none,
// and the one a device uses when it names none. A name like any other: it is
// written out in secret paths, in account names and in the picture, and it
// is simply the one you get without choosing.
const DefaultCredential = "default"

// ExportNone is the node.export value meaning: write nothing for this
// device, regardless of what the services it reaches offer.
const ExportNone = "none"

// Networks is a node's `networks` key: a mapping of network name to this
// node's address on it. It says where others can reach this node.
type Networks map[string]string

// Instance is one entry of a node's `instances` list: one service running on
// that node, and one rendered configuration file.
type Instance struct {
	ID      string `yaml:"id"`
	Service string `yaml:"service"`
	Role    string `yaml:"role"`
	Bind    string `yaml:"bind"`
	Ports   Ports  `yaml:"ports"`
	// Self says which of its service's own secrets this instance holds,
	// and the keys of each one that is a set. Unwritten means every name
	// the service declares, which is the ordinary case; written, it is the
	// whole list, so `self: {}` means none of them. A key a port hands out
	// is a key this instance has whether or not it is written here, so the
	// keys are worth writing only for one no port hands out. See
	// docs/apps/conf/inventory.md#a-services-own-secrets.
	Self map[string][]string `yaml:"self"`
	// Process names the running program this instance belongs to, for
	// instances that share one: a server listening on two ports from one
	// configuration file is two instances, because each port has its own
	// principals and its own grants, and one process. Empty means this
	// instance is a process of its own.
	Process string `yaml:"process"`
	// Values is this instance's own parameters — a Hysteria2 masquerade
	// target, a MicroBin public path, an nginx server block — whatever its
	// service and role need beyond where it runs and what it listens on.
	// dgs does not read it; a template reads it by name. See
	// docs/apps/conf/inventory.md#an-instances-own-values.
	Values map[string]any `yaml:"values"`
}

// IsUser reports whether a name is a user key, which is how a node group
// tells a person's devices from whoever hosts the machines.
func (r *Root) IsUser(name string) bool {
	_, ok := r.Users[name]
	return ok
}

// Protocol values a port may declare. TCP is the default, so a port that
// says nothing is a TCP port.
const (
	ProtocolTCP = "tcp"
	ProtocolUDP = "udp"
)

// Port is one listening port: its number, and the transport it listens on.
// Two ports collide only when they share an address, a number and a
// transport — a QUIC service on 443/udp and a web server on 443/tcp are
// both real and both correct.
type Port struct {
	Number   int    `yaml:"port"`
	Protocol string `yaml:"protocol"`
	// Self names the instance's own secrets everything granted on this port
	// also holds, in the order they are written — the order matters, since
	// the protocols taking more than one take them as a sequence. A `set`
	// secret is named <name>.<key>, anything else by name alone, and never
	// a field: a field is half a credential. See
	// docs/apps/conf/inventory.md#a-secret-several-people-hold.
	Self []string `yaml:"self"`
	// Published is the bare hostname this port answers to, for a port
	// reached from a browser. A reverse proxy in front matches its site
	// block on it, and the service behind renders the same string into its
	// own configuration — Vaultwarden's absolute URLs, cookie domain and
	// WebAuthn relying-party identifier are built from it, and a value that
	// disagrees with what the proxy serves breaks sign-in rather than the
	// page. It is a bare hostname and not a URL: the site block wants the
	// name alone, and a template needing a scheme writes one. See
	// docs/apps/conf/inventory.md#the-name-a-port-is-published-at.
	Published string `yaml:"published"`
}

// SelfRef is one entry of Port.Self, split: the secret's name, and the key
// of it for a `set` secret.
type SelfRef struct {
	Name string
	Key  string
}

// ParseSelfRef splits "psk.users" into its name and key, and "tls_key" into
// a name alone.
func ParseSelfRef(ref string) SelfRef {
	if name, key, ok := strings.Cut(ref, "."); ok {
		return SelfRef{Name: name, Key: key}
	}
	return SelfRef{Name: ref}
}

// SelfNames is which of the service's own secrets this instance holds,
// given every name the service declares. An instance that says nothing
// holds them all; one that writes `self` holds what it names, plus whatever
// its ports hand out — naming a value on a port is saying the instance has
// it. The result keeps declared's order.
func (i Instance) SelfNames(declared []string) []string {
	if i.Self == nil {
		return declared
	}
	held := map[string]bool{}
	for name := range i.Self {
		held[name] = true
	}
	for _, p := range i.Ports {
		for _, ref := range p.Self {
			held[ParseSelfRef(ref).Name] = true
		}
	}
	out := make([]string, 0, len(held))
	for _, name := range declared {
		if held[name] {
			out = append(out, name)
		}
	}
	return out
}

// SelfKeys is the keys of each `set` secret this instance holds: what its
// ports hand out, plus whatever it declares itself. The result is sorted,
// so a caller walking it produces the same order every time.
func (i Instance) SelfKeys() map[string][]string {
	seen := map[string]map[string]bool{}
	note := func(name, key string) {
		if key == "" {
			return
		}
		if seen[name] == nil {
			seen[name] = map[string]bool{}
		}
		seen[name][key] = true
	}
	for _, p := range i.Ports {
		for _, ref := range p.Self {
			r := ParseSelfRef(ref)
			note(r.Name, r.Key)
		}
	}
	for name, keys := range i.Self {
		for _, key := range keys {
			note(name, key)
		}
	}
	out := make(map[string][]string, len(seen))
	for name, keys := range seen {
		list := make([]string, 0, len(keys))
		for key := range keys {
			list = append(list, key)
		}
		sort.Strings(list)
		out[name] = list
	}
	return out
}

// ProtocolOr is the port's transport, defaulting to TCP.
func (p Port) ProtocolOr() string {
	if p.Protocol == "" {
		return ProtocolTCP
	}
	return p.Protocol
}

// Ports is an instance's `ports` key: port name to port. A name is written
// against a bare number for the common case, and against a mapping when the
// transport has to be said:
//
//	ports:
//	  web: 8080
//	  main: {port: 443, protocol: udp}
type Ports map[string]Port

// UnmarshalYAML accepts both spellings above, so adding a protocol to one
// port does not rewrite the others.
func (ps *Ports) UnmarshalYAML(value *yaml.Node) error {
	var raw map[string]yaml.Node
	if err := value.Decode(&raw); err != nil {
		return err
	}
	out := make(Ports, len(raw))
	for name, node := range raw {
		var number int
		if err := node.Decode(&number); err == nil {
			out[name] = Port{Number: number}
			continue
		}
		var port Port
		if err := node.Decode(&port); err != nil {
			return fmt.Errorf("port %q: want a number or {port, protocol}: %w", name, err)
		}
		switch port.ProtocolOr() {
		case ProtocolTCP, ProtocolUDP:
		default:
			return fmt.Errorf("port %q: protocol %q is not %s or %s", name, port.Protocol, ProtocolTCP, ProtocolUDP)
		}
		out[name] = port
	}
	*ps = out
	return nil
}

// PortsOf builds Ports from plain numbers, for a caller writing them in Go
// rather than parsing YAML — a test, or a fixture.
func PortsOf(numbers map[string]int) Ports {
	out := make(Ports, len(numbers))
	for name, n := range numbers {
		out[name] = Port{Number: n}
	}
	return out
}

// Numbers is the port numbers by name, for a caller that needs no transport
// — a template's `ports` datasource, a report of what an instance listens on.
func (ps Ports) Numbers() map[string]int {
	out := make(map[string]int, len(ps))
	for name, p := range ps {
		out[name] = p.Number
	}
	return out
}

// Node is one nodes/*.yaml file: a machine or a device.
type Node struct {
	ID    string `yaml:"id"`
	Owner string `yaml:"owner"`
	// Group is the directory this node's file sits in under nodes/, and is
	// not written in the file: nodes/doug/phone.yaml is grouped under
	// "doug", nodes/digitalocean/sfo1.yaml under "digitalocean". A file
	// directly in nodes/ has none. A group naming a user in users.yaml is
	// that user's devices, and fills Owner for every node in it; a group
	// naming no user is whoever the machines belong to — a provider, a
	// household, a company — and dgs reads nothing more into it.
	Group string `yaml:"-"`
	// Networks says where others can reach this node: network name to
	// address.
	Networks Networks `yaml:"networks"`
	// Reaches lists networks this node can open a connection on, without
	// being reachable on them — a phone or a laptop on the home LAN. Every
	// node reaches every network it has an address on, and every node
	// reaches Root.Universal implicitly, so Reaches is written only for the
	// remaining case. See
	// docs/apps/conf/inventory.md#reached-and-reaching.
	Reaches []string `yaml:"reaches"`
	// Export narrows what is written for this device to one of the ways the
	// services it reaches offer, or ExportNone to write nothing at all.
	// Empty takes every way they offer. See
	// docs/apps/conf/inventory.md#which-export-a-person-receives.
	Export    string     `yaml:"export"`
	Instances []Instance `yaml:"instances"`
	// Credential names which of its owner's credentials this device uses.
	// Empty means DefaultCredential. Two devices naming the same one hold
	// the same secret — one password across a laptop and a phone is a thing
	// people do, and the model says it rather than inventing two values the
	// server would then have to carry.
	Credential string `yaml:"credential"`

	// Path is the file's path, relative to the generator root. Set by Load,
	// not part of the YAML.
	Path string `yaml:"-"`
	// Broken is the parse error if the file is not valid YAML or not a
	// mapping, and empty otherwise. Fields above are valid only when this is
	// empty.
	Broken string `yaml:"-"`
}

// CredentialOr is the credential this device uses, defaulting to
// DefaultCredential.
func (n Node) CredentialOr() string {
	if n.Credential != "" {
		return n.Credential
	}
	return DefaultCredential
}

// User is one entry of users.yaml: a logical identity, not a Linux account.
type User struct {
	// Username is the account name the services see. Empty means it is the
	// same as the user's own map key — see UsernameOr.
	Username string `yaml:"username"`
	// Devices is DevicesNone for a person this inventory has no node file
	// for, and empty otherwise. An assertion, not a switch. See DevicesNone.
	Devices string `yaml:"devices"`
	// Export is the `export` for the files this person carries themselves:
	// the credentials no device of theirs names have no node to carry an
	// `export`, so it is written here instead and means the same thing.
	// Empty takes every way the services they reach offer.
	Export string `yaml:"export"`
	// Credentials are this person's credentials, by name. A person who
	// declares none has one called DefaultCredential.
	//
	// How many credentials someone keeps is theirs to decide, not the
	// service's: one password across every device is as legitimate as one
	// per device, and the server carries whatever number results. A device
	// names which one it uses; two naming the same one share a value.
	Credentials map[string]Credential `yaml:"credentials"`
	Access      []string              `yaml:"access"`
}

// Credential is one of a person's credentials: what it is for, and which of
// their routes it opens.
type Credential struct {
	// Note says where this credential is used — free text dgs never reads.
	Note string `yaml:"note"`
	// Access narrows this credential to some of the person's routes. Empty
	// takes every route they are granted, which is what a credential means
	// without one. It may name only routes the person holds: access is
	// granted to the person, and a credential chooses among what they
	// already have rather than reaching past it. See
	// docs/apps/conf/inventory.md#a-credential-may-open-fewer-routes.
	Access []string `yaml:"access"`
}

// CredentialNames returns this person's credential names in a stable order,
// or DefaultCredential alone when they declare none.
func (u User) CredentialNames() []string {
	if len(u.Credentials) == 0 {
		return []string{DefaultCredential}
	}
	out := make([]string, 0, len(u.Credentials))
	for name := range u.Credentials {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// AccessFor returns the routes one of this person's credentials opens: the
// credential's own list when it narrows, and the person's whole access
// otherwise. A name this person does not keep gets nothing, so a caller need
// not check first — validation reports the name, and a grant on nothing is
// the safe reading of it in the meantime.
func (u User) AccessFor(credential string) []string {
	c, ok := u.Credentials[credential]
	switch {
	case !ok && len(u.Credentials) == 0 && credential == DefaultCredential:
		// The credential a person has when they declare none.
	case !ok:
		return nil
	case len(c.Access) > 0:
		return c.Access
	}
	return u.Access
}

// OpensRoute reports whether one of this person's credentials opens a route.
func (u User) OpensRoute(credential, route string) bool {
	for _, name := range u.AccessFor(credential) {
		if name == route {
			return true
		}
	}
	return false
}

// Account is the name a service's own table calls one of this person's
// credentials: the username and the credential, always both, so that the
// table reads the same whether someone keeps one credential or five and
// adding a second never renames the first.
func (u User) Account(key, credential string) string {
	return u.UsernameOr(key) + "-" + credential
}

// UsernameOr returns u.Username, or key — the user's own map key — when
// Username is empty.
func (u User) UsernameOr(key string) string {
	if u.Username != "" {
		return u.Username
	}
	return key
}

// Route is one entry of routes.yaml: an ordered list of hops.
type Route struct {
	Hops []string `yaml:"hops"`
}

// Root is a generator root's parsed inventory.
type Root struct {
	// Nodes is every nodes/*.yaml file, sorted by file name, whether or not
	// it parses cleanly.
	Nodes []Node

	// Users is users.yaml's `users` map, keyed by user identifier. Empty and
	// UsersBroken set if the file exists but will not parse; empty and
	// UsersBroken empty if the file does not exist.
	Users       map[string]User
	UsersBroken string

	// Routes is routes.yaml's `routes` map, keyed by route name.
	Routes       map[string]Route
	RoutesBroken string

	// Networks is networks.yaml's preference-ordered list.
	Networks []string
	// Universal is networks.yaml's `universal` key: the one network every
	// node can reach without saying so. Empty means nothing is implicit.
	Universal      string
	NetworksBroken string
}

// Load parses a generator root's inventory. A missing nodes/ directory or a
// missing top-level file is an empty result for that part, not an error —
// whether the inventory is complete is validate's job, not this one's. The
// root itself not existing is an error.
func Load(root string) (*Root, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, fmt.Errorf("inventory: reading root: %w", err)
	}

	rt := &Root{}

	nodes, err := loadNodes(root)
	if err != nil {
		return nil, err
	}
	rt.Nodes = nodes

	users, brokenUsers, err := loadUsers(root)
	if err != nil {
		return nil, err
	}
	rt.Users, rt.UsersBroken = users, brokenUsers

	// A group naming a user is that user's devices, so the owner is the
	// directory and does not have to be repeated in every file in it. An
	// `owner` written anyway wins, for a device kept somewhere else.
	for i, n := range rt.Nodes {
		if n.Owner != "" || n.Group == "" {
			continue
		}
		if _, ok := rt.Users[n.Group]; ok {
			rt.Nodes[i].Owner = n.Group
		}
	}

	routes, brokenRoutes, err := loadRoutes(root)
	if err != nil {
		return nil, err
	}
	rt.Routes, rt.RoutesBroken = routes, brokenRoutes

	networks, universal, brokenNetworks, err := loadNetworks(root)
	if err != nil {
		return nil, err
	}
	rt.Networks, rt.Universal, rt.NetworksBroken = networks, universal, brokenNetworks

	return rt, nil
}

func loadNodes(root string) ([]Node, error) {
	dir := filepath.Join(root, NodesDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inventory: reading %s: %w", dir, err)
	}

	var nodes []Node
	read := func(path, group string) {
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = path
		}

		node := Node{Path: relPath, Group: group}
		data, err := os.ReadFile(path)
		if err != nil {
			node.Broken = err.Error()
			nodes = append(nodes, node)
			return
		}
		if err := decodeStrict(data, &node); err != nil {
			// decodeStrict may have partially populated node before failing;
			// start clean so a broken node carries no half-parsed fields.
			node = Node{Path: relPath, Group: group, Broken: fmt.Sprintf("%s: %s", path, err)}
		}
		node.Group = group
		nodes = append(nodes, node)
	}
	for _, entry := range entries {
		switch {
		case entry.IsDir():
			// One level of grouping: nodes/<group>/<node>.yaml. A second
			// level is not read, so a directory is a group and never a
			// path.
			group := entry.Name()
			inner, err := os.ReadDir(filepath.Join(dir, group))
			if err != nil {
				return nil, fmt.Errorf("inventory: reading %s: %w", filepath.Join(dir, group), err)
			}
			for _, sub := range inner {
				if sub.IsDir() || filepath.Ext(sub.Name()) != ".yaml" {
					continue
				}
				read(filepath.Join(dir, group, sub.Name()), group)
			}
		case filepath.Ext(entry.Name()) == ".yaml":
			read(filepath.Join(dir, entry.Name()), "")
		}
	}

	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Path < nodes[j].Path })
	return nodes, nil
}

func loadUsers(root string) (map[string]User, string, error) {
	path := filepath.Join(root, UsersFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("inventory: reading %s: %w", path, err)
	}

	var doc struct {
		Users map[string]User `yaml:"users"`
	}
	if err := decodeStrict(data, &doc); err != nil {
		return nil, fmt.Sprintf("%s: %s", path, err), nil
	}
	return doc.Users, "", nil
}

func loadRoutes(root string) (map[string]Route, string, error) {
	path := filepath.Join(root, RoutesFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil
		}
		return nil, "", fmt.Errorf("inventory: reading %s: %w", path, err)
	}

	var doc struct {
		Routes map[string]Route `yaml:"routes"`
	}
	if err := decodeStrict(data, &doc); err != nil {
		return nil, fmt.Sprintf("%s: %s", path, err), nil
	}
	return doc.Routes, "", nil
}

func loadNetworks(root string) (networks []string, universal string, broken string, err error) {
	path := filepath.Join(root, NetworksFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", "", nil
		}
		return nil, "", "", fmt.Errorf("inventory: reading %s: %w", path, err)
	}

	var doc struct {
		Networks  []string `yaml:"networks"`
		Universal string   `yaml:"universal"`
	}
	if err := decodeStrict(data, &doc); err != nil {
		return nil, "", fmt.Sprintf("%s: %s", path, err), nil
	}
	return doc.Networks, doc.Universal, "", nil
}

// decodeStrict decodes data into v, rejecting unknown fields and a document
// that is not a mapping.
func decodeStrict(data []byte, v any) error {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	return dec.Decode(v)
}
