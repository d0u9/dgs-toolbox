// Package postprocess organizes already verified and published photos.
package postprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

type Status string

const (
	Moved   Status = "moved"
	Skipped Status = "skipped"
	Failed  Status = "failed"
)

type Outcome struct {
	Original   string
	Final      string
	Date       string
	DateSource string
	Status     Status
	Error      string
}

type Result struct {
	Files []Outcome
}

type File struct {
	Source string
	Path   string
}

var rawExtensions = map[string]struct{}{
	"3FR": {}, "ARW": {}, "CR2": {}, "CR3": {}, "DNG": {}, "ERF": {},
	"IIQ": {}, "KDC": {}, "MEF": {}, "MOS": {}, "MRW": {}, "NEF": {},
	"NRW": {}, "ORF": {}, "PEF": {}, "RAF": {}, "RAW": {}, "RW2": {},
	"RWL": {}, "SR2": {}, "SRF": {},
}

// Plan reads every photo's capture date and decides where it would move,
// without changing any file. Check ExistingDates before Apply so a batch never
// silently mixes into date folders that are already there.
type Plan struct {
	Root  string
	Files []Outcome
}

func Run(root string, files []File) Result {
	return Apply(NewPlan(root, files))
}

func NewPlan(root string, files []File) Plan {
	plan := Plan{Root: root, Files: make([]Outcome, len(files))}
	jpegByStem := make(map[string]string)
	jpegDates := make(map[string]string)
	jpegErrors := make(map[string]error)
	for _, file := range files {
		if isJPEG(file.Source) {
			key := pairKey(file.Source)
			jpegByStem[key] = file.Path
			jpegDates[key], jpegErrors[key] = captureDate(file.Path)
		}
	}
	for index, file := range files {
		path := file.Path
		outcome := Outcome{Original: path, Final: path, Status: Skipped}
		if !isJPEG(file.Source) && !isRAW(file.Source) {
			plan.Files[index] = outcome
			continue
		}
		datePath, source := path, "RAW metadata"
		var date string
		if isJPEG(file.Source) {
			source = "JPG EXIF"
			date = jpegDates[pairKey(file.Source)]
		} else if jpeg, ok := jpegByStem[pairKey(file.Source)]; ok {
			datePath, source = jpeg, "sidecar JPG EXIF"
			date = jpegDates[pairKey(file.Source)]
		}
		var err error
		if isJPEG(file.Source) || datePath != path {
			err = jpegErrors[pairKey(file.Source)]
		} else {
			date, err = captureDate(datePath)
		}
		if err != nil {
			outcome.Status, outcome.Error = Failed, fmt.Sprintf("read %s: %v", source, err)
			plan.Files[index] = outcome
			continue
		}
		outcome.Date, outcome.DateSource, outcome.Final = date, source, filepath.Join(root, date, filepath.Base(path))
		outcome.Status = Moved
		plan.Files[index] = outcome
	}
	return plan
}

// Dates lists the distinct date folders the plan would move files into.
func (plan Plan) Dates() []string {
	seen := make(map[string]struct{})
	var dates []string
	for _, outcome := range plan.Files {
		if outcome.Status != Moved {
			continue
		}
		if _, ok := seen[outcome.Date]; !ok {
			seen[outcome.Date] = struct{}{}
			dates = append(dates, outcome.Date)
		}
	}
	sort.Strings(dates)
	return dates
}

// ExistingDates lists the planned date folders already present under Root.
func (plan Plan) ExistingDates() []string {
	var existing []string
	for _, date := range plan.Dates() {
		if _, err := os.Lstat(filepath.Join(plan.Root, date)); err == nil {
			existing = append(existing, date)
		}
	}
	return existing
}

// Apply moves the files a plan placed into date folders. It never overwrites.
func Apply(plan Plan) Result {
	result := Result{Files: make([]Outcome, len(plan.Files))}
	for index, outcome := range plan.Files {
		result.Files[index] = outcome
		if outcome.Status != Moved || samePath(outcome.Original, outcome.Final) {
			continue
		}
		fail := func(message string) {
			result.Files[index].Status, result.Files[index].Error = Failed, message
		}
		if _, err := os.Lstat(outcome.Final); err == nil {
			fail("destination already exists")
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			fail(err.Error())
			continue
		}
		if err := ensureDateDirectory(plan.Root, outcome.Date); err != nil {
			fail(err.Error())
			continue
		}
		if err := os.Rename(outcome.Original, outcome.Final); err != nil {
			fail(err.Error())
		}
	}
	return result
}

// FolderFiles lists the regular files directly inside dir, without recursing.
func FolderFiles(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, entry := range entries {
		if !entry.Type().IsRegular() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		files = append(files, File{Source: path, Path: path})
	}
	return files, nil
}

func ensureDateDirectory(root, date string) error {
	directory := filepath.Join(root, date)
	info, err := os.Lstat(directory)
	if err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("date destination is not a real directory: %s", directory)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Mkdir(directory, 0o755)
}

func captureDate(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	metadata, err := exif.Decode(file)
	if err != nil && metadata == nil {
		return "", err
	}
	captured, err := metadata.DateTime()
	if err != nil {
		return "", err
	}
	return captured.Format("20060102"), nil
}

func isJPEG(path string) bool {
	extension := strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
	return extension == "JPG" || extension == "JPEG"
}

func isRAW(path string) bool {
	_, ok := rawExtensions[strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))]
	return ok
}

func pairKey(path string) string {
	extension := filepath.Ext(path)
	return strings.ToLower(filepath.Join(filepath.Dir(path), filepath.Base(path)[:len(filepath.Base(path))-len(extension)]))
}

func samePath(left, right string) bool {
	return filepath.Clean(left) == filepath.Clean(right)
}
