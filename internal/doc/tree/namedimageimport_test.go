package tree

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNamedImageImport(t *testing.T) {
	for _, scenario := range []string{"keep", "PDF only", "PDF exists", "folder exists", "duplicate filename", "invalid name"} {
		t.Run(scenario, func(t *testing.T) {
			st := inPlaceStore(t)
			inputs := t.TempDir()
			var sources []string
			var originals [][]byte
			for i, name := range []string{"1.jpg", "2.jpg", "3.jpg"} {
				img := image.NewNRGBA(image.Rect(0, 0, 12, 18))
				for y := 0; y < 18; y++ {
					for x := 0; x < 12; x++ {
						img.Set(x, y, color.NRGBA{R: uint8(80 * i), A: 255})
					}
				}
				var data bytes.Buffer
				if err := jpeg.Encode(&data, img, nil); err != nil {
					t.Fatal(err)
				}
				source := filepath.Join(inputs, name)
				if err := os.WriteFile(source, data.Bytes(), 0600); err != nil {
					t.Fatal(err)
				}
				sources = append(sources, source)
				originals = append(originals, bytes.Clone(data.Bytes()))
			}
			req := importRequest(sources[0], "alice")
			req.Pages, req.Folder, req.PDFName, req.KeepOriginal = sources[1:], st.Root(), "登记表.pdf", scenario != "PDF only"
			switch scenario {
			case "PDF exists":
				if err := os.WriteFile(filepath.Join(st.Root(), req.PDFName), []byte("existing"), 0600); err != nil {
					t.Fatal(err)
				}
			case "folder exists":
				if err := os.Mkdir(filepath.Join(st.Root(), "登记表"), 0755); err != nil {
					t.Fatal(err)
				}
			case "duplicate filename":
				req.Pages = []string{sources[0], sources[2]}
			case "invalid name":
				req.PDFName = "../escape.pdf"
			}
			before := importFiles(t, st.Root())
			item, err := st.Import(context.Background(), req)
			if scenario != "keep" && scenario != "PDF only" {
				if err == nil {
					t.Fatal("conflict accepted")
				}
				if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
					t.Fatal("failed import changed files")
				}
				if scenario != "folder exists" {
					if _, err := os.Stat(filepath.Join(st.Root(), "登记表")); !os.IsNotExist(err) {
						t.Fatal("failed import left image directory")
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			r := item.Revisions[0]
			if r.Path != "登记表.pdf" {
				t.Fatal(r.Path)
			}
			pdf, err := os.ReadFile(filepath.Join(st.Root(), r.Path))
			if err != nil || !bytes.HasPrefix(pdf, []byte("%PDF-")) || !bytes.Contains(pdf, []byte("/Count 3")) {
				t.Fatal("expected three-page PDF", err)
			}
			if req.KeepOriginal {
				if len(r.Originals) != 3 {
					t.Fatal("missing originals")
				}
				for i, o := range r.Originals {
					if o.Path != "登记表/"+filepath.Base(sources[i]) {
						t.Fatal(o.Path)
					}
					data, err := os.ReadFile(st.OriginalPath(item, r.Digest, o))
					if err != nil || !bytes.Equal(data, originals[i]) {
						t.Fatal("original changed", err)
					}
				}
			} else if _, err := os.Stat(filepath.Join(st.Root(), "登记表")); !os.IsNotExist(err) {
				t.Fatal("PDF-only import created image directory")
			}
			for i, source := range sources {
				data, err := os.ReadFile(source)
				if err != nil || !bytes.Equal(data, originals[i]) {
					t.Fatal("source changed", err)
				}
			}
		})
	}
}

func TestNamedImageImportRollsBackChangedOriginal(t *testing.T) {
	st := inPlaceStore(t)
	source, _ := docImage(t, 120)
	ctx := context.Background()
	input, err := st.prepareInput(ctx, []string{source})
	if err != nil {
		t.Fatal(err)
	}
	defer input.cleanup()
	digest, err := input.digest()
	if err != nil {
		t.Fatal(err)
	}
	item := Item{ID: "00000000000000000000000001", Type: "paper", Kind: KindDocument}
	item.saveSnapshot("paper", map[string]string{"owner": "alice"}, digest, input.name, "", time.Unix(0, 0))
	if err := os.WriteFile(source, []byte("changed original"), 0600); err != nil {
		t.Fatal(err)
	}
	before := importFiles(t, st.Root())
	req := importRequest(source, "alice")
	req.Folder, req.PDFName, req.KeepOriginal = st.Root(), "registration.pdf", true
	if _, err := st.importInPlace(ctx, req, input, item, digest, nil); err == nil {
		t.Fatal("changed original accepted")
	}
	if !reflect.DeepEqual(before, importFiles(t, st.Root())) {
		t.Fatal("failed archive left published files")
	}
	if _, err := os.Stat(filepath.Join(st.Root(), "registration")); !os.IsNotExist(err) {
		t.Fatal("failed archive left image directory")
	}
}
