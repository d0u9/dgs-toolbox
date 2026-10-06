package outline

import (
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
	"time"
)

// Store composes this kind's domain operations with a tree Store.
type Store struct{ tree *tree.Store }

func ForStore(s *tree.Store) Store { return Store{tree: s} }

func (s Store) Fresh(names []string) error               { return Fresh(s.tree.Root(), names) }
func (s Store) Load() ([]Outline, error)                 { return Load(s.tree.Root()) }
func (s Store) LoadRules() (map[string]view.View, error) { return LoadRules(s.tree.Root()) }
func (s Store) RenameRule(from string, to string, now time.Time) error {
	return RenameRule(s.tree.Root(), from, to, now)
}
func (s Store) RenameSnapshot(from string, to string, now time.Time) error {
	return RenameSnapshot(s.tree.Root(), from, to, now)
}
func (s Store) Rules() ([]view.View, map[string][]string, error) { return Rules(s.tree.Root()) }
func (s Store) Save(previous string, o Outline, now time.Time) error {
	return Save(s.tree.Root(), previous, o, now)
}
func (s Store) SaveRule(previous string, r view.View, now time.Time) error {
	return SaveRule(s.tree.Root(), previous, r, now)
}
func (s Store) Trash(name string, now time.Time) (string, error) {
	return Trash(s.tree.Root(), name, now)
}
func (s Store) TrashRule(name string, now time.Time) (string, error) {
	return TrashRule(s.tree.Root(), name, now)
}
