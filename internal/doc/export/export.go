// Package export writes Views' plans into a Target folder and keeps it up to
// date. The Target's manifest, dgs-export.json, records every file an export
// wrote and the View that placed it; only those are ever replaced or removed,
// only by an export of that View, and a file that no longer reads back to the
// digest recorded for it is left alone. Files are
// published with verifiedcopy: under their final name only once read back.
package export

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
	"dgs-toolbox/internal/verifiedcopy"
)

// ManifestName is the manifest's file name at the Target root.
const ManifestName = "dgs-export.json"

// Version is the manifest format this package writes. Version 1, one View
// per Target, is still read: its files belong to that View.
const Version = 2

// Entry is one file an export wrote.
type Entry struct {
	Path     string `json:"path"`
	Digest   string `json:"digest"`
	Item     string `json:"item"`
	Type     string `json:"type"`
	Kind     string `json:"kind"`
	Revision int    `json:"revision"`
	// View is the View that placed the file.
	View string `json:"view"`
	// Head is set on the revision that was the document's HEAD.
	Head bool `json:"head,omitempty"`
	// Fields are the Item's fields, as this revision has them, when it was
	// exported, so the Target can be imported back into a tree.
	Fields map[string]string `json:"fields"`
}

// Manifest is what dgs-export.json holds.
type Manifest struct {
	Version int `json:"version"`
	// Views are the Views whose files the Target holds.
	Views []string `json:"views"`
	Files []Entry  `json:"files"`
	// View is version 1's one View; reading moves it onto the files.
	View string `json:"view,omitempty"`
}

// Action is one file of a plan.
type Action struct {
	Path     string `json:"path"`
	Digest   string `json:"digest,omitempty"`
	Item     string `json:"item,omitempty"`
	Revision int    `json:"revision,omitempty"`
	View     string `json:"view,omitempty"`
	// Reason says why a file is blocked or left.
	Reason string `json:"reason,omitempty"`
}

// Plan is what an export would do to a Target.
type Plan struct {
	Add     []Action `json:"add"`
	Replace []Action `json:"replace"`
	Remove  []Action `json:"remove"`
	Keep    []Action `json:"keep"`
	// Left are files an export wrote that are no longer wanted but were
	// changed since: they stay, and the manifest forgets them.
	Left []Action `json:"left"`
	// Blocked are wanted paths an export may not write. Apply refuses a plan
	// with any.
	Blocked []Action `json:"blocked"`
}

// Changes counts the files Apply would write or remove.
func (p Plan) Changes() int { return len(p.Add) + len(p.Replace) + len(p.Remove) }

// Result is what Apply did.
type Result struct {
	Written int `json:"written"`
	Removed int `json:"removed"`
	Kept    int `json:"kept"`
}

// CheckTarget refuses a Target that is not an absolute path, or that is
// inside the tree or holds it: an export must never write among Items.
func CheckTarget(root, target string) error {
	if !filepath.IsAbs(target) {
		return fmt.Errorf("target %q is not an absolute path", target)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	if within(absRoot, target) || within(target, absRoot) {
		return fmt.Errorf("target %s and the tree %s overlap", target, absRoot)
	}
	return nil
}

func within(parent, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && filepath.IsLocal(rel)
}

// ReadManifest reads target's manifest. A Target without one has an empty
// manifest; one that does not parse, or has another version, is an error:
// guessing which files are ours could remove someone else's.
func ReadManifest(target string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(target, ManifestName))
	if errors.Is(err, os.ErrNotExist) {
		return Manifest{Version: Version}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%s: %w", ManifestName, err)
	}
	switch m.Version {
	case 1:
		for i := range m.Files {
			m.Files[i].View = m.View
		}
		if m.View != "" {
			m.Views = []string{m.View}
		}
		m.Version, m.View = Version, ""
	case Version:
	default:
		return Manifest{}, fmt.Errorf("%s: version %d, not %d", ManifestName, m.Version, Version)
	}
	return m, nil
}

