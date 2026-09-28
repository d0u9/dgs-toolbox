package web

import (
	"net/http"
	"sort"

	"dgs-toolbox/internal/doc/country"
	"dgs-toolbox/internal/doc/outline"
	"dgs-toolbox/internal/doc/snapshot"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type outlineJSON struct {
	outline.Outline
	// Default is the Outline's own folder on this machine, ~ made home.
	Default string `json:"default"`
}

type outlinesJSON struct {
	Outlines []outlineJSON `json:"outlines"`
	// Keys is every key a layout can use with the tree's Items: the
	// Templates' fields and the keys every Item has.
	Keys []string `json:"keys"`
	// Countries maps each country's every form to its alpha-3 code, so the
	// page shows CN and 中国 as one condition value.
	Countries map[string]string `json:"countries"`
	// Rules are every rule in the tree, and Used the Outlines using each.
	Rules []view.View         `json:"rules"`
	Used  map[string][]string `json:"used"`
	// Layouts are each rule's layout parsed, by rule name, for the page to
	// edit part by part; a layout that does not parse is left out.
	Layouts map[string]view.Layout `json:"layouts"`
	// Migrated names the Outlines the tree's Views and Targets just became.
	Migrated []string `json:"migrated,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// builtIn are the keys every Item has, and those derived from its fields.
var builtIn = []string{"type", "kind", "id", "revision", "ext", "year", "month", "date"}

// outlineList answers every Outline, first making the tree's Views and
// Targets into Outlines if it still has them.
func (s server) outlineList(w http.ResponseWriter, _ *http.Request) {
	out := outlinesJSON{Outlines: []outlineJSON{}, Countries: map[string]string{}}
	for _, c := range country.All() {
		for _, f := range country.Formats {
			out.Countries[c.In(f)] = c.Alpha3
		}
	}
	seen := map[string]bool{}
	var fields []string
	if templates, err := tree.LoadTemplates(s.root); err == nil {
		for _, t := range templates {
			for _, f := range t.Fields {
				if !seen[f.Key] {
					seen[f.Key] = true
					fields = append(fields, f.Key)
				}
			}
		}
	}
	sort.Strings(fields)
	for _, k := range builtIn {
		if !seen[k] {
			out.Keys = append(out.Keys, k)
		}
	}
	out.Keys = append(fields, out.Keys...)
	if tree.Require(s.root) == nil {
		s.writing.Lock()
		migrated, err := outline.Migrate(s.root)
		s.writing.Unlock()
		out.Migrated = migrated
		if err != nil {
			out.Error = err.Error()
		}
	}
	entries, err := outline.Load(s.root)
	if err != nil {
		out.Error = err.Error()
	}
	out.Rules, out.Used = []view.View{}, map[string][]string{}
	if rules, used, err := outline.Rules(s.root); err == nil {
		out.Rules, out.Used = rules, used
	}
	out.Layouts = map[string]view.Layout{}
	for _, r := range out.Rules {
		if layout, err := view.Parse(r.Layout); err == nil {
			out.Layouts[r.Name] = layout
		}
	}
	for _, o := range entries {
		j := outlineJSON{Outline: o}
		if o.Folder != "" {
			j.Default = outline.Expand(o.Folder, home())
		}
		out.Outlines = append(out.Outlines, j)
	}
	writeJSON(w, http.StatusOK, out)
}

// outlineGroup plans an Outline, saved or not, and nests the PDFs its rules
// and Snapshots place into folders. It writes nothing. The Rules page plans
// one rule by sending an Outline of it alone.
func (s server) outlineGroup(w http.ResponseWriter, r *http.Request) {
	var o outline.Outline
	if !decode(w, r, &o) {
		return
	}
	for i := range o.Rules {
		if o.Rules[i].Selection == "" {
			o.Rules[i].Selection = view.Head
		}
	}
	if err := o.Validate(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	templates, err := tree.LoadTemplates(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	snapshots, err := snapshot.Load(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	g, err := outline.Group(o, snapshots, items, view.NamesOf(templates))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s server) outlineSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Outline  outline.Outline `json:"outline"`
		Previous string          `json:"previous"`
		// Fresh are the rules made new in this Outline, which must not
		// replace a rule of the same name; Renamed maps a rule's saved
		// name to its new one, renamed in every Outline using it.
		Fresh   []string          `json:"fresh"`
		Renamed map[string]string `json:"renamed"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	fail := func(err error) { writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()}) }
	if err := outline.Fresh(s.root, request.Fresh); err != nil {
		fail(err)
		return
	}
	for from, to := range request.Renamed {
		if err := outline.RenameRule(s.root, from, to); err != nil {
			fail(err)
			return
		}
	}
	if err := outline.Save(s.root, request.Previous, request.Outline); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, request.Outline)
}

func (s server) outlineDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := outline.Delete(s.root, request.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// ruleDelete removes a rule no Outline uses.
func (s server) ruleDelete(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := outline.DeleteRule(s.root, request.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// ruleSave writes one rule, renaming it first in every Outline using it
// when Previous names it otherwise. A new rule must not replace another.
func (s server) ruleSave(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Rule     view.View `json:"rule"`
		Previous string    `json:"previous"`
	}
	if !decode(w, r, &request) {
		return
	}
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := outline.SaveRule(s.root, request.Previous, request.Rule); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, request.Rule)
}
