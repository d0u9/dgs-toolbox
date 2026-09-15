// Package keygen writes age identity files: a newly generated one, or one
// imported from elsewhere. A file is never replaced, and what is written is
// parsed back before it gets its name. The rules are in docs/apps/cred/keys.md.
package keygen

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/publish"
)

// Written is an identity file that now exists.
type Written struct {
	Path string
	// PublicKeys are the usable identities' public keys, in file order.
	PublicKeys []string
}

// Generate creates a new X25519 identity in dir/name, in the layout
// age-keygen writes.
func Generate(dir, name string, now time.Time) (Written, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return Written{}, err
	}
	content := fmt.Sprintf("# created: %s\n# public key: %s\n%s\n", now.Format(time.RFC3339), identity.Recipient(), identity)
	return write(dir, name, []byte(content))
}

// Import copies an age identity file into dir/name. It must hold at least one
// usable age identity and nothing that is not one.
func Import(source, dir, name string) (Written, error) {
	file, err := os.Open(source)
	if err != nil {
		return Written{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, identities.DefaultMaxSize+1))
	if err != nil {
		return Written{}, err
	}
	defer clear(data)
	if len(data) > identities.DefaultMaxSize {
		return Written{}, fmt.Errorf("%s is too large to be an identity file", source)
	}
	return write(dir, name, data)
}

func write(dir, name string, content []byte) (Written, error) {
	if name == "" || name != filepath.Base(name) || name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return Written{}, fmt.Errorf("%q is not a file name", name)
	}
	found, err := check(content)
	if err != nil {
		return Written{}, err
	}
	path := filepath.Join(dir, name)
	if err := publish.Create(path, content, 0o600, 0o700); err != nil {
		return Written{}, err
	}
	return Written{Path: path, PublicKeys: found}, nil
}

// check returns the public keys of an age identity file's usable identities,
// refusing content that is not entirely age identities.
func check(content []byte) ([]string, error) {
	parsed := identities.Parse(content)
	var keys []string
	for _, identity := range parsed {
		if identity.Line == 0 {
			return nil, errors.New("not an age identity file; SSH keys belong in ~/.ssh")
		}
		if identity.Status != identities.Usable {
			return nil, fmt.Errorf("line %d: %s", identity.Line, identity.Message)
		}
		keys = append(keys, identity.Public.Key)
	}
	if len(keys) == 0 {
		return nil, errors.New("no age identity found")
	}
	return keys, nil
}
