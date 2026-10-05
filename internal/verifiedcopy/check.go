package verifiedcopy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SameVersion compares the observable identity, size and modification time of
// two regular files. It describes filesystem metadata, not protection against
// external writers that deliberately preserve that metadata.
func SameVersion(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Mode().IsRegular() && b.Mode().IsRegular() &&
		os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// FileInfo refuses symbolic links anywhere below root. The selected root may
// itself be a link. An empty root checks the file itself without following it.
func FileInfo(root, path string) (os.FileInfo, error) {
	if root != "" {
		root, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, errors.New("file is outside selected root")
		}
		current := root
		parts := strings.Split(rel, string(filepath.Separator))
		for _, part := range parts[:len(parts)-1] {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return nil, err
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("unsafe file path component")
			}
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("file is not a regular file")
	}
	return info, nil
}

// Check hashes an independently opened regular file, validating both the open
// handle and its path before and after reading. Wait and progress can be nil.
// Like Copy, it does not treat a read-only close error after a complete read as
// a failed digest (some network filesystems report such errors).
func Check(ctx context.Context, root, path string, wait func(context.Context) error, progress func(int64)) (Result, error) {
	before, err := FileInfo(root, path)
	if err != nil {
		return Result{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{}, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !SameVersion(before, opened) {
		return Result{}, errors.New("file changed while opening")
	}
	// Evidence for skipping or deleting is read from the filesystem, not from
	// pages left in memory by an earlier read; best effort, as in Copy.
	_ = bypassCache(file)
	_ = dropCache(file)
	digest := sha256.New()
	buffer := make([]byte, DefaultBufferSize)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if wait != nil {
			if err := wait(ctx); err != nil {
				return Result{}, err
			}
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			_, _ = digest.Write(buffer[:n])
			total += int64(n)
			if total > before.Size() {
				return Result{}, errors.New("file grew while reading")
			}
			if progress != nil {
				progress(total)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return Result{}, readErr
		}
	}
	after, err := file.Stat()
	if err != nil || total != before.Size() || !SameVersion(before, after) {
		return Result{}, errors.New("file changed while reading")
	}
	current, err := FileInfo(root, path)
	if err != nil || !SameVersion(before, current) {
		return Result{}, fmt.Errorf("file path changed while reading: %s", path)
	}
	return Result{Source: before, Size: total, Digest: hex.EncodeToString(digest.Sum(nil))}, nil
}
