// Package identities finds the private keys on this machine that can decrypt
// age files, derives their public keys, and matches them against the
// recipient folder. Files are recognised by content, never by name.
//
// The rules are in docs/apps/cred/keys.md.
package identities

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/recipients"
)

// DefaultMaxSize is the largest file read as a possible identity. Every age
// and SSH private key is far smaller.
const DefaultMaxSize = 128 << 10

// Status says whether an identity can be used.
type Status int

const (
	// Usable identities decrypt as they are.
	Usable Status = iota
	// Protected identities need a passphrase first.
	Protected
	// Unsupported identities are private keys dgs cannot decrypt with.
	Unsupported
	// Invalid identities look like private keys and do not parse.
	Invalid
)

func (s Status) String() string {
	return [...]string{"usable", "protected", "unsupported", "invalid"}[s]
}

// Identity is one private key found on this machine.
type Identity struct {
	// Path is the file, absolute. Line is the line of an age key within its
	// file, since one file holds any number; it is 0 for an SSH key.
	Path string
	Line int
	// Kind names the format: "age", "ssh-ed25519", "ssh-rsa", or what an
	// unsupported key is.
	Kind   string
	Status Status
	// Public is the derived public key. It is empty when it cannot be derived:
	// an invalid key, a legacy encrypted PEM key, a plugin identity.
	Public recipients.PublicKey
	// Message explains a status other than Usable.
	Message string
	// Warnings are about the file, such as its permissions.
	Warnings []string
}

// Problem is something wrong with a configured directory or a file in it,
// rather than with an identity.
type Problem struct {
	Path    string
	Message string
}

// Scan is the result of Discover.
type Scan struct {
	Identities []Identity
	Problems   []Problem
}

// Options adjusts Discover.
type Options struct {
	// MaxSize is the largest file read; 0 means DefaultMaxSize.
	MaxSize int64
}

// Discover searches dirs recursively. A missing directory is a problem, not
// an error, so one list can be shared by machines lacking some of them.
// Symbolic links to files are read; links to directories are not followed.
func Discover(dirs []string, options Options) Scan {
	if options.MaxSize <= 0 {
		options.MaxSize = DefaultMaxSize
	}
	var scan Scan
	seen := map[string]bool{}
	for _, dir := range dirs {
		// The configured directory itself may be a link, as ~/.ssh often is.
		root, err := filepath.EvalSymlinks(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				scan.Problems = append(scan.Problems, Problem{Path: dir, Message: "directory does not exist"})
			} else {
				scan.Problems = append(scan.Problems, Problem{Path: dir, Message: err.Error()})
			}
			continue
		}
		walkErr := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				scan.Problems = append(scan.Problems, Problem{Path: path, Message: err.Error()})
				if entry != nil && entry.IsDir() && path != root {
					return fs.SkipDir
				}
				return nil
			}
			if entry.IsDir() {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil || info.IsDir() || !info.Mode().IsRegular() || info.Size() > options.MaxSize {
				return nil
			}
			real, err := filepath.EvalSymlinks(path)
			if err != nil || seen[real] {
				return nil
			}
			seen[real] = true
			data, err := os.ReadFile(path)
			if err != nil {
				scan.Problems = append(scan.Problems, Problem{Path: path, Message: err.Error()})
				return nil
			}
			found := Parse(data)
			if len(found) == 0 {
				return nil
			}
			var warnings []string
			if mode := info.Mode().Perm(); mode&0o077 != 0 {
				warnings = append(warnings, fmt.Sprintf("readable by others (%04o); private keys should be 0600", mode))
			}
			for _, identity := range found {
				identity.Path = path
				identity.Warnings = warnings
				scan.Identities = append(scan.Identities, identity)
			}
			return nil
		})
		if walkErr != nil {
			scan.Problems = append(scan.Problems, Problem{Path: dir, Message: walkErr.Error()})
		}
	}
	sort.SliceStable(scan.Identities, func(i, j int) bool {
		a, b := scan.Identities[i], scan.Identities[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Line < b.Line
	})
	return scan
}

// Parse returns the identities in one file's content, with no Path. Content
// that is not a private key returns none.
func Parse(data []byte) []Identity {
	trimmed := bytes.TrimSpace(data)
	switch {
	case bytes.HasPrefix(trimmed, []byte("-----BEGIN ")) && bytes.Contains(firstLine(trimmed), []byte("PRIVATE KEY-----")):
		return []Identity{parseSSH(trimmed)}
	case containsAgeKey(data):
		return parseAge(data)
	}
	return nil
}

func firstLine(data []byte) []byte {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return data[:i]
	}
	return data
}

const (
	ageSecretPrefix   = "AGE-SECRET-KEY-1"
	agePQPrefix       = "AGE-SECRET-KEY-PQ-1"
	agePluginPrefix   = "AGE-PLUGIN-"
	ageAnySecretStart = "AGE-SECRET-KEY-"
)

func containsAgeKey(data []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, ageAnySecretStart) || strings.HasPrefix(line, agePluginPrefix) {
			return true
		}
	}
	return false
}

