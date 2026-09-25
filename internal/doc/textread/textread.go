// Package textread decides when a PDF's pages are read, and keeps what was
// read. Recognising a page accurately takes about a second, so it matters
// which page goes first: a page someone is looking at goes ahead of any page
// read in advance, a page already read is answered from the cache, and a page
// asked for twice is read once.
package textread

import (
	"context"
	"sync"

	"dgs-toolbox/internal/doc/ocr"
	"dgs-toolbox/internal/doc/textcache"
)

// DefaultWorkers is how many pages are recognised at once. Two keeps a
// second page going while the first is drawn, without starving the page
// being looked at of the machine.
const DefaultWorkers = 2

// ReadFunc reads one page of a PDF, counting from 0, and says how many pages
// it has: ocr.RecognizePage, or a stand-in.
type ReadFunc func(path string, page int) (ocr.Page, int, error)

type key struct {
	digest string
	page   int
}

type job struct {
	key
	path   string
	urgent bool
	// ahead is set for a read in advance: when its first page is done, the
	// pages after it, up to MaxPages, are queued in advance as well.
	ahead bool
	// follow is set for a first page someone is looking at: when it is done,
	// the pages after it are queued at the front, since they are asked for
	// next and would otherwise wait behind pages read in advance.
	follow bool
	done   chan struct{}
	page   ocr.Page
	count  int
	err    error
}

// Reader reads pages through a queue.
type Reader struct {
	read     ReadFunc
	store    textcache.Store
	maxPages int

	workers sync.WaitGroup

	mu      sync.Mutex
	wake    *sync.Cond
	queue   []*job
	jobs    map[key]*job
	counts  map[string]int
	stopped bool
}

// New starts a Reader with workers goroutines. A Store with no Dir keeps
// nothing on disk; maxPages bounds reading in advance, zero or less using
// ocr.DefaultMaxPages; workers zero or less uses DefaultWorkers.
func New(read ReadFunc, store textcache.Store, maxPages, workers int) *Reader {
	if maxPages <= 0 {
		maxPages = ocr.DefaultMaxPages
	}
	if workers <= 0 {
		workers = DefaultWorkers
	}
	r := &Reader{read: read, store: store, maxPages: maxPages, jobs: map[key]*job{}, counts: map[string]int{}}
	r.wake = sync.NewCond(&r.mu)
	r.workers.Add(workers)
	for range workers {
		go func() {
			defer r.workers.Done()
			r.work()
		}()
	}
	return r
}

// Stop ends the workers and returns once the pages they are reading are
// done and kept. Queued jobs are abandoned; a Page waiting on one returns
// when its context ends.
func (r *Reader) Stop() {
	r.mu.Lock()
	r.stopped = true
	r.mu.Unlock()
	r.wake.Broadcast()
	r.workers.Wait()
}

// MaxPages is how many pages are read in advance.
func (r *Reader) MaxPages() int { return r.maxPages }

// Page is one page's text and the PDF's page count, from the cache or read
// now, ahead of anything read in advance.
func (r *Reader) Page(ctx context.Context, path, digest string, page int) (ocr.Page, int, error) {
	if text, count, ok := r.store.Load(digest, page); ok {
		return text, count, nil
	}
	j := r.enqueue(path, digest, page, true, false)
	if page == 0 {
		r.mu.Lock()
		j.follow = true
		r.mu.Unlock()
	}
	select {
	case <-j.done:
		return j.page, j.count, j.err
	case <-ctx.Done():
		return ocr.Page{}, 0, ctx.Err()
	}
}

// Ahead queues a PDF to be read in advance: its first page now, and the pages
// after it once its page count is known. Pages already kept are skipped.
func (r *Reader) Ahead(path, digest string) {
	if _, _, ok := r.store.Load(digest, 0); ok {
		r.mu.Lock()
		count := r.counts[digest]
		r.mu.Unlock()
		if count == 0 {
			_, count, _ = r.store.Load(digest, 0)
		}
		r.aheadFrom(path, digest, count)
		return
	}
	r.enqueue(path, digest, 0, false, true)
}

// aheadFrom queues pages 1 up to MaxPages of a PDF of count pages; urgent
// puts them at the front, in page order.
func (r *Reader) aheadFrom(path, digest string, count int, urgent ...bool) {
	now := len(urgent) > 0 && urgent[0]
	last := count
	if last > r.maxPages {
		last = r.maxPages
	}
	// Queued last first when urgent, since each goes to the front.
	for i := 1; i < last; i++ {
		n := i
		if now {
			n = last - i
		}
		if _, _, ok := r.store.Load(digest, n); !ok {
			r.enqueue(path, digest, n, now, false)
		}
	}
}

func (r *Reader) enqueue(path, digest string, page int, urgent, ahead bool) *job {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key{digest, page}
	if j, ok := r.jobs[k]; ok {
		if urgent && !j.urgent {
			// Asked for now: it goes to the front of the queue.
			j.urgent = true
			r.promote(j)
		}
		j.ahead = j.ahead || ahead
		return j
	}
	j := &job{key: k, path: path, urgent: urgent, ahead: ahead, done: make(chan struct{})}
	r.jobs[k] = j
	if urgent {
		r.queue = append([]*job{j}, r.queue...)
	} else {
		r.queue = append(r.queue, j)
	}
	r.wake.Signal()
	return j
}

// promote moves a queued job to the front. A job already being read is not
// in the queue, and is left as it is.
func (r *Reader) promote(j *job) {
	for i, q := range r.queue {
		if q == j {
			copy(r.queue[1:i+1], r.queue[:i])
			r.queue[0] = j
			return
		}
	}
}

func (r *Reader) work() {
	for {
		r.mu.Lock()
		for len(r.queue) == 0 && !r.stopped {
			r.wake.Wait()
		}
		if r.stopped {
			r.mu.Unlock()
			return
		}
		j := r.queue[0]
		r.queue = r.queue[1:]
		r.mu.Unlock()

		j.page, j.count, j.err = r.read(j.path, j.key.page)
		if j.err == nil {
			_ = r.store.Save(j.digest, j.key.page, j.count, j.page)
		}

		r.mu.Lock()
		delete(r.jobs, j.key)
		if j.err == nil {
			r.counts[j.digest] = j.count
		}
		ahead := j.ahead && j.err == nil
		follow := j.follow && j.err == nil
		r.mu.Unlock()
		if follow {
			r.aheadFrom(j.path, j.digest, j.count, true)
		}
		close(j.done)
		if ahead && !follow {
			r.aheadFrom(j.path, j.digest, j.count)
		}
	}
}
