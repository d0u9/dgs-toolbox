package gpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"dgs-toolbox/internal/geo/sidecar"
)

// Undo and redo of unsaved edits. Every edit reaches the held state through
// saveSidecar, so the state before each one is kept here, per file, and undo
// puts it back. Saving, reverting or discarding forgets a file's history: what
// is on disk is not undone. The history lives in this process, like the edits.

// historyLimit is how many edits of one file can be undone.
const historyLimit = 200

// snapshot is a file's held state at one moment; held is false when nothing
// was held and the file read as it is on disk.
type snapshot struct {
	file sidecar.File
	held bool
}

type fileHistory struct{ undo, redo []snapshot }

type historyStore struct {
	mu    sync.Mutex
	files map[string]*fileHistory
}

var history = &historyStore{files: map[string]*fileHistory{}}

// heldState is what saveSidecar would replace for path; the stores hand out
// copies, so later edits do not reach it.
func heldState(path string) snapshot {
	var file sidecar.File
	var ok bool
	if isDraft(path) {
		file, ok = drafts.get(path)
	} else {
		file, ok = pending.get(path)
	}
	if !ok {
		return snapshot{}
	}
	return snapshot{file: file, held: true}
}

func cloneSidecar(file sidecar.File) sidecar.File {
	data, err := json.Marshal(file)
	if err != nil {
		return file
	}
	var copied sidecar.File
	if json.Unmarshal(data, &copied) != nil {
		return file
	}
	return copied
}

// restoreState puts a snapshot back as the held state of path.
func restoreState(path string, state snapshot) error {
	if isDraft(path) {
		return drafts.set(path, state.file)
	}
	if state.held {
		pending.set(path, state.file)
	} else {
		pending.drop(path)
	}
	return nil
}

// record keeps the state before an edit; a new edit ends what could be redone.
func (s *historyStore) record(path string, before snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := pendingKey(path)
	h := s.files[key]
	if h == nil {
		h = &fileHistory{}
		s.files[key] = h
	}
	h.undo = append(h.undo, before)
	if len(h.undo) > historyLimit {
		h.undo = h.undo[len(h.undo)-historyLimit:]
	}
	h.redo = nil
}

func (s *historyStore) forget(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, pendingKey(path))
}

func (s *historyStore) move(from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, to = pendingKey(from), pendingKey(to)
	if h, ok := s.files[from]; ok && from != to {
		s.files[to] = h
		delete(s.files, from)
	}
}

// counts says how many edits of path can be undone and redone.
func (s *historyStore) counts(path string) (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.files[pendingKey(path)]
	if h == nil {
		return 0, 0
	}
	return len(h.undo), len(h.redo)
}

// step moves one edit back (undo) or forward (redo), keeping the state it
// leaves on the other stack. It reports false when there is nothing to move.
func (s *historyStore) step(path string, back bool) (bool, error) {
	s.mu.Lock()
	h := s.files[pendingKey(path)]
	if h == nil {
		s.mu.Unlock()
		return false, nil
	}
	from, to := &h.undo, &h.redo
	if !back {
		from, to = to, from
	}
	if len(*from) == 0 {
		s.mu.Unlock()
		return false, nil
	}
	state := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, heldState(path))
	s.mu.Unlock()
	return true, restoreState(path, state)
}

// undoEdit and redoEdit answer the page's Cmd+Z and Shift+Cmd+Z for one file.
func (a api) undoEdit(w http.ResponseWriter, r *http.Request) { a.stepEdit(w, r, true) }
func (a api) redoEdit(w http.ResponseWriter, r *http.Request) { a.stepEdit(w, r, false) }

func (a api) stepEdit(w http.ResponseWriter, r *http.Request, back bool) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	moved, err := history.step(body.Path, back)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "moved": moved})
}