// parseAge reads an age identity file: one key per line, # comments and blank
// lines ignored. Other lines are reported, since age itself refuses the file.
func parseAge(data []byte) []Identity {
	var found []Identity
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for number := 1; scanner.Scan(); number++ {
		line := strings.TrimSpace(scanner.Text())
		identity := Identity{Line: number, Kind: "age"}
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(line, agePQPrefix):
			identity.Kind, identity.Status = "age post-quantum", Unsupported
			identity.Message = "post-quantum age identities are not supported"
		case strings.HasPrefix(line, agePluginPrefix):
			identity.Kind, identity.Status = "age plugin", Unsupported
			identity.Message = "age plugin identities are not supported"
		case strings.HasPrefix(line, ageSecretPrefix):
			key, err := age.ParseX25519Identity(line)
			if err != nil {
				identity.Status, identity.Message = Invalid, err.Error()
				break
			}
			public, err := recipients.ParsePublicKey(key.Recipient().String())
			if err != nil {
				identity.Status, identity.Message = Invalid, err.Error()
				break
			}
			identity.Public = public
		default:
			identity.Status, identity.Message = Invalid, "line is not an age identity"
		}
		found = append(found, identity)
	}
	return found
}

func parseSSH(data []byte) Identity {
	identity := Identity{Kind: "ssh"}
	raw, err := ssh.ParseRawPrivateKey(data)
	var missing *ssh.PassphraseMissingError
	switch {
	case errors.As(err, &missing):
		identity.Status, identity.Message = Protected, "protected by a passphrase"
		if missing.PublicKey == nil {
			// Legacy encrypted PEM keeps no public key outside the encryption.
			return identity
		}
		identity.Kind = missing.PublicKey.Type()
		return derived(identity, missing.PublicKey)
	case err != nil:
		identity.Status, identity.Message = Invalid, err.Error()
		return identity
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		identity.Status, identity.Message = Unsupported, err.Error()
		return identity
	}
	identity.Kind = signer.PublicKey().Type()
	switch raw.(type) {
	case *rsa.PrivateKey, ed25519.PrivateKey, *ed25519.PrivateKey:
	default:
		identity.Status = Unsupported
		identity.Message = fmt.Sprintf("age cannot decrypt with %s keys", identity.Kind)
		return identity
	}
	return derived(identity, signer.PublicKey())
}

// derived sets the public key, or marks the identity unsupported when age
// cannot use it, such as an RSA key shorter than recipients.MinRSABits.
func derived(identity Identity, key ssh.PublicKey) Identity {
	public, err := recipients.ParsePublicKey(string(ssh.MarshalAuthorizedKey(key)))
	if err != nil {
		identity.Status, identity.Message = Unsupported, err.Error()
		return identity
	}
	identity.Public = public
	return identity
}

// Match is a recipient an identity's public key belongs to.
type Match struct {
	Host string
	Key  recipients.Key
}

// Matches returns every host key equal to the identity's public key. More than
// one is possible when two hosts list the same key, which the folder warns
// about; none means the identity is unregistered.
func Matches(identity Identity, folder recipients.Folder) []Match {
	if identity.Public.Key == "" {
		return nil
	}
	var matches []Match
	for _, host := range folder.Hosts {
		for _, key := range host.Keys {
			if key.Key == identity.Public.Key {
				matches = append(matches, Match{Host: host.Name, Key: key})
			}
		}
	}
	return matches
}

// Open reads a usable identity back from its file as an age identity, ready to
// decrypt with. It is read again rather than kept from Discover, so the
// private key is held only while it is being used.
func Open(identity Identity) (age.Identity, error) {
	if identity.Status != Usable {
		return nil, fmt.Errorf("%s identity cannot be opened", identity.Status)
	}
	data, err := os.ReadFile(identity.Path)
	if err != nil {
		return nil, err
	}
	if identity.Line == 0 {
		return agessh.ParseIdentity(data)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for number := 1; scanner.Scan(); number++ {
		if number == identity.Line {
			return age.ParseX25519Identity(strings.TrimSpace(scanner.Text()))
		}
	}
	return nil, fmt.Errorf("%s has no line %d", identity.Path, identity.Line)
}

// ErrWrongPassphrase is returned when a passphrase does not unlock a key.
var ErrWrongPassphrase = errors.New("wrong passphrase")

// Unlock opens a passphrase-protected SSH identity with passphrase, for use
// straight away; nothing unlocked is kept here.
func Unlock(identity Identity, passphrase []byte) (age.Identity, error) {
	if identity.Status != Protected || identity.Line != 0 {
		return nil, fmt.Errorf("%s is not a passphrase-protected SSH key", identity.Path)
	}
	data, err := os.ReadFile(identity.Path)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	raw, err := ssh.ParseRawPrivateKeyWithPassphrase(data, passphrase)
	if errors.Is(err, x509.IncorrectPasswordError) {
		return nil, ErrWrongPassphrase
	}
	if err != nil {
		return nil, err
	}
	switch key := raw.(type) {
	case *ed25519.PrivateKey:
		return agessh.NewEd25519Identity(*key)
	case ed25519.PrivateKey:
		return agessh.NewEd25519Identity(key)
	case *rsa.PrivateKey:
		return agessh.NewRSAIdentity(key)
	}
	return nil, fmt.Errorf("age cannot decrypt with %T keys", raw)
}

// PassphraseIdentity is the identity of a file encrypted with age -p.
func PassphraseIdentity(passphrase string) (age.Identity, error) {
	return age.NewScryptIdentity(passphrase)
}
