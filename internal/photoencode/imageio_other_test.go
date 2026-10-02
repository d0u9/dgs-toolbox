//go:build !darwin || !cgo

package photoencode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeFallbackRefusesClearly(t *testing.T) {
	for _, kind := range []string{"HEIC", "TIFF"} {
		_, _, e := decodeNative(kind, []byte("x"), DefaultOptions())
		if e == nil || !strings.Contains(e.Error(), kind) || !strings.Contains(e.Error(), "ImageIO") {
			t.Fatal(e)
		}
	}
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "scan.tif"), nil, 0600)
	jobs, e := Plan(filepath.Join(root, "scan.tif"), filepath.Join(root, "out"), DefaultOptions())
	if e != nil || len(jobs) != 1 || !strings.Contains(jobs[0].Problem, "not supported") && !strings.Contains(jobs[0].Problem, "cgo") {
		t.Fatal(jobs, e)
	}
}

func TestTaggedImagesNeedColorSync(t *testing.T) {
	root := t.TempDir()
	in := filepath.Join(root, "gray.jpg")
	grayJPEG(t, in, profile("GRAY"))
	r := Encode(context.Background(), Job{Source: in, Destination: filepath.Join(root, "out.jpg")}, DefaultOptions())
	if r.Published || !strings.Contains(r.Error, "ColorSync") {
		t.Fatal(r)
	}
}
