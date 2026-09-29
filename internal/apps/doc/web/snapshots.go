package web

import (
	"fmt"
	"net/http"
	"strings"

	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type snapshotJSON struct {
	snapshot.Snapshot
	// Root is the tree of its files the tree still has; Lost are the rest.
	Root *outline.Node   `json:"root"`
	Lost []snapshot.Lost `json:"lost"`
	// Used names the Outlines putting it in their trees.
	Used []string `json:"used"`
}

// snapshotList answers every Snapshot with its tree as the Items have it
// now, what of it the tree has lost, and the Outlines using it.
func (s server) snapshotList(w http.ResponseWriter, _ *http.Request) {
	out := struct {
		Snapshots []snapshotJSON `json:"snapshots"`
		Error     string         `json:"error,omitempty"`
	}{Snapshots: []snapshotJSON{}}
	list, err := snapshot.Load(s.root)
	if err != nil {
		out.Error = err.Error()
	}
	items, err := tree.LoadItems(s.root)
	if err != nil && out.Error == "" {
		out.Error = err.Error()
	}
	outlines, err := outline.Load(s.root)
	if err != nil && out.Error == "" {
		out.Error = err.Error()
	}
	used := outline.SnapshotUsers(outlines)
	for _, sn := range list {
		r := snapshot.Resolve(sn, items)
		j := snapshotJSON{Snapshot: sn, Root: outline.Nest(r.Files), Lost: r.Lost, Used: used[sn.Name]}
		if j.Used == nil {
			j.Used = []string{}
		}
		out.Snapshots = append(out.Snapshots, j)
	}
	writeJSON(w, http.StatusOK, out)
}

// snapshotTake records what a saved rule places now, or, with no rule,
// begins an empty Snapshot.
func (s server) snapshotTake(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Rule  string `json:"rule"`
		Name  string `json:"name"`
		About string `json:"about"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	fail := func(status int, err error) { writeJSON(w, status, map[string]string{"error": err.Error()}) }
	var rule *view.View
	if request.Rule != "" {
		rules, err := outline.LoadRules(s.root)
		if err != nil {
			fail(http.StatusConflict, err)
			return
		}
		found, ok := rules[request.Rule]
		if !ok {
			fail(http.StatusBadRequest, fmt.Errorf("no rule named %s", request.Rule))
			return
		}
		rule = &found
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		fail(http.StatusConflict, err)
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		fail(http.StatusConflict, err)
		return
	}
	sn, err := snapshot.Take(request.Name, request.About, rule, items, view.TypesOf(templates), s.now())
	if err == nil {
		err = snapshot.Save(s.root, "", sn, true)
	}
	if err != nil {
		fail(http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, sn)
}

// snapshotSave writes a Snapshot as edited: its about and its files. A new
// name renames it first, in every Outline using it.
func (s server) snapshotSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Snapshot snapshot.Snapshot `json:"snapshot"`
		Previous string            `json:"previous"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	fail := func(err error) { writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()}) }
	sn := request.Snapshot
	if err := sn.Validate(); err != nil {
		fail(err)
		return
	}
	if request.Previous != "" && request.Previous != sn.Name {
		if err := outline.RenameSnapshot(s.root, request.Previous, sn.Name); err != nil {
			fail(err)
			return
		}
	}
	if err := snapshot.Save(s.root, "", sn, false); err != nil {
		fail(err)
		return
	}
	writeJSON(w, http.StatusOK, sn)
}

// snapshotDelete moves a Snapshot no Outline uses into the tree's trash.
func (s server) snapshotDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	outlines, err := outline.Load(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	if used := outline.SnapshotUsers(outlines)[request.Name]; len(used) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "the Snapshot " + request.Name + " is used by " + strings.Join(used, ", ")})
		return
	}
	if _, err := snapshot.Trash(s.root, request.Name, s.now()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// snapshotNames names PDFs about to be added to a Snapshot by a naming
// layout, as a rule would: each name, or the keys its Item lacks.
func (s server) snapshotNames(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Naming string `json:"naming"`
		Files  []struct {
			Item     string `json:"item"`
			Revision string `json:"revision"`
		} `json:"files"`
	}
	if !decode(w, r, &request) {
		return
	}
	fail := func(status int, err error) { writeJSON(w, status, map[string]string{"error": err.Error()}) }
	items, err := tree.LoadItems(s.root)
	if err != nil {
		fail(http.StatusConflict, err)
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		fail(http.StatusConflict, err)
		return
	}
	byID := map[string]tree.Item{}
	for _, it := range items {
		byID[it.ID] = it
	}
	type named struct {
		Path    string   `json:"path"`
		Lacking []string `json:"lacking,omitempty"`
	}
	out := make([]named, len(request.Files))
	for i, f := range request.Files {
		it, ok := byID[f.Item]
		if !ok {
			fail(http.StatusBadRequest, fmt.Errorf("no Item %s", f.Item))
			return
		}
		revision := 0
		for n, rev := range it.Revisions {
			if rev.Ref() == f.Revision || rev.Digest == f.Revision {
				revision = n + 1
			}
		}
		path, lacking, err := view.Name(request.Naming, it, revision, view.TypesOf(templates))
		if err != nil {
			fail(http.StatusBadRequest, err)
			return
		}
		out[i] = named{Path: path, Lacking: lacking}
	}
	writeJSON(w, http.StatusOK, map[string]any{"names": out})
}
