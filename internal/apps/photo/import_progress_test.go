package photo

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/apps/photo/importer"
	"github.com/charmbracelet/x/ansi"
)

func TestImportProgressShowsConcurrentWorkersWithoutFalseCompletion(t *testing.T) {
	var out bytes.Buffer
	p := newImportProgress(&out, 2, 3, 2)
	p.enabled = true
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseCopying, Bytes: 512, Total: 1024, Source: "a.JPG"})
	p.update(0, importer.Event{Worker: 2, Index: 1, Phase: importer.PhaseVerifying, Bytes: 1024, Total: 1024, Source: "photo\n.JPG"})
	frame := ansi.Strip(strings.Join(p.frame(120, 24), "\n"))
	for _, want := range []string{"0/3 files", "0%", "#1", "copying", "512 B / 1.0 KB", "a.JPG", "#2", "verifying", "1.0 KB / 1.0 KB", "photo .JPG"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("missing %q: %s", want, frame)
		}
	}
	if strings.Contains(frame, "\x1b") {
		t.Fatal("progress has styling escapes")
	}
	if len(p.frame(120, 24)) != 6 {
		t.Fatal("missing worker rows: one per destination", p.frame(120, 24))
	}
	for _, width := range []int{12, 32, 48, 80} {
		for _, line := range p.frame(width, 24) {
			if strings.ContainsAny(line, "\n\r") || ansi.StringWidth(line) >= width {
				t.Fatalf("line wraps at %d: %q", width, line)
			}
		}
	}
	if frame := p.frame(80, 3); len(frame) > 2 {
		t.Fatal("short terminal overflow", frame)
	}
}

func TestImportProgressRedrawsFixedRegionAndClearsBeforeLogs(t *testing.T) {
	var out bytes.Buffer
	p := newImportProgress(&out, 2, 3, 2)
	p.enabled = true
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseCopying})
	p.lastDraw = time.Time{}
	out.Reset()
	p.update(0, importer.Event{Worker: 2, Index: 1, Phase: importer.PhaseVerifying})
	if strings.Count(out.String(), "\x1b[1A") != 5 || p.rows != 6 {
		t.Fatal("redraw did not reuse worker region", out.String())
	}
	if _, err := p.Write([]byte("warning\n")); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(out.String(), "\r\x1b[2Kwarning\n") || p.rows != 0 {
		t.Fatal("log overwrote progress region", out.String())
	}
	p.clear()
	if p.rows != 0 {
		t.Fatal("progress left active on exit")
	}
}

func TestImportProgressLateCompletionDoesNotReplaceNextFile(t *testing.T) {
	p := newImportProgress(&bytes.Buffer{}, 1, 3, 2)
	p.enabled = true
	p.update(0, importer.Event{Worker: 1, Index: 1, Phase: importer.PhaseCopying, Source: "next.JPG"})
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseComplete, Source: "previous.JPG"})
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseComplete})
	if p.finished[0] != 2 || p.slots[0][0].Source != "next.JPG" {
		t.Fatal("late completion corrupted worker progress")
	}
}

func TestImportProgressLeavesNonTerminalOutputPlain(t *testing.T) {
	var out bytes.Buffer
	p := newImportProgress(&out, 2, 2, 2)
	p.update(0, importer.Event{Worker: 1, Phase: importer.PhaseCopying})
	if _, err := p.Write([]byte("complete photo.JPG\n")); err != nil {
		t.Fatal(err)
	}
	p.clear()
	if out.String() != "complete photo.JPG\n" {
		t.Fatal("nonterminal output contains rendering", out.String())
	}
}

func TestImportProgressKeepsDestinationNotesBelowWorkers(t *testing.T) {
	p := newImportProgress(&bytes.Buffer{}, 2, 3, 2)
	p.destinations = []string{"/library-a", "/library-b"}
	p.summaries = []string{"Destination 1/2: 3 imported, 0 need attention", ""}
	p.slots[0][0] = importer.Event{Source: "photo.JPG", Phase: importer.PhaseCopying}
	lines := p.frame(120, 24)
	if !strings.Contains(lines[len(lines)-2], "3 imported") || !strings.Contains(lines[len(lines)-1], "Destination 2/2: /library-b") {
		t.Fatal("destination notes not at bottom", lines)
	}
	if !strings.Contains(lines[2], "photo.JPG") {
		t.Fatal("notes displaced worker row", lines)
	}
	for _, height := range []int{3, 4, 5, 6} {
		if lines := p.frame(80, height); len(lines) >= height {
			t.Fatalf("footer overflow at height %d: %v", height, lines)
		}
	}
}

func TestImportProgressShowsEachDestinationUnderItsWorker(t *testing.T) {
	p := newImportProgress(&bytes.Buffer{}, 2, 2, 1)
	p.enabled = true
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseCopying, Bytes: 512, Total: 1024, Source: "a.JPG"})
	p.update(1, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseVerifying, Bytes: 256, Total: 1024, Source: "a.JPG"})
	lines := p.frame(120, 24)
	first, second := ansi.Strip(lines[2]), ansi.Strip(lines[3])
	if !strings.Contains(first, "#1") || !strings.Contains(first, "a.JPG") || !strings.Contains(first, "1  copying") {
		t.Fatal("first destination row", first)
	}
	if strings.Contains(second, "#1") || strings.Contains(second, "a.JPG") || !strings.Contains(second, "2  verifying") || !strings.Contains(second, "256 B / 1.0 KB") {
		t.Fatal("second destination row", second)
	}
	p.update(0, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseComplete})
	if p.done() != 0 {
		t.Fatal("file counted before every destination finished it")
	}
	p.update(1, importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseComplete})
	if p.done() != 1 {
		t.Fatal("file finished in every destination not counted")
	}
}
