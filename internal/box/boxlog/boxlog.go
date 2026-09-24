// Package boxlog appends to a Box's log: every intake and every edit, one JSON
// object per line, never rewritten.
//
// The log is what makes batch editing safe enough to offer. Editing three
// hundred records at once is necessary — it is the only way a pile of scans
// gets described at all — and it is also the only way to get three hundred
// records wrong at once. An append-only record of what changed, from what, to
// what and when is what makes that recoverable.
//
// It is not an index and nothing reads it to answer a question about the Box.
// The sidecars are the truth; this is the history of how they got that way.
package boxlog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Name is the default file name, at the Box root. It carries no leading dot:
// a Box whose history is invisible in Finder is a Box nobody knows has one.
const Name = "dgs-box-log.jsonl"

// Action is what happened.
type Action string

const (
	// ActionInit records a Box being created.
	ActionInit Action = "init"
	// ActionImport records a scan published into the Box.
	ActionImport Action = "import"
	// ActionEdit records a change to a sidecar.
	ActionEdit Action = "edit"
	// ActionDiscard records a scan moved into the trash. Nothing is ever
	// deleted, so this is as far as removal goes.
	ActionDiscard Action = "discard"
	// ActionRestore records a scan taken back out of the trash.
	ActionRestore Action = "restore"
	// ActionMove records a filed scan moving to the directory its dates now
	// place it in. Path is where it went, From where it was.
	ActionMove Action = "move"
)

// Entry is one line.
//
// Field names are short and fixed because this file is read with grep and jq as
// often as by anything else, and because it is appended to for the life of the
// Box and never rewritten — a renamed field would split the history in two.
type Entry struct {
	At     string `json:"at"` // RFC 3339 with offset, as written
	Action Action `json:"action"`
	Digest string `json:"digest,omitempty"` // which scan
	Path   string `json:"path,omitempty"`   // relative to the Box root
	Field  string `json:"field,omitempty"`  // which sidecar field, for an edit
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Append writes one entry to the log at path, creating it if it is not there.
//
// The file is opened with O_APPEND for every write, so two processes writing at
// once interleave whole lines rather than overwriting each other's. It is
// flushed before the call returns: a log entry that is lost is a change nobody
// can undo, and the whole point of the file is that it is there afterwards.
func Append(path string, entry Entry) error {
	if strings.TrimSpace(entry.At) == "" {
		entry.At = time.Now().Format(time.RFC3339)
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// Read returns every entry in the log, oldest first.
//
// A line that cannot be parsed is skipped rather than failing the read. The log
// is appended to over years by every version of the tool there will ever be,
// and one bad line from an interrupted write must not make the rest of the
// history unreadable.
func Read(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []Entry
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// PathFor is the log's path for a Box root.
func PathFor(root string) string { return filepath.Join(root, Name) }

// Edit builds the entry for one field changing.
func Edit(digest, field, from, to string) Entry {
	return Entry{Action: ActionEdit, Digest: digest, Field: field, From: from, To: to}
}

// Import builds the entry for a scan published at a path relative to the root.
func Import(digest, relativePath string) Entry {
	return Entry{Action: ActionImport, Digest: digest, Path: relativePath}
}

func (e Entry) String() string {
	return fmt.Sprintf("%s %s %s", e.At, e.Action, e.Digest)
}
