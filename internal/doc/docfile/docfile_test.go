package docfile

import "testing"

func TestKinds(t *testing.T) {
	for name, want := range map[string][2]bool{
		"scan.pdf": {true, false}, "a/SCAN.PDF": {true, false}, "photo.JPG": {false, true},
		"photo.jpeg": {false, true}, "x.png": {false, true}, "x.HEIC": {false, true}, "x.tif": {false, true},
		"x.gif": {false, false}, "pdf": {false, false}, "x.pdf.txt": {false, false},
	} {
		if IsPDF(name) != want[0] || IsImage(name) != want[1] || Importable(name) != (want[0] || want[1]) {
			t.Errorf("%s: pdf %v image %v", name, IsPDF(name), IsImage(name))
		}
	}
}
