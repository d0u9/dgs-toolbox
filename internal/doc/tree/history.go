package tree

import (
	"slices"
	"sort"
	"time"
)

// Timeline is an Item's history oldest first. A sidecar written before
// history was kept has none for its imports, so each revision not already
// recorded as imported gets an import event at its added time.
func Timeline(item Item) []HistoryEvent {
	imported := map[string]bool{}
	for _, event := range item.History {
		if event.Action == "import" || event.Action == "import_revision" {
			imported[event.Digest] = true
		}
	}
	out := append([]HistoryEvent(nil), item.History...)
	for _, r := range item.Revisions {
		if !imported[r.Digest] && r.Added != "" {
			out = insertByTime(out, HistoryEvent{At: r.Added, Action: "import", Digest: r.Digest})
		}
	}
	return out
}

// LogEntry is one event of one Item in the tree's log.
type LogEntry struct {
	Item  string       `json:"item"`
	Type  string       `json:"type"`
	Event HistoryEvent `json:"event"`
}

// Log is every Item's timeline in one list, newest first. Events whose time
// cannot be read go last. Events in the same second keep the order they were
// recorded in, reversed, so an Item's later one still comes first.
func Log(items []Item) []LogEntry {
	var out []LogEntry
	for _, item := range items {
		timeline := Timeline(item)
		for i := len(timeline) - 1; i >= 0; i-- {
			out = append(out, LogEntry{Item: item.ID, Type: item.Type, Event: timeline[i]})
		}
	}
	at := make([]time.Time, len(out))
	for i, e := range out {
		at[i], _ = time.Parse(time.RFC3339, e.Event.At)
	}
	index := make([]int, len(out))
	for i := range index {
		index[i] = i
	}
	sort.SliceStable(index, func(a, b int) bool { return at[index[a]].After(at[index[b]]) })
	sorted := make([]LogEntry, len(out))
	for i, j := range index {
		sorted[i] = out[j]
	}
	return sorted
}

// Exported is one PDF an export actually published.
type Exported struct {
	Item, Digest, Target, View string
}

// RecordExports adds an export event for each published PDF, reading the
// tree once and writing each Item's sidecar once however many of its PDFs
// were published.
func RecordExports(root string, published []Exported, now time.Time) error {
	if len(published) == 0 {
		return nil
	}
	items, err := LoadItems(root)
	if err != nil {
		return err
	}
	byID := make(map[string]int, len(items))
	for i, item := range items {
		byID[item.ID] = i
	}
	var changed []string
	for _, p := range published {
		i, ok := byID[p.Item]
		if !ok {
			continue
		}
		if !slices.Contains(changed, p.Item) {
			changed = append(changed, p.Item)
		}
		items[i].History = append(items[i].History, HistoryEvent{
			At: now.Format(time.RFC3339), Action: "export", Digest: p.Digest, Target: p.Target, View: p.View,
		})
	}
	for _, id := range changed {
		if err := WriteItem(root, items[byID[id]]); err != nil {
			return err
		}
	}
	return nil
}

// insertByTime places event before the first event that happened after it,
// so an event recorded late for something earlier keeps the history in order.
// An event whose time cannot be read goes last.
func insertByTime(history []HistoryEvent, event HistoryEvent) []HistoryEvent {
	at, err := time.Parse(time.RFC3339, event.At)
	if err != nil {
		return append(history, event)
	}
	for i, e := range history {
		if t, err := time.Parse(time.RFC3339, e.At); err == nil && t.After(at) {
			return slices.Insert(history, i, event)
		}
	}
	return append(history, event)
}
