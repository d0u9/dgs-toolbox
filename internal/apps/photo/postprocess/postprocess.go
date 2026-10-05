// Package postprocess organizes already verified and published photos.
package postprocess

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"dgs-toolbox/internal/apps/photo/skipdirs"
	"dgs-toolbox/internal/verifiedcopy"

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
	// group is the shot this file belongs to: its source folder and filename
	// stem. A RAW and its JPG move together or not at all.
	group string
}

type Result struct {
	Files []Outcome
}

type File struct {
	Source string
	Path   string
}

// ExistingFile carries verified shot context. Its source name groups it with
// new files; its destination bytes must still match before organizing. It is
// read-only unless its File also appears among the rename targets.
type ExistingFile struct {
	File
	Digest string
	Size   int64
}

var rawExtensions = map[string]struct{}{
	"3FR": {}, "ARW": {}, "CR2": {}, "CR3": {}, "DNG": {}, "ERF": {},
	"IIQ": {}, "KDC": {}, "MEF": {}, "MOS": {}, "MRW": {}, "NEF": {},
	"NRW": {}, "ORF": {}, "PEF": {}, "RAF": {}, "RAW": {}, "RW2": {},
	"RWL": {}, "SR2": {}, "SRF": {},
}

// DefaultLayout names date folders like 20261010.
const DefaultLayout = "YYYYMMDD"

// Plan reads every photo's capture date and decides where it would move,
// without changing any file. Check ExistingDates before Apply so a batch never
// silently mixes into date folders that are already there.
type Plan struct {
	Root     string
	Files    []Outcome
	existing []ExistingFile
	verified map[string]string
}

// ValidateLayout checks a date folder layout built from YYYY, MM and DD, with
// "-", "_" or "." between them and "/" to nest folders.
func ValidateLayout(layout string) error {
	rest := strings.NewReplacer("YYYY", "", "MM", "", "DD", "").Replace(layout)
	if rest == layout {
		return fmt.Errorf("folder format %q needs at least one of YYYY, MM, DD", layout)
	}
	if strings.Trim(rest, "-_./") != "" {
		return fmt.Errorf("folder format %q may only contain YYYY, MM, DD, -, _, . and /", layout)
	}
	for _, part := range strings.Split(layout, "/") {
		if strings.Trim(part, "-_.") == "" {
			return fmt.Errorf("folder format %q has an empty folder name", layout)
		}
	}
	return nil
}

// FolderName formats a capture time with a layout accepted by ValidateLayout.
func FolderName(layout string, captured time.Time) string {
	return filepath.FromSlash(strings.NewReplacer(
		"YYYY", captured.Format("2006"), "MM", captured.Format("01"), "DD", captured.Format("02"),
	).Replace(layout))
}

func Run(root string, files []File) Result {
	return Apply(NewPlan(root, files))
}

func NewPlan(root string, files []File) Plan {
	return NewLayoutPlan(root, files, DefaultLayout)
}

// NewPlanWithExisting reads verified counterparts; only files are eligible to
// move. A counterpart also listed in files retains both checks while moving.
// Cancelling ctx stops reading counterparts; their shots are then refused,
// never moved unchecked.
func NewPlanWithExisting(ctx context.Context, root string, files []File, existing []ExistingFile) Plan {
	return newLayoutPlan(ctx, root, files, DefaultLayout, existing)
}

// NewLayoutPlan plans moves into root/<layout>/, where layout must pass
// ValidateLayout. Files may live anywhere; only their date folder is under root.
func NewLayoutPlan(root string, files []File, layout string) Plan {
	return newLayoutPlan(context.Background(), root, files, layout, nil)
}

