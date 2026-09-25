package config

import (
	"dgs-toolbox/internal/doc/dates"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Doc configures dgs doc, the manager of important documents. The design is
// docs/apps/doc/.
type Doc struct {
	// Root is the document tree the pages open. Empty means the command's
	// working directory.
	Root string `json:"root"`
	// Trees names several trees, by a name the page shows, when there is
	// more than one: papers, books. It and Root are not set together.
	Trees map[string]string `json:"trees"`
	Web   DocWeb            `json:"web"`
	// CacheDir is where the discardable cache lives: the text read off PDFs,
	// by digest. Empty is the user cache directory.
	CacheDir string `json:"cache_dir"`
	// DateOrder is how a date like 03/04/2026 is read: DMY or MDY. Empty is
	// dates.DefaultOrder.
	DateOrder string `json:"date_order"`
	// Targets is no longer read: a tree's Targets are in its targets.yaml.
	// It is kept so a configuration still holding it is told where they
	// went, rather than that the key is unknown.
	Targets map[string]string `json:"targets"`
	// ExpiringWithinDays is how many days before its expiry a document is
	// shown as expiring soon. Zero means DefaultDocExpiringWithinDays.
	ExpiringWithinDays int `json:"expiring_within_days"`
}

// DefaultDocExpiringWithinDays is doc.expiring_within_days when unset:
// about three months, time enough to renew a passport or a licence.
const DefaultDocExpiringWithinDays = 90

// DocExpiringWithin is doc.expiring_within_days as a duration.
func (c Config) DocExpiringWithin() time.Duration {
	days := c.Doc.ExpiringWithinDays
	if days <= 0 {
		days = DefaultDocExpiringWithinDays
	}
	return time.Duration(days) * 24 * time.Hour
}

// DocWeb is where the doc pages listen. Only the port is configurable.
type DocWeb struct {
	// Port is the port to listen on. Zero means DefaultDocWebPort.
	Port int `json:"port"`
}

const (
	// DefaultDocWebHost is not configurable: the tree holds identity
	// documents, so reaching it from another machine is a design with access
	// control rather than a setting.
	DefaultDocWebHost = "127.0.0.1"
	// DefaultDocWebPort sits next to the Box server's.
	DefaultDocWebPort = 8767
)

// DocRoot is doc.root, already expanded by LoadPath.
func (c Config) DocRoot() string { return c.Doc.Root }

// DocTree is one tree the pages open: a name, empty for doc.root, and its
// folder, empty for the working directory.
type DocTree struct {
	Name string
	Root string
}

// DocTrees is doc.trees sorted by name, or the one tree doc.root names when
// there are none.
func (c Config) DocTrees() []DocTree {
	if len(c.Doc.Trees) == 0 {
		return []DocTree{{Root: c.Doc.Root}}
	}
	out := make([]DocTree, 0, len(c.Doc.Trees))
	for name, root := range c.Doc.Trees {
		out = append(out, DocTree{Name: name, Root: root})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// DocTreeNamed is the tree name picks: the only one when name is empty and
// there is one, else the one of that name.
func (c Config) DocTreeNamed(name string) (DocTree, error) {
	trees := c.DocTrees()
	if name == "" {
		if len(trees) == 1 {
			return trees[0], nil
		}
		names := make([]string, len(trees))
		for i, t := range trees {
			names[i] = t.Name
		}
		return DocTree{}, fmt.Errorf("doc.trees has %d trees: name one with --tree (%s)", len(trees), strings.Join(names, ", "))
	}
	for _, t := range trees {
		if t.Name == name {
			return t, nil
		}
	}
	return DocTree{}, fmt.Errorf("no tree named %s in doc.trees", name)
}

// DocWebAddr is the loopback address the doc pages listen on.
func (c Config) DocWebAddr() string {
	port := c.Doc.Web.Port
	if port == 0 {
		port = DefaultDocWebPort
	}
	return net.JoinHostPort(DefaultDocWebHost, strconv.Itoa(port))
}

// DocCacheDir is doc.cache_dir, or dgs/doc under the user cache directory. It
// is one cache for every tree: what is kept is keyed by content, so two trees
// holding the same PDF share its text.
func (c Config) DocCacheDir() (string, error) {
	if c.Doc.CacheDir != "" {
		return c.Doc.CacheDir, nil
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate the user cache directory: %w", err)
	}
	return filepath.Join(userCache, "dgs", "doc"), nil
}

// DocDateOrder is doc.date_order. LoadPath has refused anything else, so a
// bad value here is only a Config built by hand, and it reads as the default.
func (c Config) DocDateOrder() dates.Order {
	order, err := dates.ParseOrder(c.Doc.DateOrder)
	if err != nil {
		return dates.DefaultOrder
	}
	return order
}

// docName is what a tree's name may be, as a View's and a Target's are.
var docName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
