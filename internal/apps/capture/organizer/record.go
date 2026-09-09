package organizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// RecordFilename is the organizer's own file inside a Capture directory. It
// sits beside index.json and is named for who writes it: index.json is the
// producer's record of what was captured, organize.json is this tool's record
// of how it was organized. A Capture that carries one has been handled.
const RecordFilename = "organize.json"

// Record is the Capture's organizing history, written back into the Capture
// directory so the decisions outlive the session. It is deliberately not the
// Capture's index: the index stays the producer's, untouched.
//
// A Capture may be organized more than once — a new Action appears, or the
// first pass turns out to have been wrong — so the record holds a list of runs
// rather than one decision. Later runs are appended; nothing is overwritten.
type Record struct {
	Schema string `json:"schema"`
	Runs   []Run  `json:"runs"`
}

// Run is one pass over the Capture: what was decided, and what it would have
// done, at that moment.
type Run struct {
	OrganizedAt string           `json:"organizedAt"`
	Recipe      RecipeID         `json:"recipe"`
	RecipeName  string           `json:"recipeName"`
	Actions     []RecordedAction `json:"actions"`
	Fields      map[FieldID]any  `json:"fields,omitempty"`
}

// Latest is the most recent run, which is what a caller shows when it has room
// for one. A record always carries at least one run in practice, but an
// externally edited file may not.
func (r Record) Latest() (Run, bool) {
	if len(r.Runs) == 0 {
		return Run{}, false
	}
	return r.Runs[len(r.Runs)-1], true
}

// RecordedAction is one Action as it stood when the Capture was organized.
// Executed stays false until Actions are implemented; the record already
// distinguishes the two so a later run need not guess what was written.
type RecordedAction struct {
	Action   ActionID `json:"action"`
	Target   string   `json:"target"`
	Executed bool     `json:"executed"`
}

// NewRun captures a decision as it stands now.
func NewRun(recipe Recipe, selection Selection, plans []ActionPlan, at time.Time) Run {
	actions := make([]RecordedAction, 0, len(plans))
	for _, plan := range plans {
		actions = append(actions, RecordedAction{Action: plan.Action, Target: plan.Target})
	}
	fields := make(map[FieldID]any, len(selection.Enrichment))
	for field, value := range selection.Enrichment {
		fields[field] = value
	}
	if len(fields) == 0 {
		fields = nil
	}
	return Run{
		OrganizedAt: at.Format(time.RFC3339),
		Recipe:      recipe.ID,
		RecipeName:  recipe.Name,
		Actions:     actions,
		Fields:      fields,
	}
}

// AppendRun adds a run to the Capture's record, creating the record if this is
// the first one, and returns the record as it now stands. Organizing a Capture
// again adds to its history rather than replacing it, so a pass made against an
// older set of Actions stays visible next to the one that followed it.
func AppendRun(dir string, run Run) (Record, error) {
	record, ok := ReadRecord(dir)
	if !ok {
		record = Record{Schema: "v1"}
	}
	record.Schema = "v1"
	record.Runs = append(record.Runs, run)
	return record, writeRecord(dir, record)
}

func writeRecord(dir string, record Record) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode record: %w", err)
	}
	path := filepath.Join(dir, RecordFilename)
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write record: %w", err)
	}
	return nil
}

// ReadRecord returns the record a Capture carries. A Capture with no record has
// not been organized, which is not an error. A record with no run is treated the
// same way: what marks a Capture handled is a run, not the file.
func ReadRecord(dir string) (Record, bool) {
	data, err := os.ReadFile(filepath.Join(dir, RecordFilename))
	if err != nil {
		return Record{}, false
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, false
	}
	return record, len(record.Runs) > 0
}
