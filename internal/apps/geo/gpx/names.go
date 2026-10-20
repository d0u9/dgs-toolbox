// Renaming a part of a GPX. Every name is held beside the file until the edit
// is saved; saving writes it where it belongs — into a GPX dgs wrote, and into
// the sidecar of a recording, which is never written. A track added here, and
// every track of a draft, carries its name into the GPX it is written to.

package gpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"dgs-toolbox/internal/geo/stops"
)

// renamePart sets the name of one <trk>, <rte> or <wpt>, keyed as the page
// keys them: "t0", "r0", "w0"; the key "f" names the GPX itself, its
// <metadata><name>. An empty name puts back the file's own.
func (a api) renamePart(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Path string `json:"path"`
		Key  string `json:"key"`
		Name string `json:"name"`
		// Translations, when sent, replace the name in every other language,
		// keyed by BCP 47 tag; left out, they stay as they are.
		Translations *map[string]string `json:"translations"`
		// Lang, when sent, is the language of name, a language tag; "" says
		// it is not known, so a part takes the file's. Left out, it stays.
		Lang *string `json:"lang"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid JSON"))
		return
	}
	if _, _, err := partKey(body.Key); err != nil && body.Key != fileNameKey {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	name := strings.TrimSpace(body.Name)
	if strings.ContainsAny(name, "\n\r") {
		writeError(w, http.StatusBadRequest, errors.New("a name is one line"))
		return
	}
	var translations map[string]string
	if body.Translations != nil {
		var err error
		if translations, err = cleanTranslations(*body.Translations); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if body.Lang != nil {
		*body.Lang = strings.TrimSpace(*body.Lang)
		if *body.Lang != "" && !languageTag.MatchString(*body.Lang) {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%q is not a language tag such as en or zh-Hans", *body.Lang))
			return
		}
	}
	result, err := analyse(body.Path, stops.Defaults())
	if err != nil {
		writeError(w, statusFor(err), err)
		return
	}
	if result.sidecarErr != nil {
		writeError(w, http.StatusUnprocessableEntity, result.sidecarErr)
		return
	}
	var at place
	if body.Key != fileNameKey {
		if at, err = locate(result, body.Key); err != nil {
			writeError(w, statusFor(err), err)
			return
		}
	}
	// Every name is kept beside the file until the edit is saved. Saving puts
	// it where it belongs: into a GPX dgs wrote, and into the sidecar of a
	// recording, which is never written.
	before := cloneSidecar(result.cleaning)
	file := cloneSidecar(result.cleaning)
	ownName := result.file.Name
	switch at.kind {
	case 't':
		ownName = result.file.Tracks[at.shown].Name
	case 'r':
		ownName = result.file.Routes[at.shown].Name
	case 'w':
		ownName = result.file.Waypoints[at.shown].Name
	}
	// The name as the page shows it — the one given here, the part's own, or
	// for want of one the file name or "Track 2" — is what the dialog offers;
	// sent back as it is, it is no rename.
	shownName := result.name
	if body.Key != fileNameKey {
		shownName = partName(result, body.Key)
	}
	if name != shownName {
		if at.kind == 't' && at.held >= 0 {
			// A track added here is not in a GPX yet: it carries its name where
			// it is held, into the GPX it is written to.
			file.Added[at.held].Name = name
		}
		if file.Names == nil {
			file.Names = map[string]string{}
		}
		file.Names[body.Key] = name
		if name == ownName {
			// The part's own name, given back: nothing is held for it.
			delete(file.Names, body.Key)
		}
	}
	if body.Key == fileNameKey && name == "" {
		// An empty name gives the GPX back its own <name>, or its file name,
		// and its own translations with it: one given since translated the
		// name that goes.
		delete(file.Names, body.Key)
		if body.Translations == nil {
			delete(file.Translations, body.Key)
			if len(file.Translations) == 0 {
				file.Translations = nil
			}
		}
		if body.Lang == nil {
			delete(file.Langs, body.Key)
			if len(file.Langs) == 0 {
				file.Langs = nil
			}
		}
	}
	if len(file.Names) == 0 {
		file.Names = nil
	}
	// Only what differs from the file's own is held, so giving back the
	// language and translations it has leaves no edit.
	ownLang, own := result.file.Lang, result.file.Translations
	switch at.kind {
	case 't':
		ownLang, own = result.file.Tracks[at.shown].Lang, result.file.Tracks[at.shown].Translations
	case 'r':
		ownLang, own = result.file.Routes[at.shown].Lang, result.file.Routes[at.shown].Translations
	case 'w':
		ownLang, own = result.file.Waypoints[at.shown].Lang, result.file.Waypoints[at.shown].Translations
	}
	if body.Lang != nil {
		if file.Langs == nil {
			file.Langs = map[string]string{}
		}
		file.Langs[body.Key] = *body.Lang
		if *body.Lang == ownLang {
			delete(file.Langs, body.Key)
		}
		if len(file.Langs) == 0 {
			file.Langs = nil
		}
	}
	if body.Translations != nil {
		if file.Translations == nil {
			file.Translations = map[string]map[string]string{}
		}
		file.Translations[body.Key] = translations
		if maps.Equal(translations, own) {
			delete(file.Translations, body.Key)
		}
		if len(file.Translations) == 0 {
			file.Translations = nil
		}
	}
	// A rename that changes nothing — the dialog confirmed as it opened —
	// is no edit: nothing to save and nothing to undo.
	if reflect.DeepEqual(before, file) {
		writeJSON(w, map[string]any{"ok": true, "name": name})
		return
	}
	if err := saveSidecar(body.Path, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "name": name})
}

// cleanTranslations checks translated names: each keyed by a language tag
// such as "en" or "zh-Hans", each one line. Empty names are dropped.
func cleanTranslations(names map[string]string) (map[string]string, error) {
	kept, seen := map[string]string{}, map[string]bool{}
	for lang, name := range names {
		lang, name = strings.TrimSpace(lang), strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !languageTag.MatchString(lang) {
			return nil, fmt.Errorf("%q is not a language tag such as en or zh-Hans", lang)
		}
		if strings.ContainsAny(name, "\n\r") {
			return nil, errors.New("a name is one line")
		}
		// Tags match whatever their case, so one language is named once.
		if seen[strings.ToLower(lang)] {
			return nil, fmt.Errorf("%q is named twice", lang)
		}
		seen[strings.ToLower(lang)] = true
		kept[lang] = name
	}
	return kept, nil
}

// languageTag is the shape of a BCP 47 tag; which tags exist is not checked.
var languageTag = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

// fileNameKey keys the name of the GPX itself among the part names.
const fileNameKey = "f"

// partKey reads a part key: its kind ('t', 'r' or 'w') and its index.
func partKey(key string) (byte, int, error) {
	invalid := errors.New(`the part is keyed "t0", "r0" or "w0"`)
	if len(key) < 2 || (key[0] != 't' && key[0] != 'r' && key[0] != 'w') {
		return 0, 0, invalid
	}
	index, err := strconv.Atoi(key[1:])
	if err != nil || index < 0 {
		return 0, 0, invalid
	}
	return key[0], index, nil
}