func newLayoutPlan(ctx context.Context, root string, files []File, layout string, existing []ExistingFile) Plan {
	plan := Plan{Root: root, Files: make([]Outcome, len(files)), verified: make(map[string]string)}
	contextErrors := make(map[string]string)
	contextFolders := make(map[string]string)
	contextVersions := make(map[string]os.FileInfo)
	needed := make(map[string]bool)
	movable := make(map[File]bool)
	for _, file := range files {
		movable[file] = true
		if isJPEG(file.Source) || isRAW(file.Source) {
			needed[pairKey(file.Source)] = true
		}
	}
	metadataFiles := append([]File(nil), files...)
	for _, item := range existing {
		group := pairKey(item.Source)
		if !needed[group] || (!isJPEG(item.Source) && !isRAW(item.Source)) {
			continue
		}
		checked, err := verifiedcopy.Check(ctx, root, item.Path, nil, nil)
		if ctx.Err() != nil {
			contextErrors[group] = "organizing was cancelled"
			continue
		}
		if err != nil || checked.Size != item.Size || checked.Digest != item.Digest {
			contextErrors[group] = "existing shot counterpart no longer matches its verified content"
			continue
		}
		if !movable[item.File] && !strings.EqualFold(stem(item.Source), stem(item.Path)) {
			contextErrors[group] = "renamed existing counterpart has ambiguous shot ownership"
			continue
		}
		if !movable[item.File] {
			folder := filepath.Clean(filepath.Dir(item.Path))
			if prior, ok := contextFolders[group]; ok && prior != folder {
				contextErrors[group] = "existing shot counterparts occupy different folders"
			}
			contextFolders[group] = folder
		}
		plan.existing = append(plan.existing, item)
		if prior, ok := plan.verified[filepath.Clean(item.Path)]; ok && prior != group {
			contextErrors[prior], contextErrors[group] = "existing counterpart belongs to multiple source shots", "existing counterpart belongs to multiple source shots"
		}
		plan.verified[filepath.Clean(item.Path)] = group
		contextVersions[item.Path] = checked.Source
		if !movable[item.File] {
			metadataFiles = append(metadataFiles, item.File)
		}
	}
	// A shot's files pair by name. Keep both may have renamed one of them
	// (IMG_0001-2.JPG beside IMG_0001.CR2); in a date folder that pairs with
	// nothing, or with another shot, so the shot stays where it is.
	stems := make(map[string]string)
	for _, file := range metadataFiles {
		if !isJPEG(file.Source) && !isRAW(file.Source) {
			continue
		}
		group, name := pairKey(file.Source), strings.ToLower(stem(file.Path))
		if prior, ok := stems[group]; ok && prior != name && contextErrors[group] == "" {
			contextErrors[group] = "files of this shot were published under different names (Keep both); pair them by hand"
		}
		stems[group] = name
	}
	jpegByStem := make(map[string]string)
	jpegDates := make(map[string]time.Time)
	jpegErrors := make(map[string]error)
	for _, file := range metadataFiles {
		if isJPEG(file.Source) && contextErrors[pairKey(file.Source)] == "" {
			key := pairKey(file.Source)
			jpegByStem[key] = file.Path
			jpegDates[key], jpegErrors[key] = captureDate(file.Path)
		}
	}
	for _, item := range plan.existing {
		info, err := verifiedcopy.FileInfo(root, item.Path)
		if err != nil || !verifiedcopy.SameVersion(info, contextVersions[item.Path]) {
			contextErrors[pairKey(item.Source)] = "existing counterpart changed during metadata reading"
		}
	}
	for index, file := range files {
		path := file.Path
		outcome := Outcome{Original: path, Final: path, Status: Skipped, group: pairKey(file.Source)}
		if !isJPEG(file.Source) && !isRAW(file.Source) {
			plan.Files[index] = outcome
			continue
		}
		if message := contextErrors[outcome.group]; message != "" {
			outcome.Status, outcome.Error = Failed, message
			plan.Files[index] = outcome
			continue
		}
		datePath, source := path, "RAW metadata"
		var captured time.Time
		if isJPEG(file.Source) {
			source = "JPG EXIF"
			captured = jpegDates[pairKey(file.Source)]
		} else if jpeg, ok := jpegByStem[pairKey(file.Source)]; ok {
			datePath, source = jpeg, "sidecar JPG EXIF"
			captured = jpegDates[pairKey(file.Source)]
		}
		var err error
		if isJPEG(file.Source) || datePath != path {
			err = jpegErrors[pairKey(file.Source)]
		} else {
			captured, err = captureDate(datePath)
		}
		if err != nil {
			outcome.Status, outcome.Error = Failed, fmt.Sprintf("read %s: %v", source, err)
			plan.Files[index] = outcome
			continue
		}
		date := FolderName(layout, captured)
		outcome.Date, outcome.DateSource, outcome.Final = date, source, filepath.Join(root, date, filepath.Base(path))
		if existingFolder, ok := contextFolders[outcome.group]; ok && existingFolder != filepath.Clean(filepath.Dir(outcome.Final)) {
			outcome.Status, outcome.Error = Failed, "existing shot counterpart is outside the planned date folder"
			outcome.Final = outcome.Original
			plan.Files[index] = outcome
			continue
		}
		outcome.Status = Moved
		plan.Files[index] = outcome
	}
	refuseCollisions(&plan)
	return plan
}

