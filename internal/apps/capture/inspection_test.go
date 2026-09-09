package capture

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

func TestLineNumberGutterStaysThreeCellsWide(t *testing.T) {
	lines := make([]string, 1000)
	for index := range lines {
		lines[index] = "text"
	}
	got := strings.Split(withLineNumbers(strings.Join(lines, "\n")), "\n")
	for _, test := range []struct {
		line int
		want string
	}{{1, "  1 │ text"}, {99, " 99 │ text"}, {100, "100 │ text"}, {1000, "### │ text"}} {
		if got[test.line-1] != test.want {
			t.Fatalf("line %d = %q, want %q", test.line, got[test.line-1], test.want)
		}
	}
}

func TestPortraitImagePreviewFillsAvailableHeight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portrait.png")
	canvas := image.NewRGBA(image.Rect(0, 0, 100, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 100; x++ {
			canvas.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, canvas); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	rendered, err := renderImagePreview(path, 80, 40)
	if err != nil {
		t.Fatal(err)
	}
	if got := lipgloss.Height(rendered); got < 38 || got > 40 {
		t.Fatalf("rendered height = %d, want image to fill approximately 40 rows (width %d)", got, lipgloss.Width(rendered))
	}
}

func TestLandscapeImagePreviewFillsAvailableWidth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "landscape.png")
	canvas := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			canvas.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, canvas); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	rendered, err := renderImagePreview(path, 80, 40)
	if err != nil {
		t.Fatal(err)
	}
	if got := lipgloss.Width(rendered); got < 78 || got > 80 {
		t.Fatalf("rendered width = %d, want image to fill approximately 80 columns (height %d)", got, lipgloss.Height(rendered))
	}
}

func TestImageCellFitPreservesVisualAspectRatio(t *testing.T) {
	for _, test := range []struct {
		name                            string
		sourceWidth, sourceHeight       int
		availableWidth, availableHeight int
		wantColumns, wantRows           int
	}{
		{name: "portrait fills height", sourceWidth: 100, sourceHeight: 200, availableWidth: 80, availableHeight: 40, wantColumns: 40, wantRows: 40},
		{name: "landscape fills width", sourceWidth: 200, sourceHeight: 100, availableWidth: 80, availableHeight: 40, wantColumns: 80, wantRows: 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			columns, rows := fittedImageCells(test.sourceWidth, test.sourceHeight, test.availableWidth, test.availableHeight)
			if columns != test.wantColumns || rows != test.wantRows {
				t.Fatalf("fitted cells = %d×%d, want %d×%d", columns, rows, test.wantColumns, test.wantRows)
			}
		})
	}
}

func TestMapURLsUseProviderCoordinateOrder(t *testing.T) {
	if got := appleMapsURL(-33.7, 151.1); !strings.Contains(got, "/place?coordinate=-33.700000,151.100000") {
		t.Fatalf("Apple Maps URL = %q", got)
	}
	if got := googleMapsURL(-33.7, 151.1); !strings.Contains(got, "query=-33.700000,151.100000") {
		t.Fatalf("Google Maps URL = %q", got)
	}
	if got := amapURL(-33.7, 151.1); !strings.Contains(got, "position=151.100000,-33.700000") {
		t.Fatalf("Amap URL = %q", got)
	}
}

func TestViewportShowsVerticalOverflowMarkers(t *testing.T) {
	view := viewport.New(20, 2)
	view.SetContent("one\ntwo\nthree\nfour")
	if got := viewportWithMarkers(view, 20); !strings.Contains(got, "↓ more") {
		t.Fatalf("top viewport lacks lower overflow marker: %q", got)
	}
	view.GotoBottom()
	if got := viewportWithMarkers(view, 20); !strings.Contains(got, "↑ more") {
		t.Fatalf("bottom viewport lacks upper overflow marker: %q", got)
	}
}

func TestWAVDuration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.wav")
	var data bytes.Buffer
	data.WriteString("RIFF")
	_ = binary.Write(&data, binary.LittleEndian, uint32(36+16000))
	data.WriteString("WAVEfmt ")
	_ = binary.Write(&data, binary.LittleEndian, uint32(16))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint16(1))
	_ = binary.Write(&data, binary.LittleEndian, uint32(8000))
	_ = binary.Write(&data, binary.LittleEndian, uint32(16000))
	_ = binary.Write(&data, binary.LittleEndian, uint16(2))
	_ = binary.Write(&data, binary.LittleEndian, uint16(16))
	data.WriteString("data")
	_ = binary.Write(&data, binary.LittleEndian, uint32(16000))
	data.Write(make([]byte, 16000))
	if err := os.WriteFile(path, data.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	duration, ok := audioDuration(path)
	if !ok || formatDuration(duration) != "0:01" {
		t.Fatalf("WAV duration = %v, %v", duration, ok)
	}
}
