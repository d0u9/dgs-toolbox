package publish

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"dgs-toolbox/internal/verifiedcopy"
)

// Stream creates path without replacement using bounded-memory readback.
// write must finish successfully before verify is called on the synced part.
// verify may perform additional checks before publication; nil skips it.
// Only the temporary inode created by this operation is cleaned up.
func Stream(ctx context.Context, path string, perm, dirPerm fs.FileMode, write func(io.Writer) error, verify func(string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("%s %w", path, ErrExists)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return err
	}
	part := path + ".dgs-part"
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}
	defer file.Close()
	written, err := file.Stat()
	if err != nil {
		return err
	}
	defer func() {
		if current, err := os.Lstat(part); err == nil && os.SameFile(current, written) {
			_ = os.Remove(part)
		}
	}()
	hash := sha256.New()
	err = write(&contextWriter{ctx: ctx, dst: io.MultiWriter(file, hash)})
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = file.Sync()
	}
	if err == nil {
		written, err = file.Stat()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	back, err := os.Open(part)
	if err != nil {
		return err
	}
	opened, statErr := back.Stat()
	if statErr != nil || !verifiedcopy.SameVersion(opened, written) {
		back.Close()
		if statErr != nil {
			return statErr
		}
		return fmt.Errorf("temporary file was replaced before readback")
	}
	digest := sha256.New()
	_, err = io.Copy(&contextWriter{ctx: ctx, dst: digest}, back)
	if closeErr := back.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(hash.Sum(nil), digest.Sum(nil)) {
		return fmt.Errorf("%s read back differently from what was written", path)
	}
	if verify != nil {
		if err := verify(part); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	current, err := os.Lstat(part)
	if err != nil {
		return err
	}
	if !verifiedcopy.SameVersion(current, written) {
		return fmt.Errorf("temporary file was replaced or changed before publication")
	}
	if err := verifiedcopy.Publish(part, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%s %w", path, ErrExists)
		}
		return err
	}
	if handle, err := os.Open(dir); err == nil {
		handle.Sync()
		handle.Close()
	}
	return nil
}

type contextWriter struct {
	ctx context.Context
	dst io.Writer
}

func (w *contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.dst.Write(p)
}
