package gpxfile

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"
)

// A name in another language is a dgs extension: GPX 1.1 gives <name> one
// string and no language. Each translation is one element in the part's own
// <extensions>, its language a BCP 47 tag in xml:lang:
//
//	<trk>
//	  <name>西湖</name>
//	  <extensions><dgs:name xmlns:dgs="urn:dgs-toolbox:gpx:1" xml:lang="en">West Lake</dgs:name></extensions>
//	  <trkseg>…
//
// <name> keeps the name every GPX reader shows. A reader that knows the
// extension shows the translation for its language and falls back to <name>.
// <metadata>, <trk>, <rte> and <wpt> take translations.
//
// The language <name> itself is in is a <dgs:lang> in the same <extensions>,
// a BCP 47 tag as its text. The one in <metadata> is the file's <name>'s and
// the default for every element without its own; without either, the
// language of <name> is unknown. See docs/apps/geo/gpx-names.md.

// Namespace is the XML namespace of dgs's GPX extensions.
const Namespace = "urn:dgs-toolbox:gpx:1"

// xmlTranslation is any <name> in an <extensions>; only dgs's count.
type xmlTranslation struct {
	XMLName xml.Name
	Lang    string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"` // xml:lang only
	Text    string `xml:",chardata"`
}

// xmlLang is any <lang> in an <extensions>; only dgs's counts.
type xmlLang struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
}

// nameLang reads the language of <name>: dgs's last <lang>, or "".
func nameLang(elements []xmlLang) string {
	lang := ""
	for _, element := range elements {
		if text := strings.TrimSpace(element.Text); element.XMLName.Space == Namespace && text != "" {
			lang = text
		}
	}
	return lang
}

// translations reads the parsed extension elements: an element without a
// language or a name is passed over, and the last of one language wins,
// tags matching whatever their case.
func translations(elements []xmlTranslation) map[string]string {
	var out map[string]string
	for _, element := range elements {
		lang, text := strings.TrimSpace(element.Lang), strings.TrimSpace(element.Text)
		if element.XMLName.Space != Namespace || lang == "" || text == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		for tag := range out {
			if strings.EqualFold(tag, lang) {
				delete(out, tag)
			}
		}
		out[lang] = text
	}
	return out
}

// writeTranslations writes the language of <name>, when there is one, and
// one extension element per translation, in the order of their tags so the
// same names always make the same bytes. Empty entries are left out.
func writeTranslations(b *bytes.Buffer, lang string, names map[string]string) {
	if lang = strings.TrimSpace(lang); lang != "" {
		b.WriteString(`<dgs:lang xmlns:dgs="` + Namespace + `">`)
		_ = xml.EscapeText(b, []byte(lang))
		b.WriteString("</dgs:lang>")
	}
	langs := make([]string, 0, len(names))
	for lang, text := range names {
		if strings.TrimSpace(lang) != "" && strings.TrimSpace(text) != "" {
			langs = append(langs, lang)
		}
	}
	sort.Strings(langs)
	for _, lang := range langs {
		b.WriteString(`<dgs:name xmlns:dgs="` + Namespace + `" xml:lang="`)
		_ = xml.EscapeText(b, []byte(strings.TrimSpace(lang)))
		b.WriteString(`">`)
		_ = xml.EscapeText(b, []byte(strings.TrimSpace(names[lang])))
		b.WriteString("</dgs:name>")
	}
}

// hasTranslations reports whether writeTranslations writes anything.
func hasTranslations(lang string, names map[string]string) bool {
	var b bytes.Buffer
	writeTranslations(&b, lang, names)
	return b.Len() > 0
}

// PartFile names the GPX itself to SetTranslations: its <metadata>.
const PartFile Part = "metadata"

