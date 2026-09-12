package capture

import (
	"os"
	"strings"

	"dgs-toolbox/internal/apps/capture/archive"
	"dgs-toolbox/internal/apps/capture/organizer"
	"dgs-toolbox/internal/tui/datafield"
	"dgs-toolbox/internal/tui/scrolllist"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// captureView is how a Capture is named in a list. A Capture is identified by
// its index — when it was taken, what produced it — because a generated folder
// name such as `aaaa-xxxxx` says nothing about what is in it. But that name is
// what every other tool on the machine calls it, so a reader comparing a
// session with a Finder window, a backup or a terminal needs the other view as
// well.
//
// The two sessions that list Captures for organizing share this, so a Capture
// is named the same way in both and switching a view means the same thing in
// each.
type captureView struct {
	byFolder bool
}

// Toggle swaps the view.
func (v *captureView) Toggle() { v.byFolder = !v.byFolder }

// Label is the leading line of a row: what the reader is looking for first.
func (v captureView) Label(entry captureEntry) string {
	if v.byFolder {
		return entry.name + string(os.PathSeparator)
	}
	return routeCaptureLabel(entry)
}

// Detail is the muted line under it. The two views swap which fact leads and
// neither drops the other, because a row answering only one of the two
// questions would send the reader across for every Capture.
func (v captureView) Detail(entry captureEntry) string {
	if v.byFolder {
		if created := organizer.FormatTimestamp(entry.index.CreatedAt); created != "" {
			return created
		}
		return routeCaptureDetail(entry)
	}
	return routeCaptureDetail(entry)
}

// Name is the view the other one is not, for a pane that has room for both.
func (v captureView) Name() string {
	if v.byFolder {
		return "folder"
	}
	return "index"
}

// routeCaptureLabel identifies a Capture by what organizing decisions are made
// on—when it was taken—rather than by its directory name, which carries no
// meaning for the operator. The folder path remains available in the status bar.
func routeCaptureLabel(entry captureEntry) string {
	if created := organizer.FormatTimestamp(entry.index.CreatedAt); created != "" {
		return created
	}
	return entry.name + string(os.PathSeparator)
}

// routeCaptureDetail is the second row of a Route Capture: what produced the
// Capture. Together the two rows carry more identity than one row of a quarter
// column can hold without truncating the timestamp.
func routeCaptureDetail(entry captureEntry) string {
	parts := make([]string, 0, 2)
	if app := entry.index.Source.App; app != "" {
		parts = append(parts, app)
	}
	if workflow := entry.index.Source.Workflow; workflow != "" {
		parts = append(parts, workflow)
	}
	return strings.Join(parts, " · ")
}

// indentDetail lines a row's second line up under the first, past the number
// the list draws in front of it.
func indentDetail(detail string) string {
	if detail == "" {
		return ""
	}
	return "      " + detail
}

// orderByOrganized puts the Captures still to organize first and the ones
// already handled after them, each run keeping the load order. Route and
// Archive both list the Capture root this way — the rule through the column is
// the same rule — so a Capture does not sit in one place in one session and
// somewhere else in the other.
func orderByOrganized(entries []captureEntry) (ordered []captureEntry, boundary int) {
	pending := make([]captureEntry, 0, len(entries))
	done := make([]captureEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.organized {
			done = append(done, entry)
			continue
		}
		pending = append(pending, entry)
	}
	return append(pending, done...), len(pending)
}

// listMoveKey is the movement every scrolling list in Capture answers: the
// arrows and their Vim letters, the ends, and `g g`. It is one function because
// a list that moved differently from the list beside it would be a list the
// reader has to learn twice. pans says whether this list has anything to pan
// across; a list of short labels does not, and `h`/`l` mean nothing there.
//
// It reports whether the key was movement, so a caller can go on to the keys
// that are its own.
func listMoveKey(list *scrolllist.Model, key string, pendingGG *bool, pans bool) bool {
	switch key {
	case "up", "k":
		list.Move(-1)
	case "down", "j":
		list.Move(1)
	case "left", "h":
		if !pans {
			return false
		}
		list.Pan(-4)
	case "right", "l":
		if !pans {
			return false
		}
		list.Pan(4)
	case "G", "end":
		list.Last()
	case "g":
		// The second g is what jumps; the first only waits for it.
		if *pendingGG {
			list.First()
			*pendingGG = false
			return true
		}
		*pendingGG = true
		return true
	default:
		return false
	}
	*pendingGG = false
	return true
}

// cycleFieldOrder walks a Tab ring of DataFields. Focus can sit in a field that
// has since left the ring — clearing a Selection empties ACTIONS under the
// cursor — so a field that is not in the order walks from the start rather than
// from nowhere.
func cycleFieldOrder(fields *datafield.Navigator, order []string, forward bool) {
	if len(order) == 0 {
		return
	}
	current, found := 0, false
	for index, id := range order {
		if id == fields.Current() {
			current, found = index, true
		}
	}
	if !found {
		fields.Set(order[0])
		return
	}
	step := 1
	if !forward {
		step = -1
	}
	fields.Set(order[(current+step+len(order))%len(order)])
}

// moveNotice is what became of a move, said where it was asked for. Scan and
// Archive both move Captures and both report it the same way: a line in the
// status bar until the next keystroke, rather than a dialog, because the move
// has already happened and is taken back rather than confirmed.
type moveNotice struct {
	text string
	bad  bool
}

func (n moveNotice) empty() bool { return n.text == "" }

// status is the line as the status bar shows it.
func (n moveNotice) status() string {
	if n.bad {
		return "! " + n.text
	}
	return n.text
}

func moved(name, destination string) moveNotice {
	return moveNotice{text: name + " → " + destination}
}

func failed(text string) moveNotice { return moveNotice{text: text, bad: true} }

// unsetFolderNotice is the same refusal in both sessions: the key to write,
// rather than the fact that something is missing.
func unsetFolderNotice(destination archiveDestination) moveNotice {
	return failed("No " + folderName(destination) + " folder configured — set " + folderKey(destination))
}

// moveCaptureTo moves one Capture directory into a folder and returns it as it
// now stands, with what to say about it. The move itself belongs to the archive
// package; this is the part Scan and Archive would otherwise each write.
func moveCaptureTo(entry captureEntry, folder, destination string) (captureEntry, moveNotice, bool) {
	path, err := archive.Move(entry.path, folder)
	if err != nil {
		return entry, failed(entry.name + ": " + err.Error()), false
	}
	// The Capture is the same Capture at a new path, so it is carried across
	// rather than re-read: its index and its organizing record did not change
	// by being moved.
	relocated := entry
	relocated.path = path
	relocated.name = baseName(path)
	return relocated, moved(entry.name, destination), true
}

func baseName(path string) string {
	trimmed := strings.TrimRight(path, string(os.PathSeparator))
	if index := strings.LastIndex(trimmed, string(os.PathSeparator)); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

// pageActionRow lays the controls a session acts through under its rightmost
// column: a blank row, the buttons right-aligned side by side, and a row below
// them, so a control is not pressed into the corner of the screen. Route has
// one of these and Archive two, and they are drawn by one function so the two
// screens put them in the same place.
func pageActionRow(width int, buttons [][]string) string {
	blank := strings.Repeat(" ", max(0, width))
	lines := []string{blank}
	for row := 0; row < 2; row++ {
		parts := make([]string, 0, len(buttons))
		for _, button := range buttons {
			parts = append(parts, button[row])
		}
		line := strings.Join(parts, " ")
		indent := strings.Repeat(" ", max(0, width-lipgloss.Width(line)-1))
		lines = append(lines, indent+line+" ")
	}
	return strings.Join(lines, "\n") + "\n" + blank
}

// reloadAfterMove re-reads what a move changed: the shared Capture root every
// session lists, and the destination folder Archive lists.
func reloadAfterMove(root, indexFile string, destination archiveDestination, folder string) tea.Cmd {
	return tea.Batch(loadCaptures(root, indexFile), loadArchiveFolder(destination, folder, indexFile))
}
