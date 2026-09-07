package tui

import tea "github.com/charmbracelet/bubbletea"

// Status is the three-part contribution rendered by the shared status bar.
type Status struct {
	Left   string
	Center string
	Right  string
}

// CommandModel is a leaf command component hosted by the shared shell.
type CommandModel interface {
	tea.Model
	Status() Status
}

// Command describes a leaf command and creates a fresh model each time it is
// selected.
type Command struct {
	ID          string
	Name        string
	Description string
	New         func() CommandModel
}

// App describes a command domain and its leaf commands.
type App struct {
	ID          string
	Name        string
	Description string
	Commands    []Command
}

// Launch identifies the navigation root selected by the CLI. Empty fields
// mean the global picker; App alone means a domain picker; both fields mean a
// direct leaf command.
type Launch struct {
	App     string
	Command string
}

// Runner is injected into the command tree to keep CLI tests independent of a
// terminal and to make the boundary between Cobra and Bubble Tea explicit.
type Runner func(Launch) error
