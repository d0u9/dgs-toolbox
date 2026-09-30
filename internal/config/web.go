package config

import (
	"fmt"
	"net"
	"strconv"
)

// Web is one web server a command runs, named by its use. A command's web key
// is a list of these because one command may come to serve more than one
// page; each command declares the servers it has, and an entry names one of
// them. A server with no entry listens at its defaults.
type Web struct {
	Name string `json:"name"`
	// Host is the address to listen on. Only a server that may be reached
	// from another machine accepts one; the others are loopback, always.
	Host string `json:"host,omitempty"`
	// Port is the port to listen on. Zero means the server's default.
	Port int `json:"port"`
}

// server is one web server a command declares: its name, its defaults, and
// whether a configuration may move it off its default host.
type server struct {
	name         string
	host         string
	port         int
	hostSettable bool
}

// defaultWeb is the entry list an exported default file holds.
func defaultWeb(servers []server) []Web {
	out := make([]Web, len(servers))
	for i, s := range servers {
		out[i] = Web{Name: s.name, Port: s.port}
		if s.hostSettable {
			out[i].Host = s.host
		}
	}
	return out
}

// checkWeb refuses an entry naming a server the command does not have, a
// server named twice, a host where the host is fixed, and a port out of range.
func checkWeb(entries []Web, servers []server) error {
	seen := map[string]bool{}
	for i, entry := range entries {
		s, ok := findServer(servers, entry.Name)
		if !ok {
			names := make([]string, len(servers))
			for j, s := range servers {
				names[j] = strconv.Quote(s.name)
			}
			return fmt.Errorf("web[%d]: name %q is not a server of this command; it has %v", i, entry.Name, names)
		}
		if seen[entry.Name] {
			return fmt.Errorf("web[%d]: %q is listed twice", i, entry.Name)
		}
		seen[entry.Name] = true
		if entry.Host != "" && !s.hostSettable {
			return fmt.Errorf("web[%d]: %q always listens on %s; it has no host to set", i, entry.Name, s.host)
		}
		if entry.Port < 0 || entry.Port > 65535 {
			return fmt.Errorf("web[%d]: port %d is out of range", i, entry.Port)
		}
	}
	return nil
}

// webAddr is the host:port the named server listens on.
func webAddr(entries []Web, servers []server, name string) string {
	s, _ := findServer(servers, name)
	host, port := s.host, s.port
	for _, entry := range entries {
		if entry.Name != name {
			continue
		}
		if entry.Host != "" && s.hostSettable {
			host = entry.Host
		}
		if entry.Port != 0 {
			port = entry.Port
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func findServer(servers []server, name string) (server, bool) {
	for _, s := range servers {
		if s.name == name {
			return s, true
		}
	}
	return server{}, false
}

// SetWeb returns entries with the named server's entry changed by edit, added
// when there is none: a command-line flag overriding what the file says.
func SetWeb(entries []Web, name string, edit func(*Web)) []Web {
	out := append([]Web(nil), entries...)
	for i := range out {
		if out[i].Name == name {
			edit(&out[i])
			return out
		}
	}
	entry := Web{Name: name}
	edit(&entry)
	return append(out, entry)
}
