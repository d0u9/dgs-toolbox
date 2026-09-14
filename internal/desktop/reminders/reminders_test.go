package reminders

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// fakeBridge stands in for EventKit: it records what it was asked and answers
// with answer.
type fakeBridge struct {
	op     string
	input  []byte
	answer string
	ran    bool
}

func withBridge(t *testing.T, answer string) *fakeBridge {
	t.Helper()
	fake := &fakeBridge{answer: answer}
	saved := bridge
	bridge = func(op string, input []byte) ([]byte, error) {
		fake.op, fake.input, fake.ran = op, input, true
		return []byte(fake.answer), nil
	}
	t.Cleanup(func() { bridge = saved })
	return fake
}

func TestCreateSendsTheRequest(t *testing.T) {
	fake := withBridge(t, `{"id":"abc","list":"Inbox"}`)
	request := Request{
		Title: "Buy tea",
		Due:   "2026-09-15T09:00:00+10:00",
		List:  "Inbox",
		Mark:  "dgs-capture-1",
		Location: &Location{
			Title: "Shop", Latitude: -33.86, Longitude: 151.2, Radius: 150, Proximity: Arrive,
		},
	}
	result, err := Client{}.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{ID: "abc", List: "Inbox"}) {
		t.Errorf("result = %+v", result)
	}
	if fake.op != "create" {
		t.Errorf("op = %q", fake.op)
	}
	var sent Request
	if err := json.Unmarshal(fake.input, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Title != request.Title || sent.Due != request.Due || sent.Mark != request.Mark ||
		sent.Location == nil || *sent.Location != *request.Location {
		t.Errorf("sent = %+v", sent)
	}
}

func TestCreateOmitsWhatIsNotGiven(t *testing.T) {
	fake := withBridge(t, `{"id":"abc"}`)
	if _, err := (Client{}).Create(context.Background(), Request{Title: "Call"}); err != nil {
		t.Fatal(err)
	}
	if sent := string(fake.input); sent != `{"title":"Call"}` {
		t.Errorf("sent %s", sent)
	}
}

func TestCreateReportsSkipped(t *testing.T) {
	withBridge(t, `{"id":"abc","skipped":"already a reminder"}`)
	result, err := Client{}.Create(context.Background(), Request{Title: "Call"})
	if err != nil || result.Skipped != "already a reminder" {
		t.Errorf("result = %+v, err = %v", result, err)
	}
}

func TestBridgeErrorIsTheReason(t *testing.T) {
	withBridge(t, `{"error":"reminders access was denied"}`)
	_, err := Client{}.Create(context.Background(), Request{Title: "Call"})
	if err == nil || err.Error() != "reminders access was denied" {
		t.Errorf("err = %v", err)
	}
}

func TestBridgeAnsweringNonsense(t *testing.T) {
	withBridge(t, `not json`)
	_, err := Client{}.Create(context.Background(), Request{Title: "Call"})
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Errorf("err = %v", err)
	}
}

func TestCreateRefusesBeforeRunning(t *testing.T) {
	tests := []Request{
		{Title: "  "},
		{Title: "Call", Location: &Location{Proximity: "near"}},
	}
	for _, request := range tests {
		fake := withBridge(t, `{"id":"abc"}`)
		if _, err := (Client{}).Create(context.Background(), request); err == nil {
			t.Errorf("%+v: no error", request)
		}
		if fake.ran {
			t.Errorf("%+v: EventKit was called", request)
		}
	}
}

func TestLists(t *testing.T) {
	fake := withBridge(t, `{"lists":["Inbox","Errands"],"default":"Inbox"}`)
	lists, def, err := Client{}.Lists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lists, ",") != "Inbox,Errands" || def != "Inbox" {
		t.Errorf("lists = %v, default = %q", lists, def)
	}
	if fake.op != "lists" {
		t.Errorf("op = %q", fake.op)
	}
}
