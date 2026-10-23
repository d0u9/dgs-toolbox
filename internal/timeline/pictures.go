package timeline

import "regexp"

// pictureEmbed is a picture embedded by filename alone, as dgs capture writes
// one and Obsidian pastes one: "![[name.jpg]]", with an optional size or
// caption after a "|". A link with a folder in it names its own place and is
// not this package's to move.
var pictureEmbed = regexp.MustCompile(`(?i)!\[\[([^\]|/]+\.(?:jpe?g|png|gif|webp|heic|avif))(?:\|[^\]]*)?\]\]`)

// Pictures are the filenames of the pictures content embeds, in order, each
// once. Whoever writes a timeline's files keeps each picture beside the file
// that shows it, so a year moving into its archive takes its pictures along.
func Pictures(content string) []string {
	names := []string{}
	seen := map[string]bool{}
	for _, match := range pictureEmbed.FindAllStringSubmatch(content, -1) {
		if !seen[match[1]] {
			seen[match[1]] = true
			names = append(names, match[1])
		}
	}
	return names
}
