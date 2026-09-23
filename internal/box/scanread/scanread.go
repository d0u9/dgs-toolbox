// Package scanread is the one read.
//
// Hashing already pulls every byte of a scan across the network, so both
// digests, the metadata and both thumbnail sizes come out of that same read.
// Fetching the file again for any of them would be the most expensive mistake
// available in this design: three round trips to a NAS per scan, times a
// thousand scans, to answer questions the first trip already had the bytes for.
//
// This package is only the composition. The digests are internal/box/digest,
// the metadata internal/box/scanmeta, the pictures internal/box/thumb; each is
// tested on its own and none of them knows about the others.
package scanread

import (
	"fmt"
	"io"
	"os"

	"dgs-toolbox/internal/box/digest"
	"dgs-toolbox/internal/box/scanmeta"
	"dgs-toolbox/internal/box/thumb"
)

// Result is everything one read of a scan produces.
type Result struct {
	// Size is the file's length in bytes.
	Size int64
	// Digest is the whole file. Every scan has one.
	Digest string
	// ImageDigest covers the embedded image streams alone and is empty for a
	// scan that has none. Empty is not a digest: it never matches anything,
	// including another empty one.
	ImageDigest string
	// Info is what the bytes say about themselves.
	Info scanmeta.Info
	// Thumbs are the two cache pictures, empty when there was nothing to draw.
	Thumbs thumb.Pair
	// NeedsRender records that no picture could be made. The scan is still
	// filed, described and deduplicated; only the thumbnail is missing, which
	// is what keeps the inbox drainable.
	NeedsRender bool
	// RenderError says why, for a report. It is empty when NeedsRender is false.
	RenderError string
	// Incomplete marks a file that is a PDF and will not parse, or that does
	// not end with %%EOF: a truncated file, a bad copy, a damaged old backup.
	//
	// It is a reported outcome rather than an error, because a skip that is not
	// reported means believing the inbox is empty when it is not. Such a file
	// is not taken in, but it still has a digest, so it can be recognised the
	// next time it turns up.
	Incomplete bool
	// IncompleteReason is what failed, for the report.
	IncompleteReason string
}

// Read reads a scan already held in memory and produces everything derivable
// from it.
//
// Only an unreadable file is an error. A PDF that will not parse is not a scan
// and the caller has to say so; everything past that point degrades into an
// empty field or into NeedsRender rather than stopping the import.
func Read(data []byte) (Result, error) {
	result := Result{
		Size:   int64(len(data)),
		Digest: digest.Whole(data),
	}
	// The two completeness checks are both cheap, and between them they catch a
	// truncated file, a bad copy and a damaged old backup. The page count is
	// wanted anyway, so parsing is not an extra cost.
	if scanmeta.LooksLikePDF(data) && !scanmeta.Complete(data) {
		result.Incomplete = true
		result.IncompleteReason = "the file does not end with %%EOF, so it is truncated or was copied badly"
		result.NeedsRender = true
		result.RenderError = result.IncompleteReason
		return result, nil
	}
	info, err := scanmeta.Read(data)
	if err != nil {
		if scanmeta.LooksLikePDF(data) {
			// A PDF that will not parse far enough to yield a page count is
			// incomplete, not a failure of the import.
			result.Incomplete = true
			result.IncompleteReason = err.Error()
			result.NeedsRender = true
			result.RenderError = err.Error()
			return result, nil
		}
		return Result{}, err
	}
	result.Info = info
	if imageDigest, found := digest.Images(info.Images); found {
		result.ImageDigest = imageDigest
	}
	pictures, err := thumb.Render(info)
	if err != nil {
		result.NeedsRender = true
		result.RenderError = err.Error()
		if info.ImageError != "" {
			result.RenderError = info.ImageError
		}
		return result, nil
	}
	result.Thumbs = pictures
	return result, nil
}

// ReadFile reads a scan from disk. The file is read once, whole, because every
// answer below needs the same bytes and a scan is a few megabytes.
func ReadFile(path string) (Result, error) {
	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", path, err)
	}
	result, err := Read(data)
	if err != nil {
		return Result{}, fmt.Errorf("%s: %w", path, err)
	}
	return result, nil
}
