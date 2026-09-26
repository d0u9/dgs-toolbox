package web

import (
	"net/http"

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
	// Default is the Snapshot's own folder on this machine, ~ made home.
	Default string `json:"default"`
}

// snapshotList answers every Snapshot with its tree as the Items have it
// now, and what of it the tree has lost.
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
	for _, sn := range list {
		r := snapshot.Resolve(sn, items)
		j := snapshotJSON{Snapshot: sn, Root: r.Root, Lost: r.Lost}
		if sn.Folder != "" {
			j.Default = outline.Expand(sn.Folder, home())
		}
		out.Snapshots = append(out.Snapshots, j)
	}
	writeJSON(w, http.StatusOK, out)
}

// snapshotTake records a saved Outline's tree, or one folder of it, as it
// is now.
func (s server) snapshotTake(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Outline string `json:"outline"`
		Node    string `json:"node"`
		Name    string `json:"name"`
		About   string `json:"about"`
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
	outlines, err := outline.Load(s.root)
	if err != nil {
		fail(http.StatusConflict, err)
		return
	}
	var o *outline.Outline
	for i := range outlines {
		if outlines[i].Name == request.Outline {
			o = &outlines[i]
		}
	}
	if o == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no Outline named " + request.Outline})
		return
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
	g, err := outline.Group(*o, items, view.NamesOf(templates))
	if err != nil {
		fail(http.StatusBadRequest, err)
		return
	}
	sn, err := snapshot.Take(request.Name, request.About, *o, g, request.Node, items, s.now())
	if err == nil {
		err = snapshot.Save(s.root, "", sn, true)
	}
	if err != nil {
		fail(http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, sn)
}

// snapshotSave renames a Snapshot or changes its about or folder; what it
// holds does not change.
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
	if err := snapshot.Save(s.root, request.Previous, request.Snapshot, false); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, request.Snapshot)
}

// snapshotDelete moves a Snapshot's file into the tree's trash.
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
	if _, err := snapshot.Trash(s.root, request.Name, s.now()); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}
