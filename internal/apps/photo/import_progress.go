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
// with a batch summary and one row per whole-file worker, without a Tea program.
type importProgress struct {
	out                    io.Writer
	fd                     uintptr
	enabled                bool
	rows                   int
	target, targets, total int
	finished               map[int]bool
	slots                  []importer.Event
	destinations           []string
	summaries              []string
	lastDraw               time.Time
}

func newImportProgress(out io.Writer, targets, total, workers int) *importProgress {
	p := &importProgress{out: out, targets: targets, total: total, target: -1, slots: make([]importer.Event, min(workers, total))}
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
	changed := target != p.target
	if changed {
		p.target, p.finished = target, make(map[int]bool)
		clear(p.slots)
	}
	if finalPhase(event.Phase) {
		p.finished[event.Index] = true
	}
	if event.Worker > 0 && event.Worker <= len(p.slots) {
		slot := &p.slots[event.Worker-1]
		// The coordinator can deliver completion after this worker starts its next
		// file. Count it, but never replace the newer file's progress with it.
		if !finalPhase(event.Phase) || slot.Phase == "" || slot.Index == event.Index {
			*slot = event
		}
	}
	now := time.Now()
	if !changed && now.Sub(p.lastDraw) < 100*time.Millisecond {
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

func (p *importProgress) frame(width, height int) []string {
	done := len(p.finished)
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
	if len(p.destinations) > 0 {
		for i := 0; i <= p.target && i < len(p.destinations); i++ {
			note := fmt.Sprintf("  Destination %d/%d: %s", i+1, p.targets, p.destinations[i])
			if i < len(p.summaries) && p.summaries[i] != "" {
				note = "  " + p.summaries[i]
			}
			note = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, ansi.Strip(note))
			footer = append(footer, fitProgressLine(note, width))
		}
		available := max(0, height-len(lines)-3)
		if len(footer) > available {
			footer = footer[len(footer)-available:]
		}
	}
	reserved := len(footer)
	if reserved > 0 {
		reserved++
	}
	limit := min(len(p.slots), max(0, height-len(lines)-reserved-1))
	// Reserve a row to report workers that do not fit on a short terminal.
	if limit < len(p.slots) && limit > 0 {
		limit--
	}
	for worker := range limit {
		event := p.slots[worker]
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
			name = strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return ' '
				}
				return r
			}, ansi.Strip(filepath.Base(event.Source)))
			if event.Source == "" {
				name = ""
			}
			if !finalPhase(event.Phase) && event.Total > 0 {
				fraction := min(1.0, max(0.0, float64(event.Bytes)/float64(event.Total)))
				detail = fmt.Sprintf("%s / %s", formatBytes(event.Bytes), formatBytes(event.Total))
				if width >= 80 {
					detail = progressBar(fraction, min(32, max(12, width-80))) + "  " + detail
				}
			}
		}
		nameWidth := min(28, max(8, width-64))
		name = ansi.Truncate(name, nameWidth, "…")
		name += strings.Repeat(" ", max(0, nameWidth-ansi.StringWidth(name)))
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
		line := fmt.Sprintf("  %s #%d  %s  %-13s  %s", indicator, worker+1, name, phase, detail)
		lines = append(lines, fitProgressLine(line, width))
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
