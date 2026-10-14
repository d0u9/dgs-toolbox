// Package postprocess organizes already verified and published photos.
package postprocess

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	// NoDate marks unavailable capture metadata, excluding file I/O errors: a
	// RAW container the EXIF reader does not support (CR3, RAF, ORF) without a
	// JPG, or a JPG without EXIF. The file stays where it is, unharmed; a
	// caller may treat it as not organizable rather than as a fault.
	NoDate bool
	// group is the shot this file belongs to: its source folder and filename
	// stem. A RAW and its JPG move together or not at all.
	group string
}

type Result struct {
	Files []Outcome
}

// Progress reports work within one organizing phase. Total zero means that
// phase has no measurable file count (for example collision checks).
// Callbacks run synchronously and must return promptly.
type Progress struct {
	Phase     string
	Path      string
	Completed int
	Total     int
}

func reportProgress(report func(Progress), phase, path string, completed, total int) {
	if report != nil {
		report(Progress{Phase: phase, Path: path, Completed: completed, Total: total})
	}
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
	// rootInfo is Root's identity when the plan was made. The user may leave
	// the plan at its confirmation while a disk is unmounted; Apply then finds
	// another directory, or an empty mount point, and must not fill it.
	rootInfo os.FileInfo
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
	return newLayoutPlan(ctx, root, files, DefaultLayout, existing, nil)
}

// NewPlanWithProgress is NewPlanWithExisting with per-phase activity reports.
func NewPlanWithProgress(ctx context.Context, root string, files []File, existing []ExistingFile, report func(Progress)) Plan {
	return newLayoutPlan(ctx, root, files, DefaultLayout, existing, report)
}

// NewLayoutPlan plans moves into root/<layout>/, where layout must pass
// ValidateLayout. Files may live anywhere; only their date folder is under root.
func NewLayoutPlan(root string, files []File, layout string) Plan {
	return newLayoutPlan(context.Background(), root, files, layout, nil, nil)
}

func newLayoutPlan(ctx context.Context, root string, files []File, layout string, existing []ExistingFile, report func(Progress)) Plan {
	plan := Plan{Root: root, Files: make([]Outcome, len(files)), verified: make(map[string]string)}
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		plan.rootInfo = info
	}
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
	for index, item := range existing {
		reportProgress(report, "Checking existing copies", item.Path, index, len(existing))
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
	for index, file := range metadataFiles {
		reportProgress(report, "Reading capture dates", file.Path, index, len(metadataFiles))
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
		reportProgress(report, "Planning date folders", file.Path, index, len(files))
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
			outcome.NoDate = errors.Is(err, errNoCaptureDate)
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
	reportProgress(report, "Checking date folders", "", 0, 0)
	refuseCollisions(&plan)
	reportProgress(report, "Planning complete", "", len(files), len(files))
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
	return ApplyWithProgress(ctx, plan, nil)
}

// ApplyWithProgress preserves ApplyContext's shot and cancellation contract,
// and reports checks and file outcomes without changing the move order.
func ApplyWithProgress(ctx context.Context, plan Plan, report func(Progress)) Result {
	plan.Files = append([]Outcome(nil), plan.Files...)
	result := Result{Files: make([]Outcome, len(plan.Files))}
	// failedGroups holds why a shot stays where it is.
	failedGroups := make(map[string]string)
	// The user may have left a planned import at the date-folder confirmation.
	// Re-read witnesses before any move; stale context cannot waive a collision.
	for index, item := range plan.existing {
		reportProgress(report, "Rechecking existing copies", item.Path, index, len(plan.existing))
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
	reportProgress(report, "Rechecking date folders", "", 0, 0)
	refuseCollisions(&plan)
	for index, outcome := range plan.Files {
		reportProgress(report, "Organizing files", outcome.Original, index, len(plan.Files))
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
		if err := verifiedcopy.CheckRoot(plan.Root, plan.rootInfo); err != nil {
			fail(err.Error())
			continue
		}
		if err := verifiedcopy.EnsureDirectory(plan.Root, filepath.Dir(outcome.Final)); err != nil {
			fail(err.Error())
			continue
		}
		if err := verifiedcopy.CheckRoot(plan.Root, plan.rootInfo); err != nil {
			fail(err.Error())
			continue
		}
		if err := verifiedcopy.Publish(outcome.Original, outcome.Final); err != nil {
			fail(err.Error())
			continue
		}
		for _, directory := range []string{filepath.Dir(outcome.Final), filepath.Dir(outcome.Original)} {
			if err := syncDirectory(directory); err != nil {
				// The move happened: retain its real path even when durability is unknown.
				result.Files[index].Status = Failed
				result.Files[index].Error = fmt.Sprintf("moved to %s; sync directory: %v", outcome.Final, err)
				failedGroups[outcome.group] = "another file of this shot could not be synced"
				break
			}
		}
	}
	reportProgress(report, "Organizing files", "", len(plan.Files), len(plan.Files))
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

// syncDirectory lets tests fail a rename's durability check.
var syncDirectory = verifiedcopy.SyncDirectory

func captureDate(path string) (time.Time, error) {
	file, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer file.Close()
	return captureDateReader(file)
}

var errNoCaptureDate = errors.New("no readable capture date")

// The EXIF decoder formats reader errors without wrapping them. Keep the
// underlying non-EOF error so a disconnected mount is not treated as missing
// metadata or an unsupported container.
type captureReader struct {
	io.Reader
	err error
}

func (r *captureReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		r.err = err
	}
	return n, err
}

func captureDateReader(input io.Reader) (time.Time, error) {
	reader := &captureReader{Reader: input}
	metadata, err := exif.Decode(reader)
	if reader.err != nil {
		return time.Time{}, reader.err
	}
	if err != nil && metadata == nil {
		return time.Time{}, fmt.Errorf("%w: %v", errNoCaptureDate, err)
	}
	date, err := metadata.DateTime()
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %v", errNoCaptureDate, err)
	}
	return date, nil
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
