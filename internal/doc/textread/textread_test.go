package textread

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"dgs-toolbox/internal/doc/ocr"
	"dgs-toolbox/internal/doc/textcache"
)

// fake is a PDF reader that records the order pages were read in, and can be
// held so the queue fills up behind it.
type fake struct {
	mu    sync.Mutex
	order []string
	hold  chan struct{}
	// started, when set, is told each page as it is taken, before hold.
	started chan string
	count   int
}

func (f *fake) read(path string, page int) (ocr.Page, int, error) {
	if f.started != nil {
		f.started <- path + ":" + strconv.Itoa(page)
	}
	if f.hold != nil {
		<-f.hold
	}
	f.mu.Lock()
	f.order = append(f.order, path+":"+strconv.Itoa(page))
	f.mu.Unlock()
	return ocr.Page{Lines: []ocr.Line{{Text: path + " " + strconv.Itoa(page)}}}, f.count, nil
}

func (f *fake) seen() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.order...)
}

func digest(c string) string { return strings.Repeat(c, 64) }

func wait(t *testing.T, f *fake, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for len(f.seen()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("read only %v", f.seen())
		}
		time.Sleep(5 * time.Millisecond)
	}
	return f.seen()
}

func TestPageIsReadOnceAndKept(t *testing.T) {
	f := &fake{count: 2}
	store := textcache.Store{Dir: t.TempDir()}
	r := New(f.read, store, 0, 1)
	defer r.Stop()
	for range 2 {
		page, count, err := r.Page(context.Background(), "a", digest("a"), 0)
		if err != nil || count != 2 || page.Text() != "a 0" {
			t.Fatalf("%+v %d %v", page, count, err)
		}
	}
	// Page 0 once; page 1 follows it on its own.
	got := wait(t, f, 2)
	if got[0] != "a:0" || got[1] != "a:1" {
		t.Fatalf("read %v", got)
	}
	r.Stop()
	// A new Reader over the same cache reads nothing.
	again := New(f.read, store, 0, 1)
	defer again.Stop()
	for n := range 2 {
		if _, _, err := again.Page(context.Background(), "a", digest("a"), n); err != nil {
			t.Fatal(err)
		}
	}
	time.Sleep(30 * time.Millisecond)
	if len(f.seen()) != 2 {
		t.Fatalf("after restart: %v", f.seen())
	}
}

func TestLookingJumpsAheadOfReadingAhead(t *testing.T) {
	f := &fake{count: 3, hold: make(chan struct{}), started: make(chan string, 16)}
	r := New(f.read, textcache.Store{Dir: t.TempDir()}, 0, 1)
	defer r.Stop()
	r.Ahead("b", digest("b"))
	<-f.started // taken by the one worker, and held
	r.Ahead("c", digest("c"))
	r.Ahead("d", digest("d"))
	done := make(chan struct{})
	go func() {
		_, _, _ = r.Page(context.Background(), "d", digest("d"), 0) // queued ahead: promoted
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	close(f.hold)
	<-done
	got := wait(t, f, 2)
	if got[0] != "b:0" || got[1] != "d:0" {
		t.Fatalf("order %v", got)
	}
	// Reading ahead goes on to the later pages of each PDF.
	got = wait(t, f, 9)
	for _, want := range []string{"b:1", "b:2", "c:0", "c:1", "c:2", "d:1", "d:2"} {
		if !contains(got, want) {
			t.Errorf("%s never read: %v", want, got)
		}
	}
}

func TestReadingAheadStopsAtMaxPages(t *testing.T) {
	f := &fake{count: 10}
	r := New(f.read, textcache.Store{Dir: t.TempDir()}, 2, 1)
	defer r.Stop()
	r.Ahead("e", digest("e"))
	wait(t, f, 2)
	time.Sleep(50 * time.Millisecond)
	if got := f.seen(); len(got) != 2 {
		t.Fatalf("read %v", got)
	}
}

func TestAWaitGivesUpWithItsContext(t *testing.T) {
	f := &fake{count: 1, hold: make(chan struct{})}
	r := New(f.read, textcache.Store{Dir: t.TempDir()}, 0, 1)
	defer r.Stop()
	defer close(f.hold) // before Stop, which waits for the held page
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := r.Page(ctx, "f", digest("f"), 0); err == nil {
		t.Fatal("waited past its context")
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// The pages after a first page someone is looking at go ahead of anything
// read in advance.
func TestLaterPagesFollowTheFirst(t *testing.T) {
	f := &fake{count: 3, hold: make(chan struct{}), started: make(chan string, 16)}
	r := New(f.read, textcache.Store{Dir: t.TempDir()}, 0, 1)
	defer r.Stop()
	go func() { _, _, _ = r.Page(context.Background(), "a", digest("a"), 0) }()
	<-f.started
	r.Ahead("b", digest("b"))
	r.Ahead("c", digest("c"))
	close(f.hold)
	got := wait(t, f, 3)
	if got[0] != "a:0" || got[1] != "a:1" || got[2] != "a:2" {
		t.Fatalf("order %v", got)
	}
}
