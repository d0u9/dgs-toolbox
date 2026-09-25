package web

import (
	"net/http"
	"strings"

	"dgs-toolbox/internal/doc/search"
	"dgs-toolbox/internal/doc/tree"
)

// DefaultSimilarLimit is how many Items "more like this" lists.
const DefaultSimilarLimit = 10

type searchJSON struct {
	Hits []search.Hit `json:"hits"`
	// Unread counts the Items whose text is not in the cache yet, which no
	// search can find.
	Unread int `json:"unread"`
}

// texts is the cached text of every Item's current PDF, and the Items that
// have none yet. Only what the cache holds is used: nothing is read here.
func (s server) texts() ([]search.Doc, []tree.Item, error) {
	items, err := tree.LoadItems(s.root)
	if err != nil {
		return nil, nil, err
	}
	var docs []search.Doc
	var unread []tree.Item
	for _, item := range items {
		head := item.Current()
		first, count, ok := s.store.Load(head, 0)
		if !ok {
			unread = append(unread, item)
			continue
		}
		pages := []string{first.Text()}
		for n := 1; n < count && n < s.reader.MaxPages(); n++ {
			if page, _, ok := s.store.Load(head, n); ok {
				pages = append(pages, page.Text())
			}
		}
		docs = append(docs, search.Doc{ID: item.ID, Text: strings.Join(pages, "\n")})
	}
	return docs, unread, nil
}

func (s server) search(w http.ResponseWriter, r *http.Request) {
	docs, unread, err := s.texts()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, searchJSON{Hits: search.Find(docs, r.URL.Query().Get("q"), search.DefaultSnippet), Unread: len(unread)})
}

func (s server) similar(w http.ResponseWriter, r *http.Request) {
	docs, unread, err := s.texts()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	hits := search.More(docs, r.URL.Query().Get("item"), search.DefaultSimilar, DefaultSimilarLimit)
	writeJSON(w, http.StatusOK, searchJSON{Hits: hits, Unread: len(unread)})
}

// readAll has the text of every Item not yet read queued, behind whatever
// the page asks for itself.
func (s server) readAll(w http.ResponseWriter, _ *http.Request) {
	_, unread, err := s.texts()
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
		return
	}
	for _, item := range unread {
		s.reader.Ahead(tree.PDFPath(s.root, item.ID, item.Current()), item.Current())
	}
	writeJSON(w, http.StatusOK, map[string]int{"queued": len(unread)})
}
