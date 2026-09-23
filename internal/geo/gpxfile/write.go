package gpxfile

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"dgs-toolbox/internal/geo"
)

// ErrExists is returned by Create when the file is already there.
var ErrExists = errors.New("file already exists")

// Creator identifies GPX files written by dgs and eligible for in-place edits.
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
	return RenamePart(path, PartTrack, index, name)
}

// RenamePart replaces the <name> of the index-th <trk>, <rte> or <wpt> of a
// file written by this program, keeping every other byte. A file created by
// something else is refused.
func RenamePart(path string, part Part, index int, name string) error {
	data, parsed, err := readOurs(path)
	if err != nil {
		return err
	}
	if index < 0 || index >= parsed.count(part) {
		return fmt.Errorf("%s: no %s %d", path, part, index)
	}
	start, end, err := partSpan(data, part, index)
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
		// The part has no <name>: it opens one right after its own tag.
		tag := bytes.IndexByte(data[start:end], '>')
		if tag < 0 {
			return fmt.Errorf("%s: %s %d is not closed", path, part, index)
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

// AddWaypoint inserts one standalone <wpt> into a dgs-created GPX, preserving
// its tracks, routes and embedded state byte for byte.
func AddWaypoint(path string, waypoint Waypoint) error {
	if math.IsNaN(waypoint.Lat) || math.IsNaN(waypoint.Lon) || math.Abs(waypoint.Lat) > 90 || math.Abs(waypoint.Lon) > 180 {
		return errors.New("waypoint coordinates are outside the globe")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	file, err := Parse(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if !file.IsOurs() {
		return fmt.Errorf("%s: %w", path, ErrNotOurs)
	}
	end := bytes.LastIndex(data, []byte("</gpx>"))
	if end < 0 {
		return errors.New("GPX root is not closed")
	}
	for _, tag := range [][]byte{[]byte("<rte>"), []byte("<trk>"), []byte("<extensions>")} {
		if at := bytes.Index(data, tag); at >= 0 && at < end {
			end = at
		}
	}
	var b bytes.Buffer
	b.Write(data[:end])
	fmt.Fprintf(&b, "  <wpt lat=\"%s\" lon=\"%s\">", coordinate(waypoint.Lat), coordinate(waypoint.Lon))
	if waypoint.HasElevation {
		fmt.Fprintf(&b, "<ele>%s</ele>", strconv.FormatFloat(waypoint.Elevation, 'f', -1, 64))
	}
	if !waypoint.Time.IsZero() {
		b.WriteString("<time>" + waypoint.Time.UTC().Format(time.RFC3339) + "</time>")
	}
	if waypoint.Name != "" {
		b.WriteString("<name>")
		_ = xml.EscapeText(&b, []byte(waypoint.Name))
		b.WriteString("</name>")
	}
	if waypoint.Description != "" {
		b.WriteString("<desc>")
		_ = xml.EscapeText(&b, []byte(waypoint.Description))
		b.WriteString("</desc>")
	}
	b.WriteString("</wpt>\n")
	b.Write(data[end:])
	return replaceFile(path, b.Bytes())
}

// elementSpan finds the text of the first <tag> of element, before any nested
// <trkseg>. It returns the offsets its text starts and ends at, or -1 when the
// element has no such child.
func elementSpan(element []byte, tag string) (int, int) {
	limit := len(element)
	// A name inside a point of the element is not the element's own.
	for _, nested := range []string{"<trkseg", "<trkpt", "<rtept"} {
		if at := bytes.Index(element, []byte(nested)); at >= 0 && at < limit {
			limit = at
		}
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

// Part names the three kinds of element a file holds one after another.
type Part string

const (
	PartTrack    Part = "trk"
	PartRoute    Part = "rte"
	PartWaypoint Part = "wpt"
)

// RemovePart takes the index-th <trk>, <rte> or <wpt> out of a file written
// by this program, keeping every other byte, and replaces the file in one
// step. A file created by something else is refused, so a recording is never
// rewritten.
func RemovePart(path string, part Part, index int) error {
	data, parsed, err := readOurs(path)
	if err != nil {
		return err
	}
	if index < 0 || index >= parsed.count(part) {
		return fmt.Errorf("%s: no %s %d", path, part, index)
	}
	start, end, err := partSpan(data, part, index)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// The line the element sits on goes with it, so no blank line is left.
	for start > 0 && (data[start-1] == ' ' || data[start-1] == '\t') {
		start--
	}
	if end < len(data) && data[end] == '\n' {
		end++
	}
	var b bytes.Buffer
	b.Write(data[:start])
	b.Write(data[end:])
	return replaceFile(path, b.Bytes())
}

// MoveWaypoint puts the index-th <wpt> of a file written by this program at
// another position, keeping everything else it holds. A file created by
// something else is refused.
func MoveWaypoint(path string, index int, position geo.LatLon) error {
	if math.IsNaN(position.Lat) || math.IsNaN(position.Lon) || math.Abs(position.Lat) > 90 || math.Abs(position.Lon) > 180 {
		return errors.New("waypoint coordinates are outside the globe")
	}
	data, parsed, err := readOurs(path)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(parsed.Waypoints) {
		return fmt.Errorf("%s: no waypoint %d", path, index)
	}
	start, end, err := partSpan(data, PartWaypoint, index)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	open := bytes.IndexByte(data[start:end], '>')
	if open < 0 {
		return fmt.Errorf("%s: waypoint %d is not closed", path, index)
	}
	tag := string(data[start : start+open+1])
	for attribute, value := range map[string]string{"lat": coordinate(position.Lat), "lon": coordinate(position.Lon)} {
		replaced, err := setAttribute(tag, attribute, value)
		if err != nil {
			return fmt.Errorf("%s: waypoint %d: %w", path, index, err)
		}
		tag = replaced
	}
	var b bytes.Buffer
	b.Write(data[:start])
	b.WriteString(tag)
	b.Write(data[start+open+1:])
	return replaceFile(path, b.Bytes())
}

// AddRoute inserts one <rte> into a file written by this program, before its
// tracks, as GPX orders a document. A file created by something else is
// refused.
func AddRoute(path string, route Route) error {
	data, _, err := readOurs(path)
	if err != nil {
		return err
	}
	end := bytes.LastIndex(data, []byte("</gpx>"))
	if end < 0 {
		return fmt.Errorf("%s: no closing </gpx>", path)
	}
	for _, tag := range [][]byte{[]byte("<trk>"), []byte("<trk "), []byte("<extensions>")} {
		if at := bytes.Index(data, tag); at >= 0 && at < end {
			end = at
		}
	}
	for end > 0 && (data[end-1] == ' ' || data[end-1] == '\t') {
		end--
	}
	var b bytes.Buffer
	b.Write(data[:end])
	b.WriteString("  <rte>\n")
	if route.Name != "" {
		b.WriteString("    <name>")
		_ = xml.EscapeText(&b, []byte(route.Name))
		b.WriteString("</name>\n")
	}
	for _, pt := range route.Points {
		fmt.Fprintf(&b, "    <rtept lat=\"%s\" lon=\"%s\"></rtept>\n", coordinate(pt.Lat), coordinate(pt.Lon))
	}
	b.WriteString("  </rte>\n")
	b.Write(data[end:])
	return replaceFile(path, b.Bytes())
}

// readOurs reads a GPX this program wrote, refusing any other file so a
// recording is never rewritten.
func readOurs(path string) ([]byte, *File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := Parse(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if !parsed.IsOurs() {
		return nil, nil, fmt.Errorf("%s: %w", path, ErrNotOurs)
	}
	return data, parsed, nil
}

func (f *File) count(part Part) int {
	switch part {
	case PartTrack:
		return len(f.Tracks)
	case PartRoute:
		return len(f.Routes)
	}
	return len(f.Waypoints)
}

// setAttribute replaces the value of one attribute of an opening tag.
func setAttribute(tag, name, value string) (string, error) {
	at := strings.Index(tag, " "+name+"=")
	if at < 0 {
		return "", fmt.Errorf("no %s attribute", name)
	}
	quote := at + len(name) + 2
	if quote >= len(tag) || (tag[quote] != '"' && tag[quote] != '\'') {
		return "", fmt.Errorf("the %s attribute is not quoted", name)
	}
	end := strings.IndexByte(tag[quote+1:], tag[quote])
	if end < 0 {
		return "", fmt.Errorf("the %s attribute is not closed", name)
	}
	return tag[:quote+1] + value + tag[quote+1+end:], nil
}

// partSpan returns the byte range of the index-th <trk>, <rte> or <wpt>
// element, from its opening angle bracket to the end of its closing tag. The
// elements nested inside them — <trkseg>, <trkpt>, <rtept> — start with the
// same letters and are passed over.
func partSpan(data []byte, part Part, index int) (int, int, error) {
	open, seen := []byte("<"+string(part)), 0
	at := 0
	for {
		next := bytes.Index(data[at:], open)
		if next < 0 {
			return 0, 0, fmt.Errorf("no %s %d", part, index)
		}
		start := at + next
		after := data[start+len(open):]
		at = start + len(open)
		if len(after) == 0 || (after[0] != '>' && after[0] != '/' && after[0] != ' ' && after[0] != '\t' && after[0] != '\n' && after[0] != '\r') {
			continue // <trkseg>, <trkpt> or <rtept>
		}
		if seen != index {
			seen++
			continue
		}
		tag := bytes.IndexByte(data[start:], '>')
		if tag < 0 {
			return 0, 0, fmt.Errorf("%s %d is not closed", part, index)
		}
		// An element written as <wpt … /> holds nothing and ends with its tag.
		if data[start+tag-1] == '/' {
			return start, start + tag + 1, nil
		}
		stop := bytes.Index(data[start:], []byte("</"+string(part)+">"))
		if stop < 0 {
			return 0, 0, fmt.Errorf("%s %d is not closed", part, index)
		}
		return start, start + stop + len("</"+string(part)+">"), nil
	}
}

// ReorderParts puts the <trk>, <rte> or <wpt> elements of a file written by
// this program in another order, keeping every byte of each element. order is
// the current index of the element that goes first, then second, and so on,
// so it names every element of that kind exactly once. A file created by
// something else is refused, so a recording is never rewritten.
//
// Only whitespace may separate the elements: anything else between them — a
// comment, an extension — belongs to a place in the file this cannot keep, so
// the file is left as it is.
func ReorderParts(path string, part Part, order []int) error {
	data, parsed, err := readOurs(path)
	if err != nil {
		return err
	}
	count := parsed.count(part)
	if len(order) != count {
		return fmt.Errorf("%s: %d %s in the file, %d in the order", path, count, part, len(order))
	}
	seen := make([]bool, count)
	for _, index := range order {
		if index < 0 || index >= count || seen[index] {
			return fmt.Errorf("%s: the order does not name every %s once", path, part)
		}
		seen[index] = true
	}
	// The elements as they are on disk, each with the indentation before it.
	type span struct{ start, end int }
	spans := make([]span, count)
	for i := range spans {
		start, end, err := partSpan(data, part, i)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for start > 0 && (data[start-1] == ' ' || data[start-1] == '\t') {
			start--
		}
		spans[i] = span{start, end}
	}
	if count < 2 {
		return nil
	}
	for i := 1; i < count; i++ {
		if spans[i].start < spans[i-1].end {
			return fmt.Errorf("%s: the %s elements overlap", path, part)
		}
		if between := bytes.TrimSpace(data[spans[i-1].end:spans[i].start]); len(between) > 0 {
			return fmt.Errorf("%s: something sits between the %s elements; they are left as they are", path, part)
		}
	}
	same := true
	for i, index := range order {
		if i != index {
			same = false
			break
		}
	}
	if same {
		return nil
	}
	var b bytes.Buffer
	b.Write(data[:spans[0].start])
	for i, index := range order {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.Write(data[spans[index].start:spans[index].end])
	}
	b.Write(data[spans[count-1].end:])
	return replaceFile(path, b.Bytes())
}