// refuseCollisions keeps a shot from landing beside a different shot of the
// same name.
//
// Date folders are flat, so two folders of one card that both hold IMG_0001 —
// a camera whose counter rolled over in one day — meet in one folder. Moving
// file by file would let the first IMG_0001.JPG in and the other shot's
// IMG_0001.CR2 after it, and every tool reading that folder would pair them.
// So a name is checked per shot, against the batch and against what the
// folder already holds, and a shot that collides stays where it is whole.
func refuseCollisions(plan *Plan) {
	type claim struct {
		group    string
		original string
	}
	claims := make(map[string]claim)
	conflicts := make(map[string]string)
	originals := make(map[string]struct{})
	for _, outcome := range plan.Files {
		originals[filepath.Clean(outcome.Original)] = struct{}{}
	}
	folders := make(map[string][]os.DirEntry)
	for _, outcome := range plan.Files {
		if outcome.Status != Moved {
			continue
		}
		key := stemKey(outcome.Final)
		if other, ok := claims[key]; ok && other.group != outcome.group {
			message := fmt.Sprintf("another shot named %s also goes to %s: %s", stem(outcome.Final), outcome.Date, other.original)
			conflicts[outcome.group] = message
			conflicts[other.group] = fmt.Sprintf("another shot named %s also goes to %s: %s", stem(outcome.Final), outcome.Date, outcome.Original)
			continue
		}
		claims[key] = claim{group: outcome.group, original: outcome.Original}
		folder := filepath.Dir(outcome.Final)
		entries, read := folders[folder]
		if !read {
			entries, _ = os.ReadDir(folder)
			folders[folder] = entries
		}
		for _, entry := range entries {
			existing := filepath.Join(folder, entry.Name())
			if _, own := originals[filepath.Clean(existing)]; own {
				continue
			}
			if stemKey(existing) == key {
				if group, ok := plan.verified[filepath.Clean(existing)]; ok && group == outcome.group {
					continue
				}
				conflicts[outcome.group] = fmt.Sprintf("%s already has %s", outcome.Date, entry.Name())
				break
			}
		}
	}
	for index, outcome := range plan.Files {
		if message, ok := conflicts[outcome.group]; ok && outcome.Status == Moved {
			plan.Files[index].Status, plan.Files[index].Error = Failed, message
			plan.Files[index].Final = outcome.Original
		}
	}
}

func stem(path string) string {
	base := filepath.Base(path)
	return base[:len(base)-len(filepath.Ext(base))]
}

// stemKey compares names the way the case-insensitive volumes photos live on do.
func stemKey(path string) string {
	return strings.ToLower(filepath.Join(filepath.Dir(path), stem(path)))
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
	return ApplyContext(context.Background(), plan)
}

// ApplyContext is Apply whose counterpart checks stop when ctx is cancelled.
// Cancellation is honoured only before the first move, so a shot is never
// left half moved by it.
func ApplyContext(ctx context.Context, plan Plan) Result {
	plan.Files = append([]Outcome(nil), plan.Files...)
	result := Result{Files: make([]Outcome, len(plan.Files))}
	// failedGroups holds why a shot stays where it is.
	failedGroups := make(map[string]string)
	// The user may have left a planned import at the date-folder confirmation.
	// Re-read witnesses before any move; stale context cannot waive a collision.
	for _, item := range plan.existing {
		checked, err := verifiedcopy.Check(ctx, plan.Root, item.Path, nil, nil)
		if err != nil || checked.Size != item.Size || checked.Digest != item.Digest {
			failedGroups[pairKey(item.Source)] = "existing shot counterpart changed before organizing"
		}
	}
	if err := ctx.Err(); err != nil {
		for index, outcome := range plan.Files {
			result.Files[index] = outcome
			if outcome.Status == Moved && !samePath(outcome.Original, outcome.Final) {
				result.Files[index].Status, result.Files[index].Error = Failed, "organizing was cancelled"
				result.Files[index].Final = outcome.Original
			}
		}
		return result
	}
	// Recheck shot-level occupancy after the confirmation as well.
	refuseCollisions(&plan)
	for index, outcome := range plan.Files {
		result.Files[index] = outcome
		if outcome.Status != Moved || samePath(outcome.Original, outcome.Final) {
			continue
		}
		fail := func(message string) {
			result.Files[index].Status, result.Files[index].Error = Failed, message
			result.Files[index].Final = outcome.Original
			if _, recorded := failedGroups[outcome.group]; !recorded {
				failedGroups[outcome.group] = "another file of this shot could not be moved"
			}
		}
		// Once part of a shot could not move, the rest stays with it.
		if reason, failed := failedGroups[outcome.group]; failed {
			fail(reason)
			continue
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
		if err := verifiedcopy.Publish(outcome.Original, outcome.Final); err != nil {
			fail(err.Error())
		}
	}
	return result
}

// TreeFiles lists the regular files in dir and every subfolder, skipping
// hidden files and folders and not following symlinks.
func TreeFiles(dir string) ([]File, error) {
	var files []File
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != dir && (strings.HasPrefix(entry.Name(), ".") || entry.IsDir() && skipdirs.Skip(entry.Name())) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			files = append(files, File{Source: path, Path: path})
		}
		return nil
	})
	return files, err
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

// ensureDateDirectory creates each folder of a possibly nested date path
// under root, refusing any part that is a symlink or not a directory.
func ensureDateDirectory(root, date string) error {
	directory := root
	for _, part := range strings.Split(date, string(filepath.Separator)) {
		directory = filepath.Join(directory, part)
		info, err := os.Lstat(directory)
		if err == nil {
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("date destination is not a real directory: %s", directory)
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Mkdir(directory, 0o755); err != nil {
			return err
		}
	}
	return nil
}

func captureDate(path string) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()
	metadata, err := exif.Decode(file)
	if err != nil && metadata == nil {
		return time.Time{}, err
	}
	return metadata.DateTime()
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
