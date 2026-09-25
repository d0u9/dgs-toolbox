package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"time"

	"dgs-toolbox/internal/box/doctype"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/tag"
	"dgs-toolbox/internal/desktop"
	"dgs-toolbox/internal/zonesearch"
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
	Covers   string `json:"covers"`
	Key      string `json:"key"`
	Nature   string `json:"nature"`
	Lifetime *int   `json:"lifetime"`
}

func (a api) types(w http.ResponseWriter, _ *http.Request) {
	options := make([]typeOption, 0, len(doctype.All()))
	for _, entry := range doctype.All() {
		options = append(options, typeOption{
			Name:     entry.Name,
			Label:    entry.Label,
			Covers:   entry.Covers,
			Key:      string(entry.Key),
			Nature:   entry.Nature.String(),
			Lifetime: entry.Lifetime,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"types": options, "unsorted": doctype.Unsorted})
}

// tags is every tag the Box already uses and how many scans carry it, so the
// pages can offer the spelling that exists instead of inventing a near twin.
// A scan counts once for a tag, whether the file or one of its splits says it.
func (a api) tags(w http.ResponseWriter, _ *http.Request) {
	scans := a.settings.Source.Scans()
	records := make([][]string, 0, len(scans))
	for _, scan := range scans {
		record := append([]string{}, scan.Tags...)
		for _, document := range scan.Documents {
			record = append(record, document.Tags...)
		}
		records = append(records, record)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": tag.Count(records)})
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
	// A digest names the file's bytes, not the picture drawn from them: a
	// better drawing — a page turned as its /Rotate says — changes the picture
	// under the same address. So the browser keeps it but asks each time, and
	// an unchanged picture is answered with 304 and no body.
	sum := sha256.Sum256(body)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:12])+`"`)
	w.Header().Set("Cache-Control", "private, no-cache")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
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

func (a api) rejected(w http.ResponseWriter, _ *http.Request) {
	found := a.settings.Source.Rejected()
	if found == nil {
		found = []RejectedScan{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"rejected": found, "count": len(found)})
}

// rejectedFile answers with a rejected scan's own file, shown inline so the
// browser opens it in its own viewer.
func (a api) rejectedFile(w http.ResponseWriter, r *http.Request) {
	body, name, mediaType, err := a.settings.Source.RejectedFile(r.URL.Query().Get("digest"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (a api) restore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.settings.Source.Restore(body.Digest); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a api) unfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := a.settings.Source.Unfile(body.Digest); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// reveal shows a filed scan in the file manager of the machine serving the
// page, selected in its folder. The page is local, so that is the machine in
// front of the person clicking.
func (a api) reveal(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Digest string `json:"digest"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	path, err := a.settings.Source.Locate(body.Digest)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err := desktop.Reveal(path); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// zones answers the zone field's search: "syd", "aus", "chi".
func (a api) zones(w http.ResponseWriter, r *http.Request) {
	found := zonesearch.Search(r.URL.Query().Get("q"), 0)
	if found == nil {
		found = []zonesearch.Zone{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": found})
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
// redraw mends broken thumbnails: one scan when the body names a digest, every
// filed scan when it does not.
func (a api) redraw(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Digest string `json:"digest"`
	}
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	result, err := a.settings.Source.Redraw(r.Context(), body.Digest)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

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
