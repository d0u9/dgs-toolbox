package organizer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// The location note is a running list, newest first, grouped under a date
// marker. The marker's shape is fixed here rather than left to a template
// because archiving reads the year back out of it: a marker a reader could
// reshape is one the archive could no longer recognise.
//
// Only the weekday is open, through the "weekday" mapping table, so a vault
// that writes them in another language says so without the shape changing.
const (
	locationDateLayout = "2006-01-02"
	locationIndent     = "    "
)

// locationMarker matches a date marker and captures its year.
var locationMarker = regexp.MustCompile(`^- \*(\d{4})-\d{2}-\d{2} .+\*$`)

// weekdaysZH are the day names the marker uses when nothing maps them.
var weekdaysZH = map[time.Weekday]string{
	time.Sunday: "周日", time.Monday: "周一", time.Tuesday: "周二", time.Wednesday: "周三",
	time.Thursday: "周四", time.Friday: "周五", time.Saturday: "周六",
}

// ErrNoLocationNote is returned when nothing says which note the running list
// lives in.
var ErrNoLocationNote = errors.New("no location note is configured")

// appendToLocationNote puts one Capture at the top of the running list, under
// its day's marker, and moves any year that is no longer the current one into
// the archive on the way past.
func appendToLocationNote(ctx Context, plan ActionPlan) (skipped string, err error) {
	if strings.TrimSpace(ctx.Settings.LocationNote) == "" {
		return "", ErrNoLocationNote
	}
	entry, err := locationEntry(ctx)
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

	// A Capture from a year that has already rolled over belongs in that
	// year's archive, not at the top of this year's list. Organizing is done
	// long after capturing, so this is the ordinary case rather than an edge:
	// the running list is what happened this year, whenever it is written.
	target := ctx.Settings.LocationNote
	running := true
	if year := day.Format(yearLayout); ctx.Settings.LocationArchive != "" && year != currentYear() {
		target, running = filepath.Join(ctx.Settings.LocationArchive, year+".md"), false
	}
	path, ok := ctx.Settings.vaultPath(target)
	if !ok {
		return "", ErrNoVault
	}

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read %s: %w", target, err)
	}
	// The note is the record of what it holds, as the daily note is.
	if containsMark(string(existing), captureMark(ctx.Capture)) {
		return fmt.Sprintf("this capture is already in %s, as %s", target, captureMark(ctx.Capture)), nil
	}

	content := string(existing)
	if content == "" && !running {
		content = archiveHeader(ctx, day.Format(yearLayout))
	}
	// The whole file is rewritten either way — a list that grows at the top
	// cannot be appended to — so archiving costs nothing extra here, and reads
	// the content already in hand.
	if running && ctx.Settings.LocationArchive != "" {
		if content, err = archivePastYears(ctx, content); err != nil {
			return "", err
		}
	}

	updated := prependUnderMarker(content, locationDateMarker(ctx, day), strings.Join(entry, "\n"))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create the note's folder: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", target, err)
	}
	return "", nil
}

const yearLayout = "2006"

// currentYear is the year the running list is for: the wall clock's, not the
// Capture's. A Capture being organized says when it was taken, which is exactly
// the thing that must not decide what counts as this year — organizing one from
// two years ago would otherwise archive everything since.
func currentYear() string { return time.Now().Format(yearLayout) }

// archiveHeader introduces a year's file the way the vault's own script does,
// naming the note the entries came from so a reader opening a year on its own
// knows what they are looking at.
func archiveHeader(ctx Context, year string) string {
	source := filepath.Base(ctx.Settings.LocationNote)
	return "---\ncssclasses:\n  - locations\n---\n\n" +
		fmt.Sprintf("> [!note]- %s 年的位置速记\n", year) +
		fmt.Sprintf("> 从 %s 归档过来的，格式一样，新的在最上面。\n", source)
}

// locationDateMarker is the day's tick on the timeline. The italics are what
// the vault's stylesheet recognises, and what the archive matches on.
func locationDateMarker(ctx Context, day time.Time) string {
	weekday := weekdaysZH[day.Weekday()]
	if mapped, ok := ctx.Settings.Mappings["weekday"][day.Weekday().String()]; ok {
		weekday = mapped
	}
	return fmt.Sprintf("- *%s %s*", day.Format(locationDateLayout), weekday)
}

// prependUnderMarker puts the entry at the top of the list: under today's
// marker when it is already there, and under a new one otherwise. The marker
// and its entries are items of one list, so no blank line may separate them —
// a blank line there breaks the list, and with it the timeline's rule.
func prependUnderMarker(content, marker, entry string) string {
	preamble, body := splitPreamble(content)
	rest := strings.TrimLeft(body, "\n")
	lines := strings.Split(rest, "\n")

	var updated string
	switch {
	case len(lines) > 0 && strings.TrimSpace(lines[0]) == marker:
		updated = strings.Join(append([]string{marker, entry}, trimLeadingBlank(lines[1:])...), "\n")
	case strings.TrimSpace(rest) != "":
		updated = marker + "\n" + entry + "\n" + rest
	default:
		updated = marker + "\n" + entry + "\n"
	}
	if preamble != "" {
		return preamble + "\n\n" + updated
	}
	return updated
}

