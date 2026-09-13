package gpxfile

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ErrExists is returned by Create when the file is already there.
var ErrExists = errors.New("file already exists")

// Creator is written as the creator of new files.
const Creator = "dgs-toolbox"

// EncodeTracks writes tracks as <trk> elements, each segment a <trkseg>. Only
// what a Point holds is written: position, and elevation, time, satellites and
// HDOP when present, and the source of a point that was not recorded.
func EncodeTracks(w io.Writer, tracks []Track) error {
	var b bytes.Buffer
	for _, trk := range tracks {
		b.WriteString("  <trk>\n")
		if trk.Name != "" {
			b.WriteString("    <name>")
			_ = xml.EscapeText(&b, []byte(trk.Name))
			b.WriteString("</name>\n")
		}
		for _, seg := range trk.Segments {
			b.WriteString("    <trkseg>\n")
			for _, pt := range seg.Points {
				fmt.Fprintf(&b, `      <trkpt lat="%s" lon="%s">`, coordinate(pt.Lat), coordinate(pt.Lon))
				if pt.HasElevation {
					fmt.Fprintf(&b, "<ele>%s</ele>", strconv.FormatFloat(pt.Elevation, 'f', -1, 64))
				}
				if !pt.Time.IsZero() {
					fmt.Fprintf(&b, "<time>%s</time>", pt.Time.UTC().Format(time.RFC3339Nano))
				}
				if pt.Source != "" {
					b.WriteString("<src>")
					_ = xml.EscapeText(&b, []byte(pt.Source))
					b.WriteString("</src>")
				}
				if pt.HasSatellites {
					fmt.Fprintf(&b, "<sat>%d</sat>", pt.Satellites)
				}
				if pt.HasHDOP {
					fmt.Fprintf(&b, "<hdop>%s</hdop>", strconv.FormatFloat(pt.HDOP, 'f', -1, 64))
				}
				b.WriteString("</trkpt>\n")
			}
			b.WriteString("    </trkseg>\n")
		}
		b.WriteString("  </trk>\n")
	}
	_, err := w.Write(b.Bytes())
	return err
}

// coordinate keeps seven decimals, about a centimetre.
func coordinate(degrees float64) string { return strconv.FormatFloat(degrees, 'f', 7, 64) }

// Create writes a new GPX 1.1 file holding tracks. It refuses to replace a
// file that is already there.
func Create(path string, name string, tracks []Track) error {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx version="1.1" creator="` + Creator + `" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
	if name != "" {
		b.WriteString("  <metadata><name>")
		_ = xml.EscapeText(&b, []byte(name))
		b.WriteString("</name></metadata>\n")
	}
	if err := EncodeTracks(&b, tracks); err != nil {
		return err
	}
	b.WriteString("</gpx>\n")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s: %w", path, ErrExists)
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(b.Bytes()); err != nil {
		file.Close()
		os.Remove(path)
		return err
	}
	return file.Close()
}

// Append adds tracks after the last element of an existing GPX file: before
// its closing </gpx>, or before the file's own <extensions>, which GPX 1.1
// requires to come last. The rest of the file is kept byte for byte — its waypoints,
// routes, tracks and anything this package does not read — and the file is
// replaced in one step. A file that does not parse as GPX is refused.
func Append(path string, tracks []Track) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if _, err := Parse(bytes.NewReader(data)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	end := bytes.LastIndex(data, []byte("</gpx>"))
	if end < 0 {
		return fmt.Errorf("%s: no closing </gpx>", path)
	}
	// A root <extensions> opens after the last track, route and waypoint closes.
	if ext := bytes.LastIndex(data[:end], []byte("<extensions")); ext >= 0 {
		last := max(
			bytes.LastIndex(data[:end], []byte("</trk>")),
			bytes.LastIndex(data[:end], []byte("</rte>")),
			bytes.LastIndex(data[:end], []byte("</wpt>")),
			bytes.LastIndex(data[:end], []byte("</metadata>")),
		)
		if ext > last {
			end = bytes.LastIndex(data[:ext], []byte("\n")) + 1
		}
	}
	var b bytes.Buffer
	b.Write(data[:end])
	if end > 0 && data[end-1] != '\n' {
		b.WriteByte('\n')
	}
	if err := EncodeTracks(&b, tracks); err != nil {
		return err
	}
	b.Write(data[end:])

	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(b.Bytes()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Chmod(info.Mode().Perm()); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}
