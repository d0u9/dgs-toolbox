package gpx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"dgs-toolbox/internal/geo/gpxfile"
)

// Requests from two pages share pending state. Every accepted addition must
// survive a simultaneous Save, and a repeated Save must not apply it twice.
func TestConcurrentEditsAndSavesKeepEveryWaypoint(t *testing.T) {
	path := ownRenameGPX(t)
	defer pending.drop(path)
	handler := Handler(Settings{})
	const count = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	request := func(method, endpoint string, body any) {
		defer wg.Done()
		data, err := json.Marshal(body)
		if err != nil {
			t.Error(err)
			return
		}
		req := httptest.NewRequest(method, endpoint, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		<-start
		handler.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Errorf("%s = %d %s", endpoint, response.Code, response.Body)
		}
	}
	for i := 0; i < count; i++ {
		wg.Add(2)
		go request(http.MethodPost, "/api/waypoint", map[string]any{"path": path, "name": fmt.Sprintf("Point %d", i), "lat": 0, "lon": 0})
		go request(http.MethodPost, "/api/save", map[string]any{"path": path})
	}
	close(start)
	wg.Wait()
	if err := flush(path); err != nil {
		t.Fatal(err)
	}
	parsed, err := gpxfile.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, point := range parsed.Waypoints {
		names[point.Name] = true
	}
	if len(parsed.Waypoints) != count || len(names) != count {
		t.Fatalf("accepted edits lost or duplicated: %d waypoints, %d names", len(parsed.Waypoints), len(names))
	}
}
