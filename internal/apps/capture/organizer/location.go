package organizer

import (
	"fmt"
	"strings"
)

// copyLink shows the position as "(latitude, longitude)" and links it to the
// vault's copy command, which puts it on the clipboard the other way round.
// What is read and what is copied deliberately differ: maps want one order and
// the services that take a pasted pair want the other.
func copyLink(ctx Context, latitude, longitude, choice string) string {
	// No position is nothing to show: the line is left out rather than
	// written as an empty pair.
	if latitude == "" || longitude == "" {
		return ""
	}
	shown := fmt.Sprintf("(%s, %s)", latitude, longitude)
	choice = strings.TrimSpace(choice)
	if choice == "" {
		return shown
	}
	return fmt.Sprintf("[%s](obsidian://quickadd?choice=%s&value-coordinates=%s)",
		shown, escapeURIComponent(choice), escapeURIComponent(latitude+", "+longitude))
}

// escapeURIComponent encodes the way a browser's encodeURIComponent does, which
// is what wrote the links already in these notes: a space is %20 rather than +,
// and the characters JavaScript leaves alone are left alone. A + would be read
// back as a literal plus by some parsers, and the command it names would not be
// found.
func escapeURIComponent(value string) string {
	const unreserved = "-_.!~*'()"
	var out strings.Builder
	for _, b := range []byte(value) {
		switch {
		case b >= 'A' && b <= 'Z', b >= 'a' && b <= 'z', b >= '0' && b <= '9',
			strings.IndexByte(unreserved, b) >= 0:
			out.WriteByte(b)
		default:
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}
