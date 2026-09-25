package config

import (
	"net"
	"strconv"
)

// Doc configures dgs doc, the manager of important documents. The design is
// docs/apps/doc/.
type Doc struct {
	// Root is the document tree the pages open. Empty means the command's
	// working directory.
	Root string `json:"root"`
	Web  DocWeb `json:"web"`
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
