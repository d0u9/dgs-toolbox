package ocr

import (
	"errors"
	"math"
	"testing"
)

func TestDecode(t *testing.T) {
	r, err := decode([]byte(`{"pages":[
		{"source":"recognised","lines":[{"text":" A ","x":0.1,"y":0.8,"w":0.5,"h":0.1,"space":"display"},{"text":"B","x":0,"y":0,"w":1,"h":0.1,"space":"display"},{"text":"  ","x":0,"y":0,"w":0,"h":0}]},
		{"source":"text","lines":[]},
		{"source":"text","lines":[{"text":"C","x":0,"y":0,"w":0.2,"h":0.1,"space":"page","rotate":0}]}]}`))
	if err != nil || len(r.Pages) != 3 || r.Text() != "A\nB\n\n\n\nC" {
		t.Fatalf("%+v %q %v", r, r.Text(), err)
	}
	if got := r.Pages[0].Lines[0].Box; !near(got, Box{0.1, 0.1, 0.5, 0.1}) {
		t.Errorf("box = %+v", got)
	}
	if _, err := decode([]byte(`{"error":"locked"}`)); err == nil || err.Error() != "locked" {
		t.Fatalf("error answer: %v", err)
	}
	if _, err := decode([]byte(`nope`)); err == nil {
		t.Fatal("bad JSON accepted")
	}
}

func near(a, b Box) bool {
	const e = 1e-9
	return math.Abs(a.X-b.X) < e && math.Abs(a.Y-b.Y) < e && math.Abs(a.W-b.W) < e && math.Abs(a.H-b.H) < e
}

// A line at the top left of an unturned page ends up where the page's turn
// takes that corner.
func TestPlacedFollowsTheTurn(t *testing.T) {
	line := rawLine{X: 0, Y: 0.9, W: 0.2, H: 0.1, Space: "page"} // top left, unturned
	for rotate, want := range map[int]Box{
		0:   {0, 0, 0.2, 0.1},
		90:  {0.9, 0, 0.1, 0.2}, // clockwise: the top left goes to the top right
		180: {0.8, 0.9, 0.2, 0.1},
		270: {0, 0.8, 0.1, 0.2},
		-90: {0, 0.8, 0.1, 0.2},
	} {
		line.Rotate = rotate
		if got := placed(line); !near(got, want) {
			t.Errorf("rotate %d: %+v, want %+v", rotate, got, want)
		}
	}
}

func TestLocate(t *testing.T) {
	r := Result{Pages: []Page{
		{Lines: []Line{{Text: "ab"}, {Text: "cd"}}},
		{Lines: []Line{{Text: "ef"}}},
	}}
	text := r.Text() // "ab\ncd\n\nef"
	for offset, want := range map[int][3]int{0: {0, 0, 1}, 1: {0, 0, 1}, 3: {0, 1, 1}, 7: {1, 0, 1}, 2: {0, 0, 0}, 5: {0, 0, 0}, 99: {0, 0, 0}} {
		p, l, ok := r.Locate(offset)
		if ok != (want[2] == 1) || (ok && (p != want[0] || l != want[1])) {
			t.Errorf("offset %d in %q: %d %d %v", offset, text, p, l, ok)
		}
	}
}

func TestUnavailableSaysSo(t *testing.T) {
	if Available() {
		t.Skip("recognition is compiled in")
	}
	if _, err := Recognize("x.pdf", 0); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}
