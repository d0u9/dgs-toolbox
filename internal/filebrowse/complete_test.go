package filebrowse

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompleteFolders(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"books", "boxes", "中文甲", "中文乙", ".hidden"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "boring.txt"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input, suffix string
		count         int
	}{
		{"bo", "bo", 2}, {"book", "books" + string(os.PathSeparator), 1},
		{"中文", "中文", 2}, {"missing", "", 0}, {".", ".hidden" + string(os.PathSeparator), 1},
	} {
		matches, common, err := Complete(tc.input, root)
		want := ""
		if tc.count > 0 {
			want = filepath.Join(root, tc.suffix)
			if tc.count == 1 {
				want += string(os.PathSeparator)
			}
		}
		if err != nil || len(matches) != tc.count || common != want {
			t.Fatalf("%q: %v %q %v, want %q", tc.input, matches, common, err, want)
		}
	}
	matches, _, err := Complete(root+string(os.PathSeparator), root)
	if err != nil || len(matches) != 4 {
		t.Fatalf("folder contents: %v %v", matches, err)
	}
}
