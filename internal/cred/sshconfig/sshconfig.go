// Package sshconfig writes the small pieces of OpenSSH client configuration
// dgs installs: a Host block per server, kept one file per host in a config.d
// directory, and the Include line that makes ssh read them.
package sshconfig

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// IncludePattern is what ~/.ssh/config includes to read one file per host.
const IncludePattern = "~/.ssh/config.d/*"

// Host is one server's entry.
type Host struct {
	Alias        string
	HostName     string
	User         string
	Port         string
	IdentityFile string
	// ProxyJump reaches the server through another host, such as bastion or
	// alice@jump.example.com:2222.
	ProxyJump string
	// ProxyCommand is a command line that connects to the server, for routes
	// ProxyJump cannot take, such as through a SOCKS proxy. ssh expands %h and
	// %p in it. It excludes ProxyJump: ssh would use only the first it reads.
	ProxyCommand string
	// Multiplex shares one connection among sessions to the server; it stays
	// open for MultiplexPersist after the last session closes. Without it the
	// block turns multiplexing off, unless ssh has already read another value:
	// the first value read wins.
	Multiplex bool
	// KeepAlive asks the server every KeepAliveInterval and drops a
	// connection that misses KeepAliveCountMax replies.
	KeepAlive bool
	// ForwardAgent lets the server use the local ssh-agent.
	ForwardAgent bool
	// Compression compresses traffic, useful on slow links.
	Compression bool
	// TrueColor sends COLORTERM=truecolor; the server applies it only when its
	// sshd_config has AcceptEnv COLORTERM.
	TrueColor bool
}

// Named defaults for the optional settings.
const (
	MultiplexPath     = "~/.ssh/cm-%C"
	MultiplexPersist  = "10m"
	KeepAliveInterval = 60
	KeepAliveCountMax = 3
)

// unsafeChars may not appear in a value: whitespace would end it early and a
// line break would start a directive of its own.
const unsafeChars = " \t\r\n\"'"

var aliasPattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

// Check reports what is wrong with a host's fields, or nil.
func (h Host) Check() error {
	switch {
	case !aliasPattern.MatchString(h.Alias):
		return fmt.Errorf("alias %q may use only letters, digits, -, _ and ., and not start with .", h.Alias)
	case h.HostName == "":
		return errors.New("host name is required: the server's IP address or domain")
	case strings.ContainsAny(h.HostName, unsafeChars):
		return fmt.Errorf("host name %q has spaces, line breaks or quotes", h.HostName)
	case strings.ContainsAny(h.User, unsafeChars):
		return fmt.Errorf("user %q has spaces, line breaks or quotes", h.User)
	case strings.ContainsAny(h.ProxyJump, unsafeChars):
		return fmt.Errorf("jump host %q has spaces, line breaks or quotes", h.ProxyJump)
	case strings.ContainsAny(h.ProxyCommand, "\r\n"):
		return fmt.Errorf("proxy command %q has line breaks", h.ProxyCommand)
	case h.ProxyJump != "" && h.ProxyCommand != "":
		return errors.New("give a jump host or a proxy command, not both")
	}
	if h.Port != "" {
		if port, err := strconv.Atoi(h.Port); err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port %q is not a number from 1 to 65535", h.Port)
		}
	}
	return nil
}

// Block is the host's configuration. IdentitiesOnly makes ssh offer only the
// named key, so many keys elsewhere do not use up a server's attempts.
func (h Host) Block() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Host %s\n    HostName %s\n", h.Alias, h.HostName)
	if h.User != "" {
		fmt.Fprintf(&b, "    User %s\n", h.User)
	}
	if h.Port != "" {
		fmt.Fprintf(&b, "    Port %s\n", h.Port)
	}
	fmt.Fprintf(&b, "    IdentityFile %s\n    IdentitiesOnly yes\n", h.IdentityFile)
	if h.ProxyJump != "" {
		fmt.Fprintf(&b, "    ProxyJump %s\n", h.ProxyJump)
	}
	if h.ProxyCommand != "" {
		fmt.Fprintf(&b, "    ProxyCommand %s\n", h.ProxyCommand)
	}
	if h.Multiplex {
		fmt.Fprintf(&b, "    ControlMaster auto\n    ControlPath %s\n    ControlPersist %s\n", MultiplexPath, MultiplexPersist)
	} else {
		b.WriteString("    ControlMaster no\n    ControlPath none\n")
	}
	if h.KeepAlive {
		fmt.Fprintf(&b, "    ServerAliveInterval %d\n    ServerAliveCountMax %d\n", KeepAliveInterval, KeepAliveCountMax)
	}
	if h.ForwardAgent {
		b.WriteString("    ForwardAgent yes\n")
	}
	if h.Compression {
		b.WriteString("    Compression yes\n")
	}
	if h.TrueColor {
		b.WriteString("    SetEnv COLORTERM=truecolor\n")
	}
	return b.String()
}

// Includes reports whether the configuration file at path already has an
// Include directive naming pattern. A missing file includes nothing.
func Includes(path, pattern, home string) (bool, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	want := expand(pattern, home)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(strings.ReplaceAll(scanner.Text(), "=", " "))
		if len(fields) < 2 || !strings.EqualFold(fields[0], "include") {
			continue
		}
		for _, arg := range fields[1:] {
			if expand(strings.Trim(arg, `"`), home) == want {
				return true, nil
			}
		}
	}
	return false, scanner.Err()
}

func expand(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return filepath.Clean(path)
}

// WithInclude returns config with an Include of pattern as its first line. It
// goes first because an Include after a Host block applies only inside it.
func WithInclude(config []byte, pattern string) []byte {
	line := "Include " + pattern + "\n"
	if len(config) == 0 {
		return []byte(line)
	}
	return append([]byte(line+"\n"), config...)
}
