package organizer

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/desktop/reminders"
)

// The reminder Actions' parameters. A list and a radius are how a reminder is
// written rather than what it says, so they have configured defaults and a
// per-Capture override, as the daily note's section does.
const (
	ParameterList      = "list"
	ParameterRadius    = "radius"
	ParameterProximity = "when"
)

// DefaultReminderRadius is how close counts as being there, in metres. Wide
// enough that a phone's position indoors still triggers it, narrow enough that
// the next street over does not.
const DefaultReminderRadius = 150

// reminderClient creates reminders. It is a variable so a test replaces the
// helper rather than writing into the reader's Reminders.
var reminderClient = func() (reminderCreator, error) {
	helper, err := reminders.FindHelper()
	if err != nil {
		return nil, err
	}
	return reminders.Client{Helper: helper}, nil
}

type reminderCreator interface {
	Create(context.Context, reminders.Request) (reminders.Result, error)
}

// reminderTimeout bounds one helper run. The first run waits on the reader
// answering the permission prompt, so it is generous.
const reminderTimeout = 2 * time.Minute

// createReminder is apple.reminders.create: a reminder due at a time.
func createReminder(ctx Context, plan ActionPlan) (string, error) {
	due, err := parseDue(ctx.String(FieldDueAt))
	if err != nil {
		return "", err
	}
	request := reminderRequest(ctx, plan.Action)
	request.Due = due
	return sendReminder(request)
}

// createPlaceReminder is apple.reminders.at_place: a reminder that fires on
// arriving at, or leaving, where the Capture was.
func createPlaceReminder(ctx Context, plan ActionPlan) (string, error) {
	position, ok := ctx.Position()
	if !ok {
		return "", errors.New("this capture has no position to be reminded at")
	}
	proximity := reminders.Proximity(strings.TrimSpace(ctx.Parameter(plan.Action, ParameterProximity)))
	if proximity != reminders.Arrive && proximity != reminders.Leave {
		return "", fmt.Errorf("when %q is neither %s nor %s", proximity, reminders.Arrive, reminders.Leave)
	}
	radius, err := parseRadius(ctx.Parameter(plan.Action, ParameterRadius))
	if err != nil {
		return "", err
	}
	request := reminderRequest(ctx, plan.Action)
	request.Location = &reminders.Location{
		Title:     ctx.String(FieldPlaceName),
		Latitude:  position.Latitude,
		Longitude: position.Longitude,
		Radius:    radius,
		Proximity: proximity,
	}
	return sendReminder(request)
}

// reminderRequest is what both reminder Actions say: the title, the Capture's
// text as the notes, and the mark that keeps a Capture from becoming two
// reminders.
func reminderRequest(ctx Context, action ActionID) reminders.Request {
	return reminders.Request{
		Title: strings.TrimSpace(ctx.String(FieldTitle)),
		Notes: strings.TrimSpace(ctx.String(FieldContent)),
		List:  strings.TrimSpace(ctx.Parameter(action, ParameterList)),
		Mark:  captureMark(ctx.Capture),
	}
}

func sendReminder(request reminders.Request) (string, error) {
	client, err := reminderClient()
	if err != nil {
		return "", err
	}
	run, cancel := context.WithTimeout(context.Background(), reminderTimeout)
	defer cancel()
	result, err := client.Create(run, request)
	if err != nil {
		return "", err
	}
	return result.Skipped, nil
}

// dueLayouts are what a due time may be written as: RFC 3339, as a payload
// would carry it, or a wall clock read in local time, as someone would type
// it.
var dueLayouts = []string{"2006-01-02 15:04", "2006-01-02T15:04", "2006-01-02"}

// parseDue returns the due time as RFC 3339, which is what the helper takes.
func parseDue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if due, err := time.Parse(time.RFC3339, value); err == nil {
		return due.Format(time.RFC3339), nil
	}
	for _, layout := range dueLayouts {
		if due, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return due.Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("due %q is not a time; write it as 2026-09-15 09:00", value)
}

func parseRadius(value string) (float64, error) {
	radius, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || radius <= 0 {
		return 0, fmt.Errorf("radius %q is not a distance in metres", value)
	}
	return radius, nil
}

func (s Settings) reminderRadius() string {
	if s.ReminderRadius > 0 {
		return strconv.FormatFloat(s.ReminderRadius, 'f', -1, 64)
	}
	return strconv.Itoa(DefaultReminderRadius)
}

// reminderTarget names what a reminder Action will write: the title and when
// it fires, which is what tells two reminders apart in a plan.
func reminderTarget(ctx Context, action ActionID, when string) string {
	title := ctx.String(FieldTitle)
	if title == "" || when == "" {
		return ""
	}
	if list := ctx.Parameter(action, ParameterList); list != "" {
		return list + " · " + title + " · " + when
	}
	return title + " · " + when
}
