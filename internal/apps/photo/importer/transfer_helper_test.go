package importer

import "context"

// transfer imports one file into one target, as a worker of Run would.
func transfer(ctx context.Context, index int, plan Plan, events chan<- Event) (FileResult, Event) {
	var out outcome
	transferAll(ctx, index, []*targetRun{{plan: plan, events: events, active: true}}, func(_ int, finished outcome) { out = finished })
	return out.result, out.event
}
