// Package recipients reads the recipient folder: the hosts dgs cred can
// encrypt for, each with its public keys, and the groups of those hosts. The
// folder is edited by hand, so a mistake in one file leaves that file out and
// is reported, rather than refusing the whole folder.
//
// The layout, and which problems are errors and which are warnings, are in
// docs/apps/cred/keys.md.
package recipients

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	HostsDir    = "hosts"
	GroupsDir   = "groups"
	FileSuffix  = ".json"
	GroupPrefix = "g-"
)

// Meta is what a host file records about a key besides the key itself. Only
// the description is written by hand; the rest is filled in by dgs when it adds
// the key, and may be absent from a file written by hand.
type Meta struct {
	Description string `json:"description"`
	// Origin is how the key came to be listed: generated, imported, registered
	// or added.
	Origin string `json:"origin,omitempty"`
	// Added is the date it was listed, as YYYY-MM-DD.
	Added string `json:"added,omitempty"`
	// PrivateKey is where the private key lives on its own host, as that host
	// would write it, when dgs knows.
	PrivateKey string `json:"private_key,omitempty"`
	// Comment is the comment an SSH public key line carried, often user@host.
	Comment string `json:"comment,omitempty"`
}

// Origins a key may record.
const (
	OriginGenerated  = "generated"
	OriginImported   = "imported"
	OriginRegistered = "registered"
	OriginAdded      = "added"
)

// Key is one public key of a host.
type Key struct {
	PublicKey
	Meta
}

// Host is a machine and the keys it can decrypt with.
type Host struct {
	Name string
	// File is the host file, relative to the folder.
	File string
	Keys []Key
}

// Group is a named set of hosts.
type Group struct {
	Name string
	File string
	// Hosts are the names of loaded hosts, spelled as their files are, in the
	// order the group lists them. Members that could not be resolved are left
	// out and reported.
	Hosts []string
}

// Severity says whether a problem left its file out.
type Severity int

const (
	// Warning leaves the file loaded.
	Warning Severity = iota
	// Error leaves the file out.
	Error
)

func (s Severity) String() string {
	if s == Error {
		return "error"
	}
	return "warning"
}

// Problem is something wrong in the folder.
type Problem struct {
	Severity Severity
	// File is relative to the folder.
	File    string
	Message string
}

// Folder is a loaded recipient folder.
type Folder struct {
	Root     string
	Hosts    []Host
	Groups   []Group
	Problems []Problem
}

// Errors reports whether any file was left out.
func (f Folder) Errors() bool {
	for _, problem := range f.Problems {
		if problem.Severity == Error {
			return true
		}
	}
	return false
}

