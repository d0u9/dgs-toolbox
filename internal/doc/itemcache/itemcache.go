// Package itemcache keeps a doc tree's Items parsed, so a question the page
// asks costs one read of the tree's change mark and not one read per
// sidecar. The sidecars stay the only truth: the cache is rebuilt from them
// whenever it is missing, unreadable, of another version, or the change
// mark says the tree changed. The design is docs/apps/doc/cache.md.
package itemcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"dgs-toolbox/internal/doc/tree"
)

// Version is the cache file's format. A file of any other is rebuilt.
const Version = 1

// Cache is one tree's Items. Its methods are safe to call at once; a
// rebuild and a write never overlap, so a write is never lost to a rebuild
// that began before it.
type Cache struct {
	root string
	// file is where the cache is kept between runs; empty keeps it only in
	// memory.
	file string

	mu     sync.Mutex
	loaded bool
	token  string
	items  []tree.Item
}

type onDisk struct {
	Version int         `json:"version"`
	Root    string      `json:"root"`
	Token   string      `json:"token"`
	Items   []tree.Item `json:"items"`
}

// New is the cache of the tree at root, kept under dir, one file per tree.
// An empty dir keeps it only in memory.
func New(root, dir string) *Cache {
	c := &Cache{root: root}
	if dir != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			abs = root
		}
		sum := sha256.Sum256([]byte(abs))
		c.file = filepath.Join(dir, "items", hex.EncodeToString(sum[:8])+".json")
	}
	return c
}

// Items is every Item, sorted by ID, as the sidecars say. Each is a copy
// the caller may change.
func (c *Cache) Items() ([]tree.Item, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	token, err := tree.ReadChangeMark(c.root)
	if err != nil {
		return nil, err
	}
	if !c.loaded && token != "" {
		c.readFile(token)
	}
	if !c.loaded || token == "" || token != c.token {
		if err := c.rebuild(token); err != nil {
			return nil, err
		}
	}
	return clone(c.items), nil
}

// Reload reads every sidecar again: for a sidecar edited by hand, which
// writes no change mark.
func (c *Cache) Reload() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	token, err := tree.ReadChangeMark(c.root)
	if err != nil {
		return err
	}
	return c.rebuild(token)
}

// Wrote is told once a write to the tree has succeeded. It gives the change
// mark a new token and brings the cache up to date: only the Items named
// are read again, or, when none is, all of them.
func (c *Cache) Wrote(ids ...string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	token, err := tree.WriteChangeMark(c.root)
	if err != nil {
		c.loaded = false
		return err
	}
	if !c.loaded || len(ids) == 0 {
		return c.rebuild(token)
	}
	for _, id := range ids {
		item, err := tree.LoadItem(c.root, id)
		if err != nil {
			c.loaded = false
			return err
		}
		at := sort.Search(len(c.items), func(i int) bool { return c.items[i].ID >= id })
		found := at < len(c.items) && c.items[at].ID == id
		switch {
		case item == nil && found:
			c.items = append(c.items[:at], c.items[at+1:]...)
		case item != nil && found:
			c.items[at] = *item
		case item != nil:
			c.items = append(c.items, tree.Item{})
			copy(c.items[at+1:], c.items[at:])
			c.items[at] = *item
		}
	}
	c.token = token
	c.writeFile()
	return nil
}

// rebuild reads every sidecar. A tree with no change mark gets one, so the
// next question can tell whether anything changed.
func (c *Cache) rebuild(token string) error {
	items, err := tree.LoadItems(c.root)
	if err != nil {
		c.loaded = false
		return err
	}
	if token == "" {
		if token, err = tree.WriteChangeMark(c.root); err != nil {
			c.loaded = false
			return err
		}
	}
	c.items, c.token, c.loaded = items, token, true
	c.writeFile()
	return nil
}

// readFile takes the kept cache when it is this version, this tree's, and
// built at token; anything else is left for a rebuild.
func (c *Cache) readFile(token string) {
	if c.file == "" {
		return
	}
	data, err := os.ReadFile(c.file)
	if err != nil {
		return
	}
	var kept onDisk
	if json.Unmarshal(data, &kept) != nil || kept.Version != Version || kept.Root != c.root || kept.Token != token {
		return
	}
	c.items, c.token, c.loaded = kept.Items, kept.Token, true
}

// writeFile keeps the cache for the next run. Failing to is no error: the
// next run reads the tree instead.
func (c *Cache) writeFile() {
	if c.file == "" {
		return
	}
	data, err := json.Marshal(onDisk{Version: Version, Root: c.root, Token: c.token, Items: c.items})
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(c.file), 0o755) != nil {
		return
	}
	temp, err := os.CreateTemp(filepath.Dir(c.file), ".items-*.part")
	if err != nil {
		return
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return
	}
	if temp.Close() != nil {
		return
	}
	os.Rename(temp.Name(), c.file)
}

func clone(items []tree.Item) []tree.Item {
	out := make([]tree.Item, len(items))
	for i, it := range items {
		out[i] = it.Clone()
	}
	return out
}

// Marked is told once a write to the tree that changed no Item has
// succeeded — a Template, an Outline — and gives the change mark a new
// token, so other runs look again.
func (c *Cache) Marked() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	token, err := tree.WriteChangeMark(c.root)
	if err != nil {
		c.loaded = false
		return err
	}
	if c.loaded {
		c.token = token
		c.writeFile()
	}
	return nil
}
