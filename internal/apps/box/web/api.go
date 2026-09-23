package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/money"
)

type api struct {
	settings Settings
}

// pageConfig is what a page needs before it can draw anything: where the Box
// is, what an amount with no code means, and whether the scans it is about to
// show are real.
type pageConfig struct {
	Root     string `json:"root"`
	Inbox    string `json:"inbox"`
	Currency string `json:"currency"`
	Zone     string `json:"zone"`
	// Sample is true while the pages are backed by made-up scans. The pages say
	// so in a banner: describing scans that do not exist is worse than an empty
	// page.
	Sample bool `json:"sample"`
}

func (a api) config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, pageConfig{
		Root:     a.settings.Root,
		Inbox:    a.settings.Inbox,
		Currency: a.settings.Currency,
		Zone:     a.settings.Zone,
		Sample:   a.settings.Source.Sample(),
	})
}

// typeOption is one entry of the catalogue as the pages need it: the name that
// goes into a sidecar, what to show, the key that chooses it, and what its
// expiry would be.
type typeOption struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Key      string `json:"key"`
	Nature   string `json:"nature"`
	Lifetime *int   `json:"lifetime"`
	// ExpiryExpected marks a type whose expiry is printed on the document, so
	// the page can ask for it rather than guess.
	ExpiryExpected bool `json:"expiryExpected"`
}

func (a api) types(w http.ResponseWriter, _ *http.Request) {
	options := make([]typeOption, 0, len(doctype.All()))
	for _, entry := range doctype.All() {
		options = append(options, typeOption{
			Name:           entry.Name,
			Label:          entry.Label,
			Key:            string(entry.Key),
			Nature:         entry.Nature.String(),
			Lifetime:       entry.Lifetime,
			ExpiryExpected: entry.ExpiryExpected,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": options, "unsorted": doctype.Unsorted})
}

func (a api) intake(w http.ResponseWriter, _ *http.Request) {
	pending := a.settings.Source.Pending()
	writeJSON(w, http.StatusOK, map[string]any{"pending": pending, "count": len(pending)})
}

func (a api) scans(w http.ResponseWriter, _ *http.Request) {
	scans := a.settings.Source.Scans()
	amounts := make([]money.Amount, 0, len(scans))
	for _, scan := range scans {
		if scan.Total == "" {
			continue
		}
		amount, err := money.Parse(scan.Total, "")
		if err != nil {
			continue
		}
		amounts = append(amounts, amount)
	}
	type totalLine struct {
		Currency string `json:"currency"`
		Display  string `json:"display"`
		Count    int    `json:"count"`
	}
	// Per currency, never combined: a single figure would need a rate, which is
	// a fact about an instant and is not available offline.
	totals := make([]totalLine, 0)
	for _, total := range money.Sum(amounts) {
		totals = append(totals, totalLine{
			Currency: total.Amount.Currency,
			Display:  total.Amount.Display(),
			Count:    total.Count,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": scans, "totals": totals})
}

func (a api) exceptions(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"exceptions": a.settings.Source.Exceptions()})
}

func (a api) patch(w http.ResponseWriter, r *http.Request) {
	var edit Edit
	if err := json.NewDecoder(r.Body).Decode(&edit); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	scan, err := a.settings.Source.Apply(edit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (a api) file(w http.ResponseWriter, r *http.Request) {
	var edit Edit
	if err := json.NewDecoder(r.Body).Decode(&edit); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if edit.Digest == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no scan named"))
		return
	}
	scan, err := a.settings.Source.File(edit.Digest, edit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

func (a api) trash(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Digest string `json:"digest"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.settings.Source.Trash(request.Digest, request.Reason); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// image answers with a scan's thumbnail or preview. The request names a scan
// by digest, never by path: a path in a request is a path out of the Box.
//
// page names one page of a multi-page scan, counting from 1. Leaving it out
// asks for the first page, which is what every picture was before there was a
// page view.
func (a api) image(w http.ResponseWriter, r *http.Request) {
	digest := r.URL.Query().Get("digest")
	size := r.URL.Query().Get("size")
	if size != "preview" {
		size = "thumb"
	}
	page := 1
	if text := r.URL.Query().Get("page"); text != "" {
		parsed, err := strconv.Atoi(text)
		if err != nil || parsed < 1 {
			writeError(w, http.StatusBadRequest, fmt.Errorf("page %q is not a page number", text))
			return
		}
		page = parsed
	}
	body, mediaType, err := a.settings.Source.Image(digest, size, page)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// incomplete is what the inbox held back. It is its own endpoint rather than a
// flag on a pending scan, because a file that was never taken in is not in the
// list of things waiting to be described — and a skip nobody is told about
// means believing the inbox is empty when it is not.
func (a api) incomplete(w http.ResponseWriter, _ *http.Request) {
	held := a.settings.Source.Incomplete()
	if held == nil {
		held = []Exception{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"incomplete": held, "count": len(held)})
}

// batchRequest is one edit and the scans to give it to.
type batchRequest struct {
	Digests []string `json:"digests"`
	Edit    Edit     `json:"edit"`
}

func (a api) patchMany(w http.ResponseWriter, r *http.Request) {
	var request batchRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(request.Digests) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no scans named"))
		return
	}
	result, err := a.settings.Source.ApplyMany(request.Digests, request.Edit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (a api) trashMany(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Digests []string `json:"digests"`
		Reason  string   `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(request.Digests) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("no scans named"))
		return
	}
	result, err := a.settings.Source.TrashMany(request.Digests, request.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// trashSummary is advice, not an offer to empty anything: box never removes a
// file, so what this answers is how much has accumulated and how old it is.
func (a api) trashSummary(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.settings.Source.TrashSummary())
}

// adopt describes a scan that is in the Box with no sidecar. The request names
// a path because that is what an exception is — a file the Box knows about and
// the cache does not — and the Source refuses any path it did not itself
// report as an orphan.
func (a api) adopt(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Path string `json:"path"`
		Edit Edit   `json:"edit"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	scan, err := a.settings.Source.Adopt(request.Path, request.Edit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, scan)
}

// verify reads every byte in the Box. It is a POST because it costs tens of
// gigabytes over a network filesystem, which is not something a page reload
// should start.
func (a api) verify(w http.ResponseWriter, r *http.Request) {
	found, err := a.settings.Source.Verify(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if found == nil {
		found = []Exception{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"mismatches": found, "count": len(found)})
}
