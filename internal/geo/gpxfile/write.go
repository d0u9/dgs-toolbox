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
	return CreateWith(path, name, nil, tracks)
}

// CreateWith writes a new GPX 1.1 file holding routes, then tracks, as GPX
// orders them. It refuses to replace a file that is already there.
func CreateWith(path string, name string, routes []Route, tracks []Track) error {
	return CreateAll(path, name, nil, routes, tracks)
}

// CreateAll writes a new GPX 1.1 file holding waypoints, routes and tracks,
// in the order GPX gives them. It refuses to replace a file that is already
// there.
func CreateAll(path string, name string, waypoints []Waypoint, routes []Route, tracks []Track) error {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<gpx version="1.1" creator="` + Creator + `" xmlns="http://www.topografix.com/GPX/1/1">` + "\n")
	if name != "" {
		b.WriteString("  <metadata><name>")
		_ = xml.EscapeText(&b, []byte(name))
		b.WriteString("</name></metadata>\n")
	}
	for _, wpt := range waypoints {
		fmt.Fprintf(&b, "  <wpt lat=\"%s\" lon=\"%s\">", coordinate(wpt.Lat), coordinate(wpt.Lon))
		if wpt.HasElevation {
			fmt.Fprintf(&b, "<ele>%s</ele>", strconv.FormatFloat(wpt.Elevation, 'f', -1, 64))
		}
		if !wpt.Time.IsZero() {
			b.WriteString("<time>" + wpt.Time.UTC().Format(time.RFC3339) + "</time>")
		}
		if wpt.Name != "" {
			b.WriteString("<name>")
			_ = xml.EscapeText(&b, []byte(wpt.Name))
			b.WriteString("</name>")
		}
		if wpt.Description != "" {
			b.WriteString("<desc>")
			_ = xml.EscapeText(&b, []byte(wpt.Description))
			b.WriteString("</desc>")
		}
		b.WriteString("</wpt>\n")
	}
	for _, rte := range routes {
		b.WriteString("  <rte>\n")
		if rte.Name != "" {
			b.WriteString("    <name>")
			_ = xml.EscapeText(&b, []byte(rte.Name))
			b.WriteString("</name>\n")
		}
		for _, pt := range rte.Points {
			fmt.Fprintf(&b, "    <rtept lat=\"%s\" lon=\"%s\"></rtept>\n", coordinate(pt.Lat), coordinate(pt.Lon))
		}
		b.WriteString("  </rte>\n")
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
	return replaceFile(path, b.Bytes())
}

// IsOurs reports whether a parsed file was written by this program, and so
// may be edited in place rather than through a sidecar.
func (f *File) IsOurs() bool { return f != nil && f.Creator == Creator }

// RenameTrack replaces the <name> of the index-th <trk> of a file written by
// this program, keeping every other byte, and replaces the file in one step.
// A file created by something else is refused, so a recording is never
// rewritten.
func RenameTrack(path string, index int, name string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	parsed, err := Parse(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if !parsed.IsOurs() {
		return fmt.Errorf("%s: %w", path, ErrNotOurs)
	}
	if index < 0 || index >= len(parsed.Tracks) {
		return fmt.Errorf("%s: no track %d", path, index)
	}
	start, end, err := trackSpan(data, index)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(name)); err != nil {
		return err
	}
	var b bytes.Buffer
	b.Write(data[:start])
	textFrom, textTo := elementSpan(data[start:end], "name")
	if textFrom < 0 {
		// The track has no <name>: it opens one right after the <trk> tag.
		tag := bytes.IndexByte(data[start:end], '>')
		if tag < 0 {
			return fmt.Errorf("%s: track %d is not closed", path, index)
		}
		b.Write(data[start : start+tag+1])
		if name != "" {
			b.WriteString("\n    <name>")
			b.Write(escaped.Bytes())
			b.WriteString("</name>")
		}
		b.Write(data[start+tag+1 : end])
	} else {
		b.Write(data[start : start+textFrom])
		b.Write(escaped.Bytes())
		b.Write(data[start+textTo : end])
	}
	b.Write(data[end:])
	return replaceFile(path, b.Bytes())
}

// ErrNotOurs is returned when a file this program did not write would be
// changed in place.
var ErrNotOurs = errors.New("the file was not written by " + Creator)

// trackSpan returns the byte range of the index-th <trk> element, from its
// opening angle bracket to the end of its </trk>.
func trackSpan(data []byte, index int) (int, int, error) {
	at, seen := 0, 0
	for {
		next := bytes.Index(data[at:], []byte("<trk"))
		if next < 0 {
			return 0, 0, fmt.Errorf("no track %d", index)
		}
		start := at + next
		after := data[start+4:]
		at = start + 4
		if len(after) == 0 || (after[0] != '>' && after[0] != ' ' && after[0] != '\t' && after[0] != '\n' && after[0] != '\r') {
			continue // <trkseg> or <trkpt>
		}
		if seen != index {
			seen++
			continue
		}
		stop := bytes.Index(data[start:], []byte("</trk>"))
		if stop < 0 {
			return 0, 0, fmt.Errorf("track %d is not closed", index)
		}
		return start, start + stop + len("</trk>"), nil
	}
}

// elementSpan finds the text of the first <tag> of element, before any nested
// <trkseg>. It returns the offsets its text starts and ends at, or -1 when the
// element has no such child.
func elementSpan(element []byte, tag string) (int, int) {
	limit := bytes.Index(element, []byte("<trkseg"))
	if limit < 0 {
		limit = len(element)
	}
	from := bytes.Index(element[:limit], []byte("<"+tag+">"))
	if from < 0 {
		return -1, -1
	}
	to := bytes.Index(element[from:limit], []byte("</"+tag+">"))
	if to < 0 {
		return -1, -1
	}
	return from + len("<"+tag+">"), from + to
}

// replaceFile writes data over path through a temporary file in its folder,
// so a reader sees either the old file or the new one.
func replaceFile(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err != nil {
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
