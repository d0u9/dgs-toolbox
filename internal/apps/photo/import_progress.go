package photo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"dgs-toolbox/internal/apps/photo/importer"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
)

// Engine hooks are serialized. The display owns one fixed terminal region,
// with a batch summary and, per whole-file worker, one row per destination,
// without a Tea program. Destinations run at once: a worker's file is copied
// into all of them from one source read.
type importProgress struct {
	out            io.Writer
	fd             uintptr
	enabled        bool
	rows           int
	targets, total int
	// finished counts, per file, the destinations that have finished it.
	finished     map[int]int
	slots        [][]importer.Event
	destinations []string
	summaries    []string
	lastDraw     time.Time
}

func newImportProgress(out io.Writer, targets, total, workers int) *importProgress {
	slots := make([][]importer.Event, min(workers, total))
	for i := range slots {
		slots[i] = make([]importer.Event, max(targets, 1))
	}
	p := &importProgress{out: out, targets: targets, total: total, finished: make(map[int]int), slots: slots}
	if file, ok := out.(interface{ Fd() uintptr }); ok {
		p.fd = file.Fd()
		p.enabled = term.IsTerminal(p.fd) && os.Getenv("TERM") != "dumb"
	}
	return p
}

func (p *importProgress) Write(data []byte) (int, error) {
	p.clear()
	return p.out.Write(data)
}

// Leave the cursor at the region's first row, ready for the next frame or log.
func (p *importProgress) clear() {
	for p.rows > 0 {
		fmt.Fprint(p.out, "\r\x1b[2K")
		p.rows--
		if p.rows > 0 {
			fmt.Fprint(p.out, "\x1b[1A")
		}
	}
}

func (p *importProgress) update(target int, event importer.Event) {
	if !p.enabled {
		return
	}
	if finalPhase(event.Phase) {
		p.finished[event.Index]++
	}
	if event.Worker > 0 && event.Worker <= len(p.slots) && target >= 0 && target < len(p.slots[event.Worker-1]) {
		slot := &p.slots[event.Worker-1][target]
		// The coordinator can deliver completion after this worker starts its next
		// file. Count it, but never replace the newer file's progress with it.
		if !finalPhase(event.Phase) || slot.Phase == "" || slot.Index == event.Index {
			*slot = event
		}
	}
	now := time.Now()
	if now.Sub(p.lastDraw) < 100*time.Millisecond {
		return
	}
	p.lastDraw = now
	width, height, err := term.GetSize(p.fd)
	if err != nil || width < 1 {
		width, height = 80, 24
	}
	lines := p.frame(width, height)
	p.clear()
	fmt.Fprint(p.out, strings.Join(lines, "\r\n"))
	p.rows = len(lines)
}

// done counts the files every destination has finished.
func (p *importProgress) done() int {
	count := 0
	for _, targets := range p.finished {
		if targets >= max(p.targets, 1) {
			count++
		}
	}
	return count
}

func plainName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(name))
}

func (p *importProgress) frame(width, height int) []string {
	done := p.done()
	fraction := 0.0
	if p.total > 0 {
		fraction = float64(done) / float64(p.total)
	}
	label := fmt.Sprintf("  Progress  %d/%d files", done, p.total)
	if width >= 60 {
		label += fmt.Sprintf(" %s %3.0f%%", progressBar(fraction, min(32, max(16, width-42))), fraction*100)
	}
	lines := []string{fitProgressLine(label, width)}
	if height >= 4 {
		lines = append(lines, "")
	}
	var footer []string
	for i, destination := range p.destinations {
		note := fmt.Sprintf("  Destination %d/%d: %s", i+1, p.targets, destination)
		if i < len(p.summaries) && p.summaries[i] != "" {
			note = "  " + p.summaries[i]
		}
		footer = append(footer, fitProgressLine(plainName(note), width))
	}
	if available := max(0, height-len(lines)-3); len(footer) > available {
		footer = footer[len(footer)-available:]
	}
	reserved := len(footer)
	if reserved > 0 {
		reserved++
	}
	perWorker := 1
	if len(p.slots) > 0 {
		perWorker = len(p.slots[0])
	}
	rows := max(0, height-len(lines)-reserved-1)
	limit := min(len(p.slots), rows/perWorker)
	// Reserve a row to report workers that do not fit on a short terminal.
	if limit < len(p.slots) && limit > 0 && rows-limit*perWorker < 1 {
		limit--
	}
	nameWidth := min(28, max(8, width-64))
	for worker := range limit {
		for target, event := range p.slots[worker] {
			lines = append(lines, fitProgressLine(p.row(worker, target, event, nameWidth, width), width))
		}
	}
	if limit < len(p.slots) && height > 2 {
		lines = append(lines, fitProgressLine(fmt.Sprintf("  %d more workers (terminal too short)", len(p.slots)-limit), width))
	}
	if len(footer) > 0 {
		lines = append(lines, "")
		lines = append(lines, footer...)
	}
	return lines
}

// row is one destination's line of a worker. The first names the worker and
// its file; the others leave those columns blank under it.
func (p *importProgress) row(worker, target int, event importer.Event, nameWidth, width int) string {
	phase, name, detail := "waiting", "", ""
	if event.Phase != "" {
		phase = string(event.Phase)
		switch event.Phase {
		case importer.PhaseCheckingSource:
			phase = "check source"
		case importer.PhaseCheckingDestination:
			phase = "check copy"
		case importer.PhaseAlreadyPresent:
			phase = "present"
		case importer.PhaseDeletingSource:
			phase = "delete source"
		case importer.PhaseSourceKept:
			phase = "source kept"
		}
		if event.Source != "" {
			name = plainName(filepath.Base(event.Source))
		}
		if !finalPhase(event.Phase) && event.Total > 0 {
			fraction := min(1.0, max(0.0, float64(event.Bytes)/float64(event.Total)))
			detail = fmt.Sprintf("%s / %s", formatBytes(event.Bytes), formatBytes(event.Total))
			if width >= 80 {
				detail = progressBar(fraction, min(32, max(12, width-84))) + "  " + detail
			}
		}
	}
	indicator := "·"
	if event.Phase != "" {
		indicator = string([]rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")[(time.Now().UnixMilli()/100+int64(worker))%10])
		if finalPhase(event.Phase) {
			indicator = "!"
			if imported(event.Phase) && event.Phase != importer.PhaseSourceKept {
				indicator = "✓"
			}
		}
	}
	label := fmt.Sprintf("#%d", worker+1)
	if target > 0 {
		label = strings.Repeat(" ", len(label))
		if first := p.slots[worker][0]; first.Source != "" && first.Source == event.Source {
			name = ""
		}
	}
	name = ansi.Truncate(name, nameWidth, "…")
	name += strings.Repeat(" ", max(0, nameWidth-ansi.StringWidth(name)))
	destination := ""
	if p.targets > 1 {
		destination = fmt.Sprintf("%d  ", target+1)
	}
	return fmt.Sprintf("  %s %s  %s  %s%-13s  %s", indicator, label, name, destination, phase, detail)
}

func progressBar(fraction float64, width int) string {
	filled := min(width, max(0, int(fraction*float64(width))))
	bar := "[" + strings.Repeat("⣿", filled) + strings.Repeat("⣀", width-filled) + "]"
	if _, disabled := os.LookupEnv("NO_COLOR"); disabled {
		return bar
	}
	return "\x1b[38;2;132;169;140m" + bar + "\x1b[0m"
}

func fitProgressLine(line string, width int) string {
	return ansi.Truncate(line, max(0, width-1), "…")
}
