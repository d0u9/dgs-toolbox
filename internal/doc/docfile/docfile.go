// Package docfile names the files dgs doc imports: PDFs, and the images it
// turns into PDF pages. It reads only names, so listing a folder needs none
// of the decoders.
package docfile

import (
	"path"
	"strings"
)

// ImageExtensions are the image formats an import turns into PDF pages,
// lowercase and with their dot.
var ImageExtensions = []string{".jpg", ".jpeg", ".png", ".heic", ".heif", ".tif", ".tiff"}

// IsPDF reports whether name is a PDF, by its extension in any case.
func IsPDF(name string) bool { return strings.EqualFold(path.Ext(name), ".pdf") }

// IsImage reports whether name is an image an import takes, by its
// extension in any case.
func IsImage(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	for _, e := range ImageExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

// Importable reports whether name is a PDF or an image an import takes.
func Importable(name string) bool { return IsPDF(name) || IsImage(name) }
