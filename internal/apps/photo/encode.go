package photo

import (
	"context"
	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/photoencode"
	"dgs-toolbox/internal/tui"
	"dgs-toolbox/internal/tui/confirm"
	"dgs-toolbox/internal/tui/fieldset"
	"dgs-toolbox/internal/tui/fileexplorer"
	"dgs-toolbox/internal/tui/form"
	"dgs-toolbox/internal/tui/overlay"
	"dgs-toolbox/internal/tui/pageactions"
	"fmt"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"strconv"
	"strings"
	"sync/atomic"
)

var encodeSession atomic.Uint64

var encodeIDs = []string{"input", "output", "quality", "size", "ppi", "background", "recursive", "gps"}

type encodePlanMsg struct {
	session uint64
	jobs    []photoencode.Job
	err     error
}
type encodeResultMsg struct {
	session uint64
	result  photoencode.Outcome
}
type encodeModel struct {
	session       uint64
	finished      chan struct{}
	controls      form.Model
	settings      config.PhotoEncode
	width, height int
	picker        fileexplorer.Model
	picking       string
	stage         string
	notice        string
	fieldErrors   map[string]string
	jobs          []photoencode.Job
	results       []photoencode.Outcome
	index         int
	cancel        context.CancelFunc
	ctx           context.Context
	report        viewport.Model
	stopping      bool
	confirming    bool
	confirmation  confirm.Model
}

