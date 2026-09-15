// Package vault reads a folder of age-encrypted files: which files there are,
// and what each one's header says about who can open it. It reads headers only;
// no payload is decrypted here.
//
// The rules are in docs/apps/cred/vault.md.
package vault

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"
)

// Suffix is the extension a file must have to be considered.
const Suffix = ".age"

const (
	binaryIntro  = "age-encryption.org/v1\n"
	armoredIntro = "-----BEGIN AGE ENCRYPTED FILE-----"
)

// File is one .age file found in the vault.
type File struct {
	// Path is relative to the vault root, with forward slashes.
	Path    string
	Size    int64
	ModTime time.Time
}

// Problem is something in the folder that could not be read.
type Problem struct {
	Path    string
	Message string
}

// Listing is the result of List.
type Listing struct {
	Root     string
	Files    []File
	Problems []Problem
}

// List finds the .age files under root. Hidden files and directories are
// skipped, and links to directories are not followed; links to files are.
// The error is only for a root that cannot be read.
func List(root string) (Listing, error) {
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Listing{}, fmt.Errorf("vault folder: %w", err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return Listing{}, fmt.Errorf("vault folder: %w", err)
	}
	if !info.IsDir() {
		return Listing{}, fmt.Errorf("vault folder %s is not a directory", root)
	}
	listing := Listing{Root: root}
	err = filepath.WalkDir(real, func(path string, entry fs.DirEntry, err error) error {
		rel, _ := filepath.Rel(real, path)
		if err != nil {
			listing.Problems = append(listing.Problems, Problem{Path: filepath.ToSlash(rel), Message: err.Error()})
			if entry != nil && entry.IsDir() && path != real {
				return fs.SkipDir
			}
			return nil
		}
		if path == real {
			return nil
		}
		if strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), Suffix) {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			listing.Problems = append(listing.Problems, Problem{Path: filepath.ToSlash(rel), Message: err.Error()})
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		listing.Files = append(listing.Files, File{Path: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime()})
		return nil
	})
	if err != nil {
		return Listing{}, fmt.Errorf("vault folder: %w", err)
	}
	sort.Slice(listing.Files, func(i, j int) bool { return listing.Files[i].Path < listing.Files[j].Path })
	return listing, nil
}

// Status is what a header says about opening a file on this machine.
type Status int

const (
	// Unchecked is a file not inspected yet.
	Unchecked Status = iota
	Damaged
	Decryptable
	NeedsPassphrase
	Passphrase
	Unsupported
	NotForThisMachine
)

func (s Status) String() string {
	return [...]string{"unchecked", "damaged", "decryptable", "needs passphrase", "passphrase", "unsupported", "not for this machine"}[s]
}

// Tag is the four bytes an SSH stanza carries of the key it is for.
type Tag [4]byte

// TagOf is the tag of an SSH public key in authorized_keys form.
func TagOf(authorizedKey string) (Tag, bool) {
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(authorizedKey))
	if err != nil {
		return Tag{}, false
	}
	sum := sha256.Sum256(key.Marshal())
	var tag Tag
	copy(tag[:], sum[:4])
	return tag, true
}

// Opener is an identity to try, named for display.
type Opener struct {
	Name     string
	Identity age.Identity
}

// Tagged is an SSH key known by name and tag: a protected identity on this
// machine, or a host's key in the recipient folder.
type Tagged struct {
	Name string
	Tag  Tag
}

// Keyring is what a header is checked against.
type Keyring struct {
	Openers   []Opener
	Protected []Tagged
	Hosts     []Tagged
}

// Stanza is one recipient entry in a header.
type Stanza struct {
	Type string
	// Tag is set for SSH stanzas.
	Tag    Tag
	HasTag bool
}

