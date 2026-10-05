// Package pdflist finds PDFs and supported document images under a folder. It reads names and sizes and
// nothing else: listing a tree must never write to it.
package pdflist

import (
	"dgs-toolbox/internal/doc/imagepdf"
	"io/fs"
	"path"
	"sort"
	"strings"
	"time"
)

// File is one importable file, named by its slash-separated path relative to the root.
type File struct {
	Path     string
	Size     int64
	Modified time.Time
}

// List walks fsys and returns every PDF and supported image, sorted by path.
// Extensions are case insensitive. Hidden files and folders — a leading dot — are
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
		if !entry.Type().IsRegular() || !Supported(name) {
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

// Supported reports the PDF and image formats accepted for import.
func Supported(name string) bool {
	return strings.EqualFold(path.Ext(name), ".pdf") || imagepdf.Supported(name)
}
