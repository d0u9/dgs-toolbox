package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/timeline"
)

// A timeline is a note that grows at the top, newest first, under a marker for
// each day, and sheds the years that have rolled over into one file per year.
// The list of places is one and the family timeline another. What they share
// is the algorithm, in internal/timeline; what differs between them is only
// this table, so another timeline is one more entry here, its two settings,
// and their documentation.
type timelineKind struct {
	action ActionID
	label  string
	// what is the list in the Action's effects: "the running list of places".
	what string
	// paths reads the running note and the archive folder, both relative to
	// the vault. An empty archive is the note's own folder.
	paths func(Settings) (note, archive string)
	// templates are the entry templates tried in order, first in the template
	// directory and then compiled in, so a timeline may borrow another's.
	templates []string
	// header introduces a year's archive; Source is filled from the note.
	header timeline.Header
	// missing is returned when no note is configured.
	missing error
	// contentRequired says an entry is nothing without its text. A place is
	// worth recording unannotated; an event is not.
	contentRequired bool
}

// ErrNoLocationNote is returned when nothing says which note the running list
// of places lives in.
var ErrNoLocationNote = errors.New("no location note is configured")

// ErrNoTimelineNote is returned when nothing says which note the family
// timeline lives in.
var ErrNoTimelineNote = errors.New("no timeline note is configured")

var timelines = []timelineKind{
	{
		action:    ActionLocationAppend,
		label:     "Location note",
		what:      "the running list of places",
		paths:     func(s Settings) (string, string) { return s.LocationNote, s.LocationArchive },
		templates: []string{LocationEntryTemplate},
		header:    timeline.Header{CSSClass: "locations", Title: "位置速记"},
		missing:   ErrNoLocationNote,
	},
	{
		action:          ActionTimelineAppend,
		label:           "Timeline",
		what:            "the timeline",
		paths:           func(s Settings) (string, string) { return s.TimelineNote, s.TimelineArchive },
		templates:       []string{TimelineEntryTemplate, LocationEntryTemplate},
		header:          timeline.Header{CSSClass: "timeline", Title: "时间线"},
		missing:         ErrNoTimelineNote,
		contentRequired: true,
	},
}

