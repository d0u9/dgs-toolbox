package tree

import (
	"dgs-toolbox/internal/doc/docfile"
	"dgs-toolbox/internal/doc/pdflist"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func SourcePath(dir, path string, all ...bool) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", errors.New("open a folder by its full path")
	}
	if !fs.ValidPath(path) || path == "." || (!docfile.Importable(path) && !(len(all) > 0 && all[0])) {
		return "", errors.New("not a PDF or image in the opened folder")
	}
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ".") {
			return "", errors.New("not a PDF in the opened folder")
		}
	}
	full := filepath.Join(dir, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return full, nil
}

func (st *Store) ImportSource(dir, path string) (string, error) {
	full, err := SourcePath(dir, path, st.InPlace())
	if err != nil {
		return "", err
	}
	if err := st.ValidateImportSource(full); err != nil {
		return "", err
	}
	return full, nil
}
func (st *Store) ValidateImportSource(full string) error {
	if !st.InPlace() {
		return nil
	}
	_, err := st.relativePath(full)
	return err
}
func (st *Store) ImportFiles(dir string) ([]pdflist.File, error) {
	files, err := pdflist.List(os.DirFS(dir), st.InPlace())
	if err != nil || !st.InPlace() {
		return files, err
	}
	scoped := *st
	scoped.sourceNames = map[string]map[string][]string{}
	safe := files[:0]
	for _, file := range files {
		if scoped.ValidateImportSource(filepath.Join(dir, filepath.FromSlash(file.Path))) == nil {
			safe = append(safe, file)
		}
	}
	return safe, nil
}
