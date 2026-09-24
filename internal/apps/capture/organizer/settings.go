package organizer

import (
	"path/filepath"
	"strings"
)

// Settings are the paths the Actions write to. They are declared here rather
// than read from the configuration package, so the organizer stays free of the
// application's configuration shape and a test can name a directory without
// building a config file.
//
// Nothing here is discovered. An Obsidian vault does keep its own daily note
// settings, and reading them looked like saving the reader a duplicate — but a
// vault says where the plugin in use puts notes, not where this tool should
// write them, and the two are not the same question. Guessing wrongly writes a
// Capture into a file nobody was looking at, so what is not configured is an
// error rather than a guess.
type Settings struct {
	// GPXDirectory holds one GPX of Capture waypoints per day.
	GPXDirectory string
	// ObsidianVault is an absolute path. Empty means no vault has been
	// configured, and the Obsidian Actions refuse to run.
	ObsidianVault string
	// DailyNote is where a day's note lives, relative to the vault, as a
	// template over the date: "00 Daily Log/{{.Year}}/{{.Date}}.md".
	DailyNote string
	// LocationNote is the running list of places, relative to the vault, newest
	// first. LocationArchive is where a year that has rolled over is moved to;
	// empty means nothing is archived and the list grows without limit.
	LocationNote    string
	LocationArchive string
	// DailySection is the heading a Capture is appended under. Its own section
	// rather than the end of the note, so what this tool writes stays
	// distinguishable from what the reader wrote.
	DailySection string
	// ImageFolder is where a daily note's pictures go, relative to the note's
	// folder, as a template over the note: "assets/{{.Note}}". Empty is
	// DefaultImageFolder.
	ImageFolder string
	// ImageMaxSide is the longest side, in pixels, a picture is shrunk to on
	// its way into the vault, and ImageQuality the JPEG quality it is encoded
	// at, 1 to 100. Zero is imaging's default for either.
	ImageMaxSide int
	ImageQuality int
	// Mappings translate a value on its way into a note, by table name:
	// "Australia" is what a Capture records, and a vault may file it under
	// "🇦🇺_Australia". Declared rather than coded because which names a vault
	// uses is that vault's business, and there is no end to them.
	Mappings map[string]map[string]string
	// Sources say where a workflow keeps a logical field, by workflow and then
	// by field: {"quick_note": {"content": ["payload.text"]}}. A logical field
	// is the same question — what did they write? — asked of Captures that
	// answer it under different names, and which name a workflow uses is that
	// workflow's business rather than something to compile in.
	//
	// Several paths may be given for one field and the first that resolves is
	// used, and AnyWorkflow answers for a workflow with no entry of its own, so
	// a dozen workflows that each keep their text somewhere slightly different
	// are one list rather than a dozen blocks. A field with no source at all
	// falls back to what the toolbox knows about its own workflows.
	Sources map[string]map[FieldID][]string
	// TemplateDir holds every template that is not compiled in, by filename:
	// the daily note's, and any that replaces a compiled-in default. One place
	// to look, rather than a path in the configuration for each.
	TemplateDir string
	// ReminderList is the Reminders list the reminder Actions write into by
	// default. Empty is the list Reminders files new reminders in.
	ReminderList string
	// ReminderRadius is how close counts as arriving, in metres, for a
	// reminder at a place. Zero is DefaultReminderRadius.
	ReminderRadius float64
}

// DefaultSettings are the values that have a sensible default. The paths do
// not: where a reader keeps their notes is not something to assume.
func DefaultSettings() Settings {
	return Settings{DailySection: DefaultDailySection}
}

// DefaultDailySection is the heading this tool writes under. It is a template
// over the Capture rather than a fixed word: a note is read months later, and
// "Captured - Doug's iPhone" says where the entries under it came from, which
// "DGS" does not.
//
// The device is written with "with" rather than as a bare placeholder, so a
// Capture that records no device name leaves "Captured" on its own instead of a
// heading trailing a separator with nothing after it.
const DefaultDailySection = "Captured{{with .Device}} - {{.}}{{end}}"

func (s Settings) section() string {
	if strings.TrimSpace(s.DailySection) == "" {
		return DefaultDailySection
	}
	return strings.TrimSpace(s.DailySection)
}

// vaultPath resolves a vault-relative target to where it will actually be
// written. It reports false when no vault is configured.
func (s Settings) vaultPath(target string) (string, bool) {
	if s.ObsidianVault == "" || target == "" {
		return "", false
	}
	return filepath.Join(s.ObsidianVault, target), true
}
