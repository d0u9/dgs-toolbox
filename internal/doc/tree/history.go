package tree

import "time"

// RecordExport records one PDF actually published by an export.
func RecordExport(root, id, digest, target, view string, now time.Time) error {
	item, _, err := FindItem(root, id)
	if err != nil {
		return err
	}
	item.History = append(item.History, HistoryEvent{At: now.Format(time.RFC3339), Action: "export", Digest: digest, Target: target, View: view})
	return WriteItem(root, item)
}
