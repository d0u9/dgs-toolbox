package desktop

import (
	"reflect"
	"testing"
)

func TestCommands(t *testing.T) {
	tests := []struct {
		got, want []string
	}{
		{openCommand("darwin", "http://x/"), []string{"open", "http://x/"}},
		{openCommand("linux", "http://x/"), []string{"xdg-open", "http://x/"}},
		{revealCommand("darwin", "/a/b.gpx"), []string{"open", "-R", "/a/b.gpx"}},
		{revealCommand("windows", `C:\a\b.gpx`), []string{"explorer", `/select,C:\a\b.gpx`}},
		{revealCommand("linux", "/a/b.gpx"), []string{"xdg-open", "/a"}},
	}
	for _, test := range tests {
		if !reflect.DeepEqual(test.got, test.want) {
			t.Errorf("got %q, want %q", test.got, test.want)
		}
	}
}
