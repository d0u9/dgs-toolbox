package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/cred/publish"
	"dgs-toolbox/internal/entrytemplate"
	"dgs-toolbox/internal/timeline"
)

// ErrNotImplemented is what an Action that has only been declared returns. It
// is an error rather than a silent success: a plan reporting everything done
// while nothing was written is worse than one that refuses.
var ErrNotImplemented = errors.New("this action is not implemented yet")

// ErrNoVault is returned when an Obsidian Action has nowhere to write. Writing
// into a directory nobody named is worse than refusing to write.
var ErrNoVault = errors.New("no obsidian vault is configured")

// Result is what became of one Action.
type Result struct {
	Action   ActionID
	Target   string
	Executed bool
	// Skipped marks an Action that had nothing left to do, which is a success
	// with a different meaning: the entry was already in the note. Reason says
	// which, so what did not happen explains itself rather than leaving the
	// reader to work it out from the note.
	Skipped bool
	Reason  string
	Err     error
}

// Execute carries out a plan in order and stops at the first failure. It stops
// because the Actions of one Recipe are usually related — a location note and
// the daily entry pointing at it — and running the rest after one has failed
// leaves a state nobody asked for. What did happen is reported rather than
// rolled back: an entry added to a note cannot be un-added safely.
func Execute(ctx Context, plans []ActionPlan) []Result {
	results := make([]Result, 0, len(plans))
	for _, plan := range plans {
		result := Result{Action: plan.Action, Target: plan.Target}
		def, ok := LookupAction(plan.Action)
		switch {
		case !ok:
			result.Err = fmt.Errorf("unknown action %q", plan.Action)
		case !plan.Ready():
			result.Err = fmt.Errorf("blocked: %s", missingList(plan.Missing))
		case def.Run == nil:
			result.Err = ErrNotImplemented
		default:
			skipped, err := def.Run(ctx, plan)
			result.Err, result.Reason, result.Skipped = err, skipped, skipped != ""
			result.Executed = err == nil
		}
		results = append(results, result)
		if result.Err != nil {
			break
		}
	}
	return results
}

// Unimplemented names the Actions of a plan that have been declared but cannot
// run. A caller asks before offering to run one, so a plan that cannot work is
// refused with a reason rather than accepted and then failed.
func Unimplemented(plans []ActionPlan) []ActionID {
	var pending []ActionID
	for _, plan := range plans {
		def, ok := LookupAction(plan.Action)
		if !ok || def.Run == nil {
			pending = append(pending, plan.Action)
		}
	}
	return pending
}

// Executed reports whether every Action in the plan ran.
func Executed(results []Result) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if !result.Executed {
			return false
		}
	}
	return true
}

// Skipped reports whether any Action of a run had nothing left to do. It is a
// success, but not the one the reader asked for: they wrote a note and nothing
// was added to it, so a caller asks in order to say so rather than closing over
// it.
func Skipped(results []Result) bool {
	for _, result := range results {
		if result.Skipped {
			return true
		}
	}
	return false
}

func missingList(missing []FieldRequirement) string {
	names := make([]string, 0, len(missing))
	for _, req := range missing {
		names = append(names, string(req.Field))
	}
	return strings.Join(names, ", ")
}

// appendEntry adds one entry under the note's own section, creating the note
// and the section when they are missing. It rewrites the note from what it
// held, so a note edited by hand keeps everything it already has. images are
// the pictures the entry shows, by the filename the vault knows them by; they
// are already in place.
func appendEntry(ctx Context, images []string) (skipped string, err error) {
	// The target is resolved again rather than taken from the plan, so a plan
	// whose target could not be worked out fails with the reason it could not
	// be — which is what the reader has to fix — instead of with a missing
	// vault.
	target, err := dailyNotePath(ctx)
	if err != nil {
		return "", err
	}
	path, ok := ctx.Settings.vaultPath(target)
	if !ok {
		return "", ErrNoVault
	}
	entry, err := dailyEntry(ctx, images)
	if err != nil {
		return "", err
	}
	if len(entry) == 0 {
		return "", errors.New("nothing to append")
	}

	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// The day has no note yet, so it is created from the configured
		// template before the entry is added to it.
		if err := createDailyNote(ctx, path); err != nil {
			return "", err
		}
		if existing, err = os.ReadFile(path); err != nil {
			return "", fmt.Errorf("read %s: %w", target, err)
		}
	} else if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	// The note itself is the record of what was written. A Capture already
	// present is skipped rather than added twice, and deleting the entry by
	// hand is enough to have it written again — bookkeeping kept anywhere else
	// could claim a line exists that no longer does.
	if containsMark(string(existing), captureMark(ctx.Capture)) {
		return fmt.Sprintf("this capture is already in %s, as %s", target, captureMark(ctx.Capture)), nil
	}

	section, err := sectionFor(ctx)
	if err != nil {
		return "", err
	}
	updated := timeline.InsertUnderSection(string(existing), section, entry)
	if workflow := ctx.Capture.Workflow(); workflow != "" {
		updated = addToListProperty(updated, WorkflowProperty, workflow)
	}
	if err := publish.Replace(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	return "", nil
}

