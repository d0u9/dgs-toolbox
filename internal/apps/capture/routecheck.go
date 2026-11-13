package capture

import (
	"fmt"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/desktop"
	"dgs-toolbox/internal/entrytemplate"

	"github.com/charmbracelet/x/ansi"
)

// Checking a Capture before it is written somewhere: where its position lands
// on a map, and marking one found to be wrong so Archive sets it aside.

// openURL hands a map to the operating system. It is a variable so a test
// records what would have opened instead of opening it.
var openURL = desktop.Open

// discardNow is when a mark is made, a variable so a test can name it.
var discardNow = time.Now

// routeMapServices are the links the map row carries, in the order written.
const routeMapServices = "Apple, Google, Amap"

// positionInUse is the position a map is offered for: the Capture's resolved
// position, when the Recipe chosen for it has an enabled Action that writes it.
// A position nothing will write is not worth checking.
func (m routeModel) positionInUse() (organizer.Position, bool) {
	selection, recipe, ok := m.currentSelection()
	if !ok || !organizer.UsesPosition(recipe, selection.EnabledActions(recipe)) {
		return organizer.Position{}, false
	}
	return m.capturePosition()
}

// capturePosition is the selected Capture's resolved position, whatever Recipe
// is chosen for it, if any. Where a Capture was taken is often what recalls
// what it was, so `o` opens it from the first look rather than only once a
// Recipe that writes it is chosen.
func (m routeModel) capturePosition() (organizer.Position, bool) {
	ctx, ok := m.context()
	if !ok {
		return organizer.Position{}, false
	}
	return ctx.Position()
}

// mapHint offers the map key only when the Capture has a position to open.
func (m routeModel) mapHint() string {
	if _, ok := m.capturePosition(); ok {
		return "  o Map"
	}
	return ""
}

// openMap opens the Capture's position in Apple Maps, which is the map this
// machine opens without a browser.
func (m routeModel) openMap() routeModel {
	position, ok := m.capturePosition()
	if !ok {
		m.notice = failed("This Capture has no position")
		return m
	}
	links := mapLinks(position, m.placeLabel(), "Apple")
	if len(links) == 0 {
		return m
	}
	if err := openURL(links[0].URL); err != nil {
		m.notice = failed("Could not open the map: " + err.Error())
		return m
	}
	m.notice = moveNotice{text: fmt.Sprintf("Opened %.5f, %.5f in Apple Maps", position.Latitude, position.Longitude)}
	return m
}

func mapLinks(position organizer.Position, label, services string) []entrytemplate.MapLink {
	return entrytemplate.MapLinks(
		fmt.Sprintf("%.6f", position.Latitude),
		fmt.Sprintf("%.6f", position.Longitude),
		label, services,
	)
}

// placeLabel names the pin by the place the Actions would write, when there is
// one, so the card says which place is meant rather than only where.
func (m routeModel) placeLabel() string {
	ctx, ok := m.context()
	if !ok {
		return ""
	}
	return ctx.String(organizer.FieldPlaceName)
}

// mapLinksText is the map row's value: each service's English name, linked to
// the position, so a terminal that follows links opens it with a click.
func mapLinksText(position organizer.Position, label string) string {
	links := mapLinks(position, label, routeMapServices)
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, ansi.SetHyperlink(link.URL)+link.English+ansi.ResetHyperlink())
	}
	return strings.Join(parts, " · ")
}