// Compute is what exporting views' files into target would do. Every file
// the manifest records for them is read and hashed, so a file changed by hand
// is found. Files other Views wrote there are theirs: never removed, and a
// wanted path one of them holds is blocked.
func Compute(ctx context.Context, target string, views []string, files []view.File) (Plan, error) {
	plan := Plan{Add: []Action{}, Replace: []Action{}, Remove: []Action{}, Keep: []Action{}, Left: []Action{}, Blocked: []Action{}}
	m, err := ReadManifest(target)
	if err != nil {
		return plan, err
	}
	exporting := map[string]bool{}
	for _, v := range views {
		exporting[v] = true
	}
	owned := map[string]Entry{}
	ownedFold := map[string]bool{}
	others := map[string]Entry{}
	for _, e := range m.Files {
		if !exporting[e.View] {
			others[strings.ToLower(e.Path)] = e
			continue
		}
		owned[e.Path] = e
		ownedFold[strings.ToLower(e.Path)] = true
	}
	wanted := map[string]bool{}
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		wanted[f.Path] = true
		a := Action{Path: f.Path, Digest: f.Digest, Item: f.Item, Revision: f.Revision, View: f.View}
		if !filepath.IsLocal(filepath.FromSlash(f.Path)) || f.Path == ManifestName {
			a.Reason = "not a path inside the Target"
			plan.Blocked = append(plan.Blocked, a)
			continue
		}
		if e, ok := others[strings.ToLower(f.Path)]; ok {
			a.Reason = "the View " + e.View + " exported a file there"
			plan.Blocked = append(plan.Blocked, a)
			continue
		}
		onDisk, err := digestOf(ctx, filepath.Join(target, filepath.FromSlash(f.Path)))
		if err != nil {
			return plan, err
		}
		e, ours := owned[f.Path]
		switch {
		case onDisk == f.Digest:
			plan.Keep = append(plan.Keep, a)
		case onDisk == "":
			plan.Add = append(plan.Add, a)
		case ours && onDisk == e.Digest:
			plan.Replace = append(plan.Replace, a)
		case !ours && ownedFold[strings.ToLower(f.Path)]:
			// The same path in other case, which this export removes first.
			plan.Add = append(plan.Add, a)
		case ours:
			a.Reason = "changed since it was exported"
			plan.Blocked = append(plan.Blocked, a)
		default:
			a.Reason = "a file an export did not write is there"
			plan.Blocked = append(plan.Blocked, a)
		}
	}
	for _, e := range m.Files {
		if wanted[e.Path] || !exporting[e.View] {
			continue
		}
		onDisk, err := digestOf(ctx, filepath.Join(target, filepath.FromSlash(e.Path)))
		if err != nil {
			return plan, err
		}
		a := Action{Path: e.Path, Digest: e.Digest, Item: e.Item, Revision: e.Revision, View: e.View}
		switch onDisk {
		case "":
			// Already gone; the manifest forgets it.
		case e.Digest:
			plan.Remove = append(plan.Remove, a)
		default:
			a.Reason = "changed since it was exported"
			plan.Left = append(plan.Left, a)
		}
	}
	return plan, nil
}

