package photo

import (
	"bytes"
	"fmt"
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

func TestTUIProgressUsesCLIRowsAndWorkerGrouping(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := newImportModel().(importModel)
	m.processing = processingState{files: []scannedFile{{path: "photo.JPG"}}, finished: make(map[int]importer.Phase)}
	m.targetProcessing = make([]processingState, 2)
	for target := range m.targetProcessing {
		m.targetProcessing[target] = processingState{files: m.processing.files, finished: make(map[int]importer.Phase), workers: []processingWorker{{number: target + 1, jobIndex: -1}}}
	}
	first := importer.Event{Worker: 1, Index: 0, Source: "photo.JPG", Phase: importer.PhaseCopying, Bytes: 512, Total: 1024}
	second := first
	second.Phase = importer.PhaseVerifying
	second.Bytes = 256
	m.applyMultipleEvent(multipleTransferEventMsg{target: 0, event: first})
	m.applyMultipleEvent(multipleTransferEventMsg{target: 1, event: second})
	cli := importProgress{targets: 2, slots: [][]importer.Event{{first, second}}}
	got := m.workerRows(120, 8)
	expected := strings.Join([]string{fitProgressLine(cli.row(0, 0, first, 28, 120), 120), fitProgressLine(cli.row(0, 1, second, 28, 120), 120)}, "\n")
	// The activity glyph changes every 100 ms; compare the remaining columns.
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			if strings.ContainsRune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", r) {
				return '*'
			}
			return r
		}, s)
	}
	if normalize(got) != normalize(expected) {
		t.Fatalf("TUI and CLI differ:\n%s\n%s", got, expected)
	}
	if m.progressWorkerCount() != 1 {
		t.Fatal("destinations counted as workers")
	}
	m.applyMultipleEvent(multipleTransferEventMsg{target: 0, event: importer.Event{Worker: 1, Index: 0, Source: "photo.JPG", Phase: importer.PhaseComplete}})
	got = m.workerRows(120, 8)
	if !strings.Contains(got, "✓") || !strings.Contains(got, "verifying") {
		t.Fatalf("lost independent target outcome: %s", got)
	}
	for _, width := range []int{24, 48, 80, 120} {
		for _, height := range []int{1, 2, 3, 8} {
			rows := m.workerRows(width, height)
			if len(strings.Split(rows, "\n")) > height {
				t.Fatalf("height overflow: %q", rows)
			}
			for _, line := range strings.Split(rows, "\n") {
				if ansi.StringWidth(line) >= width {
					t.Fatalf("width overflow: %q", line)
				}
			}
		}
	}
}

func TestTUIProgressPreservesNewFileOnLateCompletion(t *testing.T) {
	m := newImportModel().(importModel)
	m.processing = processingState{files: []scannedFile{{path: "previous.JPG"}, {path: "next.JPG"}}, finished: make(map[int]importer.Phase), workers: []processingWorker{{number: 1, jobIndex: -1}}}
	m.applyTransferEvent(importer.Event{Worker: 1, Index: 1, Source: "next.JPG", Phase: importer.PhaseCopying, Bytes: 512, Total: 1024})
	m.applyTransferEvent(importer.Event{Worker: 1, Index: 0, Source: "previous.JPG", Phase: importer.PhaseComplete})
	rows := m.workerRows(120, 8)
	if !strings.Contains(rows, "next.JPG") || strings.Contains(rows, "previous.JPG") {
		t.Fatal(rows)
	}
	if !strings.Contains(rows, "512 B / 1.0 KB") {
		t.Fatal("single destination lost byte progress", rows)
	}
}

func TestTUIProgressScrollsCompleteWorkerGroupsToEnd(t *testing.T) {
	m := newImportModel().(importModel)
	m.targetProcessing = make([]processingState, 2)
	for target := range m.targetProcessing {
		for worker := range 4 {
			m.targetProcessing[target].workers = append(m.targetProcessing[target].workers, processingWorker{event: importer.Event{Source: fmt.Sprintf("file-%d.JPG", worker+1), Phase: importer.PhaseCopying}})
		}
	}
	m.processing.workerOffset = 3
	rows := m.workerRows(120, 5)
	if !strings.Contains(rows, "#4") || !strings.Contains(rows, "file-4.JPG") || strings.Contains(rows, "↓") {
		t.Fatal("last worker inaccessible", rows)
	}
	if len(strings.Split(rows, "\n")) > 5 {
		t.Fatal("scroll overflow", rows)
	}
}

func TestTUIOverallProgressCountsFinishedFilesLikeCLI(t *testing.T) {
	m := newImportModel().(importModel)
	m.processing = processingState{files: []scannedFile{{}, {}}, finished: make(map[int]importer.Phase), workers: []processingWorker{{jobIndex: -1}}}
	m.applyTransferEvent(importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseVerifying, Bytes: 1024, Total: 1024})
	if m.processingPercent() != 0 {
		t.Fatal("bytes counted as a finished file")
	}
	m.applyTransferEvent(importer.Event{Worker: 1, Index: 0, Phase: importer.PhaseFailed})
	if m.processingPercent() != 50 || len(m.processing.verified) != 0 {
		t.Fatal("failed outcome progress differs from CLI")
	}
}
