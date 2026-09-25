// Package country turns the ways a country is written — ISO codes, English
// and Chinese names, common aliases — into one form, so "cn", "CHN", "China"
// and "中华人民共和国" are the same value.
package country

import (
	"strings"
	"unicode"
)

// Country is one ISO 3166-1 entry.
type Country struct {
	Alpha2, Alpha3, English, Chinese string
}

// Format is the form a country is written in.
type Format string

const (
	Chinese Format = "zh"
	English Format = "en"
	Alpha2  Format = "alpha2"
	Alpha3  Format = "alpha3"
)

// DefaultFormat is used when a field names none.
const DefaultFormat = Chinese

// Formats are the accepted Formats.
var Formats = []Format{Chinese, English, Alpha2, Alpha3}

// Valid reports whether f is a Format.
func (f Format) Valid() bool {
	for _, x := range Formats {
		if f == x {
			return true
		}
	}
	return false
}

// In writes c in format f; an unknown or empty f is DefaultFormat.
func (c Country) In(f Format) string {
	switch f {
	case English:
		return c.English
	case Alpha2:
		return c.Alpha2
	case Alpha3:
		return c.Alpha3
	}
	return c.Chinese
}

var (
	all    []Country
	byName = map[string]int{}
)

func init() {
	for _, line := range strings.Split(strings.TrimSpace(table), "\n") {
		parts := strings.Split(line, "|")
		c := Country{Alpha2: parts[0], Alpha3: parts[1], English: parts[2], Chinese: parts[3]}
		names := append([]string{c.Alpha2, c.Alpha3, c.English, c.Chinese}, strings.Split(parts[4], ";")...)
		for _, name := range names {
			if k := key(name); k != "" {
				if _, taken := byName[k]; !taken {
					byName[k] = len(all)
				}
			}
		}
		all = append(all, c)
	}
}

// key folds case and drops what varies in how a name is typed: spaces,
// dots, brackets, apostrophes, hyphens and a leading "the".
func key(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "the ")
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Find is the country s names, in any form the table knows.
func Find(s string) (Country, bool) {
	i, ok := byName[key(s)]
	if !ok {
		return Country{}, false
	}
	return all[i], true
}

// Normalize writes s in format f, and reports whether s named a country.
func Normalize(s string, f Format) (string, bool) {
	c, ok := Find(s)
	if !ok {
		return "", false
	}
	return c.In(f), true
}

// All is every country, in table order.
func All() []Country { return append([]Country(nil), all...) }
