package seal

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/publish"
	"filippo.io/age"
)

// Encrypt streams a standalone file or folder without a size limit or record.
// With local identities, publication requires decrypted readback to match the
// source stream's SHA-256. Otherwise ciphertext readback is checked alone.
func Encrypt(ctx context.Context, source, destination string, keys []Recipient, ids []age.Identity, skip []string) (Result, error) {
	recipients, _, err := encryptTo(keys, "")
	if err != nil {
		return Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	info, err := SourceInfo(source, true)
	if err != nil {
		return Result{}, err
	}

	result := Result{Path: destination, Archived: info.IsDir()}
	if info.IsDir() {
		// The part must never be walked into its own archive, even through a symlink.
		folder, err := filepath.EvalSymlinks(source)
		if err != nil {
			return Result{}, err
		}
		folder, err = filepath.Abs(folder)
		if err != nil {
			return Result{}, err
		}
		source = folder
		parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
		if err != nil {
			return Result{}, err
		}
		parent, err = filepath.Abs(parent)
		if err != nil {
			return Result{}, err
		}
		rel, err := filepath.Rel(folder, parent)
		if err != nil {
			return Result{}, err
		}
		if rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return Result{}, fmt.Errorf("output must be outside the source directory")
		}
	}
	err = publish.Stream(ctx, destination, 0600, 0700, func(dst io.Writer) error {
		encrypted, err := age.Encrypt(dst, recipients...)
		if err != nil {
			return err
		}
		digest := sha256.New()
		plaintext := &streamCounter{Writer: &streamContextWriter{ctx: ctx, dst: io.MultiWriter(encrypted, digest)}}
		if info.IsDir() {
			result.Skipped, err = writeArchive(ctx, plaintext, source, 0, skip)
		} else {
			var file *os.File
			file, err = os.Open(source)
			if err == nil {
				_, err = io.Copy(plaintext, file)
				if closeErr := file.Close(); err == nil {
					err = closeErr
				}
				if err == nil && plaintext.n != info.Size() {
					err = fmt.Errorf("%s changed size while encrypting", source)
				}
			}
		}
		if err != nil {
			return err
		}
		if err := encrypted.Close(); err != nil {
			return err
		}
		result.PlaintextSize = plaintext.n
		copy(result.SHA256[:], digest.Sum(nil))
		return nil
	}, func(part string) error {
		if len(ids) == 0 {
			return nil
		}
		if err := verifyContext(ctx, part, result.SHA256, ids); err != nil {
			return err
		}
		result.Verified = true
		return nil
	})
	return result, err
}

// Decrypt streams binary or armored age plaintext to a 0600 part, authenticates
// the entire input, then checks readback and publishes without replacement.
// No archive expansion or application size limit is applied.
func Decrypt(ctx context.Context, source, destination string, ids []age.Identity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := SourceInfo(source, false); err != nil {
		return err
	}
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	decrypted, err := age.Decrypt(armoredOrNot(file), ids...)
	if err != nil {
		return err
	}
	return publish.Stream(ctx, destination, 0600, 0700, func(dst io.Writer) error {
		_, err := io.Copy(dst, decrypted)
		return err
	}, nil)
}

type streamCounter struct {
	io.Writer
	n int64
}

func (w *streamCounter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	return n, err
}

// streamContextWriter checks cancellation between bounded stream writes.
type streamContextWriter struct {
	ctx context.Context
	dst io.Writer
}

func (w *streamContextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.dst.Write(p)
}
