package web

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"

	"dgs-toolbox/internal/doc/cases"
	"dgs-toolbox/internal/doc/export"
	"dgs-toolbox/internal/doc/tree"
	"dgs-toolbox/internal/doc/view"
)

type caseJSON struct {
	cases.Case
	// Missing are the entries whose Item, or recorded revision, the tree no
	// longer has.
	Missing []string `json:"missing"`
}

func (s server) caseList(w http.ResponseWriter, _ *http.Request) {
	out := struct {
		Cases []caseJSON `json:"cases"`
		Error string     `json:"error,omitempty"`
	}{Cases: []caseJSON{}}
	all, err := cases.Load(s.root)
	if err != nil {
		out.Error = err.Error()
	}
	items, _ := tree.LoadItems(s.root)
	for _, c := range all {
		_, missing := c.Items(items)
		if missing == nil {
			missing = []string{}
		}
		out.Cases = append(out.Cases, caseJSON{Case: c, Missing: missing})
	}
	writeJSON(w, http.StatusOK, out)
}

// caseChange is one change to a Case, named by Op.
type caseChange struct {
	Name   string `json:"name"`
	Op     string `json:"op"`
	Title  string `json:"title"`
	Notes  string `json:"notes"`
	Layout string `json:"layout"`
	Item   string `json:"item"`
	Note   string `json:"note"`
	Text   string `json:"text"`
	Need   int    `json:"need"`
}

// caseChange reads the Case, changes it and writes it back. An Item put in
// must be one the tree has.
func (s server) caseChange(w http.ResponseWriter, r *http.Request) {
	var request caseChange
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	if err := tree.Require(s.root); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	now := s.now()
	if request.Op == "new" {
		c, err := cases.New(request.Name, request.Title, now)
		if err == nil {
			err = cases.Save(s.root, c, true)
		}
		s.answerCase(w, c, err)
		return
	}
	if request.Op == "delete" {
		if _, err := cases.Trash(s.root, request.Name, now); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	c, err := cases.Find(s.root, request.Name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	items, err := tree.LoadItems(s.root)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	known := func(id string) error {
		for _, item := range items {
			if item.ID == id {
				return nil
			}
		}
		return errors.New("no Item " + id + " in the tree")
	}
	switch request.Op {
	case "edit":
		if c.Status == cases.Archived {
			err = errors.New("the Case is archived: reopen it to change it")
			break
		}
		c.Title, c.Notes, c.Layout = request.Title, request.Notes, request.Layout
	case "add":
		if err = known(request.Item); err == nil {
			err = c.Add(request.Item, request.Note, now)
		}
	case "remove":
		err = c.Remove(request.Item)
	case "need":
		err = c.AddNeed(request.Text, now)
	case "meet":
		if err = known(request.Item); err == nil {
			err = c.Meet(request.Need, request.Item, now)
		}
	case "drop":
		err = c.DropNeed(request.Need)
	case "archive":
		err = c.Archive(items, now)
	case "reopen":
		err = c.Reopen()
	default:
		err = errors.New("no change " + request.Op)
	}
	if err == nil {
		err = cases.Save(s.root, c, false)
	}
	s.answerCase(w, c, err)
}

func (s server) answerCase(w http.ResponseWriter, c cases.Case, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	items, _ := tree.LoadItems(s.root)
	_, missing := c.Items(items)
	if missing == nil {
		missing = []string{}
	}
	writeJSON(w, http.StatusOK, caseJSON{Case: c, Missing: missing})
}

type caseExportRequest struct {
	Name   string `json:"name"`
	Folder string `json:"folder"`
}

// casePlan is what exporting a Case into a folder would do: its Items, at
// the revisions it takes, named by its layout. An entry whose Item is gone
// stops it, as does anything that stops a View's export.
func (s server) casePlan(ctx context.Context, request caseExportRequest) (exportPlanJSON, []tree.Item, int, error) {
	out := exportPlanJSON{Jobs: []export.JobPlan{}, Problems: []string{}}
	if !filepath.IsAbs(request.Folder) {
		return out, nil, http.StatusBadRequest, errors.New("the folder to export to must be an absolute path")
	}
	c, err := cases.Find(s.root, request.Name)
	if err != nil {
		return out, nil, http.StatusNotFound, err
	}
	all, err := tree.LoadItems(s.root)
	if err != nil {
		return out, nil, http.StatusConflict, err
	}
	items, missing := c.Items(all)
	for _, id := range missing {
		out.Problems = append(out.Problems, "Item "+id+" is in the Case but no longer in the tree, at the revision it had")
	}
	if len(items) == 0 && len(missing) == 0 {
		out.Problems = append(out.Problems, "the Case has no Items to export")
	}
	jobs := []export.Job{{Path: filepath.Clean(request.Folder), Views: []view.View{c.View()}}}
	if out.Jobs, err = export.PlanJobs(ctx, s.root, jobs, items); err != nil {
		return out, nil, http.StatusConflict, err
	}
	export.AgainstOthers(out.Jobs, Others(s.trees, s.root))
	out.Ready = len(out.Problems) == 0
	for _, j := range out.Jobs {
		out.Ready = out.Ready && j.Ready()
	}
	return out, items, http.StatusOK, nil
}

func (s server) caseExportPlan(w http.ResponseWriter, r *http.Request) {
	var request caseExportRequest
	if !decode(w, r, &request) {
		return
	}
	out, _, status, err := s.casePlan(r.Context(), request)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// caseExportRun plans again and writes only when nothing stops it.
func (s server) caseExportRun(w http.ResponseWriter, r *http.Request) {
	var request caseExportRequest
	if !decode(w, r, &request) {
		return
	}
	s.writing.Lock()
	defer s.writing.Unlock()
	out, items, status, err := s.casePlan(r.Context(), request)
	if err != nil {
		writeJSON(w, status, map[string]string{"error": err.Error()})
		return
	}
	if !out.Ready {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the export has problems: nothing was written", "plan": out})
		return
	}
	j := out.Jobs[0]
	result, err := s.applyExport(r, j, items)
	result.Name = ""
	results := []exportResultJSON{result}
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"error": j.Path + ": " + err.Error(), "results": results})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}
