package tree

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ChangeMarkName is the file at a tree's root whose token every write dgs
// makes replaces, so a cache of the tree can tell it changed by one small
// read. It says only that something changed; it holds no metadata.
const ChangeMarkName = "dgs-doctree.changed"

// ReadChangeMark is the token in root's change mark; none yet is "".
func ReadChangeMark(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ChangeMarkName))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(string(data)), err
}

// WriteChangeMark gives root's change mark a new random token, through a
// temporary file and a rename, and returns it.
func WriteChangeMark(root string) (string, error) {
	if err := Require(root); err != nil {
		return "", err
	}
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b[:])
	temp, err := os.CreateTemp(root, ".changed-*.dgs-part")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.WriteString(token + "\n"); err != nil {
		temp.Close()
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	return token, os.Rename(temp.Name(), filepath.Join(root, ChangeMarkName))
}

// LoadItem reads the Item id's sidecar; none there is nil and no error.
func LoadItem(root, id string) (*Item, error) { return loadItem(root, id) }

// Clone is a copy of i sharing no map or slice with it, so a caller may
// change one without changing the other. A history event's own maps are
// shared: nothing changes an event once written.
func (i Item) Clone() Item {
	out := i
	out.Fields = cloneMap(i.Fields)
	out.Tags = cloneSlice(i.Tags)
	out.SharedWith = cloneSlice(i.SharedWith)
	out.History = cloneSlice(i.History)
	if i.Revisions != nil {
		out.Revisions = make([]Revision, len(i.Revisions))
		for n, r := range i.Revisions {
			r.Fields = cloneMap(r.Fields)
			r.Tags = cloneSlice(r.Tags)
			out.Revisions[n] = r
		}
	}
	return out
}

func cloneMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func cloneSlice[T any](s []T) []T {
	if s == nil {
		return nil
	}
	return append(make([]T, 0, len(s)), s...)
}
