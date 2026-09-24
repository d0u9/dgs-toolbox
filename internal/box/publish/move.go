package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/verifiedcopy"
)

// MoveRequest is one filed scan whose sidecar has just been saved.
type MoveRequest struct {
	Root       string
	MarkerName string
	// Path is the published file, inside the Box.
	Path string
	// Digest names the sidecar beside it.
	Digest string
	// Prefix is how many hex digits name that sidecar. Zero uses the default.
	Prefix int
	// Record is the sidecar as saved: its dates decide where the scan belongs.
	Record sidecar.File
	Now    time.Time
}

// MoveResult is where the scan and its sidecar are now.
type MoveResult struct {
	Path        string
	SidecarPath string
	// Moved is false when the scan was already where its dates place it.
	Moved bool
}

// Move puts a filed scan and its sidecar into the directory its dates place it
// in, when that is not where they are.
//
// A Box is one volume, so this is a rename: atomic, no copy and no readback,
// and the bytes and the digest are untouched. The file goes first and the
// sidecar follows; a sidecar that cannot follow puts the file back, so the two
// are never left in different directories. The name keeps its stem and gains
// a longer digest prefix only if the new directory already uses the old one.
func Move(request MoveRequest) (MoveResult, error) {
	if err := box.RequireBox(request.Root, request.MarkerName); err != nil {
		return MoveResult{}, err
	}
	prefix := request.Prefix
	if prefix <= 0 {
		prefix = sidecar.DefaultPrefix
	}
	from, err := filepath.Rel(request.Root, request.Path)
	if err != nil || strings.HasPrefix(from, "..") {
		return MoveResult{}, fmt.Errorf("move: %s is not inside %s", request.Path, request.Root)
	}
	if strings.HasPrefix(filepath.ToSlash(from), TrashDirectory+"/") {
		return MoveResult{}, fmt.Errorf("move: %s is in the trash", from)
	}
	source := filepath.Dir(request.Path)
	sourceSidecar := sidecar.PathFor(source, request.Digest, prefix)
	fallback := IntakeDateOf(request.Record)
	if fallback.Zero() {
		fallback = box.DateOf(moveClock(request), nil)
	}
	directory := filepath.Join(request.Root, DirectoryFor(FilingDate(request.Record, fallback)))
	if filepath.Clean(directory) == filepath.Clean(source) {
		return MoveResult{Path: request.Path, SidecarPath: sourceSidecar}, nil
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return MoveResult{}, fmt.Errorf("create filing directory: %w", err)
	}

	stem, extension := splitName(filepath.Base(request.Path))
	stem = strings.TrimSuffix(stem, "-"+digest.Short(request.Digest, prefix))
	short, err := freePrefix(directory, request.Digest, stem, extension)
	if err != nil {
		return MoveResult{}, err
	}
	destination := filepath.Join(directory, stem+"-"+short+extension)
	destinationSidecar := sidecar.PathFor(directory, request.Digest, len(short))

	if err := os.Rename(request.Path, destination); err != nil {
		return MoveResult{}, fmt.Errorf("move scan: %w", err)
	}
	if err := os.Rename(sourceSidecar, destinationSidecar); err != nil {
		if back := os.Rename(destination, request.Path); back != nil {
			return MoveResult{}, fmt.Errorf("moved %s but not its sidecar (%v), and could not move it back: %w", destination, err, back)
		}
		return MoveResult{}, fmt.Errorf("move sidecar: %w", err)
	}
	for _, each := range []string{directory, source} {
		if err := verifiedcopy.SyncDirectory(each); err != nil {
			return MoveResult{}, err
		}
	}
	// A month left empty is taken away, and its year with it, so the tree
	// shows only where scans are. Remove refuses a directory that still holds
	// anything, which is the only case that matters.
	root := filepath.Clean(request.Root)
	for empty := source; filepath.Dir(empty) != empty && filepath.Clean(empty) != root; empty = filepath.Dir(empty) {
		if os.Remove(empty) != nil {
			break
		}
	}

	to, err := filepath.Rel(request.Root, destination)
	if err != nil {
		to = destination
	}
	entry := boxlog.Entry{
		Action: boxlog.ActionMove,
		Digest: request.Digest,
		Path:   filepath.ToSlash(to),
		From:   filepath.ToSlash(from),
		At:     moveClock(request).Format(time.RFC3339),
	}
	if err := boxlog.Append(boxlog.PathFor(request.Root), entry); err != nil {
		return MoveResult{}, fmt.Errorf("moved %s but could not append to the log: %w", destination, err)
	}
	return MoveResult{Path: destination, SidecarPath: destinationSidecar, Moved: true}, nil
}

func moveClock(request MoveRequest) time.Time {
	if request.Now.IsZero() {
		return time.Now()
	}
	return request.Now
}
