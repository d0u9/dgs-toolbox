package capture

import (
	"fmt"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/desktop"

	"github.com/charmbracelet/x/ansi"
)

// Checking a Capture before it is written somewhere: where its position lands
// on a map, and marking one found to be wrong so Archive sets it aside.

// openURL hands a map to the operating system. It is a variable so a test
// records what would have opened instead of opening it.
var openURL = desktop.Open

// flagNow is when a mark is made, a variable so a test can name it.
var flagNow = time.Now

// routeMapServices are the links the map row carries, in the order written.
const routeMapServices = "Apple, Google, 高德"

// positionInUse is the position a map is offered for: the Capture's resolved
// position, when the Recipe chosen for it has an enabled Action that writes it.
// A position nothing will write is not worth checking.
func (m routeModel) positionInUse() (organizer.Position, bool) {
	selection, recipe, ok := m.currentSelection()
	if !ok || !organizer.UsesPosition(recipe, selection.EnabledActions(recipe)) {
		return organizer.Position{}, false
	}
	ctx, ok := m.context()
	if !ok {
		return organizer.Position{}, false
	}
	return ctx.Position()
}

// mapHint offers the map key only when there is a position in use to open.
func (m routeModel) mapHint() string {
	if _, ok := m.positionInUse(); ok {
		return "  o Map"
	}
	return ""
}

// openMap opens the position in use in Apple Maps, which is the map this
// machine opens without a browser.
func (m routeModel) openMap() routeModel {
	position, ok := m.positionInUse()
	if !ok {
		m.notice = failed("No position in use — choose a recipe that writes one")
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

func mapLinks(position organizer.Position, label, services string) []organizer.MapLink {
	return organizer.MapLinks(
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

// mapLinksText is the map row's value: each service's short name, linked to
// the position, so a terminal that follows links opens it with a click.
func mapLinksText(position organizer.Position, label string) string {
	links := mapLinks(position, label, routeMapServices)
	parts := make([]string, 0, len(links))
	for _, link := range links {
		parts = append(parts, ansi.SetHyperlink(link.URL)+link.Short+ansi.ResetHyperlink())
	}
	return strings.Join(parts, " · ")
}

// toggleFlag marks the selected Capture wrong, or takes the mark away. The
// mark is written into the Capture's record at once, so it is there when
// Archive is opened, in this session or a later one.
func (m routeModel) toggleFlag() routeModel {
	entry, ok := m.selectedCapture()
	if !ok {
		return m
	}
	flagged := !entry.record.Flagged()
	record, err := organizer.SetFlag(entry.path, flagged, flagNow())
	if err != nil {
		m.notice = failed(entry.name + ": " + err.Error())
		return m
	}
	for index := range m.entries {
		if m.entries[index].path == entry.path {
			m.entries[index].record = record
		}
	}
	if flagged {
		m.notice = moveNotice{text: "⚑ " + entry.name + " flagged — Archive will reject, not archive, it"}
	} else {
		m.notice = moveNotice{text: entry.name + " unflagged"}
	}
	m.refresh()
	return m
}
