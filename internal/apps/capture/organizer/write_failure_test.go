//go:build darwin || linux

package organizer

import (
	"bytes"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Only the subprocess is limited, so other tests can still write normally.
func TestFailedWritesPreserveNotesAndRecords(t *testing.T) {
	if os.Getenv("DGS_CAPTURE_WRITE_FAILURE") != "1" {
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(executable, "-test.run=^TestFailedWritesPreserveNotesAndRecords$")
		cmd.Env = append(os.Environ(), "DGS_CAPTURE_WRITE_FAILURE=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("write failure check: %v\n%s", err, output)
		}
		return
	}
	ctx, vault := vaultContext(t, beenHere(), strings.Repeat("new content", 200))
	target, err := dailyNotePath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	daily := filepath.Join(vault, target)
	if err := os.MkdirAll(filepath.Dir(daily), 0755); err != nil {
		t.Fatal(err)
	}
	original := []byte("original note must survive a failed write")
	if err := os.WriteFile(daily, original, 0600); err != nil {
		t.Fatal(err)
	}
	timeline := filepath.Join(vault, "timeline.md")
	if err := os.WriteFile(timeline, original, 0640); err != nil {
		t.Fatal(err)
	}
	capture := t.TempDir()
	if _, err := SetFlag(capture, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	recordPath := filepath.Join(capture, RecordFilename)
	recordBefore, err := os.ReadFile(recordPath)
	if err != nil {
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
	_, dailyErr := appendEntry(ctx, nil)
	timelineErr := writeNote(timeline, "timeline.md", strings.Repeat("new content", 200))
	_, appendErr := AppendRun(capture, Run{Recipe: "daily"})
	restoreErr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &limit)
	if restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if dailyErr == nil || timelineErr == nil || appendErr == nil {
		t.Fatalf("write failures ignored: %v %v %v", dailyErr, timelineErr, appendErr)
	}
	for _, path := range []string{daily, timeline} {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, original) {
			t.Fatalf("original note changed: %q %v", after, err)
		}
	}
	after, err := os.ReadFile(recordPath)
	if err != nil || !bytes.Equal(after, recordBefore) {
		t.Fatalf("original record changed: %q %v", after, err)
	}
	for _, dir := range []string{vault, filepath.Dir(daily), capture} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".dgs-part") {
				t.Fatal("temporary file left after failure")
			}
		}
	}
}
