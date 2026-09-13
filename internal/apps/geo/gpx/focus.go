package gpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Summary is what the TUI shows of the track focused on the page.
type Summary struct {
	Name     string
	Path     string
	Distance float64 // metres
	Duration time.Duration
	Stops    int
}

// focusBoard holds the page's focused track for the TUI. The page reports
// every change of focus or stop parameters; the TUI waits for the next one.
type focusBoard struct {
	mu      sync.Mutex
	summary *Summary
	changed chan struct{} // closed and replaced on every change
}

func newFocusBoard() *focusBoard {
	return &focusBoard{changed: make(chan struct{})}
}

// focused is the board of this process's server, which runs once.
var focused = newFocusBoard()

func (b *focusBoard) set(summary *Summary) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.summary = summary
	close(b.changed)
	b.changed = make(chan struct{})
}

// current is the focused track, nil when none, and a channel closed when it
// next changes.
func (b *focusBoard) current() (*Summary, <-chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.summary, b.changed
}

// focus records the track the page focused. An empty path clears it.
func (a api) focus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path         string   `json:"path"`
		StopDistance *float64 `json:"stopDistance"`
		StopDuration *float64 `json:"stopDuration"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if body.Path == "" {
		a.board.set(nil)
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	params, err := stopParams(number(body.StopDistance), number(body.StopDuration))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	result, err := analyse(body.Path, params)
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	a.board.set(&Summary{
		Name:     result.name,
		Path:     result.path,
		Distance: result.stats.Distance,
		Duration: result.stats.Duration,
		Stops:    len(result.stops),
	})
	writeJSON(w, map[string]bool{"ok": true})
}

func number(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}