// Host finds a host by name, compared case-insensitively.
func (f Folder) Host(name string) (Host, bool) {
	for _, host := range f.Hosts {
		if strings.EqualFold(host.Name, name) {
			return host, true
		}
	}
	return Host{}, false
}

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]*$`)

type hostFile struct {
	Keys []keyFile `json:"keys"`
}

type keyFile struct {
	PublicKey string `json:"public_key"`
	Meta
}

type groupFile struct {
	Hosts []string `json:"hosts"`
}

// Load reads the folder at root. The error is only for a root that cannot be
// read at all; everything inside it is reported as Problems.
func Load(root string) (Folder, error) {
	info, err := os.Stat(root)
	if err != nil {
		return Folder{}, fmt.Errorf("recipient folder: %w", err)
	}
	if !info.IsDir() {
		return Folder{}, fmt.Errorf("recipient folder %s is not a directory", root)
	}
	folder := Folder{Root: root}
	loader := &loader{folder: &folder}

	hostPaths, err := loader.list(HostsDir)
	if err != nil {
		return Folder{}, err
	}
	// Names that differ only in case are an error on every file sharing them,
	// so the whole set is looked at before any host is kept.
	byName := map[string][]string{}
	for _, path := range hostPaths {
		name := strings.TrimSuffix(filepath.Base(path), FileSuffix)
		byName[strings.ToLower(name)] = append(byName[strings.ToLower(name)], path)
	}
	failed := map[string]string{}
	for _, path := range hostPaths {
		name := strings.TrimSuffix(filepath.Base(path), FileSuffix)
		if same := byName[strings.ToLower(name)]; len(same) > 1 {
			loader.report(Error, path, "names that differ only in case: %s", strings.Join(same, ", "))
			failed[strings.ToLower(name)] = path
			continue
		}
		if host, ok := loader.host(path, name); ok {
			folder.Hosts = append(folder.Hosts, host)
		} else {
			failed[strings.ToLower(name)] = path
		}
	}
	loader.sharedKeys()

	groupPaths, err := loader.list(GroupsDir)
	if err != nil {
		return Folder{}, err
	}
	byName = map[string][]string{}
	for _, path := range groupPaths {
		name := strings.TrimSuffix(filepath.Base(path), FileSuffix)
		byName[strings.ToLower(name)] = append(byName[strings.ToLower(name)], path)
	}
	for _, path := range groupPaths {
		name := strings.TrimSuffix(filepath.Base(path), FileSuffix)
		if same := byName[strings.ToLower(name)]; len(same) > 1 {
			loader.report(Error, path, "names that differ only in case: %s", strings.Join(same, ", "))
			continue
		}
		if group, ok := loader.group(path, name, failed); ok {
			folder.Groups = append(folder.Groups, group)
		}
	}

	sort.SliceStable(folder.Hosts, func(i, j int) bool {
		return strings.ToLower(folder.Hosts[i].Name) < strings.ToLower(folder.Hosts[j].Name)
	})
	sort.SliceStable(folder.Groups, func(i, j int) bool {
		return strings.ToLower(folder.Groups[i].Name) < strings.ToLower(folder.Groups[j].Name)
	})
	sort.SliceStable(folder.Problems, func(i, j int) bool {
		return folder.Problems[i].File < folder.Problems[j].File
	})
	return folder, nil
}

type loader struct {
	folder *Folder
}

func (l *loader) report(severity Severity, file, format string, args ...any) {
	l.folder.Problems = append(l.folder.Problems, Problem{Severity: severity, File: file, Message: fmt.Sprintf(format, args...)})
}

// list returns the .json files of one subdirectory, relative to the root. A
// missing subdirectory is empty; hidden files and other names are ignored;
// subdirectories are reported and not read.
func (l *loader) list(dir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(l.folder.Root, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("recipient folder: %w", err)
	}
	var paths []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(filepath.Join(l.folder.Root, path))
		if err != nil {
			l.report(Error, path, "%v", err)
			continue
		}
		if info.IsDir() {
			l.report(Warning, path, "subdirectories are not read")
			continue
		}
		if strings.HasSuffix(name, FileSuffix) {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func (l *loader) decode(path string, into any) bool {
	data, err := os.ReadFile(filepath.Join(l.folder.Root, path))
	if err != nil {
		l.report(Error, path, "%v", err)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		l.report(Error, path, "invalid JSON: %v", err)
		return false
	}
	if _, err := decoder.Token(); err != io.EOF {
		l.report(Error, path, "invalid JSON: content after the object")
		return false
	}
	return true
}

func (l *loader) host(path, name string) (Host, bool) {
	if !namePattern.MatchString(name) {
		l.report(Error, path, "host name %q may use only letters, digits, -, _ and ., and not start with .", name)
		return Host{}, false
	}
	if strings.HasPrefix(strings.ToLower(name), GroupPrefix) {
		l.report(Error, path, "host name %q starts with %s, which is kept for groups", name, GroupPrefix)
		return Host{}, false
	}
	var file hostFile
	if !l.decode(path, &file) {
		return Host{}, false
	}
	host := Host{Name: name, File: path}
	seen := map[string]int{}
	ok := true
	for i, entry := range file.Keys {
		key, err := ParsePublicKey(entry.PublicKey)
		if err != nil {
			l.report(Error, path, "key %d: %v", i+1, err)
			ok = false
			continue
		}
		if err := checkMeta(entry.Meta); err != nil {
			l.report(Error, path, "key %d: %v", i+1, err)
			ok = false
			continue
		}
		if strings.TrimSpace(entry.Description) == "" {
			l.report(Error, path, "key %d: description is required", i+1)
			ok = false
			continue
		}
		if first, dup := seen[key.Key]; dup {
			l.report(Error, path, "key %d is the same public key as key %d", i+1, first)
			ok = false
			continue
		}
		seen[key.Key] = i + 1
		meta := entry.Meta
		meta.Description = strings.TrimSpace(meta.Description)
		host.Keys = append(host.Keys, Key{PublicKey: key, Meta: meta})
	}
	if !ok {
		return Host{}, false
	}
	if len(host.Keys) == 0 {
		l.report(Warning, path, "host has no keys")
	}
	return host, true
}

// sharedKeys warns about a public key that more than one host lists.
func (l *loader) sharedKeys() {
	owners := map[string][]Host{}
	var order []string
	for _, host := range l.folder.Hosts {
		for _, key := range host.Keys {
			if _, seen := owners[key.Key]; !seen {
				order = append(order, key.Key)
			}
			owners[key.Key] = append(owners[key.Key], host)
		}
	}
	for _, key := range order {
		hosts := owners[key]
		if len(hosts) < 2 {
			continue
		}
		names := make([]string, len(hosts))
		for i, host := range hosts {
			names[i] = host.Name
		}
		for _, host := range hosts {
			l.report(Warning, host.File, "public key %s is also listed by %s", shorten(key), strings.Join(names, ", "))
		}
	}
}

func (l *loader) group(path, name string, failed map[string]string) (Group, bool) {
	if !namePattern.MatchString(name) {
		l.report(Error, path, "group name %q may use only letters, digits, -, _ and ., and not start with .", name)
		return Group{}, false
	}
	if !strings.HasPrefix(strings.ToLower(name), GroupPrefix) {
		l.report(Error, path, "group name %q must start with %s", name, GroupPrefix)
		return Group{}, false
	}
	var file groupFile
	if !l.decode(path, &file) {
		return Group{}, false
	}
	group := Group{Name: name, File: path}
	seen := map[string]bool{}
	for _, member := range file.Hosts {
		lower := strings.ToLower(member)
		if seen[lower] {
			l.report(Warning, path, "host %q is listed twice", member)
			continue
		}
		seen[lower] = true
		if host, ok := l.folder.Host(member); ok {
			group.Hosts = append(group.Hosts, host.Name)
			continue
		}
		if file, ok := failed[lower]; ok {
			l.report(Warning, path, "host %q is left out: %s has an error", member, file)
			continue
		}
		l.report(Warning, path, "host %q does not exist", member)
	}
	return group, true
}

// shorten keeps the start and end of a long key, enough to recognise it.
func shorten(key string) string {
	if len(key) <= 24 {
		return key
	}
	return key[:12] + "…" + key[len(key)-8:]
}

func checkMeta(meta Meta) error {
	switch meta.Origin {
	case "", OriginGenerated, OriginImported, OriginRegistered, OriginAdded:
	default:
		return fmt.Errorf("origin %q is not one of generated, imported, registered, added", meta.Origin)
	}
	if meta.Added != "" {
		if _, err := time.Parse("2006-01-02", meta.Added); err != nil {
			return fmt.Errorf("added %q is not a YYYY-MM-DD date", meta.Added)
		}
	}
	return nil
}
