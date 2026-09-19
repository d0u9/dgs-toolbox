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

// Closer lets a command let go of what it holds — plaintext in memory, say —
// when the shell leaves it for the picker or quits. Close is called once, and
// the model is not used afterwards.
type Closer interface {
	Close()
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
	// Flags are command-line options of this command. The CLI registers them
	// on the command's leaf, and a value given at launch is applied to the
	// configuration before the command is built, so it wins over the file.
	Flags []Flag
}

// Flag is a string option that overrides one configuration setting.
type Flag struct {
	Name  string
	Usage string
	Apply func(global *config.Config, value string) error
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

// Action is a CLI-only subcommand such as `dgs photo organize <folder>`. It
// reads answers from in and writes progress to out instead of opening the TUI.
type Action struct {
	ID          string
	Usage       string
	Description string
	Args        int
	// MinArgs, when set, accepts that many positional arguments or more,
	// in place of Args' exact count. A selector is one or more terms.
	MinArgs int
	// MaxArgs, when set, caps MinArgs' open end. A directory that defaults
	// to a configured one is none or one, and a second would be a typo
	// worth reporting rather than ignoring.
	MaxArgs int
	Flags   []ActionFlag
	// Run receives every declared flag by name; a boolean flag is "true" or
	// "false" and a string flag not given holds its Default.
	Run func(in io.Reader, out io.Writer, args []string, flags map[string]string) error
	// RunWithConfig is Run for an action that reads the toolbox's own
	// configuration — a generator root, a destination directory — the same
	// way Command.NewWithConfig is New for a command that does. An action
	// declaring it is called through it, and Run is not consulted.
	RunWithConfig func(in io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error
}

// ActionFlag is an option of an Action. Bool flags take no value.
type ActionFlag struct {
	Name      string
	Shorthand string
	Usage     string
	Bool      bool
	Default   string
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
	// Actions are non-interactive subcommands that take positional arguments
	// and run in the shell. They are not listed in the TUI picker.
	Actions []Action
}

// Launch identifies the navigation root selected by the CLI. Empty fields
// mean the global picker; App alone means a domain picker; both fields mean a
// direct leaf command.
type Launch struct {
	App        string
	Command    string
	ConfigPath string
	// Flags holds the command flags given on the command line, by name.
	Flags map[string]string
}

// Runner is injected into the command tree to keep CLI tests independent of a
// terminal and to make the boundary between Cobra and Bubble Tea explicit.
type Runner func(Launch) error
