package filebrowse_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"dgs-toolbox/internal/filebrowse"
)

func TestConcurrentRenameNeverOverwritesTheWinner(t *testing.T) {
	dir := t.TempDir()
	const count = 32
	for i := 0; i < count; i++ {
		write(t, filepath.Join(dir, fmt.Sprintf("source-%d", i)))
	}
	start := make(chan struct{})
	results := make(chan error, count)
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := filebrowse.Rename(filepath.Join(dir, fmt.Sprintf("source-%d", i)), "target")
			results <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, filebrowse.ErrExists) {
			t.Error(err)
		}
	}
	if winners != 1 {
		t.Fatalf("%d renames overwrote the destination", winners)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != count {
		t.Fatal("rename lost a source", err)
	}
}