// containsMark reports whether the note already carries this exact block
// reference. The mark is matched as a whole word rather than as a substring:
// one id is often a prefix of another — an edited "^dgs-x" becomes "^dgs-x-2",
// which contains the first — and a substring test would then report an entry
// present that is not there.
func containsMark(note, mark string) bool {
	for _, line := range strings.Split(note, "\n") {
		for _, field := range strings.Fields(line) {
			if field == mark {
				return true
			}
		}
	}
	return false
}

// sectionFor is the heading this Capture is written under, rendered against the
// Capture itself: a heading naming the app or the device tells a reader months
// later where the entries under it came from.
func sectionFor(ctx Context) (string, error) {
	section := ctx.Parameter(ActionDailyAppend, ParameterSection)
	if !strings.Contains(section, "{{") {
		return section, nil
	}
	rendered, err := renderSection(ctx, section)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered), nil
}

// dailyEntry is what one Capture becomes in a daily note. The shape comes from
// the template rather than from here. A Capture with no text is still written:
// being somewhere is worth the line, and the template drops the lines that
// would have held the text.
func dailyEntry(ctx Context, images []string) ([]string, error) {
	data := entryData(ctx)
	data.Images = images
	parsed, err := LoadTemplate(ctx.Settings, DailyEntryTemplate)
	if err != nil {
		return nil, err
	}
	return renderEntry(parsed, data)
}

// captureMark is the block reference identifying a Capture's entry. Obsidian
// allows letters, numbers and dashes in a block id, which a Capture id already
// keeps to.
func captureMark(capture Capture) string {
	id := capture.Index.ID
	if id == "" {
		id = capture.Name
	}
	return "^dgs-" + blockIDSafe(id)
}

func blockIDSafe(id string) string {
	var safe strings.Builder
	for _, char := range id {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			safe.WriteRune(char)
		default:
			safe.WriteByte('-')
		}
	}
	return strings.Trim(safe.String(), "-")
}

// ErrNoDailyNote is returned when nothing says where a day's note lives.
var ErrNoDailyNote = errors.New("no daily note path is configured")

// dailyNotePath is where the day's note lives, from the configured path
// template. Configured rather than discovered: a vault's own settings say where
// the plugin in use puts notes, which is not the same question, and a wrong
// guess writes a Capture into a file nobody was looking at.
func dailyNotePath(ctx Context) (string, error) {
	if strings.TrimSpace(ctx.Settings.DailyNote) == "" {
		return "", ErrNoDailyNote
	}
	day, err := captureDay(ctx)
	if err != nil {
		return "", err
	}
	path, err := renderNote(ctx.Settings, "daily note path", ctx.Settings.DailyNote, noteData(ctx, day))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(path), nil
}

// captureDay is the day the Capture was taken, in the offset it recorded: the
// entry belongs to that day, not to the day it happened to be somewhere else.
func captureDay(ctx Context) (time.Time, error) {
	created := ctx.String(FieldCreatedAt)
	day, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return time.Time{}, fmt.Errorf("createdAt %q is not a timestamp", created)
	}
	return day, nil
}

// createDailyNote writes the day's note from the vault's daily note template,
// else the compiled-in one. A note is never created from nothing: an empty
// note among templated ones is one the reader has to repair by hand later.
func createDailyNote(ctx Context, path string) error {
	text, err := LoadNoteTemplate(ctx.Settings.TemplateDir)
	if err != nil {
		return err
	}
	day, err := captureDay(ctx)
	if err != nil {
		return err
	}
	rendered, err := renderNote(ctx.Settings, DailyNoteTemplate, text, noteData(ctx, day))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the note's folder: %w", err)
	}
	return os.WriteFile(path, []byte(rendered), 0o644)
}

// timeOf takes the wall clock the Capture recorded, without converting it.
func timeOf(createdAt string) string { return entrytemplate.TimeOf(createdAt) }

// FormatTimestamp renders an RFC 3339 timestamp as a compact wall clock keeping
// the offset it was written with: 2026-09-09 21:31:22 +10.
func FormatTimestamp(value string) string { return entrytemplate.FormatTimestamp(value) }

// FormatTimestampDay is FormatTimestamp with the day of the week after the
// date: 2026-09-09 Wed 21:31:22 +10.
func FormatTimestampDay(value string) string { return entrytemplate.FormatTimestampDay(value) }
