package publish

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/boxlog"
	"dgs-toolbox/internal/box/sidecar"
	"dgs-toolbox/internal/verifiedcopy"
)

// Discard moves a published scan and its sidecar into the Box's trash.
//
// Nothing in a Box is deleted. The trash is inside the Box root, so this is a
// same-volume rename: atomic, instant, no copy and no readback. Emptying it is
// a manual act done in Finder or a shell, and box never removes a file — with
// no snapshots behind it the trash is the only undo in the system.
//
// The sidecar travels with the file and gains where it came from and why, so
// the scan keeps taking part in deduplication. Being shown a rejected file
// again on the next run, and having to make the same judgement a second time,
// is exactly the tedium this tool exists to remove.
//
// Rejecting during intake and discarding after filing are the same mechanism.
// Only the reason differs, because two directories would be two things to
// empty.
func Discard(request DiscardRequest) (DiscardResult, error) {
	if err := box.RequireBox(request.Root, request.MarkerName); err != nil {
		return DiscardResult{}, err
	}
	if strings.TrimSpace(request.Path) == "" {
		return DiscardResult{}, errors.New("discard: no file")
	}
	if request.DiscardDate.Zero() {
		return DiscardResult{}, errors.New("discard: no discard date")
	}
	from, err := filepath.Rel(request.Root, request.Path)
	if err != nil || strings.HasPrefix(from, "..") {
		return DiscardResult{}, fmt.Errorf("discard: %s is not inside %s", request.Path, request.Root)
	}
	// Bucketing by the discard date matches how emptying works — "clear out
	// what I threw away before June". Bucketing by the original date would mean
	// walking the whole tree to do it.
	directory := filepath.Join(request.Root, TrashDirectory, request.DiscardDate.String())
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return DiscardResult{}, err
	}
	destination := filepath.Join(directory, filepath.Base(request.Path))
	if _, err := os.Lstat(destination); err == nil {
		return DiscardResult{}, fmt.Errorf("%s already in the trash", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return DiscardResult{}, err
	}
	if err := os.Rename(request.Path, destination); err != nil {
		return DiscardResult{}, fmt.Errorf("move to trash: %w", err)
	}

	// The sidecar follows, and only then records the move. Writing it first
	// would leave a sidecar claiming a file is in the trash when the rename
	// that puts it there has not happened yet.
	sourceSidecar := sidecar.PathFor(filepath.Dir(request.Path), request.Digest, request.ShortLength())
	record, err := sidecar.Load(sourceSidecar)
	if err != nil && !errors.Is(err, sidecar.ErrNotFound) {
		return DiscardResult{}, err
	}
	record.TrashedAt = sidecar.Timestamp(discardClock(request).Format(time.RFC3339))
	record.TrashedFrom = filepath.ToSlash(from)
	record.Reason = request.Reason
	trashSidecar := sidecar.PathFor(directory, request.Digest, request.ShortLength())
	if err := sidecar.Save(trashSidecar, record); err != nil {
		return DiscardResult{}, fmt.Errorf("write trash sidecar: %w", err)
	}
	if err := os.Remove(sourceSidecar); err != nil && !errors.Is(err, os.ErrNotExist) {
		return DiscardResult{}, fmt.Errorf("remove old sidecar: %w", err)
	}
	if err := verifiedcopy.SyncDirectory(directory); err != nil {
		return DiscardResult{}, err
	}

	entry := boxlog.Entry{
		Action: boxlog.ActionDiscard,
		Digest: request.Digest,
		Path:   filepath.ToSlash(from),
		Note:   request.Reason,
		At:     discardClock(request).Format(time.RFC3339),
	}
	if err := boxlog.Append(boxlog.PathFor(request.Root), entry); err != nil {
		return DiscardResult{}, fmt.Errorf("discarded %s but could not append to the log: %w", request.Path, err)
	}
	return DiscardResult{Path: destination, SidecarPath: trashSidecar}, nil
}

// DiscardRequest is one scan to throw away.
type DiscardRequest struct {
	Root       string
	MarkerName string
	// Path is the published file, inside the Box.
	Path string
	// Digest names the sidecar beside it.
	Digest string
	// Prefix is how many hex digits name that sidecar. Zero uses the default.
	Prefix int
	// DiscardDate is the day this is being thrown away, which is the only thing
	// the trash path encodes.
	DiscardDate box.Date
	// Reason is why, kept in the sidecar. Rejecting during intake and
	// discarding after filing differ in nothing else.
	Reason string
	Now    time.Time
}

// ShortLength is how many digest digits name this scan's sidecar.
func (r DiscardRequest) ShortLength() int {
	if r.Prefix <= 0 {
		return sidecar.DefaultPrefix
	}
	return r.Prefix
}

// DiscardResult is where the scan and its sidecar ended up.
type DiscardResult struct {
	Path        string
	SidecarPath string
}

func discardClock(request DiscardRequest) time.Time {
	if request.Now.IsZero() {
		return time.Now()
	}
	return request.Now
}
