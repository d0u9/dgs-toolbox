package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

// appendToDailyNote adds one entry under the note's own section, creating the
// note and the section when they are missing. It rewrites the note from what it
// held, so a note edited by hand keeps everything it already has.
func appendToDailyNote(ctx Context, plan ActionPlan) (skipped string, err error) {
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
	entry, err := dailyEntry(ctx)
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
	updated := insertUnderSection(string(existing), section, entry)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
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

// insertUnderSection puts the entry at the end of the named section, adding the
// section at the end of the note when it has none. A section that already holds
// entries keeps them: the new one goes after the last line of that section,
// before whatever heading follows it.
func insertUnderSection(note, section string, entry []string) string {
	heading, level := sectionHeading(section)
	block := strings.Join(entry, "\n")
	if strings.TrimSpace(note) == "" {
		return heading + "\n\n" + block + "\n"
	}
	lines := strings.Split(strings.TrimRight(note, "\n"), "\n")

	start := -1
	for index, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = index
			break
		}
	}
	if start < 0 {
		return strings.Join(lines, "\n") + "\n\n" + heading + "\n\n" + block + "\n"
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		// The section ends at the next heading that is not inside it: one of
		// the same level or shallower. A deeper heading is part of what this
		// section holds, so an entry still goes after it.
		if next := headingLevel(lines[index]); next > 0 && next <= level {
			end = index
			break
		}
	}
	// Trailing blank lines belong to the gap before the next heading rather
	// than to the section, so the entry goes above them.
	for end > start+1 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	updated := append([]string{}, lines[:end]...)
	updated = append(updated, entry...)
	updated = append(updated, lines[end:]...)
	return strings.Join(updated, "\n") + "\n"
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

// sectionHeading is the heading a Capture is written under, and its level. A
// configured section may carry its own hashes — "## Captured" asks for a
// second-level heading — and otherwise it is a first-level one.
func sectionHeading(section string) (heading string, level int) {
	section = strings.TrimSpace(section)
	if level := headingLevel(section); level > 0 {
		return section, level
	}
	return "# " + section, 1
}

// headingLevel is the number of leading hashes of a Markdown heading, or 0 for
// a line that is not one.
func headingLevel(line string) int {
	trimmed := strings.TrimLeft(line, " ")
	level := len(trimmed) - len(strings.TrimLeft(trimmed, "#"))
	if level == 0 || level > 6 || !strings.HasPrefix(trimmed[level:], " ") {
		return 0
	}
	return level
}

// dailyEntry is what one Capture becomes in a daily note. The shape comes from
// the template rather than from here; this only refuses to write an entry with
// no content, because the time and the place of a Capture nobody wrote anything
// about is not worth a line in a diary.
func dailyEntry(ctx Context) ([]string, error) {
	data := entryData(ctx)
	if data.Content == "" {
		return nil, nil
	}
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

// ErrNoDailyTemplate is returned when a note has to be created and the template
// directory holds nothing to create it from.
var ErrNoDailyTemplate = errors.New("no daily note template")

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

// createDailyNote writes the day's note from the configured template. A note is
// never created from nothing: an empty note among templated ones is one the
// reader has to repair by hand later, and this tool has no way to know what it
// should have held.
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

// timeOf takes the wall clock the Capture recorded, without converting it: the
// entry belongs to the day the Capture was taken, in the offset it was taken
// in.
func timeOf(createdAt string) string {
	if len(createdAt) < 16 || createdAt[10] != 'T' {
		return ""
	}
	return createdAt[11:16]
}

// FormatTimestamp renders an RFC 3339 timestamp as a compact wall clock keeping
// the offset it was written with: 2026-09-09 21:31:22 +10. Whole-hour offsets
// drop their minutes and UTC reads as Z. A value that is not RFC 3339 is
// returned unchanged, so a malformed index stays visible rather than being
// hidden behind a blank.
func FormatTimestamp(value string) string {
	if value == "" {
		return ""
	}
	created, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return created.Format("2006-01-02 15:04:05") + " " + timestampOffset(created)
}

func timestampOffset(created time.Time) string {
	_, seconds := created.Zone()
	if seconds == 0 {
		return "Z"
	}
	sign := "+"
	if seconds < 0 {
		sign, seconds = "-", -seconds
	}
	hours, minutes := seconds/3600, (seconds%3600)/60
	if minutes == 0 {
		return fmt.Sprintf("%s%d", sign, hours)
	}
	return fmt.Sprintf("%s%d:%02d", sign, hours, minutes)
}
