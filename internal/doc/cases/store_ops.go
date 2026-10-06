package cases

import (
	"dgs-toolbox/internal/doc/tree"
	"time"
)

// Store composes this kind's domain operations with a tree Store.
type Store struct{ tree *tree.Store }

func ForStore(s *tree.Store) Store { return Store{tree: s} }

func (s Store) Find(name string) (Case, error) { return Find(s.tree.Root(), name) }
func (s Store) Load() ([]Case, error)          { return Load(s.tree.Root()) }
func (s Store) Save(c Case, create bool) error { return Save(s.tree.Root(), c, create) }
func (s Store) Trash(name string, now time.Time) (string, error) {
	return Trash(s.tree.Root(), name, now)
}

func (s Store) SaveAs(previous string, c Case, now time.Time) error {
	return SaveAs(s.tree.Root(), previous, c, now)
}
