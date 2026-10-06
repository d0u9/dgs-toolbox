package merge

import (
	"context"
	"dgs-toolbox/internal/doc/tree"
	"time"
)

// Store composes merge planning and publication with a tree's storage.
type Store struct{ tree *tree.Store }

func ForStore(s *tree.Store) Store               { return Store{tree: s} }
func (s Store) Compute(src Source) (Plan, error) { return Compute(s.tree.Root(), src) }
func (s Store) Apply(ctx context.Context, src Source, plan Plan, choices map[string]string, now time.Time) (Result, error) {
	return Apply(ctx, s.tree.Root(), src, plan, choices, now)
}
