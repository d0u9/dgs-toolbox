package gpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerServesPageAndHealth(t *testing.T) {
	server := httptest.NewServer(Handler())
	defer server.Close()

	for path, want := range map[string]string{"/": "<h1>GPX</h1>", "/api/health": `"ok":true`} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := new(strings.Builder)
		_, _ = io.Copy(body, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || !strings.Contains(body.String(), want) {
			t.Fatalf("GET %s = %d %q, want %q", path, response.StatusCode, body, want)
		}
	}
}