func newEncodeModel(c config.PhotoEncode) encodeModel {
	m := encodeModel{session: encodeSession.Add(1), settings: c, stage: "parameters", width: 100, height: 30, report: viewport.New(92, 15), controls: form.New(
		form.Field{ID: "input", Kind: form.Path, Label: "Source", Value: c.Source}, form.Field{ID: "output", Kind: form.Path, Label: "Destination", Value: c.Destination},
		form.Field{ID: "quality", Kind: form.Number, Label: "JPEG quality", Value: strconv.Itoa(c.Quality), Min: 1, Max: 100, Step: 1},
		form.Field{ID: "size", Kind: form.Number, Label: "Longest edge", Value: strconv.Itoa(c.MaxSide), Min: 1, Max: 65535, Step: 1},
		form.Field{ID: "ppi", Kind: form.Number, Label: "PPI", Value: strconv.Itoa(c.PPI), Min: 1, Max: 65535, Step: 1},
		form.Field{ID: "background", Kind: form.Text, Label: "Background", Value: c.Background},
		form.Field{ID: "recursive", Kind: form.Checkbox, Label: "Include children", Checked: c.Recursive}, form.Field{ID: "gps", Kind: form.Checkbox, Label: "Preserve GPS", Checked: c.PreserveGPS})}
	m.report.SetHorizontalStep(8)
	return m
}
func (m encodeModel) Init() tea.Cmd { return nil }
func (m encodeModel) options() photoencode.Options {
	o := m.settings.Options()
	o.Quality = m.controls.IntValue("quality")
	o.Size = m.controls.IntValue("size")
	o.PPI = m.controls.IntValue("ppi")
	o.Background = m.controls.Value("background")
	o.Recursive = m.controls.Checked("recursive")
	o.PreserveGPS = m.controls.Checked("gps")
	return o
}
func (m encodeModel) CapturesShellKey(key string) bool {
	return m.picking != "" || m.controls.IsActive() || m.confirming || (m.stage == "processing" && (key == "esc" || key == "q" || key == "ctrl+c"))
}
func (m encodeModel) Close() {
	if m.cancel != nil {
		m.cancel()
		if m.finished != nil {
			<-m.finished
		}
	}
}
func (m encodeModel) CommandPath() []string { return []string{m.stage} }
func (m *encodeModel) runNext() tea.Cmd {
	job, o, ctx, session := m.jobs[m.index], m.options(), m.ctx, m.session
	finished := make(chan struct{})
	m.finished = finished
	return func() tea.Msg {
		defer close(finished)
		return encodeResultMsg{session: session, result: photoencode.Encode(ctx, job, o)}
	}
}
func (m *encodeModel) refreshReport() {
	var lines []string
	if m.stage == "plan" {
		for _, j := range m.jobs {
			line := j.Source + "\n  → " + j.Destination
			if j.Problem != "" {
				line += "\n  ! " + j.Problem
			}
			lines = append(lines, line)
		}
	} else {
		for _, r := range m.results {
			if r.Published {
				lines = append(lines, fmt.Sprintf("✓ %s\n  %d × %d · %d → %d bytes", r.Job.Destination, r.Width, r.Height, r.BytesBefore, r.BytesAfter))
			} else {
				lines = append(lines, "! "+r.Job.Source+"\n  "+r.Error)
			}
		}
	}
	m.report.SetContent(strings.Join(lines, "\n\n"))
}
func (m encodeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.report.Width = max(1, min(96, v.Width-8))
		m.report.Height = max(1, v.Height-11)
		if m.picking != "" {
			m.picker.SetSize(max(20, min(88, v.Width-8)), max(8, v.Height-8))
		}
		return m, nil
	case encodePlanMsg:
		if v.session != m.session {
			return m, nil
		}
		if v.err != nil {
			m.notice = v.err.Error()
			m.fieldErrors = map[string]string{}
			for _, pair := range [][2]string{{"quality", "quality"}, {"size", "size"}, {"ppi", "ppi"}, {"background", "background"}} {
				if strings.HasPrefix(m.notice, pair[0]) {
					m.fieldErrors[pair[1]] = m.notice
				}
			}
			if len(m.fieldErrors) == 0 {
				id := "input"
				if strings.Contains(m.notice, m.controls.Value("output")) && m.controls.Value("output") != "" {
					id = "output"
				}
				m.fieldErrors[id] = m.notice
			}
			for _, id := range []string{"input", "output"} {
				if m.controls.Value(id) == "" {
					m.fieldErrors[id] = "Required"
				}
			}
			m.stage = "parameters"
		} else {
			m.jobs = v.jobs
			m.stage = "plan"
			m.notice = ""
			m.refreshReport()
			m.report.GotoTop()
		}
		return m, nil
	case encodeResultMsg:
		if v.session != m.session {
			return m, nil
		}
		m.results = append(m.results, v.result)
		m.index++
		m.refreshReport()
		m.report.GotoBottom()
		if m.stopping || m.index == len(m.jobs) {
			m.stage = "results"
			if m.stopping {
				m.notice = fmt.Sprintf("Stopped · %d jobs not started", len(m.jobs)-m.index)
			}
			m.confirming = false
			m.cancel()
			return m, nil
		}
		cmd := m.runNext()
		return m, cmd
	}
	if m.picking != "" {
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" && !m.picker.HasDialog() {
			m.picking = ""
			return m, nil
		}
		var selected string
		var cmd tea.Cmd
		if mouse, ok := msg.(tea.MouseMsg); ok {
			pw := min(92, m.width-4)
			ph := lipgloss.Height(fieldset.View("FILE EXPLORER", m.picker.View()+"\n"+pageactions.Footer(min(88, m.width-8), "Enter select · Esc cancel"), pw))
			mouse.X -= (m.width-pw)/2 + 2
			mouse.Y -= (m.height-ph)/2 + 1
			msg = mouse
		}
		m.picker, selected, cmd = m.picker.Update(msg)
		if selected != "" {
			m.controls.SetValue(m.picking, selected)
			m.picking = ""
		}
		return m, cmd
	}
	if mouse, ok := msg.(tea.MouseMsg); ok {
		if m.confirming {
			dialog := m.confirmation.ViewSize(m.width, m.height)
			dw, dh := lipgloss.Width(dialog), lipgloss.Height(dialog)
			x, y := mouse.X-(m.width-dw)/2, mouse.Y-(m.height-dh)/2
			if mouse.Button == tea.MouseButtonLeft && mouse.Action == tea.MouseActionPress && y == dh-2 {
				line := ansi.Strip(strings.Split(dialog, "\n")[dh-2])
				safe, stop := strings.Index(line, "Continue"), strings.Index(line, "Stop")
				if safe >= 0 && x >= safe-4 && x < safe+len("Continue")+3 {
					return m.Update(tea.KeyMsg{Type: tea.KeyEsc})
				}
				if stop >= 0 && x >= stop-4 && x < stop+len("Stop")+3 {
					if m.confirmation.CancelChosen() {
						m.confirmation, _ = m.confirmation.Update("tab")
					}
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
			}
			return m, nil
		}
		if m.picking != "" {
			return m, nil
		}
		if mouse.Button == tea.MouseButtonLeft && mouse.Action == tea.MouseActionPress {
			w := min(tui.DefaultContentWidth, m.width-4)
			x := mouse.X - (m.width-w)/2
			rows := lipgloss.Height(fieldset.View("JPEG ENCODE", m.parameterContent(w-4)+"\n\nJPEG / PNG / HEIC / TIFF → JPEG · opaque background\nsRGB · no enlargement", w))
			if m.stage == "plan" || m.stage == "results" {
				rows = lipgloss.Height(fieldset.View("PLAN", m.report.View(), w))
			}
			direction := pageactions.Hit(m.pageActions(), w, x, mouse.Y-rows)
			if direction != pageactions.None {
				key := "n"
				if direction == pageactions.Prev {
					key = "esc"
				} else if m.stage == "results" {
					key = "r"
				}
				return m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
			}
			if m.stage == "parameters" {
				id, handled := m.clickParameter(x-2, mouse.Y-1, w-4)
				if handled && (id == "input" || id == "output") {
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
				return m, nil
			}
		}
	}
	if key, ok := msg.(tea.KeyMsg); ok {
		k := key.String()
		if m.confirming {
			var decision confirm.Decision
			m.confirmation, decision = m.confirmation.Update(k)
			if decision == confirm.Confirmed {
				m.cancel()
				m.stopping = true
				m.confirming = false
				m.notice = "Cancelling; waiting for current file"
			} else if decision == confirm.Cancelled {
				m.confirming = false
			}
			return m, nil
		}
		if m.stage == "processing" {
			if k == "esc" || k == "q" || k == "ctrl+c" {
				m.confirming = true
				m.confirmation = confirm.New(confirm.Config{Title: "Stop encoding?", Message: "Published files remain complete. Cancel the current file and remaining jobs?", ConfirmLabel: "Stop", CancelLabel: "Continue"})
			}
			return m, nil
		}
		if m.stage == "parameters" {
			if m.controls.IsActive() {
				m.controls.HandleInteraction(k)
				return m, nil
			}
			if k == "n" {
				m.fieldErrors = nil
				m.notice = "Planning…"
				m.stage = "planning"
				src, dst, o, session := m.controls.Value("input"), m.controls.Value("output"), m.options(), m.session
				return m, func() tea.Msg {
					jobs, err := photoencode.Plan(src, dst, o)
					return encodePlanMsg{session: session, jobs: jobs, err: err}
				}
			}
			if k == "enter" && (m.controls.FocusedID() == "input" || m.controls.FocusedID() == "output") {
				m.picking = m.controls.FocusedID()
				filter := fileexplorer.AllFiles()
				if m.picking == "output" {
					filter = fileexplorer.Directories()
				}
				m.picker = fileexplorer.New(pickerStart(m.controls.Value(m.picking)), max(20, min(88, m.width-8)), max(8, m.height-8), fileexplorer.WithFilter(filter))
				return m, m.picker.Init()
			}
			if !m.controls.UpdateNavigation(k) {
				m.controls.HandleInteraction(k)
			}
			return m, nil
		}
		if m.stage == "plan" {
			if k == "esc" {
				m.stage = "parameters"
				return m, nil
			}
			if k == "n" {
				m.ctx, m.cancel = context.WithCancel(context.Background())
				m.results = nil
				m.index = 0
				m.stage = "processing"
				m.report.SetContent("")
				cmd := m.runNext()
				return m, cmd
			}
		}
		if m.stage == "results" && k == "r" {
			m.stage = "parameters"
			m.notice = ""
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.report, cmd = m.report.Update(msg)
	return m, cmd
}
func (m encodeModel) View() string {
	if m.width < 55 || m.height < 23 {
		return "Photo Encode needs at least 55 columns × 23 rows. Resize to continue."
	}
	w := min(tui.DefaultContentWidth, m.width-4)
	var body string
	switch m.stage {
	case "parameters", "planning":
		body = fieldset.View("JPEG ENCODE", m.parameterContent(w-4)+"\n\nJPEG / PNG / HEIC / TIFF → JPEG · opaque background\nsRGB · no enlargement", w)
		body += "\n" + pageactions.View(m.pageActions(), w)
	case "plan":
		body = fieldset.View("PLAN · originals kept", m.report.View(), w) + "\n" + pageactions.View(m.pageActions(), w)
	case "processing":
		body = fieldset.View(fmt.Sprintf("ENCODING %d / %d", m.index+1, len(m.jobs)), m.report.View(), w)
	case "results":
		body = fieldset.View("RESULTS", m.report.View(), w) + "\n" + pageactions.View(m.pageActions(), w)
	}
	if m.notice != "" {
		body += "\n" + ansi.Wrap("! "+m.notice, w, "")
	}
	if lipgloss.Height(body) > m.height {
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, "Resize to show the Encode form and its validation errors.")
	}
	view := lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Top, body)
	if m.picking != "" {
		front := fieldset.View("FILE EXPLORER · "+strings.ToUpper(m.picking), m.picker.View()+"\n"+pageactions.Footer(min(88, m.width-8), "Enter select · Esc cancel"), min(92, m.width-4))
		view = overlay.Place(view, front, m.width, m.height)
	}
	if m.confirming {
		view = overlay.Place(view, m.confirmation.ViewSize(m.width, m.height), m.width, m.height)
	}
	return view
}
func (m encodeModel) Status() tui.Status {
	hint := "Tab move · Enter edit · n plan"
	if m.picking != "" {
		hint = "Enter select · Esc cancel"
	} else if m.controls.IsActive() {
		hint = "Enter apply · Esc cancel"
	} else if m.stage == "plan" {
		hint = "↑/k ↓/j · ←/h →/l scroll · n start · Esc parameters"
	} else if m.stage == "processing" {
		hint = "Esc stop"
	} else if m.stage == "results" {
		hint = "↑/k ↓/j · ←/h →/l scroll · r again · Esc leave"
	}
	if m.confirming {
		hint = ""
	}
	complete, failed := 0, 0
	for _, r := range m.results {
		if r.Published {
			complete++
		} else {
			failed++
		}
	}
	return tui.Status{Left: strings.ToUpper(m.stage), Center: fmt.Sprintf("%d published · %d failed", complete, failed), Right: hint}
}

func (m encodeModel) pageActions() pageactions.Config {
	switch m.stage {
	case "parameters", "planning":
		return pageactions.Config{Next: &pageactions.Action{Title: "Plan →  n", Destination: "Review outputs"}}
	case "plan":
		return pageactions.Config{Prev: &pageactions.Action{Destination: "Parameters"}, Next: &pageactions.Action{Title: "Start →  n", Destination: "Write verified JPEGs"}}
	case "results":
		return pageactions.Config{Next: &pageactions.Action{Title: "Again  r", Destination: "Parameters"}}
	}
	return pageactions.Config{}
}

func (m encodeModel) parameterContent(width int) string {
	var rows []string
	for _, id := range encodeIDs {
		rows = append(rows, m.controls.ViewFocusedWidth([]string{id}, true, width))
		if err := m.fieldErrors[id]; err != "" {
			rows = append(rows, "  ! "+err)
		}
	}
	return strings.Join(rows, "\n")
}
func (m *encodeModel) clickParameter(x, y, width int) (string, bool) {
	if y < 0 {
		return "", false
	}
	for _, id := range encodeIDs {
		height := lipgloss.Height(m.controls.ViewFocusedWidth([]string{id}, true, width))
		if y < height {
			return m.controls.Click([]string{id}, x, y)
		}
		y -= height
		if m.fieldErrors[id] != "" {
			if y == 0 {
				return "", false
			}
			y--
		}
	}
	return "", false
}
