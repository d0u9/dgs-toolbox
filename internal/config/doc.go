package config

import (
	"dgs-toolbox/internal/doc/dates"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
)

// Doc configures dgs doc, the manager of important documents. The design is
// docs/apps/doc/.
type Doc struct {
	// Root is the document tree the pages open. Empty means the command's
	// working directory.
	Root string `json:"root"`
	Web  DocWeb `json:"web"`
	// CacheDir is where the discardable cache lives: the text read off PDFs,
	// by digest. Empty is the user cache directory.
	CacheDir string `json:"cache_dir"`
	// DateOrder is how a date like 03/04/2026 is read: DMY or MDY. Empty is
	// dates.DefaultOrder.
	DateOrder string `json:"date_order"`
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
