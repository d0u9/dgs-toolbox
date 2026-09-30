package tree

import (
	"path/filepath"
	"sync"
)

// A write checks an Item against the others — a unique field, a link, a
// PDF already kept. By default those others are read from the sidecars. A
// process that keeps the tree's Items current, the page's cache, may lend
// them instead with UseForChecks, so a write does not read the whole tree.
// The Item being written is always read from its own sidecar.
var (
	checkMu      sync.RWMutex
	checkSources = map[string]func() ([]Item, error){}
)

// UseForChecks makes items the source of the Items writes to root check
// against. Nil goes back to reading the sidecars.
func UseForChecks(root string, items func() ([]Item, error)) {
	checkMu.Lock()
	defer checkMu.Unlock()
	key := checkKey(root)
	if items == nil {
		delete(checkSources, key)
		return
	}
	checkSources[key] = items
}

// others is every Item a write to root checks against.
func others(root string) ([]Item, error) {
	checkMu.RLock()
	source := checkSources[checkKey(root)]
	checkMu.RUnlock()
	if source != nil {
		return source()
	}
	return LoadItems(root)
}

func checkKey(root string) string {
	if abs, err := filepath.Abs(root); err == nil {
		return abs
	}
	return root
}
