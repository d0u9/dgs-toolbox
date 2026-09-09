package capture

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"dgs-toolbox/internal/tui/divider"

	termimg "github.com/blacktop/go-termimg"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	mp3 "github.com/hajimehoshi/go-mp3"
	"github.com/rwcarlsen/goexif/exif"
)

type property struct {
	name      string
	value     string
	indent    int
	section   bool
	separator bool
	fixed     bool
}

func loadFilePreview(path string, width, height int, jsonHint ...bool) tea.Cmd {
	hint := len(jsonHint) > 0 && jsonHint[0]
	return loadFilePreviewRequest(path, width, height, hint, 0)
}

func loadFilePreviewRequest(path string, width, height int, jsonHint bool, requestID uint64) tea.Cmd {
	return func() tea.Msg {
		props, kind, err := inspectFile(path)
		if err != nil {
			return previewLoadedMsg{path: path, requestID: requestID, properties: props, err: err}
		}
		if strings.HasPrefix(kind, "image/") {
			content, err := renderImagePreview(path, width, height)
			if err == nil {
				content = lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
			}
			return previewLoadedMsg{path: path, requestID: requestID, content: content, properties: props, err: err}
		}

		data, err := readPreview(path)
		if err != nil {
			return previewLoadedMsg{path: path, requestID: requestID, properties: props, err: err}
		}
		isJSON := filepath.Ext(path) == ".json" || kind == "application/json" || jsonHint
		if isJSON {
			var formatted bytes.Buffer
			if err := json.Indent(&formatted, data, "", "  "); err != nil {
				return previewLoadedMsg{path: path, requestID: requestID, properties: props, err: fmt.Errorf("invalid JSON: %w", err)}
			}
			tree, err := newJSONTree(data)
			if err != nil {
				return previewLoadedMsg{path: path, requestID: requestID, properties: props, err: err}
			}
			return previewLoadedMsg{
				path: path, requestID: requestID, properties: props,
				content: withLineNumbers(formatted.String()), tree: tree, isJSON: true,
			}
		}
		if !isText(data, kind) {
			return previewLoadedMsg{path: path, requestID: requestID, properties: props, content: "· No inline preview for this file type", centered: true}
		}
		return previewLoadedMsg{path: path, requestID: requestID, properties: props, content: withLineNumbers(string(data))}
	}
}

func renderImagePreview(path string, width, height int) (string, error) {
	img, err := termimg.Open(path)
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}
	columns, rows := fittedImageCells(img.Bounds.Dx(), img.Bounds.Dy(), width, height)
	// Mosaic combines a 2x2 sample into one terminal cell. Supplying twice the
	// already-fitted cell dimensions makes it render exactly columns × rows;
	// ScaleStretch is safe here because fittedImageCells has already preserved
	// the source aspect ratio for terminal cells.
	rendered, err := img.Protocol(termimg.Halfblocks).
		Width(columns * 2).Height(rows * 2).
		Scale(termimg.ScaleStretch).Dither(true).Render()
	return strings.TrimSuffix(rendered, "\n"), err
}

func fittedImageCells(sourceWidth, sourceHeight, availableWidth, availableHeight int) (int, int) {
	if sourceWidth <= 0 || sourceHeight <= 0 {
		return 1, 1
	}
	availableWidth = max(1, availableWidth)
	availableHeight = max(1, availableHeight)
	const cellHeightToWidth = 2.0
	columnsPerRow := float64(sourceWidth) / float64(sourceHeight) * cellHeightToWidth
	columns := availableWidth
	rows := max(1, int(float64(columns)/columnsPerRow+0.5))
	if rows > availableHeight {
		rows = availableHeight
		columns = max(1, int(float64(rows)*columnsPerRow+0.5))
	}
	return min(columns, availableWidth), min(rows, availableHeight)
}

func loadJSONPreview(path string) tea.Cmd { return loadFilePreview(path, 80, 20, true) }

func inspectFile(path string) ([]property, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", fmt.Errorf("stat file: %w", err)
	}
	kind := detectMIME(path)
	props := []property{
		{name: "Name", value: info.Name()},
		{name: "Type", value: kind},
		{name: "Size", value: formatBytes(info.Size())},
	}
	if created, ok := fileCreatedAt(info); ok {
		props = append(props, property{name: "Created", value: created.Format(time.RFC3339)})
	} else {
		props = append(props, property{name: "Created", value: "Unavailable"})
	}
	props = append(props, property{name: "Modified", value: info.ModTime().Format(time.RFC3339)})

	if strings.HasPrefix(kind, "image/") {
		props = append(props, imageProperties(path)...)
	}
	if duration, ok := audioDuration(path); ok {
		props = append(props, property{name: "Duration", value: formatDuration(duration)})
	}
	return props, kind, nil
}

func audioDuration(path string) (time.Duration, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".wav", ".wave":
		return wavDuration(path)
	case ".mp3":
		file, err := os.Open(path)
		if err != nil {
			return 0, false
		}
		defer file.Close()
		decoder, err := mp3.NewDecoder(file)
		if err != nil || decoder.SampleRate() <= 0 || decoder.Length() < 0 {
			return 0, false
		}
		return time.Duration(float64(decoder.Length()) / float64(decoder.SampleRate()*4) * float64(time.Second)), true
	default:
		return 0, false
	}
}

