// Package pdflist finds the PDFs under a folder. It reads names and sizes and
// nothing else: listing a tree must never write to it.
package pdflist

import (
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// File is one PDF, named by its slash-separated path relative to the root.
type File struct {
	Path     string
	Size     int64
	Modified time.Time
}

// List walks fsys and returns every PDF in it, sorted by path. A name ending
// in .pdf in any case counts. Hidden files and folders — a leading dot — are
// skipped, which keeps out macOS's ._ files and the part files a copy leaves
// behind. A folder that cannot be read is skipped rather than ending the list.
func List(fsys fs.FS) ([]File, error) {
	var files []File
	err := fs.WalkDir(fsys, ".", func(name string, entry fs.DirEntry, err error) error {
		if name != "." && strings.HasPrefix(path.Base(name), ".") {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if err != nil {
			if name == "." {
				return err
			}
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !strings.EqualFold(path.Ext(name), ".pdf") {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		files = append(files, File{Path: name, Size: info.Size(), Modified: info.ModTime()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}
