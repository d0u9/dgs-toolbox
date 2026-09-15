package fileexplorer

import (
	"path/filepath"
	"strings"
)

type filterKind int

const (
	directoryFilter filterKind = iota
	extensionFilter
	allFilesFilter
)

// Filter controls which filesystem entries are shown and selectable.
// Directories remain visible for navigation when filtering for files.
type Filter struct {
	kind       filterKind
	extensions []string
}

// Directories returns a filter that shows and selects directories only.
func Directories() Filter {
	return Filter{kind: directoryFilter}
}

// Extensions returns a file filter for the supplied filename extensions.
func Extensions(extensions ...string) Filter {
	cleaned := make([]string, 0, len(extensions))
	seen := make(map[string]bool)
	for _, extension := range extensions {
		extension = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), "."))
		if extension != "" && !seen[extension] {
			cleaned = append(cleaned, extension)
			seen[extension] = true
		}
	}
	if len(cleaned) == 0 {
		return AllFiles()
	}
	return Filter{kind: extensionFilter, extensions: cleaned}
}

// AllFiles returns a filter that shows every file and directory and selects
// either.
func AllFiles() Filter {
	return Filter{kind: allFilesFilter}
}

// Label is the compact filter name intended for an explorer header.
func (f Filter) Label() string {
	switch f.kind {
	case extensionFilter:
		labels := make([]string, len(f.extensions))
		for i, extension := range f.extensions {
			labels[i] = strings.ToUpper(extension)
		}
		return strings.Join(labels, ", ")
	case allFilesFilter:
		return "ALL FILES"
	default:
		return "DIR"
	}
}

func (f Filter) includesFile(name string) bool {
	if f.kind == directoryFilter {
		return false
	}
	if f.kind == allFilesFilter {
		return true
	}
	extension := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	for _, allowed := range f.extensions {
		if extension == allowed {
			return true
		}
	}
	return false
}

type config struct {
	filter     Filter
	showHidden bool
}

// Option configures a File Explorer model.
type Option func(*config)

// WithHidden opens the explorer with entries starting with . shown.
func WithHidden(show bool) Option {
	return func(config *config) {
		config.showHidden = show
	}
}

// WithFilter sets the entries displayed and selectable by the explorer.
func WithFilter(filter Filter) Option {
	return func(config *config) {
		config.filter = filter
	}
}
