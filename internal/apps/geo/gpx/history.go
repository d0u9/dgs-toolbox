package gpx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"dgs-toolbox/internal/geo/gpxfile"
	"dgs-toolbox/internal/geo/sidecar"
)

// Undo and redo of edits. Every edit reaches the held state through
// saveSidecar, so the state before each one is kept here, per file, and undo
// puts it back. A save is a step too: it keeps what it replaced on disk — the
// GPX dgs wrote, or a recording's sidecar — and undoing it puts those bytes
// back with the edits that were unsaved, so a save does not end what can be
// undone. A save is undone or redone only while the file on disk is as that
// step left it; a file changed by anything else is never overwritten.
// Closing, reverting or discarding forgets a file's history, and so does
// renaming once a save is in it. The history lives in this process, like the
// edits.

// historyLimit is how many edits of one file can be undone. A save keeps the
// whole file it replaced, so a large file saved often holds that much memory.
// ponytail: whole-file copies per save; keep only a diff if memory matters.
const historyLimit = 200

// snapshot is a file's held state at one moment; held is false when nothing
// was held and the file read as it is on disk. disk is set on the step a
// save made: what is put on disk with that state.
type snapshot struct {
	file sidecar.File
	held bool
	disk *diskState
}

// diskState is the content of the file a save writes, at one moment.
type diskState struct {
	data   []byte
	exists bool
	// expect is the fingerprint the file must have now for this state to be
	// put back: what the step that is undone or redone left there.
	expect string
}

// errChangedOnDisk refuses to undo or redo a save over a file changed since.
var errChangedOnDisk = errors.New("the file changed on disk since it was saved here, so the save is not undone or redone; reload it")

// savedPath is the file a save of path writes: the GPX itself when dgs wrote
// it, else its sidecar. A draft has nothing on disk.
func savedPath(path string) string {
	if isDraft(path) {
		return ""
	}
	if parsed, err := gpxfile.Open(path); err == nil && parsed.IsOurs() {
		return path
	}
	return sidecar.PathFor(path)
}

func readDisk(path string) (diskState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return diskState{}, nil
	}
	if err != nil {
		return diskState{}, err
	}
	return diskState{data: data, exists: true}, nil
}

func (d diskState) fingerprint() string {
	if !d.exists {
		return "absent"
	}
	sum := sha256.Sum256(d.data)
	return hex.EncodeToString(sum[:])
}

// writeDisk puts a saved state back: a GPX dgs wrote is replaced as an edit
// replaces it; a sidecar is written or removed. Either the whole state is on
// disk afterwards or, on an error, the file is as it was.
func writeDisk(path string, d diskState) error {
	target := savedPath(path)
	if target == path {
		return gpxfile.Update(path, func(staged string) error {
			return os.WriteFile(staged, d.data, 0o644)
		})
	}
	if !d.exists {
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	// The very bytes go back, so the next step finds what it expects.
	temp, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(d.data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), target)
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

// restoreState puts a snapshot back as the held state of path. What it holds
// for the disk is already there. The photos of a GPX dgs wrote then follow:
// a save may have sent to the trash a photo that only an earlier edit linked,
// so every step takes back out of the trash what the file or the held edits
// link; a step that put a saved file back also trashes, as a save does, what
// nothing links. A photo that cannot be moved is reported as errPhotoCleanup;
// the step itself is done.
func restoreState(path string, state snapshot) error {
	if isDraft(path) {
		return drafts.set(path, state.file)
	}
	if state.held {
		pending.set(path, state.file)
	} else {
		pending.drop(path)
	}
	if savedPath(path) != path {
		return nil
	}
	if err := untrashLinked(path); err != nil {
		return fmt.Errorf("%w: %v", errPhotoCleanup, err)
	}
	if state.disk == nil {
		return nil
	}
	if err := trashUnlinked(path); err != nil {
		return fmt.Errorf("%w: %v", errPhotoCleanup, err)
	}
	return nil
}

// errPhotoCleanup says a step was done but its photos were not all moved.
var errPhotoCleanup = errors.New("done, but moving its photos failed")

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

// move follows a renamed file. A saved state names the file's photo folder
// by the old name, so a history holding a save is forgotten instead.
func (s *historyStore) move(from, to string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	from, to = pendingKey(from), pendingKey(to)
	h, ok := s.files[from]
	if !ok || from == to {
		return
	}
	delete(s.files, from)
	for _, state := range append(append([]snapshot{}, h.undo...), h.redo...) {
		if state.disk != nil {
			return
		}
	}
	s.files[to] = h
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
	current := heldState(path)
	if state.disk != nil {
		// The file must be as the step left it; what is there now goes on the
		// other stack, to be put back in turn.
		now, err := readDisk(savedPath(path))
		if err != nil {
			s.mu.Unlock()
			return false, err
		}
		if now.fingerprint() != state.disk.expect {
			s.mu.Unlock()
			return false, errChangedOnDisk
		}
		// The disk is written before the history moves, so a failed write
		// leaves both as they were and the step can be tried again.
		if err := writeDisk(path, *state.disk); err != nil {
			s.mu.Unlock()
			return false, err
		}
		now.expect = state.disk.fingerprint()
		current.disk = &now
	}
	*from = (*from)[:len(*from)-1]
	*to = append(*to, current)
	s.mu.Unlock()
	return true, restoreState(path, state)
}

// stepEdit answers the page's Cmd+Z (back) and Shift+Cmd+Z for one file.
func (a api) stepEdit(w http.ResponseWriter, r *http.Request, back bool) {
	var body struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	moved, err := history.step(body.Path, back)
	if moved && errors.Is(err, errPhotoCleanup) {
		writeJSON(w, map[string]any{"ok": true, "moved": true, "warning": err.Error()})
		return
	}
	if errors.Is(err, errChangedOnDisk) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "moved": moved})
}