// Report is what Inspect found.
type Report struct {
	Status  Status
	Armored bool
	Stanzas []Stanza
	// OpenedBy names the opener that decrypts the file.
	OpenedBy string
	// Protected names protected identities whose tag the header carries.
	Protected []string
	// Hosts names host keys whose tag the header carries: a hint, since tags
	// can collide and X25519 recipients carry none.
	Hosts []string
	// Message explains a damaged file.
	Message string
}

// maxHeader bounds how much is read looking for a header's end.
const maxHeader = 1 << 20

var supported = map[string]bool{"X25519": true, "ssh-ed25519": true, "ssh-rsa": true, "scrypt": true}

// Inspect reads the header of the file at path and checks it against ring.
func Inspect(path string, ring Keyring) Report {
	file, err := os.Open(path)
	if err != nil {
		return Report{Status: Damaged, Message: err.Error()}
	}
	defer file.Close()
	return InspectReader(file, ring)
}

// InspectReader is Inspect on already-open content.
func InspectReader(src io.Reader, ring Keyring) Report {
	var report Report
	buffered := bufio.NewReader(src)
	peek, _ := buffered.Peek(len(armoredIntro))
	var reader io.Reader = buffered
	switch {
	case bytes.HasPrefix(peek, []byte(binaryIntro)):
	case bytes.Equal(peek, []byte(armoredIntro)):
		report.Armored = true
		reader = armor.NewReader(buffered)
	default:
		report.Status, report.Message = Damaged, "not an age file: the header does not start with "+strings.TrimSpace(binaryIntro)
		return report
	}
	header, err := age.ExtractHeader(io.LimitReader(reader, maxHeader))
	if err != nil {
		report.Status, report.Message = Damaged, err.Error()
		return report
	}
	report.Stanzas = parseStanzas(header)

	for _, opener := range ring.Openers {
		_, err := age.DecryptHeader(header, opener.Identity)
		if err == nil {
			report.Status, report.OpenedBy = Decryptable, opener.Name
			break
		}
		var noMatch *age.NoIdentityMatchError
		if !errors.As(err, &noMatch) {
			// The identity unwrapped a stanza and the header still failed.
			report.Status, report.Message = Damaged, err.Error()
			return report
		}
	}

	tags := map[Tag]bool{}
	allUnsupported := len(report.Stanzas) > 0
	scrypt := false
	for _, stanza := range report.Stanzas {
		if stanza.HasTag {
			tags[stanza.Tag] = true
		}
		if supported[stanza.Type] {
			allUnsupported = false
		}
		if stanza.Type == "scrypt" {
			scrypt = true
		}
	}
	for _, key := range ring.Protected {
		if tags[key.Tag] {
			report.Protected = append(report.Protected, key.Name)
		}
	}
	for _, key := range ring.Hosts {
		if tags[key.Tag] {
			report.Hosts = append(report.Hosts, key.Name)
		}
	}

	switch {
	case report.Status == Decryptable:
	case len(report.Protected) > 0:
		report.Status = NeedsPassphrase
	case scrypt:
		report.Status = Passphrase
	case allUnsupported:
		report.Status = Unsupported
	default:
		report.Status = NotForThisMachine
	}
	return report
}

// parseStanzas lists the "-> type args…" lines of a serialized header.
func parseStanzas(header []byte) []Stanza {
	var stanzas []Stanza
	for _, line := range strings.Split(string(header), "\n") {
		fields, ok := strings.CutPrefix(line, "-> ")
		if !ok {
			continue
		}
		args := strings.Fields(fields)
		if len(args) == 0 {
			continue
		}
		stanza := Stanza{Type: args[0]}
		if (args[0] == "ssh-ed25519" || args[0] == "ssh-rsa") && len(args) > 1 {
			if raw, err := base64.RawStdEncoding.DecodeString(args[1]); err == nil && len(raw) == 4 {
				copy(stanza.Tag[:], raw)
				stanza.HasTag = true
			}
		}
		stanzas = append(stanzas, stanza)
	}
	return stanzas
}
