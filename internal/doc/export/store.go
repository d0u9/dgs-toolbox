package export

import (
	"context"
	"dgs-toolbox/internal/doc/tree"
)

// Store composes export planning and publication with a tree's storage.
type Store struct{ tree *tree.Store }

func ForStore(s *tree.Store) Store { return Store{tree: s} }
func (s Store) PlanJobs(ctx context.Context, jobs []Job, items []tree.Item) ([]JobPlan, error) {
	return PlanJobs(ctx, s.tree.Root(), jobs, items)
}
func (s Store) Apply(ctx context.Context, target string, views []string, plan Plan, items []tree.Item, progress func(done, total int), published func(Action)) (Result, error) {
	return Apply(ctx, s.tree.Root(), target, views, plan, items, progress, published)
}
