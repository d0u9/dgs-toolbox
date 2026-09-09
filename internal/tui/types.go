package tui

import (
	"io"

	"dgs-toolbox/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// RequestQuitMsg lets a mouse action inside a command request the shell-owned
// quit confirmation instead of terminating the Bubble Tea program directly.
type RequestQuitMsg struct{}

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

// ShellKeyCapturer lets a command retain keys that normally belong to the
// shell. Text editors use this so q is text and Esc cancels editing.
type ShellKeyCapturer interface {
	CapturesShellKey(key string) bool
}

// CommandPathContributor lets an active command append workflow state to the
// shell breadcrumb without taking ownership of the top bar.
type CommandPathContributor interface {
	CommandPath() []string
}

// Tab describes one command-owned session in the shell's top-left tab bar.
// Commands without a TabContributor keep the shell's derived command label.
type Tab struct {
	Label  string
	Active bool
}

// TabContributor lets a command populate the shell-owned tab bar without
// taking ownership of the rest of the top bar.
type TabContributor interface {
	Tabs() []Tab
}

// TabSelectedMsg reports a primary click on a shell-rendered tab. The shell
// owns tab hit testing; the command owns what activating that tab means.
type TabSelectedMsg struct {
	Index int
}

// Command describes a leaf command and creates a fresh model each time it is
// selected.
type Command struct {
	ID            string
	Name          string
	Description   string
	New           func() CommandModel
	NewWithConfig func(config.Config) CommandModel
}

// Report writes something an app knows about itself to stdout instead of
// opening its TUI: what it supports, what it loaded, why a file was rejected.
// The CLI turns each one into a flag on the app's command, so an app adds a
// report without the command hierarchy learning anything app-specific.
type Report struct {
	Flag        string
	Description string
	Run         func(out io.Writer, global config.Config) error
}

// App describes a command domain and its leaf commands.
type App struct {
	ID          string
	Name        string
	Description string
	// Direct exposes an app with one command as `dgs <app>` and launches that
	// command immediately instead of opening an app-scoped picker.
	Direct   bool
	Commands []Command
	// Reports are the app's non-interactive outputs, exposed as flags on its
	// command.
	Reports []Report
}

// Launch identifies the navigation root selected by the CLI. Empty fields
// mean the global picker; App alone means a domain picker; both fields mean a
// direct leaf command.
type Launch struct {
	App        string
	Command    string
	ConfigPath string
}

// Runner is injected into the command tree to keep CLI tests independent of a
// terminal and to make the boundary between Cobra and Bubble Tea explicit.
type Runner func(Launch) error
