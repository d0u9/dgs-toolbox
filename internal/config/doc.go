package config

import (
	"dgs-toolbox/internal/doc/dates"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Doc configures dgs doc, the manager of important documents. The design is
// docs/apps/doc/.
type Doc struct {
	// Trees are the document trees the pages open, by a name the page shows:
	// papers, books. Empty means one tree, the command's working directory.
	Trees map[string]string `json:"trees"`
	// Web lists the servers by use; the only one is "pages", the doc pages.
	Web []Web `json:"web"`
	// Cache is where the discardable cache lives: the text read off PDFs, by
	// digest.
	Cache Cache `json:"cache"`
	// DateOrder is how a date like 03/04/2026 is read: DMY or MDY. Empty is
	// dates.DefaultOrder.
	DateOrder string `json:"date_order"`
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

const (
	// DocPagesServer is the web entry of the doc pages.
	DocPagesServer = "pages"
	// DefaultDocWebHost is not configurable: the tree holds identity
	// documents, so reaching it from another machine is a design with access
	// control rather than a setting.
	DefaultDocWebHost = "127.0.0.1"
	// DefaultDocWebPort sits next to the Box server's.
	DefaultDocWebPort = 8767
)

var docServers = []server{{name: DocPagesServer, host: DefaultDocWebHost, port: DefaultDocWebPort}}

// DocRoot is the folder of the only tree, or empty — the working directory —
// when there is none or more than one.
func (c Config) DocRoot() string {
	if len(c.Doc.Trees) != 1 {
		return ""
	}
	for _, root := range c.Doc.Trees {
		return root
	}
	return ""
}

// DocTree is one tree the pages open: a name, empty for the working
// directory, and its folder, empty for the working directory.
type DocTree struct {
	Name string
	Root string
}

// DocTrees is trees sorted by name, or the working directory when there are
// none.
func (c Config) DocTrees() []DocTree {
	if len(c.Doc.Trees) == 0 {
		return []DocTree{{}}
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
		return DocTree{}, fmt.Errorf("trees has %d trees: name one with --tree (%s)", len(trees), strings.Join(names, ", "))
	}
	for _, t := range trees {
		if t.Name == name {
			return t, nil
		}
	}
	return DocTree{}, fmt.Errorf("no tree named %s in trees", name)
}

// DocWebAddr is the loopback address the doc pages listen on.
func (c Config) DocWebAddr() string { return webAddr(c.Doc.Web, docServers, DocPagesServer) }

// DocCacheDir is cache.dir, or dgs/doc under the user cache directory. It
// is one cache for every tree: what is kept is keyed by content, so two trees
// holding the same PDF share its text.
func (c Config) DocCacheDir() (string, error) {
	if c.Doc.Cache.Dir != "" {
		return c.Doc.Cache.Dir, nil
	}
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate the user cache directory: %w", err)
	}
	return filepath.Join(userCache, "dgs", "doc"), nil
}

// DocDateOrder is date_order. LoadDir has refused anything else, so a
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

// checkDoc refuses a bad date order, a negative day count, a badly named tree
// and a bad web entry, and expands the paths.
func checkDoc(c *Config) error {
	if c.Doc.ExpiringWithinDays < 0 {
		return fmt.Errorf("expiring_within_days: %d is negative", c.Doc.ExpiringWithinDays)
	}
	if _, err := dates.ParseOrder(c.Doc.DateOrder); err != nil {
		return fmt.Errorf("date_order: %w", err)
	}
	if err := checkWeb(c.Doc.Web, docServers); err != nil {
		return err
	}
	fields := map[string]*string{"cache.dir": &c.Doc.Cache.Dir}
	for name := range c.Doc.Trees {
		if !docName.MatchString(name) || c.Doc.Trees[name] == "" {
			return fmt.Errorf("trees: a tree needs a name of lowercase letters, digits, _ and -, and a folder")
		}
	}
	if err := expandAll(fields); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	for name, root := range c.Doc.Trees {
		expanded, err := ExpandPath(root, os.LookupEnv, home)
		if err != nil {
			return fmt.Errorf("trees.%s: %w", name, err)
		}
		c.Doc.Trees[name] = expanded
	}
	return nil
}
