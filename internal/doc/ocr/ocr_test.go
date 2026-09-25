package ocr

import (
	"errors"
	"testing"
)

func TestDecode(t *testing.T) {
	r, err := decode([]byte(`{"pages":[{"text":"  A\nB ","source":"text"},{"text":"","source":"recognised"},{"text":"C","source":"recognised"}]}`))
	if err != nil || len(r.Pages) != 3 || r.Text() != "A\nB\n\nC" {
		t.Fatalf("%+v %q %v", r, r.Text(), err)
	}
	if _, err := decode([]byte(`{"error":"locked"}`)); err == nil || err.Error() != "locked" {
		t.Fatalf("error answer: %v", err)
	}
	if _, err := decode([]byte(`nope`)); err == nil {
		t.Fatal("bad JSON accepted")
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