// digestOf is the SHA-256 of the regular file at path, or "" when nothing
// is there. Anything else there is reported as a digest nothing matches.
func digestOf(ctx context.Context, path string) (string, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "not a regular file", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, readerWith(ctx, f)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (c ctxReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}

func readerWith(ctx context.Context, r io.Reader) io.Reader { return ctxReader{ctx, r} }

// Apply carries out plan, computed for views: removals first, then
// replacements, then new files, each checked again just before it is
// touched. The manifest is rewritten at the end with every file the Target
// then holds for those Views, and the other Views' entries as they were;
// also when Apply stops early, so what was done is recorded. Progress, when
// set, is called after each file. Published, when set, is called for each
// file written, after it is in place.
func Apply(ctx context.Context, root, target string, views []string, plan Plan, items []tree.Item, progress func(done, total int), published func(Action)) (Result, error) {
	var result Result
	if len(plan.Blocked) > 0 {
		return result, fmt.Errorf("%d file(s) cannot be written; nothing was changed", len(plan.Blocked))
	}
	if err := CheckTarget(root, target); err != nil {
		return result, err
	}
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	entry := func(a Action) Entry {
		it := byID[a.Item]
		return Entry{Path: a.Path, Digest: a.Digest, Item: a.Item, Type: it.Type, Kind: string(it.Kind),
			Revision: a.Revision, View: a.View, Head: it.Kind == tree.KindDocument && it.Current() == a.Digest, Fields: it.FieldsAt(a.Digest)}
	}
	var done []Entry
	for _, a := range plan.Keep {
		done = append(done, entry(a))
	}
	result.Kept = len(plan.Keep)
	// pending are files still recorded because they were not yet removed or
	// replaced when Apply stopped.
	old, err := ReadManifest(target)
	if err != nil {
		return result, err
	}
	exporting := map[string]bool{}
	for _, v := range views {
		exporting[v] = true
	}
	oldByPath := map[string]Entry{}
	var others []Entry
	for _, e := range old.Files {
		if !exporting[e.View] {
			others = append(others, e)
			continue
		}
		oldByPath[e.Path] = e
	}
	pending := map[string]Entry{}
	for _, a := range append(append([]Action{}, plan.Remove...), plan.Replace...) {
		pending[a.Path] = oldByPath[a.Path]
	}
	finish := func(err error) (Result, error) {
		files := append(append([]Entry{}, others...), done...)
		for _, e := range pending {
			files = append(files, e)
		}
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		named := map[string]bool{}
		held := []string{}
		for _, e := range files {
			if !named[e.View] {
				named[e.View] = true
				held = append(held, e.View)
			}
		}
		sort.Strings(held)
		if werr := writeManifest(target, Manifest{Version: Version, Views: held, Files: files}); werr != nil && err == nil {
			err = werr
		}
		return result, err
	}

	total, step := plan.Changes(), 0
	tick := func() {
		step++
		if progress != nil {
			progress(step, total)
		}
	}
	for _, a := range plan.Remove {
		if err := removeOwned(ctx, target, a.Path, a.Digest); err != nil {
			return finish(err)
		}
		delete(pending, a.Path)
		result.Removed++
		tick()
	}
	for _, a := range plan.Replace {
		if err := removeOwned(ctx, target, a.Path, oldByPath[a.Path].Digest); err != nil {
			return finish(err)
		}
		delete(pending, a.Path)
		if err := publish(ctx, root, target, a); err != nil {
			return finish(err)
		}
		done = append(done, entry(a))
		result.Written++
		if published != nil {
			published(a)
		}
		tick()
	}
	for _, a := range plan.Add {
		if err := publish(ctx, root, target, a); err != nil {
			return finish(err)
		}
		done = append(done, entry(a))
		result.Written++
		if published != nil {
			published(a)
		}
		tick()
	}
	return finish(nil)
}

// removeOwned removes the file at rel when it still hashes to digest, then
// every folder above it the removal left empty, up to the Target.
func removeOwned(ctx context.Context, target, rel, digest string) error {
	path := filepath.Join(target, filepath.FromSlash(rel))
	onDisk, err := digestOf(ctx, path)
	if err != nil {
		return err
	}
	if onDisk == "" {
		return nil
	}
	if onDisk != digest {
		return fmt.Errorf("%s changed while exporting; left as it is", rel)
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	for dir := filepath.Dir(path); dir != filepath.Clean(target) && within(target, dir); dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			break
		}
	}
	return nil
}

func publish(ctx context.Context, root, target string, a Action) error {
	result, err := verifiedcopy.Copy(ctx, verifiedcopy.Request{
		Source:      tree.PDFPath(root, a.Item, a.Digest),
		Destination: filepath.Join(target, filepath.FromSlash(a.Path)),
	})
	if err != nil {
		return fmt.Errorf("%s: %w", a.Path, err)
	}
	if result.Digest != a.Digest {
		return fmt.Errorf("%s: the stored PDF hashes to %s, not its name", a.Path, result.Digest)
	}
	return nil
}

// writeManifest replaces the manifest through a temporary file and a rename,
// so a reader sees the old manifest or the new one, never half of one.
func writeManifest(target string, m Manifest) error {
	if m.Files == nil {
		m.Files = []Entry{}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(target, ManifestName+verifiedcopy.PartSuffix)
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(target, ManifestName))
}
