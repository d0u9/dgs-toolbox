//go:build darwin || linux

package recipients

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
)

// Limit only a subprocess so a real partial-write failure cannot affect other tests.
func TestWriteJSONFailureKeepsOriginal(t *testing.T) {
	if os.Getenv("DGS_TEST_WRITE_FAILURE") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestWriteJSONFailureKeepsOriginal$")
		cmd.Env = append(os.Environ(), "DGS_TEST_WRITE_FAILURE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write failure check: %v\n%s", err, output)
		}
		return
	}
	root := t.TempDir()
	path := filepath.Join(root, "g-team.json")
	original := []byte(`{"hosts":["host-a"]}`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &limit); err != nil {
		t.Fatal(err)
	}
	signal.Ignore(syscall.SIGXFSZ)
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 1, Max: limit.Max}); err != nil {
		t.Fatal(err)
	}
	err := writeJSON(root, "g-team", groupFile{Hosts: []string{"host-b"}})
	restoreErr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if err == nil {
		t.Fatal("write failure was ignored")
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatalf("original replaced: %q %v", data, err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 {
		t.Fatalf("temporary files left: %v", entries)
	}
}
