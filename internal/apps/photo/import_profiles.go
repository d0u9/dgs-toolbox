package photo

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/tui/form"
)

// Import profiles on Setup: each fills Source and Destinations at once, and
// presets Parameters after the scan. A path changed by hand makes the import
// Manual; nothing chosen here is written back to the configuration.

// withProfiles lists the configured profiles first on Setup and puts the
// cursor on the first of them.
func (m importModel) withProfiles(global config.Config) importModel {
	m.profileNames, m.profiles = global.PhotoImportProfileNames(), global.Photo.Import.Profiles
	m.checkProfiles()
	if len(m.profileNames) > 0 {
		m.profileFocus = true
		m.chooseProfile(0)
	}
	return m
}

// profilePaths are a profile's source and destinations as Setup shows them.
func profilePaths(profile config.PhotoImportProfile) []string {
	paths := []string{displayPath(expandHome(profile.Source))}
	for _, root := range profile.Dest {
		paths = append(paths, displayPath(expandHome(root)))
	}
	return paths
}

// missingPath is the first of a profile's directories that is not there, as
// when its card is not inserted; "" when all are.
func missingPath(profile config.PhotoImportProfile) string {
	for _, path := range profilePaths(profile) {
		if info, err := os.Stat(expandHome(path)); err != nil || !info.IsDir() {
			return path
		}
	}
	return ""
}

// checkProfiles notes which profiles miss a directory. View only reads the
// result: a stat of a hung network mount there would freeze every redraw.
func (m *importModel) checkProfiles() {
	m.profileMissing = map[string]string{}
	for _, name := range m.profileNames {
		if missing := missingPath(m.profiles[name]); missing != "" {
			m.profileMissing[name] = missing
		}
	}
}

// chooseProfile moves the cursor; a profile fills Directories, Manual leaves
// them as they are.
func (m *importModel) chooseProfile(index int) {
	m.profileIndex = index
	if index >= len(m.profileNames) {
		return
	}
	paths := profilePaths(m.profiles[m.profileNames[index]])
	m.paths[sourceField] = paths[0]
	m.paths[destinationField] = paths[1]
	m.extraDestinations = append([]string(nil), paths[2:]...)
	m.selectedTarget = 0
	m.rebuildDirectoryControls()
}

// matchingProfile is the chosen profile when Directories still hold its
// paths, else "": a path changed by hand makes the import Manual.
func (m importModel) matchingProfile() string {
	if m.profileIndex >= len(m.profileNames) {
		return ""
	}
	name := m.profileNames[m.profileIndex]
	current := append([]string{m.paths[sourceField]}, m.destinationRoots()...)
	if !slices.Equal(current, profilePaths(m.profiles[name])) {
		return ""
	}
	return name
}

// profile is the one the scan ran with, if any.
func (m importModel) profile() (config.PhotoImportProfile, bool) {
	profile, ok := m.profiles[m.activeProfile]
	return profile, ok && m.activeProfile != ""
}

// applyProfileParameters presets Parameters from the profile the scan ran
// with; the user can still change every one of them.
func (m *importModel) applyProfileParameters() {
	profile, ok := m.profile()
	if !ok {
		return
	}
	if len(profile.Ext) > 0 {
		var chosen []string
		for _, extension := range m.sourceExtensions() {
			for _, wanted := range profile.Ext {
				if strings.EqualFold(strings.TrimPrefix(wanted, "."), extension) {
					chosen = append(chosen, extension)
				}
			}
		}
		m.controls.SetValues(extensionsID, chosen)
	}
	if profile.Duplicates == "keep-both" {
		m.controls.SetValue(duplicatesID, "Keep both")
	}
	if profile.Workers > 0 {
		m.controls.SetValue(parallelID, strconv.Itoa(profile.Workers))
	}
	if profile.Incremental != nil {
		m.controls.SetChecked(incrementalID, *profile.Incremental)
	}
	m.controls.SetChecked(checkRenameID, profile.AllowCheckRename)
}

// updateProfileKey handles Setup's keys while the cursor is in Profiles, and
// Shift+Tab from Source back into them.
func (m importModel) updateProfileKey(key string) (tea.Model, tea.Cmd, bool) {
	if len(m.profileNames) == 0 || m.picking {
		return m, nil, false
	}
	if !m.profileFocus {
		if key == "shift+tab" && m.controls.Focused(sourceID) {
			m.profileFocus = true
			return m, nil, true
		}
		return m, nil, false
	}
	switch key {
	case "up", "k":
		m.chooseProfile(max(0, m.profileIndex-1))
	case "down", "j":
		m.chooseProfile(min(len(m.profileNames), m.profileIndex+1))
	case "tab":
		m.profileFocus = false
		m.controls.SetFocusID(sourceID)
	case "enter", form.PrimaryActionKey:
		if m.profileIndex >= len(m.profileNames) {
			m.profileFocus = false
			m.controls.SetFocusID(sourceID)
			return m, nil, true
		}
		// Checked again now: a card inserted since Setup opened counts.
		m.checkProfiles()
		if missing := m.profileMissing[m.profileNames[m.profileIndex]]; missing != "" {
			m.actionNotice = missing + " is not there"
			return m, nil, true
		}
		m.profileFocus = false
		next, cmd := m.startScan()
		return next, cmd, true
	default:
		return m, nil, false
	}
	m.actionNotice = ""
	return m, nil, true
}

// profilesView lists the profiles and Manual, one row each.
func (m importModel) profilesView(width int) string {
	rows := make([]string, 0, len(m.profileNames)+1)
	nameWidth := len("Manual")
	for _, name := range m.profileNames {
		nameWidth = max(nameWidth, lipgloss.Width(name))
	}
	row := func(index int, name, detail string, dim bool) string {
		cursor := "  "
		if index == m.profileIndex {
			cursor = "› "
		}
		line := cursor + name + strings.Repeat(" ", nameWidth-lipgloss.Width(name)+2) + detail
		line = lipgloss.NewStyle().MaxWidth(width).Render(line)
		switch {
		case index == m.profileIndex && m.profileFocus:
			return importTitleStyle.Render(line)
		case dim:
			return importNoteStyle.Render(line)
		}
		return line
	}
	// A long path loses its start, never the end that names the folder.
	room := max(8, width-nameWidth-4)
	for i, name := range m.profileNames {
		profile := m.profiles[name]
		dest := " → " + destinationsLabel(profile.Dest)
		detail := tailFit(displayPath(expandHome(profile.Source)), room-lipgloss.Width(dest)) + dest
		missing := m.profileMissing[name]
		if missing != "" {
			detail = "not there: " + tailFit(missing, room-len("not there: "))
		}
		rows = append(rows, row(i, name, detail, missing != ""))
	}
	rows = append(rows, row(len(m.profileNames), "Manual", "choose directories below", false))
	if m.actionNotice != "" && m.profileFocus {
		rows = append(rows, importNoteStyle.Render(tailFit(m.actionNotice, width)))
	}
	return "\n" + strings.Join(rows, "\n") + "\n"
}

// tailFit keeps the end of text within width cells, marking a cut with "…".
func tailFit(text string, width int) string {
	if lipgloss.Width(text) <= width {
		return text
	}
	runes := []rune(text)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[1:]
	}
	return "…" + string(runes)
}

func destinationsLabel(dest []string) string {
	if len(dest) == 1 {
		return tailFit(displayPath(expandHome(dest[0])), 24)
	}
	return fmt.Sprintf("%d destinations", len(dest))
}
