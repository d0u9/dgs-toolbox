// Package reminders creates Apple Reminders through EventKit. The Reminders
// scripting dictionary has no location alarm, so the work is done by a small
// Objective-C bridge compiled into dgs with cgo; this package speaks JSON to
// it. On anything but macOS, or in a build without cgo, it is unavailable.
//
// It knows nothing about Captures: a caller decides what a reminder says.
package reminders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrUnavailable is returned when this build of dgs cannot reach EventKit.
var ErrUnavailable = errors.New("reminders need dgs built on macOS with cgo")

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

// Result is what EventKit did. Skipped is the reason nothing was created,
// empty when a reminder was.
type Result struct {
	ID      string `json:"id"`
	List    string `json:"list"`
	Skipped string `json:"skipped"`
}

// bridge runs one operation and returns its JSON answer. It is a variable so
// a test stands in for EventKit.
var bridge = eventKit

// Available reports whether this build can write reminders.
func Available() bool { return available }

// Client writes reminders through EventKit.
type Client struct{}

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
	if err := call(ctx, "create", input, &result); err != nil {
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
	if err := call(ctx, "lists", nil, &answer); err != nil {
		return nil, "", err
	}
	return answer.Lists, answer.Default, nil
}

// call runs the bridge and decodes its one JSON answer. The bridge reports a
// failure as {"error": ...}. EventKit cannot be cancelled, so a cancelled
// context returns at once and leaves the call to finish on its own.
func call(ctx context.Context, op string, input []byte, into any) error {
	done := make(chan []byte, 1)
	errc := make(chan error, 1)
	go func() {
		answer, err := bridge(op, input)
		if err != nil {
			errc <- err
			return
		}
		done <- answer
	}()
	var answer []byte
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	case answer = <-done:
	}

	var failure struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(answer, &failure) == nil && failure.Error != "" {
		return errors.New(failure.Error)
	}
	if err := json.Unmarshal(answer, into); err != nil {
		return fmt.Errorf("EventKit answered something that is not JSON: %q", strings.TrimSpace(string(answer)))
	}
	return nil
}