// toggleDiscard marks the selected Capture wrong, or takes the mark away. The
// mark is written into the Capture's record at once, so it is there when
// Archive is opened, in this session or a later one.
func (m routeModel) toggleDiscard() routeModel {
	entry, ok := m.selectedCapture()
	if !ok {
		return m
	}
	discarded := !entry.record.Discarded()
	_, _, organizedAt := captureGroups(m.entries)
	row := m.captures.Cursor()
	previous := entry.record.Discard
	record, err := organizer.SetDiscard(entry.path, discarded, discardNow())
	if err != nil {
		m.notice = failed(entry.name + ": " + err.Error())
		return m
	}
	m.setRecord(entry.path, record)
	m.history = append(m.history, routeStep{path: entry.path, name: entry.name, marked: true, previous: previous})
	if discarded {
		m.notice = moveNotice{text: "✕ " + entry.name + " marked to discard — Archive will reject, not archive, it"}
	} else {
		m.notice = moveNotice{text: entry.name + " no longer marked to discard"}
	}
	m.refresh()
	m.settleAfterDiscard(entry, discarded, row-organizedAt)
	return m
}

// settleAfterDiscard puts the cursor where the reader goes on from, once a
// mark has moved the Capture into another group. Marking one takes it out of
// what the reader is walking through, so the cursor stays there: from the work
// it moves on as a run does, to the top of what is left; from the organized
// ones, to the Capture that took its place. Taking a mark back is done to work
// on the Capture, so the cursor goes with it.
func (m *routeModel) settleAfterDiscard(entry captureEntry, discarded bool, organizedRow int) {
	if !discarded {
		return
	}
	if !entry.organized {
		m.advanceToNextPending(entry)
		m.refresh()
		return
	}
	ordered, _, organizedAt := captureGroups(m.entries)
	if organizedAt < len(ordered) {
		m.captures.SelectID("capture:" + ordered[organizedAt+min(max(0, organizedRow), len(ordered)-organizedAt-1)].path)
		m.refresh()
	}
}

// routeStep is one change `u` takes back: a discard mark set or taken back, or
// a Selection cleared. A run is not one: what it wrote into notes stays.
type routeStep struct {
	path, name string
	// marked is a discard mark changed; previous is the mark it replaced,
	// nil when there was none.
	marked   bool
	previous *organizer.Discard
	// cleared is the Selection `c` dropped.
	cleared *organizer.Selection
}

func (m *routeModel) setRecord(path string, record organizer.Record) {
	for index := range m.entries {
		if m.entries[index].path == path {
			m.entries[index].record = record
		}
	}
}

// undo takes the last step back and puts the cursor on its Capture, wherever
// the step moved it: a mark sends a Capture under another rule, and finding it
// again is what undoing is for. A Capture no longer in the root — filed by
// Archive since — cannot be stepped back and is named.
func (m routeModel) undo() routeModel {
	if len(m.history) == 0 {
		m.notice = failed("Nothing to undo")
		return m
	}
	step := m.history[len(m.history)-1]
	m.history = m.history[:len(m.history)-1]
	present := false
	for _, entry := range m.entries {
		present = present || entry.path == step.path
	}
	if !present {
		m.notice = failed(step.name + " is no longer in the Capture root")
		return m
	}
	switch {
	case step.marked:
		at := discardNow()
		if step.previous != nil {
			if parsed, err := time.Parse(time.RFC3339, step.previous.DiscardedAt); err == nil {
				at = parsed
			}
		}
		record, err := organizer.SetDiscard(step.path, step.previous != nil, at)
		if err != nil {
			m.notice = failed(step.name + ": " + err.Error())
			return m
		}
		m.setRecord(step.path, record)
		if step.previous != nil {
			m.notice = moveNotice{text: "↶ " + step.name + " marked to discard again"}
		} else {
			m.notice = moveNotice{text: "↶ " + step.name + " no longer marked to discard"}
		}
	case step.cleared != nil:
		m.selections[step.path] = *step.cleared
		m.notice = moveNotice{text: "↶ " + step.name + " has its Selection back"}
	}
	m.refresh()
	m.captures.SelectID("capture:" + step.path)
	m.fields.Set(routeCapturesField)
	m.refresh()
	return m
}

// undoHint offers `u` only when there is a step to take back.
func (m routeModel) undoHint() string {
	if len(m.history) > 0 {
		return "  u Undo"
	}
	return ""
}
