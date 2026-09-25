package web

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"sync"

	"dgs-toolbox/internal/box/scanmeta"
	"dgs-toolbox/internal/box/thumb"
	"dgs-toolbox/internal/doc/tree"
)

// A scanned PDF is shown as pictures of its pages rather than in the
// browser's PDF viewer: a 300 dpi scan redrawn by the viewer on every scroll
// stutters, where a 1600 px JPEG scrolls like any image. The picture is the
// page's own embedded scan, as box draws it (thumb), so nothing is rasterised
// and no C library is compiled in. A page that is not a scan — a PDF made on
// a computer — has no picture, and the page falls back to the viewer.

// pictures keeps drawn pages for the life of the process, the newest
// maxPictures of them.
type pictures struct {
	mu    sync.Mutex
	order []string
	jpegs map[string][]byte
	// open is the last PDF parsed for paging, so the pages of one document
	// asked for together parse it once. scanmeta.Pages is not safe for
	// concurrent use, so it is only touched under mu.
	openDigest string
	open       *scanmeta.Pages
}

const maxPictures = 96

// The sizes a page is drawn at, by the name a request gives.
const (
	// sizeThumb is the strip of pages beside the preview: thumb.GridSize.
	sizeThumb = "thumb"
	// sizePage is the page as first shown: thumb.PreviewSize.
	sizePage = "page"
	// sizeLarge is for a page zoomed past sizePage: largeSize on the long
	// side, or the scan's own size when that is smaller.
	sizeLarge = "large"
)

// largeSize is enough for a 300 dpi A4 scan at its own resolution.
const largeSize = 3600

func newPictures() *pictures { return &pictures{jpegs: map[string][]byte{}} }

// page is the JPEG of one page, counting from 1, at a size above.
func (p *pictures) page(path, digest string, n int, size string) ([]byte, error) {
	key := digest + "/" + strconv.Itoa(n) + "/" + size
	p.mu.Lock()
	defer p.mu.Unlock()
	if jpeg, ok := p.jpegs[key]; ok {
		return jpeg, nil
	}
	if p.openDigest != digest {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		pages, err := scanmeta.OpenPages(data)
		if err != nil {
			return nil, err
		}
		p.openDigest, p.open = digest, pages
	}
	picture, err := p.open.Page(n)
	if err != nil {
		return nil, err
	}
	if size == sizeLarge {
		large, err := thumb.RenderPictureAt(picture, largeSize)
		if err != nil {
			return nil, err
		}
		p.keep(key, large)
		return large, nil
	}
	// The two smaller sizes come together, the thumbnail drawn from the page.
	pair, err := thumb.RenderPicture(picture)
	if err != nil {
		return nil, err
	}
	base := digest + "/" + strconv.Itoa(n) + "/"
	p.keep(base+sizePage, pair.Preview)
	p.keep(base+sizeThumb, pair.Grid)
	if size == sizeThumb {
		return pair.Grid, nil
	}
	return pair.Preview, nil
}

// keep holds a drawn page, dropping the oldest past maxPictures. Callers
// hold mu.
func (p *pictures) keep(key string, jpeg []byte) {
	if _, ok := p.jpegs[key]; !ok {
		p.order = append(p.order, key)
	}
	p.jpegs[key] = jpeg
	if len(p.order) > maxPictures {
		delete(p.jpegs, p.order[0])
		p.order = p.order[1:]
	}
}

// count is the PDF's number of pages.
func (p *pictures) count(path, digest string) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.openDigest != digest {
		data, err := os.ReadFile(path)
		if err != nil {
			return 0, err
		}
		pages, err := scanmeta.OpenPages(data)
		if err != nil {
			return 0, err
		}
		p.openDigest, p.open = digest, pages
	}
	return p.open.Count(), nil
}

// pages answers how many pages a PDF has, and the digest its pictures are
// asked for by, so a browser may keep them.
func (s server) pages(w http.ResponseWriter, r *http.Request) {
	path, err := s.pdfPath(r.URL.Query())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	digest, err := tree.FileDigest(path)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	// The first page is drawn now: it is wanted next anyway, and a first page
	// with no picture means a PDF made on a computer, which the viewer shows.
	count, err := s.pictures.count(path, digest)
	if err == nil {
		_, err = s.pictures.page(path, digest, 1, sizePage)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"count": 0, "digest": digest})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"count": count, "digest": digest})
}

// page is one page's picture. 204 says the page has none, and the viewer
// should be used instead.
func (s server) page(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	n, err := strconv.Atoi(query.Get("n"))
	if err != nil || n < 1 {
		http.Error(w, "page numbers count from 1", http.StatusBadRequest)
		return
	}
	path, err := s.pdfPath(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	digest, err := tree.FileDigest(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	size := query.Get("size")
	switch size {
	case "":
		size = sizePage
	case sizePage, sizeThumb, sizeLarge:
	default:
		http.Error(w, "size is thumb, page or large", http.StatusBadRequest)
		return
	}
	jpeg, err := s.pictures.page(path, digest, n, size)
	if err != nil {
		if errors.Is(err, scanmeta.ErrNoPageImage) || errors.Is(err, thumb.ErrNoImage) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	// The request names the digest, so the picture never changes under it.
	if query.Get("v") == digest {
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
	}
	_, _ = w.Write(jpeg)
}