func wavDuration(path string) (time.Duration, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" {
		return 0, false
	}
	var byteRate uint32
	for {
		chunk := make([]byte, 8)
		if _, err := io.ReadFull(file, chunk); err != nil {
			return 0, false
		}
		size := binary.LittleEndian.Uint32(chunk[4:])
		switch string(chunk[:4]) {
		case "fmt ":
			data := make([]byte, size)
			if _, err := io.ReadFull(file, data); err != nil || len(data) < 12 {
				return 0, false
			}
			byteRate = binary.LittleEndian.Uint32(data[8:12])
		case "data":
			if byteRate == 0 {
				return 0, false
			}
			return time.Duration(float64(size) / float64(byteRate) * float64(time.Second)), true
		default:
			if _, err := file.Seek(int64(size), io.SeekCurrent); err != nil {
				return 0, false
			}
		}
		if size%2 != 0 {
			_, _ = file.Seek(1, io.SeekCurrent)
		}
	}
}

func formatDuration(duration time.Duration) string {
	seconds := int64(duration.Round(time.Second) / time.Second)
	if seconds < 0 {
		seconds = 0
	}
	hours, remainder := seconds/3600, seconds%3600
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, remainder/60, remainder%60)
	}
	return fmt.Sprintf("%d:%02d", remainder/60, remainder%60)
}

func imageProperties(path string) []property {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	config, format, decodeErr := image.DecodeConfig(file)
	_ = file.Close()
	props := make([]property, 0, 8)
	if decodeErr == nil {
		props = append(props,
			property{name: "Format", value: strings.ToUpper(format)},
			property{name: "Pixels", value: fmt.Sprintf("%d × %d", config.Width, config.Height)},
		)
	}
	file, err = os.Open(path)
	if err != nil {
		return props
	}
	defer file.Close()
	x, err := exif.Decode(file)
	if err != nil {
		return props
	}
	if captured, err := x.DateTime(); err == nil {
		props = append(props, property{name: "Captured", value: captured.Format(time.RFC3339)})
	}
	for _, item := range []struct {
		name  string
		field exif.FieldName
	}{
		{"Camera", exif.Model}, {"Maker", exif.Make}, {"Lens", exif.LensModel},
	} {
		if tag, err := x.Get(item.field); err == nil {
			props = append(props, property{name: item.name, value: strings.Trim(tag.String(), `"`)})
		}
	}
	return props
}

func detectMIME(path string) string {
	if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); byExt != "" {
		return strings.Split(byExt, ";")[0]
	}
	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	buffer := make([]byte, 512)
	n, _ := file.Read(buffer)
	return http.DetectContentType(buffer[:n])
}

func isInlineImagePath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	default:
		return false
	}
}

func readPreview(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxPreviewBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if len(data) > maxPreviewBytes {
		return nil, fmt.Errorf("preview exceeds 1 MiB")
	}
	return data, nil
}

func isText(data []byte, kind string) bool {
	return strings.HasPrefix(kind, "text/") || kind == "application/json" || (utf8.Valid(data) && !bytes.ContainsRune(data, '\x00'))
}

func withLineNumbers(text string) string {
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i := range lines {
		lineNumber := strconv.Itoa(i + 1)
		if len(lineNumber) > 3 {
			lineNumber = "###"
		}
		lines[i] = fmt.Sprintf("%3s │ %s", lineNumber, lines[i])
	}
	return strings.Join(lines, "\n")
}

func renderProperties(properties []property, width int, selected int, focused bool, fixedKeyWidth int) string {
	if len(properties) == 0 {
		return scanMutedStyle.Render("· Nothing selected")
	}
	lines := make([]string, 0, len(properties))
	keyWidth := fixedKeyWidth
	if keyWidth <= 0 {
		for _, item := range properties {
			if item.value == "" {
				continue
			}
			keyWidth = max(keyWidth, lipgloss.Width(strings.Repeat("  ", item.indent)+item.name))
		}
	}
	keyWidth = min(keyWidth, max(1, width/3))
	for index, item := range properties {
		if item.separator {
			lines = append(lines, divider.Anchored(width))
			continue
		}
		prefix := strings.Repeat("  ", item.indent)
		if item.section {
			lines = append(lines, scanTitleStyle.Render(ansi.Truncate(prefix+item.name, width, "…")))
			continue
		}
		marker := "  "
		if focused && index == selected {
			marker = "› "
		}
		keyText := ansi.Truncate(prefix+item.name, keyWidth, "…")
		key := marker + strings.Repeat(" ", max(0, keyWidth-lipgloss.Width(keyText))) + keyText
		available := max(1, width-lipgloss.Width(key)-2)
		value := ansi.Truncate(item.value, available, "…")
		row := scanMutedStyle.Render(key) + "  " + value
		if focused && index == selected {
			row = scanFocusedPathStyle.Width(width).Render(key + "  " + value)
		}
		lines = append(lines, row)
	}
	return strings.Join(lines, "\n")
}

func viewportWithMarkers(view viewport.Model, width int) string {
	lines := strings.Split(view.View(), "\n")
	if len(lines) == 0 {
		return ""
	}
	if !view.AtTop() {
		lines[0] = scanMutedStyle.Render(ansi.Truncate("↑ more", width, ""))
	}
	if !view.AtBottom() {
		lines[len(lines)-1] = scanMutedStyle.Render(ansi.Truncate("↓ more", width, ""))
	}
	return strings.Join(lines, "\n")
}

func mapURLLink(url string) string { return ansi.SetHyperlink(url) + url + ansi.ResetHyperlink() }

func appleMapsURL(latitude, longitude float64) string {
	return fmt.Sprintf("https://maps.apple.com/place?coordinate=%.6f,%.6f", latitude, longitude)
}

func googleMapsURL(latitude, longitude float64) string {
	return fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", latitude, longitude)
}

func amapURL(latitude, longitude float64) string {
	return fmt.Sprintf("https://uri.amap.com/marker?position=%.6f,%.6f", longitude, latitude)
}

func formatBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	value := float64(size)
	unit := "B"
	for _, candidate := range units {
		value /= 1024
		unit = candidate
		if value < 1024 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}
