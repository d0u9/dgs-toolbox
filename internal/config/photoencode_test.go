package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPhotoEncodeDefaultsAndOverrides(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "photo"), 0700)
	path := filepath.Join(root, "photo", PartFile)
	os.WriteFile(path, []byte(`{"encode":{"quality":91,"max_side":1600,"preserve_gps":true}}`), 0600)
	c, e := LoadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.PartErr("photo"); e != nil {
		t.Fatal(e)
	}
	v := c.Photo.Encode
	if v.Quality != 91 || v.MaxSide != 1600 || v.PPI != 240 || v.MaxPixels != 40000000 || !v.PreserveGPS {
		t.Fatal(v)
	}
	os.WriteFile(path, []byte(`{"encode":{"quality":0}}`), 0600)
	c, e = LoadDir(root)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.PartErr("photo"); e == nil || !strings.Contains(e.Error(), "encode.quality") {
		t.Fatalf("error: %v", e)
	}
}
