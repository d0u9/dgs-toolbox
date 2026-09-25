package pdflist

import (
	"testing"
	"testing/fstest"
)

func TestListFindsPDFsAndSkipsTheRest(t *testing.T) {
	fsys := fstest.MapFS{
		"b.pdf":                {Data: []byte("bb")},
		"A/licence.PDF":        {Data: []byte("a")},
		"notes.txt":            {Data: []byte("x")},
		".hidden.pdf":          {Data: []byte("x")},
		"._b.pdf":              {Data: []byte("x")},
		".trash/old.pdf":       {Data: []byte("x")},
		"A/deep/payslip.pdf":   {Data: []byte("ccc")},
		"A/deep/scan.pdf.part": {Data: []byte("x")},
	}
	files, err := List(fsys)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		path string
		size int64
	}{{"A/deep/payslip.pdf", 3}, {"A/licence.PDF", 1}, {"b.pdf", 2}}
	if len(files) != len(want) {
		t.Fatalf("got %d files %+v, want %d", len(files), files, len(want))
	}
	for i, w := range want {
		if files[i].Path != w.path || files[i].Size != w.size {
			t.Errorf("file %d = %s (%d), want %s (%d)", i, files[i].Path, files[i].Size, w.path, w.size)
		}
	}
}

func TestListEmpty(t *testing.T) {
	files, err := List(fstest.MapFS{})
	if err != nil || len(files) != 0 {
		t.Fatalf("got %v, %v", files, err)
	}
}
