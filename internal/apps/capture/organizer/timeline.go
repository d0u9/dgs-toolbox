package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"dgs-toolbox/internal/timeline"
)

// A timeline is a note that grows at the top, newest first, under a marker for
// each day, and sheds the years that have rolled over into one file per year.
// The list of places is one and the family timeline another. What they share
// is the algorithm, in internal/timeline; what differs between them is a
// definition in the vault's timelines file, which the Obsidian plugin reads
// too. Each definition is an Action of its own, obsidian.timeline.<name>, made
// when it is looked up rather than registered: the file is the reader's, and
// a timeline added there is one more Action without anything compiled.

// TimelineActionPrefix opens the id of every timeline's Action.
const TimelineActionPrefix = "obsidian.timeline."

// TimelineAction is the Action that writes onto the named timeline.
func TimelineAction(name string) ActionID {
	return ActionID(TimelineActionPrefix + name)
}

// ErrNoTimelinesFile is returned when nothing says where the timelines are
// defined.
var ErrNoTimelinesFile = errors.New("no timelines file is configured")

// timelineAction is the definition of one timeline's Action. What it needs from
// the timelines file — the note, whether text is required — is read when it
// runs, so editing the file needs no restart.
func timelineAction(name string) ActionDefinition {
	id := TimelineAction(name)
	contentRequired := func(ctx Context) bool {
		definition, err := ctx.Settings.timeline(name)
		return err == nil && definition.ContentRequired
	}
	required := multiline(FieldContent, "Note")
	required.When = contentRequired
	offered := optional(multiline(FieldContent, "Note"))
	offered.When = func(ctx Context) bool { return !contentRequired(ctx) }
	return ActionDefinition{
		ID:           id,
		Label:        "Timeline: " + name,
		UsesPosition: true,
		Effects: []string{
			fmt.Sprintf("Adds the Capture to the %s timeline, under its day, with its place when it has one", name),
			"Moves any year that has rolled over into the archive as it goes",
			"Writes nothing the second time: an entry carries the Capture's id and is added once",
		},
		Required: []FieldRequirement{
			{Field: FieldCreatedAt, Label: "Created", Required: true, Input: InputText},
			required,
			offered,
		},
		Target: func(ctx Context) string {
			definition, err := ctx.Settings.timeline(name)
			if err != nil {
				return ""
			}
			return definition.Note
		},
		Run: func(ctx Context, _ ActionPlan) (string, error) { return insertOnTimeline(ctx, name) },
	}
}

// timeline reads the named timeline from the timelines file. The file is read
// again only when it changes, since a plan asks for its target more often than
// anything writes.
func (s Settings) timeline(name string) (timeline.Definition, error) {
	if strings.TrimSpace(s.TimelinesFile) == "" {
		return timeline.Definition{}, ErrNoTimelinesFile
	}
	path, ok := s.vaultPath(s.TimelinesFile)
	if !ok {
		return timeline.Definition{}, ErrNoVault
	}
	definitions, err := readDefinitions(path)
	if err != nil {
		return timeline.Definition{}, fmt.Errorf("%s: %w", s.TimelinesFile, err)
	}
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, nil
		}
	}
	return timeline.Definition{}, fmt.Errorf("%s defines no timeline named %q", s.TimelinesFile, name)
}

var definitionsCache struct {
	sync.Mutex
	path        string
	modified    time.Time
	size        int64
	definitions []timeline.Definition
}

func readDefinitions(path string) ([]timeline.Definition, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	definitionsCache.Lock()
	defer definitionsCache.Unlock()
	if definitionsCache.path == path && definitionsCache.modified.Equal(info.ModTime()) && definitionsCache.size == info.Size() {
		return definitionsCache.definitions, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	definitions, err := timeline.ParseDefinitions(data)
	if err != nil {
		return nil, err
	}
	definitionsCache.path, definitionsCache.modified, definitionsCache.size = path, info.ModTime(), info.Size()
	definitionsCache.definitions = definitions
	return definitions, nil
}

// currentYear is the year the running note is for: the wall clock's, not the
// Capture's. A Capture being organized says when it was taken, which is exactly
// the thing that must not decide what counts as this year — organizing one from
// two years ago would otherwise archive everything since.
func currentYear() string { return time.Now().Format(timeline.YearLayout) }

// insertOnTimeline puts one Capture into the running note, under its day's
// marker, and moves any year that is no longer the current one into the
// archive on the way past.
func insertOnTimeline(ctx Context, name string) (skipped string, err error) {
	definition, err := ctx.Settings.timeline(name)
	if err != nil {
		return "", err
	}
	note, archive := definition.Note, definition.Archive
	entry, err := timelineEntry(ctx, definition)
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
	header := definition.Header(filepath.Base(note))

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

	updated, _ := timeline.Insert(content, day.Format(timeline.DateLayout), dayMarker(ctx, day), strings.Join(entry, "\n"))
	if err := writeNote(path, target, updated); err != nil {
		return "", err
	}
	return "", nil
}

// timelineEntry renders one Capture with the timeline's template: the one its
// definition names, else the timeline entry template, else the location
// entry's, so a timeline reads like the list of places until it is given a
// shape of its own.
func timelineEntry(ctx Context, definition timeline.Definition) ([]string, error) {
	names := []string{TimelineEntryTemplate, LocationEntryTemplate}
	if definition.Template != "" {
		names = append([]string{definition.Template}, names...)
	}
	parsed, err := LoadTemplate(ctx.Settings, firstTemplate(ctx.Settings.TemplateDir, names))
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
