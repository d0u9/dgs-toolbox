package reminders

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeHelper writes a shell script standing in for dgs-reminders: it saves its
// arguments and stdin next to itself and prints answer.
func fakeHelper(t *testing.T, answer string, exit int) (helper, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake helper is a shell script")
	}
	dir = t.TempDir()
	helper = filepath.Join(dir, HelperName)
	script := "#!/bin/sh\n" +
		"echo \"$@\" > \"$(dirname \"$0\")/args\"\n" +
		"cat > \"$(dirname \"$0\")/stdin\"\n" +
		"printf '%s' '" + answer + "'\n" +
		"exit " + string(rune('0'+exit)) + "\n"
	if err := os.WriteFile(helper, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return helper, dir
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCreateSendsTheRequest(t *testing.T) {
	helper, dir := fakeHelper(t, `{"id":"abc","list":"Inbox"}`, 0)
	request := Request{
		Title: "Buy tea",
		Due:   "2026-09-15T09:00:00+10:00",
		List:  "Inbox",
		Mark:  "dgs-capture-1",
		Location: &Location{
			Title: "Shop", Latitude: -33.86, Longitude: 151.2, Radius: 150, Proximity: Arrive,
		},
	}
	result, err := Client{Helper: helper}.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result != (Result{ID: "abc", List: "Inbox"}) {
		t.Errorf("result = %+v", result)
	}
	if args := strings.TrimSpace(read(t, filepath.Join(dir, "args"))); args != "create" {
		t.Errorf("args = %q", args)
	}
	var sent Request
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "stdin"))), &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Title != request.Title || sent.Due != request.Due || sent.Mark != request.Mark ||
		sent.Location == nil || *sent.Location != *request.Location {
		t.Errorf("sent = %+v", sent)
	}
}

func TestCreateOmitsWhatIsNotGiven(t *testing.T) {
	helper, dir := fakeHelper(t, `{"id":"abc"}`, 0)
	if _, err := (Client{Helper: helper}).Create(context.Background(), Request{Title: "Call"}); err != nil {
		t.Fatal(err)
	}
	if sent := read(t, filepath.Join(dir, "stdin")); sent != `{"title":"Call"}` {
		t.Errorf("sent %s", sent)
	}
}

func TestCreateReportsSkipped(t *testing.T) {
	helper, _ := fakeHelper(t, `{"id":"abc","skipped":"already a reminder"}`, 0)
	result, err := Client{Helper: helper}.Create(context.Background(), Request{Title: "Call"})
	if err != nil || result.Skipped != "already a reminder" {
		t.Errorf("result = %+v, err = %v", result, err)
	}
}

func TestHelperErrorIsTheReason(t *testing.T) {
	helper, _ := fakeHelper(t, `{"error":"reminders access was denied"}`, 1)
	_, err := Client{Helper: helper}.Create(context.Background(), Request{Title: "Call"})
	if err == nil || err.Error() != "reminders access was denied" {
		t.Errorf("err = %v", err)
	}
}

func TestHelperAnsweringNonsense(t *testing.T) {
	helper, _ := fakeHelper(t, `not json`, 0)
	_, err := Client{Helper: helper}.Create(context.Background(), Request{Title: "Call"})
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
		helper, dir := fakeHelper(t, `{"id":"abc"}`, 0)
		if _, err := (Client{Helper: helper}).Create(context.Background(), request); err == nil {
			t.Errorf("%+v: no error", request)
		}
		if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
			t.Errorf("%+v: the helper ran", request)
		}
	}
}

func TestNoHelper(t *testing.T) {
	if _, err := (Client{}).Create(context.Background(), Request{Title: "Call"}); err != ErrNoHelper {
		t.Errorf("err = %v", err)
	}
}

func TestLists(t *testing.T) {
	helper, dir := fakeHelper(t, `{"lists":["Inbox","Errands"],"default":"Inbox"}`, 0)
	lists, def, err := Client{Helper: helper}.Lists(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(lists, ",") != "Inbox,Errands" || def != "Inbox" {
		t.Errorf("lists = %v, default = %q", lists, def)
	}
	if args := strings.TrimSpace(read(t, filepath.Join(dir, "args"))); args != "lists" {
		t.Errorf("args = %q", args)
	}
}
