// Package secretstore reads and writes conf.secrets: one credential, one
// file, at <instance>/<port>/<kind>/<name>. It computes which paths an
// inventory's derivation implies, compares that against what is on disk,
// generates what is missing, and never deletes what sync no longer implies.
//
// The rules are in docs/apps/conf/inventory.md#secrets.
//
// Two things imply a path. A role whose auth is confgen.AuthPerPrincipal
// implies one per (principal, port) pair derive.Derive already computed. A
// role's own list implies one <instance>/own/<name> per name, for every
// instance of that role. Together they are the whole set, so a file under an
// own/ directory that no name in the list accounts for is reported orphaned
// like any other — it is a credential a template may still read and sync
// will never regenerate.
package secretstore

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/conf/confgen"
	"dgs-toolbox/internal/conf/derive"
	"dgs-toolbox/internal/conf/inventory"
)

// OwnPort is the reserved port segment an instance's own secrets sit under,
// rather than under one of its listening ports.
const OwnPort = "own"

// PreviousSuffix names a secret's previous value during a rotation.
const PreviousSuffix = ".previous"

// randomPrintableLength is the length of the printable random string
// generated for a service that declares no confgen.Secret shape.
const randomPrintableLength = 32

// Path is one credential's identity: <instance>/<port>/<kind>/<name>.
type Path struct {
	Instance string
	Port     string
	Kind     string
	Name     string
}

// String is the path relative to conf.secrets.
func (p Path) String() string {
	return filepath.Join(p.Instance, p.Port, p.Kind, p.Name)
}

// ImpliedPaths computes every secret path a derivation implies: one file per
// (principal, port) grant for a role whose auth is per-principal, plus one
// <instance>/own/<name> file for every name in its role's own list. A role's
// combine_own names one of those, so it implies nothing on its own. See the
// package doc for what it deliberately leaves out beyond that.
func ImpliedPaths(inv *inventory.Root, manifests map[string]confgen.Manifest, model *derive.Model) []Path {
	roleOf := map[string]confgen.Role{} // instance ID -> its role
	for _, n := range inv.Nodes {
		if n.Broken != "" {
			continue
		}
		for _, inst := range n.Instances {
			if inst.Service == "" && inst.Role == "" {
				continue // an override, not a real instance.
			}
			role, ok := manifests[inst.Service].Roles[inst.Role]
			if !ok {
				continue
			}
			roleOf[inst.ID] = role
		}
	}

	seen := map[Path]bool{}
	var out []Path
	add := func(p Path) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	for _, g := range model.Grants {
		if roleOf[g.Instance].Auth != confgen.AuthPerPrincipal {
			continue
		}
		add(Path{Instance: g.Instance, Port: g.Port, Kind: string(g.Principal.Kind), Name: g.Principal.ID})
	}
	for id, role := range roleOf {
		for _, name := range role.Own {
			add(Path{Instance: id, Port: OwnPort, Name: name})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// secretOf returns the confgen.Secret shape an instance's service declares.
func secretOf(inv *inventory.Root, manifests map[string]confgen.Manifest, instance string) confgen.Secret {
	for _, n := range inv.Nodes {
		for _, inst := range n.Instances {
			if inst.ID == instance {
				return manifests[inst.Service].Secret
			}
		}
	}
	return confgen.Secret{}
}

// Result is what Sync found comparing implied paths against a secrets root.
type Result struct {
	// Missing are implied paths with no file on disk yet.
	Missing []Path
	// Orphaned are files on disk that nothing implies any more. Sync never
	// deletes these; see docs/apps/conf/inventory.md#keeping-the-tree-in-step.
	Orphaned []Path
	// RenameHint is true when Missing and Orphaned are the same size and
	// both non-empty — sync cannot tell a rename from an unrelated add and
	// remove, so it only suggests `secret mv`, it never guesses which pairs.
	RenameHint bool
}

// Sync compares the paths implied against the files already in root, a
// conf.secrets tree. It reads the filesystem but changes nothing.
func Sync(root string, implied []Path) (Result, error) {
	onDisk, err := walk(root)
	if err != nil {
		return Result{}, err
	}

	impliedSet := map[string]bool{}
	for _, p := range implied {
		impliedSet[p.String()] = true
	}

	var res Result
	for _, p := range implied {
		if !onDisk[p.String()] {
			res.Missing = append(res.Missing, p)
		}
	}
	var diskPaths []string
	for rel := range onDisk {
		diskPaths = append(diskPaths, rel)
	}
	sort.Strings(diskPaths)
	for _, rel := range diskPaths {
		if strings.HasSuffix(rel, PreviousSuffix) {
			continue // a rotation's previous value, not an implied path itself.
		}
		if !impliedSet[rel] {
			res.Orphaned = append(res.Orphaned, parsePath(rel))
		}
	}

	res.RenameHint = len(res.Missing) > 0 && len(res.Missing) == len(res.Orphaned)
	return res, nil
}

func parsePath(rel string) Path {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	// An own secret is three segments, <instance>/own/<name>, since it
	// belongs to the instance rather than to a principal on one of its
	// ports. Everything else is four.
	if len(parts) == 3 && parts[1] == OwnPort {
		return Path{Instance: parts[0], Port: OwnPort, Name: parts[2]}
	}
	if len(parts) != 4 {
		return Path{Instance: rel}
	}
	return Path{Instance: parts[0], Port: parts[1], Kind: parts[2], Name: parts[3]}
}

// walk returns every regular file under root, as paths relative to root
// using "/" separators, or an empty set if root does not exist yet.
func walk(root string) (map[string]bool, error) {
	out := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = true
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("secretstore: walking %s: %w", root, err)
	}
	return out, nil
}

// Generate writes a fresh value for every path in missing, taking the shape
// its instance's service declares — base64 of Secret.Bytes, or a printable
// random string when the service declares no Secret. It refuses to
// overwrite a file that already exists.
func Generate(root string, missing []Path, inv *inventory.Root, manifests map[string]confgen.Manifest) error {
	for _, p := range missing {
		full := filepath.Join(root, p.String())
		if _, err := os.Stat(full); err == nil {
			return fmt.Errorf("secretstore: %s already exists, refusing to overwrite", full)
		}
		value, err := generateValue(secretOf(inv, manifests, p.Instance))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			return fmt.Errorf("secretstore: %w", err)
		}
		if err := os.WriteFile(full, []byte(value), 0o600); err != nil {
			return fmt.Errorf("secretstore: writing %s: %w", full, err)
		}
	}
	return nil
}

func generateValue(secret confgen.Secret) (string, error) {
	if secret.Kind == "base64" && secret.Bytes > 0 {
		buf := make([]byte, secret.Bytes)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("secretstore: generating a value: %w", err)
		}
		return base64.StdEncoding.EncodeToString(buf), nil
	}
	return randomPrintable(randomPrintableLength)
}

const printableAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

func randomPrintable(length int) (string, error) {
	var b strings.Builder
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(printableAlphabet))))
		if err != nil {
			return "", fmt.Errorf("secretstore: generating a value: %w", err)
		}
		b.WriteByte(printableAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// ReadValue reads one secret's value, trimmed of a trailing newline if any.
func ReadValue(root string, p Path) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, p.String()))
	if err != nil {
		return "", fmt.Errorf("secretstore: reading %s: %w", p, err)
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}

// ReadPrevious reads a secret's previous value — its <name>.previous
// sibling — for the second account a rotating per-principal port emits. ok
// is false, with no error, when there is no .previous file: an ordinary
// state, not a broken one.
func ReadPrevious(root string, p Path) (value string, ok bool, err error) {
	full := filepath.Join(root, p.String()) + PreviousSuffix
	data, err := os.ReadFile(full)
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("secretstore: reading %s: %w", full, err)
	}
	return strings.TrimSuffix(string(data), "\n"), true, nil
}

// PreviousModTimes finds every .previous file under root and returns its
// modification time, keyed by the path of the secret it is the previous
// value of (without the .previous suffix) — validate's own input for
// docs/apps/conf/inventory.md#validation rule 16: no .previous file older
// than seven days.
func PreviousModTimes(root string) (map[string]time.Time, error) {
	onDisk, err := walkWithInfo(root)
	if err != nil {
		return nil, err
	}
	out := map[string]time.Time{}
	for rel, info := range onDisk {
		if base, ok := strings.CutSuffix(rel, PreviousSuffix); ok {
			out[base] = info.ModTime()
		}
	}
	return out, nil
}

// walkWithInfo is walk, but keeping each file's os.FileInfo instead of just
// its presence.
func walkWithInfo(root string) (map[string]fs.FileInfo, error) {
	out := map[string]fs.FileInfo{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = info
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("secretstore: walking %s: %w", root, err)
	}
	return out, nil
}

// ReadOwn reads every file directly under <instance>/own/ — an instance's
// own secrets, by name, docs/apps/conf/inventory.md#the-render-context's own
// datasource. A name nested under a further directory (the shape this
// package does not compute paths for; see the package doc) is read too, the
// last path element becoming its name.
func ReadOwn(root, instance string) (map[string]string, error) {
	dir := filepath.Join(root, instance, OwnPort)
	own := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == dir && os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		own[d.Name()] = strings.TrimSuffix(string(data), "\n")
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("secretstore: reading %s: %w", dir, err)
	}
	return own, nil
}

// Mv moves a secret path (a node, instance or the whole tree under one
// instance) from oldPath to newPath, both relative to root. It is
// docs/apps/conf/inventory.md's explicit answer to a rename: sync only
// reports equal counts of new and orphaned paths, this performs it.
func Mv(root, oldRel, newRel string) error {
	oldFull, newFull := filepath.Join(root, oldRel), filepath.Join(root, newRel)
	if _, err := os.Stat(oldFull); err != nil {
		return fmt.Errorf("secretstore: %w", err)
	}
	if _, err := os.Stat(newFull); err == nil {
		return fmt.Errorf("secretstore: %s already exists", newFull)
	}
	if err := os.MkdirAll(filepath.Dir(newFull), 0o700); err != nil {
		return fmt.Errorf("secretstore: %w", err)
	}
	if err := os.Rename(oldFull, newFull); err != nil {
		return fmt.Errorf("secretstore: %w", err)
	}
	return nil
}
