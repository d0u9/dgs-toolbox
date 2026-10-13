package web

import (
	"bytes"
	"dgs-toolbox/internal/doc/filethumb"
	"dgs-toolbox/internal/doc/tree"
	imagejpeg "image/jpeg"
	"net/http"
)

func (s server) fileThumbnail(w http.ResponseWriter, r *http.Request) {
	item, err := s.documents.GetItem(r.URL.Query().Get("item"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	revision, ok := item.Revision(r.URL.Query().Get("digest"))
	if !ok || revision.Digest == "" {
		http.Error(w, "no such revision", http.StatusNotFound)
		return
	}
	p, err := s.documents.ReadPDF(item, revision.Digest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	digest := revision.Digest
	jpeg, ok := s.pictures.store.Load(digest, 1, "file")
	if ok {
		if _, decodeErr := imagejpeg.Decode(bytes.NewReader(jpeg)); decodeErr != nil {
			ok = false
		}
	}
	if !ok {
		actual, readErr := tree.FileDigest(p)
		if readErr != nil || actual != digest {
			http.Error(w, "file changed since import; check files before generating a thumbnail", http.StatusConflict)
			return
		}
		jpeg, err = filethumb.Render(p)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotImplemented)
			return
		}
		_ = s.pictures.store.Save(digest, 1, "file", jpeg)
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(jpeg)
}
