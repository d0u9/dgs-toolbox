package snapshot

import (
	"dgs-toolbox/internal/doc/tree"
	"time"
)

// Store composes this kind's domain operations with a tree Store.
type Store struct{ tree *tree.Store }

func ForStore(s *tree.Store) Store { return Store{tree: s} }

func (store Store) Load() ([]Snapshot, error) { return Load(store.tree.Root()) }
func (store Store) Save(previous string, s Snapshot, create bool, now time.Time) error {
	return Save(store.tree.Root(), previous, s, create, now)
}
func (store Store) Trash(name string, now time.Time) (string, error) {
	return Trash(store.tree.Root(), name, now)
}
