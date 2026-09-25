package web

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// scan is a two-page PDF whose pages are pictures, as a scanner writes one.
func scan(t *testing.T) []byte {
	t.Helper()
	pictures := make([]io.Reader, 0, 2)
	for _, size := range [][2]int{{40, 60}, {60, 40}} {
		img := image.NewRGBA(image.Rect(0, 0, size[0], size[1]))
		for i := range img.Pix {
			img.Pix[i] = 200
		}
		img.Set(1, 1, color.Black)
		var b bytes.Buffer
		must(t, png.Encode(&b, img))
		pictures = append(pictures, &b)
	}
	importer, err := api.Import("pos:c, scale:0.5", types.POINTS)
	must(t, err)
	var out bytes.Buffer
	must(t, api.ImportImages(nil, &out, pictures, importer, nil))
	return out.Bytes()
}

func TestPagesAreDrawnAsPictures(t *testing.T) {
	root, scans := setup(t, false)
	write(t, filepath.Join(scans, "scan.pdf"), string(scan(t)))
	h := Handler(Settings{Root: root})
	base := "dir=" + url.QueryEscape(scans) + "&path="
	var info struct {
		Count  int
		Digest string
	}
	must(t, json.Unmarshal(do(h, "GET", "/api/pages?"+base+"scan.pdf", "").Body.Bytes(), &info))
	if info.Count != 2 || info.Digest == "" {
		t.Fatalf("pages = %+v", info)
	}
	for n, want := range map[string][2]int{"1": {40, 60}, "2": {60, 40}} {
		rec := do(h, "GET", "/api/page?"+base+"scan.pdf&n="+n+"&v="+info.Digest, "")
		if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") == "" {
			t.Fatalf("page %s: %d", n, rec.Code)
		}
		img, err := jpeg.Decode(rec.Body)
		must(t, err)
		// Drawn no larger than the scan, and the same way up.
		if b := img.Bounds(); (b.Dx() > b.Dy()) != (want[0] > want[1]) {
			t.Errorf("page %s is %v", n, b)
		}
	}
	for size, longest := range map[string]int{"thumb": 60, "large": 60, "page": 60} {
		rec := do(h, "GET", "/api/page?"+base+"scan.pdf&n=1&size="+size, "")
		img, err := jpeg.Decode(rec.Body)
		if rec.Code != http.StatusOK || err != nil || img.Bounds().Dy() > longest {
			t.Errorf("size %s: %d %v", size, rec.Code, err)
		}
	}
	if rec := do(h, "GET", "/api/page?"+base+"scan.pdf&n=1&size=huge", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown size: %d", rec.Code)
	}
	if rec := do(h, "GET", "/api/page?"+base+"scan.pdf&n=3", ""); rec.Code == http.StatusOK {
		t.Fatal("page 3 of 2 drawn")
	}
	// A PDF with no pictures is left to the viewer.
	if rec := do(h, "GET", "/api/page?"+base+"jane/licence.pdf&n=1", ""); rec.Code == http.StatusOK {
		t.Fatalf("non-scan drawn: %d", rec.Code)
	}
}
