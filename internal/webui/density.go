package webui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

var pixelLength = regexp.MustCompile(`(?i)(-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+))px\b`)

// ScaleStylesheet scales fixed CSS lengths, keeping percentages, viewport units,
// quoted text, comments and asset URLs intact. Layout stays in normal browser
// coordinates, unlike a CSS zoom/transform that also changes pointer geometry.
func ScaleStylesheet(css string, scale float64) string {
	if scale <= 0 || scale == 1 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		return css
	}
	var out strings.Builder
	flush := func(part string) {
		last := 0
		for _, at := range pixelLength.FindAllStringIndex(part, -1) {
			if at[0] > 0 {
				c := part[at[0]-1]
				if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '#' {
					continue
				}
			}
			out.WriteString(part[last:at[0]])
			value := part[at[0]:at[1]]
			n, _ := strconv.ParseFloat(value[:len(value)-2], 64)
			number := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(n*scale, 'f', 3, 64), "0"), ".")
			out.WriteString(number + "px")
			last = at[1]
		}
		out.WriteString(part[last:])
	}

	start := 0
	for i := 0; i < len(css); {
		quoted := css[i] == '\'' || css[i] == '"'
		comment := strings.HasPrefix(css[i:], "/*")
		url := len(css)-i >= 4 && strings.EqualFold(css[i:i+4], "url(")
		if !quoted && !comment && !url {
			i++
			continue
		}
		flush(css[start:i])
		end := i + 1
		if comment {
			if n := strings.Index(css[i+2:], "*/"); n >= 0 {
				end = i + 2 + n + 2
			} else {
				end = len(css)
			}
		} else if url {
			end = i + 4
			depth := 1
			var quote byte
			for end < len(css) && depth > 0 {
				c := css[end]
				if c == '\\' {
					end += 2
					continue
				}
				if quote != 0 {
					if c == quote {
						quote = 0
					}
				} else if c == '\'' || c == '"' {
					quote = c
				} else if c == '(' {
					depth++
				} else if c == ')' {
					depth--
				}
				end++
			}
			if end > len(css) {
				end = len(css)
			}
		} else {
			for end < len(css) {
				if css[end] == '\\' {
					end += 2
					continue
				}
				if css[end] == css[i] {
					end++
					break
				}
				end++
			}
			if end > len(css) {
				end = len(css)
			}
		}
		out.WriteString(css[i:end])
		i = end
		start = end
	}
	flush(css[start:])
	return out.String()
}
