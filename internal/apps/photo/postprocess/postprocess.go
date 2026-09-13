// Package postprocess organizes already verified and published photos.
package postprocess

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

func Run(root string, files []File) Result {
	result := Result{Files: make([]Outcome, len(files))}
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
			result.Files[index] = outcome
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
			result.Files[index] = outcome
			continue
		}
		target := filepath.Join(root, date, filepath.Base(path))
		outcome.Date, outcome.DateSource, outcome.Final = date, source, target
		if samePath(path, target) {
			outcome.Status = Moved
			result.Files[index] = outcome
			continue
		}
		if _, err := os.Lstat(target); err == nil {
			outcome.Status, outcome.Error = Failed, "destination already exists"
			result.Files[index] = outcome
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			outcome.Status, outcome.Error = Failed, err.Error()
			result.Files[index] = outcome
			continue
		}
		if err := ensureDateDirectory(root, date); err != nil {
			outcome.Status, outcome.Error = Failed, err.Error()
			result.Files[index] = outcome
			continue
		}
		if err := os.Rename(path, target); err != nil {
			outcome.Status, outcome.Error = Failed, err.Error()
		} else {
			outcome.Status = Moved
		}
		result.Files[index] = outcome
	}
	return result
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
