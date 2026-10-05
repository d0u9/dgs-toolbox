package verifiedcopy

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// The cache requests are best effort in Copy, so a broken one would fail
// silently there. Check them directly where the platform supports them.
func TestCacheRequestsSucceedOnLocalFiles(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("no cache control on " + runtime.GOOS)
	}
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := bypassCache(file); err != nil {
		t.Fatalf("bypassCache: %v", err)
	}
	if err := file.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := dropCache(file); err != nil {
		t.Fatalf("dropCache: %v", err)
	}
}
