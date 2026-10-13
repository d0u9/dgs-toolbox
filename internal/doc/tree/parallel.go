package tree

import (
	"context"
	"sync"
)

// FileWorkers bounds concurrent file reads. A network share answers each
// open slowly, so a few reads in flight hide most of that latency.
const FileWorkers = 8

// Each calls fn for 0..n-1 on at most workers goroutines. It stops handing
// out work after the first error or cancellation and returns that error.
func Each(ctx context.Context, n, workers int, fn func(i int) error) error {
	var (
		wg    sync.WaitGroup
		once  sync.Once
		first error
	)
	stop := make(chan struct{})
	fail := func(err error) { once.Do(func() { first = err; close(stop) }) }
	jobs := make(chan int)
	for range min(workers, n) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := fn(i); err != nil {
					fail(err)
				}
			}
		}()
	}
feed:
	for i := range n {
		select {
		case jobs <- i:
		case <-stop:
			break feed
		case <-ctx.Done():
			fail(ctx.Err())
			break feed
		}
	}
	close(jobs)
	wg.Wait()
	return first
}
