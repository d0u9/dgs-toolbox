// Package reminders creates Apple Reminders through dgs-reminders, the EventKit
// helper built from helpers/reminders. Go cannot reach EventKit, and the
// Reminders scripting dictionary has no location alarm, so the work is done in
// a separate binary and this package only speaks its JSON.
//
// It knows nothing about Captures: a caller decides what a reminder says.
package reminders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HelperName is the helper's file name, installed next to dgs by make install.
const HelperName = "dgs-reminders"

// ErrNoHelper is returned when the helper cannot be found. Reminders are only
// written on macOS, and only once the helper has been built.
var ErrNoHelper = errors.New(HelperName + " is not installed; run make install on macOS")

// Proximity is when a location reminder fires.
type Proximity string

const (
	Arrive Proximity = "arrive"
	Leave  Proximity = "leave"
)

// Location is a place a reminder fires at.
type Location struct {
	Title     string  `json:"title,omitempty"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	// Radius is in metres; zero leaves the system's default.
	Radius    float64   `json:"radius,omitempty"`
	Proximity Proximity `json:"proximity"`
}

// Request is one reminder to create.
type Request struct {
	Title string `json:"title"`
	Notes string `json:"notes,omitempty"`
	// Due is RFC 3339; empty is no due date.
	Due string `json:"due,omitempty"`
	// List is a list's title; empty is the default list for new reminders.
	List string `json:"list,omitempty"`
	// Mark is written into the notes and looked for before creating, so the
	// same thing is not made into a reminder twice.
	Mark     string    `json:"mark,omitempty"`
	Location *Location `json:"location,omitempty"`
}

// Result is what the helper did. Skipped is the reason nothing was created,
// empty when a reminder was.
type Result struct {
	ID      string `json:"id"`
	List    string `json:"list"`
	Skipped string `json:"skipped"`
}

// Client runs the helper at Helper.
type Client struct {
	Helper string
}

// FindHelper looks next to the running executable first, which is where make
// install puts it, and then on PATH.
func FindHelper() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", ErrNoHelper
	}
	if self, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(self), HelperName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(HelperName); err == nil {
		return path, nil
	}
	return "", ErrNoHelper
}

// Create creates one reminder, or reports why it did not.
func (c Client) Create(ctx context.Context, request Request) (Result, error) {
	if strings.TrimSpace(request.Title) == "" {
		return Result{}, errors.New("a reminder needs a title")
	}
	if l := request.Location; l != nil && l.Proximity != Arrive && l.Proximity != Leave {
		return Result{}, fmt.Errorf("proximity %q is neither %s nor %s", l.Proximity, Arrive, Leave)
	}
	input, err := json.Marshal(request)
	if err != nil {
		return Result{}, err
	}
	var result Result
	if err := c.run(ctx, input, &result, "create"); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Lists returns the titles of the reminder lists and the default one.
func (c Client) Lists(ctx context.Context) (lists []string, defaultList string, err error) {
	var answer struct {
		Lists   []string `json:"lists"`
		Default string   `json:"default"`
	}
	if err := c.run(ctx, nil, &answer, "lists"); err != nil {
		return nil, "", err
	}
	return answer.Lists, answer.Default, nil
}

// run executes the helper and decodes its one JSON answer. The helper reports
// a failure as {"error": ...}, which is preferred over the exit status because
// it says why.
func (c Client) run(ctx context.Context, input []byte, into any, args ...string) error {
	if c.Helper == "" {
		return ErrNoHelper
	}
	cmd := exec.CommandContext(ctx, c.Helper, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	runErr := cmd.Run()

	var failure struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(stdout.Bytes(), &failure) == nil && failure.Error != "" {
		return errors.New(failure.Error)
	}
	if runErr != nil {
		if detail := strings.TrimSpace(stderr.String()); detail != "" {
			return fmt.Errorf("%s: %w: %s", HelperName, runErr, detail)
		}
		return fmt.Errorf("%s: %w", HelperName, runErr)
	}
	if err := json.Unmarshal(stdout.Bytes(), into); err != nil {
		return fmt.Errorf("%s answered something that is not JSON: %q", HelperName, strings.TrimSpace(stdout.String()))
	}
	return nil
}
