// Package intakestate remembers where an import got to.
//
// A hundred scans are not sorted in one sitting. The state lives in the inbox
// rather than in the Box, because it is a fact about this pile of files and not
// about the archive: throwing the inbox away throws its state away with it, and
// the Box is untouched either way.
//
// It records a decision per candidate, never a transfer's progress. A partial
// copy is not resumable — there is no way to tell how much of it is right — so
// a retry is always a whole file from byte zero. What resuming saves is the
// reading and the thinking, which is all of the time.
package intakestate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/box/sidecar"
)

// Version is the format this build writes. Anything else is discarded and
// started again: the state describes an inbox that is still sitting there, so
// rebuilding it costs a re-read and no decisions that were acted on.
const Version = 1

// Name is the default file, inside the inbox. It carries no leading dot: a
// person looking at their own temporary folder should see what the tool put
// there.
const Name = "dgs-box-state.json"

// State is what has been decided about one candidate.
type State string

const (
	// Pending is read but not decided.
	Pending State = "pending"
	// Classified is described but not yet in the Box. The description is held
	// here, because nothing in the inbox is in the Box yet and a half-described
	// scan must not leave a sidecar behind.
	Classified State = "classified"
	// Published is in the Box, verified, with its sidecar beside it.
	Published State = "published"
	// Duplicate is already in the Box, or in its trash.
	Duplicate State = "duplicate"
	// Incomplete is truncated or unparseable and was not taken in. It is never
	// silently skipped: a skip that is not reported means believing the inbox
	// is empty when it is not.
	Incomplete State = "incomplete"
	// Rejected was looked at and not wanted.
	Rejected State = "rejected"
)

// Settled reports whether this candidate still needs a person.
func (s State) Settled() bool {
	switch s {
	case Published, Duplicate, Incomplete, Rejected:
		return true
	default:
		return false
	}
}

// Record is one candidate.
type Record struct {
	// Path is relative to the inbox, with forward slashes, so moving or
	// renaming the folder does not invalidate every record in it.
	Path string `json:"path"`
	// Size and ModTime are what the file looked like when it was read. A file
	// whose triple moved is read again: it is not the file this record is
	// about any more.
	Size    int64 `json:"size"`
	ModTime int64 `json:"mod_time"`
	// Digest and ImageDigest are what the read produced. They are kept so a
	// candidate can be recognised without reading its bytes again.
	Digest      string `json:"digest,omitempty"`
	ImageDigest string `json:"image_digest,omitempty"`

	State State `json:"state"`
	// Draft is what has been said about a classified scan but not yet written
	// anywhere. It is empty in every other state.
	Draft *sidecar.File `json:"draft,omitempty"`
	// PublishedPath is where it landed, relative to the Box root.
	PublishedPath string `json:"published_path,omitempty"`
	// Reason is why it is incomplete, rejected or a duplicate.
	Reason string `json:"reason,omitempty"`
	// At is when this record last changed, RFC 3339.
	At string `json:"at,omitempty"`
}

// Stale reports whether the file on disk is no longer the one this record
// describes.
func (r Record) Stale(size, modTime int64) bool {
	return r.Size != size || r.ModTime != modTime
}

// File is the whole state.
type File struct {
	Version int      `json:"version"`
	Records []Record `json:"records"`
}

// PathFor is the state file inside an inbox. name is box.state_file; empty uses
// the default. It must be a bare filename, for the same reason the Box marker
// must be: a configured path could point the state at something that is not
// this inbox.
func PathFor(inbox, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		name = Name
	}
	if name != filepath.Base(name) || strings.ContainsRune(name, filepath.Separator) {
		return "", errors.New("state file name is a path, not a filename: " + name)
	}
	return filepath.Join(inbox, name), nil
}

// Load reads the state at path.
//
// Missing, unparseable, truncated, another version — all the same answer, an
// empty state, and no error. The inbox is still there to be read again, so
// there is nothing to migrate and nothing to mourn.
func Load(path string) File {
	data, err := os.ReadFile(path)
	if err != nil {
		return File{Version: Version}
	}
	var state File
	if err := json.Unmarshal(data, &state); err != nil {
		return File{Version: Version}
	}
	if state.Version != Version {
		return File{Version: Version}
	}
	return state
}

// Save writes the state whole.
//
// A temporary file beside it and a rename, with a sync before it: losing this
// file costs re-reading the inbox, which is minutes, and re-making the
// decisions in it, which is the part nobody wants to do twice.
func Save(path string, state File) error {
	state.Version = Version
	sort.SliceStable(state.Records, func(a, b int) bool { return state.Records[a].Path < state.Records[b].Path })
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

// Index keys the records by path, for a caller matching them against a walk of
// the inbox.
func (f File) Index() map[string]Record {
	byPath := make(map[string]Record, len(f.Records))
	for _, record := range f.Records {
		byPath[record.Path] = record
	}
	return byPath
}

// Counts is how many candidates are in each state, for a summary line.
func (f File) Counts() map[State]int {
	counts := map[State]int{}
	for _, record := range f.Records {
		counts[record.State]++
	}
	return counts
}

// Remaining is how many candidates still need a person.
func (f File) Remaining() int {
	remaining := 0
	for _, record := range f.Records {
		if !record.State.Settled() {
			remaining++
		}
	}
	return remaining
}

// Set replaces or adds one record, stamping when it changed.
func (f *File) Set(record Record, now time.Time) {
	if record.At == "" {
		record.At = now.Format(time.RFC3339)
	}
	for position, existing := range f.Records {
		if existing.Path == record.Path {
			f.Records[position] = record
			return
		}
	}
	f.Records = append(f.Records, record)
}

// Prune drops records for files that are no longer in the inbox, and returns
// how many went.
//
// A record whose file is gone describes nothing: the file was moved, renamed or
// deleted by its owner, and keeping the record would make the inbox look
// fuller than it is. What was published stays in the Box regardless, because
// the Box is the truth and this file never was.
func (f *File) Prune(present map[string]bool) int {
	kept := f.Records[:0]
	dropped := 0
	for _, record := range f.Records {
		if present[record.Path] {
			kept = append(kept, record)
			continue
		}
		dropped++
	}
	f.Records = kept
	return dropped
}