// splitPreamble separates what introduces the note from what it records: the
// frontmatter, and a callout following it. A new entry goes below those rather
// than at the physical top of the file.
func splitPreamble(content string) (preamble, body string) {
	lines := strings.Split(content, "\n")
	index := 0
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		for at := 1; at < len(lines); at++ {
			if strings.TrimSpace(lines[at]) == "---" {
				index = at + 1
				break
			}
		}
	}
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	for index < len(lines) && strings.HasPrefix(lines[index], ">") {
		index++
	}
	end := index
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return strings.Join(lines[:end], "\n"), strings.Join(lines[end:], "\n")
}

func trimLeadingBlank(lines []string) []string {
	index := 0
	for index < len(lines) && strings.TrimSpace(lines[index]) == "" {
		index++
	}
	return lines[index:]
}

// locationSegment is one date marker and the entries under it.
type locationSegment struct {
	year string
	text string
}

// archivePastYears moves everything that is not this year into
// <archive>/<year>.md and returns what is left. Without it the running list
// grows without limit and every new entry rewrites all of it; the archive is
// not opened day to day, so its size does not matter.
func archivePastYears(ctx Context, content string) (string, error) {
	preamble, body := splitPreamble(content)
	segments := splitByMarker(body)
	current := currentYear()

	var stale, kept []locationSegment
	for _, segment := range segments {
		if segment.year == current {
			kept = append(kept, segment)
			continue
		}
		stale = append(stale, segment)
	}
	if len(stale) == 0 {
		// Nothing has rolled over, so not a single extra write happens.
		return content, nil
	}

	for _, year := range years(stale) {
		var moving []string
		for _, segment := range stale {
			if segment.year == year {
				moving = append(moving, segment.text)
			}
		}
		if err := prependToArchive(ctx, year, strings.Join(moving, "\n")); err != nil {
			return "", err
		}
	}

	var remaining []string
	for _, segment := range kept {
		remaining = append(remaining, segment.text)
	}
	rest := strings.Join(remaining, "\n")
	if preamble != "" {
		return preamble + "\n\n" + rest + "\n", nil
	}
	return rest + "\n", nil
}

// years lists the years to write, each once: a year is one file, opened once.
func years(segments []locationSegment) []string {
	var order []string
	seen := map[string]bool{}
	for _, segment := range segments {
		if !seen[segment.year] {
			seen[segment.year] = true
			order = append(order, segment.year)
		}
	}
	return order
}

// splitByMarker cuts the body into date markers with their entries. Anything
// before the first marker — normally nothing — stays where it is.
func splitByMarker(body string) []locationSegment {
	var segments []locationSegment
	var current *locationSegment
	var year string

	for _, line := range strings.Split(body, "\n") {
		if match := locationMarker.FindStringSubmatch(strings.TrimRight(line, " ")); match != nil {
			if current != nil {
				segments = append(segments, *current)
			}
			year = match[1]
			current = &locationSegment{year: year, text: line}
			continue
		}
		if current != nil {
			current.text += "\n" + line
		}
	}
	if current != nil {
		segments = append(segments, *current)
	}
	for index := range segments {
		segments[index].text = strings.TrimRight(segments[index].text, " \n\t")
	}
	return segments
}

func prependToArchive(ctx Context, year, moving string) error {
	target := filepath.Join(ctx.Settings.LocationArchive, year+".md")
	path, ok := ctx.Settings.vaultPath(target)
	if !ok {
		return ErrNoVault
	}
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("create the archive folder: %w", err)
		}
		return os.WriteFile(path, []byte(archiveHeader(ctx, year)+"\n"+moving+"\n"), 0o644)
	}
	if err != nil {
		return fmt.Errorf("read %s: %w", target, err)
	}
	preamble, body := splitPreamble(string(existing))
	rest := strings.TrimLeft(body, "\n")
	merged := moving + "\n"
	if rest != "" {
		merged = moving + "\n" + rest
	}
	if preamble != "" {
		merged = preamble + "\n\n" + merged
	}
	return os.WriteFile(path, []byte(merged), 0o644)
}

// copyLink shows the position as "(latitude, longitude)" and links it to the
// vault's copy command, which puts it on the clipboard the other way round.
// What is read and what is copied deliberately differ: maps want one order and
// the services that take a pasted pair want the other.
func copyLink(ctx Context, latitude, longitude string) string {
	shown := fmt.Sprintf("(%s, %s)", latitude, longitude)
	choice := strings.TrimSpace(ctx.Settings.CoordinateChoice)
	if choice == "" || latitude == "" || longitude == "" {
		return shown
	}
	return fmt.Sprintf("[%s](obsidian://quickadd?choice=%s&value-coordinates=%s)",
		shown, escapeURIComponent(choice), escapeURIComponent(latitude+", "+longitude))
}

// escapeURIComponent encodes the way a browser's encodeURIComponent does, which
// is what wrote the links already in these notes: a space is %20 rather than +,
// and the characters JavaScript leaves alone are left alone. A + would be read
// back as a literal plus by some parsers, and the command it names would not be
// found.
func escapeURIComponent(value string) string {
	const unreserved = "-_.!~*'()"
	var out strings.Builder
	for _, b := range []byte(value) {
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9',
			strings.IndexByte(unreserved, b) >= 0:
			out.WriteByte(b)
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}