// SetTranslations replaces the language of <name> and every translation of
// the index-th <trk>, <rte> or <wpt> of a file written by this program — or,
// for PartFile, of the file itself — keeping every other byte. An empty
// language and no names take them all out, and an <extensions> left holding
// nothing goes too. A file created by something else is refused.
func SetTranslations(path string, part Part, index int, lang string, names map[string]string) error {
	data, parsed, err := readOurs(path)
	if err != nil {
		return err
	}
	var start, end int
	if part == PartFile {
		start = bytes.Index(data, []byte("<metadata>"))
		if start < 0 {
			if !hasTranslations(lang, names) {
				return nil
			}
			// The file has no <metadata>: it opens one first under <gpx>.
			root := bytes.Index(data, []byte("<gpx"))
			tag := -1
			if root >= 0 {
				tag = bytes.IndexByte(data[root:], '>')
			}
			if tag < 0 {
				return fmt.Errorf("%s: no <gpx> element", path)
			}
			at := root + tag + 1
			var b bytes.Buffer
			b.Write(data[:at])
			b.WriteString("\n  <metadata><extensions>")
			writeTranslations(&b, lang, names)
			b.WriteString("</extensions></metadata>")
			b.Write(data[at:])
			return replaceFile(path, b.Bytes())
		}
		close := bytes.Index(data[start:], []byte("</metadata>"))
		if close < 0 {
			return fmt.Errorf("%s: <metadata> is not closed", path)
		}
		end = start + close + len("</metadata>")
	} else {
		if index < 0 || index >= parsed.count(part) {
			return fmt.Errorf("%s: no %s %d", path, part, index)
		}
		if start, end, err = partSpan(data, part, index); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	element, err := withTranslations(data[start:end], part, lang, names)
	if err != nil {
		return fmt.Errorf("%s: %s %d: %w", path, part, index, err)
	}
	var b bytes.Buffer
	b.Write(data[:start])
	b.Write(element)
	b.Write(data[end:])
	return replaceFile(path, b.Bytes())
}

// withTranslations is one element with its translations replaced. Its own
// children end where its points start; GPX 1.1 puts <extensions> last among
// them, so new translations go into the <extensions> there is, or into one
// opened just before the points or the closing tag.
func withTranslations(element []byte, part Part, lang string, names map[string]string) ([]byte, error) {
	open := bytes.IndexByte(element, '>')
	if open < 0 {
		return nil, fmt.Errorf("not closed")
	}
	closing := []byte("</" + string(part) + ">")
	if element[open-1] == '/' {
		// <wpt … /> holds nothing: it is opened so it can.
		if !hasTranslations(lang, names) {
			return element, nil
		}
		element = append(append(append([]byte(nil), element[:open-1]...), '>'), closing...)
		open = bytes.IndexByte(element, '>')
	}
	limit := len(element) - len(closing)
	for _, nested := range []string{"<trkseg", "<rtept"} {
		if at := bytes.Index(element, []byte(nested)); at >= 0 && at < limit {
			limit = at
		}
	}
	// What is there now, without the language and translations it holds.
	own := []byte(nil)
	own = append(own, element[open+1:limit]...)
	for _, tag := range []string{"name", "lang"} {
		for {
			from := bytes.Index(own, []byte("<dgs:"+tag+" "))
			if from < 0 {
				break
			}
			to := bytes.Index(own[from:], []byte("</dgs:"+tag+">"))
			if to < 0 {
				return nil, fmt.Errorf("a dgs:%s is not closed", tag)
			}
			own = append(own[:from], own[from+to+len("</dgs:"+tag+">"):]...)
		}
	}
	own = bytes.Replace(own, []byte("<extensions></extensions>"), nil, 1)
	var translated bytes.Buffer
	writeTranslations(&translated, lang, names)
	if translated.Len() > 0 {
		if at := bytes.Index(own, []byte("<extensions>")); at >= 0 {
			at += len("<extensions>")
			own = append(own[:at], append(translated.Bytes(), own[at:]...)...)
		} else {
			// Before the whitespace that leads to the points or the closing tag.
			at := len(bytes.TrimRight(own, " \t\r\n"))
			ext := append(append([]byte("<extensions>"), translated.Bytes()...), "</extensions>"...)
			if part == PartTrack || part == PartRoute {
				ext = append([]byte("\n    "), ext...)
			}
			own = append(own[:at], append(ext, own[at:]...)...)
		}
	}
	var b bytes.Buffer
	b.Write(element[:open+1])
	b.Write(own)
	b.Write(element[limit:])
	return b.Bytes(), nil
}