// Each timeline is registered as an Action from its row, so the definitions
// cannot drift apart from one another.
func init() {
	for _, kind := range timelines {
		content := multiline(FieldContent, "Note")
		if !kind.contentRequired {
			content = optional(content)
		}
		actionDefinitions[kind.action] = ActionDefinition{
			ID:           kind.action,
			Label:        kind.label,
			UsesPosition: true,
			Effects: []string{
				fmt.Sprintf("Adds the Capture to the top of %s, under its day, with its place when it has one", kind.what),
				"Moves any year that has rolled over into the archive as it goes",
				"Writes nothing the second time: an entry carries the Capture's id and is added once",
			},
			Required: []FieldRequirement{
				{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
				content,
			},
			Target: func(ctx Context) string {
				note, _ := kind.paths(ctx.Settings)
				return note
			},
			Run: func(ctx Context, _ ActionPlan) (string, error) { return kind.prepend(ctx) },
		}
	}
}

// currentYear is the year the running note is for: the wall clock's, not the
// Capture's. A Capture being organized says when it was taken, which is exactly
// the thing that must not decide what counts as this year — organizing one from
// two years ago would otherwise archive everything since.
func currentYear() string { return time.Now().Format(timeline.YearLayout) }

// prepend puts one Capture at the top of the running note, under its day's
// marker, and moves any year that is no longer the current one into the
// archive on the way past.
func (kind timelineKind) prepend(ctx Context) (skipped string, err error) {
	note, archive := kind.paths(ctx.Settings)
	if strings.TrimSpace(note) == "" {
		return "", kind.missing
	}
	entry, err := kind.entry(ctx)
	if err != nil {
		return "", err
	}
	if len(entry) == 0 {
		return "", errors.New("nothing to record")
	}
	day, err := captureDay(ctx)
	if err != nil {
		return "", err
	}
	// Every timeline is archived: a list that is never shed grows without
	// limit and every entry rewrites all of it. Without a folder of its own,
	// a year goes beside the note.
	if strings.TrimSpace(archive) == "" {
		archive = filepath.Dir(note)
	}
	header := kind.header
	header.Source = filepath.Base(note)

	// A Capture from a year that has already rolled over belongs in that
	// year's archive, not at the top of this year's note. Organizing is done
	// long after capturing, so this is the ordinary case rather than an edge.
	target := note
	running := true
	if year := day.Format(timeline.YearLayout); year != currentYear() {
		target, running = filepath.Join(archive, year+".md"), false
	}
	path, ok := ctx.Settings.vaultPath(target)
	if !ok {
		return "", ErrNoVault
	}

	existing, err := readNote(path, target)
	if err != nil {
		return "", err
	}
	// The note is the record of what it holds, as the daily note is.
	if containsMark(existing, captureMark(ctx.Capture)) {
		return fmt.Sprintf("this capture is already in %s, as %s", target, captureMark(ctx.Capture)), nil
	}

	content := existing
	if content == "" && !running {
		content = header.Render(day.Format(timeline.YearLayout))
	}
	// The whole file is rewritten either way — a list that grows at the top
	// cannot be appended to — so archiving costs nothing extra here, and reads
	// the content already in hand.
	if running {
		var moved []timeline.Year
		content, moved = timeline.Archive(content, currentYear())
		for _, year := range moved {
			if err := writeArchive(ctx, archive, header, year); err != nil {
				return "", err
			}
		}
	}

	updated := timeline.Prepend(content, dayMarker(ctx, day), strings.Join(entry, "\n"))
	if err := writeNote(path, target, updated); err != nil {
		return "", err
	}
	return "", nil
}

// entry renders one Capture with the timeline's template.
func (kind timelineKind) entry(ctx Context) ([]string, error) {
	parsed, err := LoadTemplate(ctx.Settings, firstTemplate(ctx.Settings.TemplateDir, kind.templates))
	if err != nil {
		return nil, err
	}
	return renderEntry(parsed, entryData(ctx))
}

// firstTemplate is the first of the names the reader has in their template
// directory, else the first compiled in. A reader's own template for one list
// is what they meant for it; a compiled-in default is only a fallback, so it
// never outranks a reader's template for a list this one borrows from.
func firstTemplate(dir string, names []string) string {
	if dir != "" {
		for _, name := range names {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return name
			}
		}
	}
	for _, name := range names {
		if _, err := builtinTemplates.ReadFile("templates/" + name); err == nil {
			return name
		}
	}
	return names[0]
}

// dayMarker names the day through the weekday mapping table, which ships and
// is laid down by --init: a vault writing its days in another language says so
// in that file. With no table, the day is named the way the clock names it.
func dayMarker(ctx Context, day time.Time) string {
	weekday := day.Weekday().String()
	if mapped, ok := ctx.Settings.Mappings["weekday"][weekday]; ok {
		weekday = mapped
	}
	return timeline.Marker(day, weekday)
}

// writeArchive puts one year leaving the running note at the top of its file.
func writeArchive(ctx Context, archive string, header timeline.Header, year timeline.Year) error {
	target := filepath.Join(archive, year.Year+".md")
	path, ok := ctx.Settings.vaultPath(target)
	if !ok {
		return ErrNoVault
	}
	existing, err := readNote(path, target)
	if err != nil {
		return err
	}
	return writeNote(path, target, timeline.Merge(existing, header, year))
}

// readNote is a note's text, empty when it does not exist yet.
func readNote(path, target string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	return string(data), nil
}

func writeNote(path, target, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the folder of %s: %w", target, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}
	return nil
}
