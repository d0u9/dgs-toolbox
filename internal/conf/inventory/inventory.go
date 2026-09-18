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

// DevicesManaged and DevicesUnmanaged are the two values a user's `devices`
// key may take. A user with neither set is DevicesManaged.
const (
	DevicesManaged   = "managed"
	DevicesUnmanaged = "unmanaged"
)

// ClientRoleNone is the node.client_role value meaning: derive nothing on
// this node, regardless of what a service's reached_by says.
const ClientRoleNone = "none"

// Networks is a node's `networks` key: a mapping of network name to this
// node's address on it. It says where others can reach this node.
type Networks map[string]string

// Instance is one entry of a node's `instances` list: one service running on
// that node, and one rendered configuration file.
type Instance struct {
	ID      string         `yaml:"id"`
	Service string         `yaml:"service"`
	Role    string         `yaml:"role"`
	Bind    string         `yaml:"bind"`
	Ports   map[string]int `yaml:"ports"`
	// Values is this instance's own parameters — a Hysteria2 masquerade
	// target, a MicroBin public path, an nginx server block — whatever its
	// service and role need beyond where it runs and what it listens on.
	// dgs does not read it; a template reads it by name. See
	// docs/apps/conf/inventory.md#an-instances-own-values.
	Values map[string]any `yaml:"values"`
}

// Node is one nodes/*.yaml file: a machine or a device.
type Node struct {
	ID    string `yaml:"id"`
	Owner string `yaml:"owner"`
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
	// ClientRole names the role every client this node derives takes,
	// overriding a service's own `reached_by`, or ClientRoleNone to derive
	// nothing regardless of it. Empty defers to `reached_by`. See
	// docs/apps/conf/inventory.md#which-role-a-client-derives-as.
	ClientRole string     `yaml:"client_role"`
	Instances  []Instance `yaml:"instances"`

	// Path is the file's path, relative to the generator root. Set by Load,
	// not part of the YAML.
	Path string `yaml:"-"`
	// Broken is the parse error if the file is not valid YAML or not a
	// mapping, and empty otherwise. Fields above are valid only when this is
	// empty.
	Broken string `yaml:"-"`
}

// User is one entry of users.yaml: a logical identity, not a Linux account.
type User struct {
	// Username is the account name the services see. Empty means it is the
	// same as the user's own map key — see UsernameOr.
	Username string `yaml:"username"`
	Devices  string `yaml:"devices"`
	// ClientRole is an unmanaged user's client_role: it has no node to carry
	// one, so it is written here instead, and means the same thing. Empty
	// for a managed user, who is not asked.
	ClientRole string   `yaml:"client_role"`
	Access     []string `yaml:"access"`
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
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		relPath, err := filepath.Rel(root, path)
		if err != nil {
			relPath = path
		}

		node := Node{Path: relPath}
		data, err := os.ReadFile(path)
		if err != nil {
			node.Broken = err.Error()
			nodes = append(nodes, node)
			continue
		}
		if err := decodeStrict(data, &node); err != nil {
			// decodeStrict may have partially populated node before failing;
			// start clean so a broken node carries no half-parsed fields.
			node = Node{Path: relPath, Broken: fmt.Sprintf("%s: %s", path, err)}
		}
		nodes = append(nodes, node)
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
