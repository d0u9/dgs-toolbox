package desktop

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// Trash moves path into the user's trash rather than deleting it, and returns
// where it went: ~/.Trash on macOS, and the freedesktop.org trash
// ($XDG_DATA_HOME/Trash) elsewhere, with the info file that lets a file manager
// restore it. It moves within one filesystem only; a path on another volume is
// refused rather than copied.
func Trash(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return trash(runtime.GOOS, home, os.Getenv("XDG_DATA_HOME"), path, time.Now())
}

func trash(goos, home, dataHome, path string, now time.Time) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err != nil {
		return "", err
	}
	var files, info string
	if goos == "darwin" {
		files = filepath.Join(home, ".Trash")
	} else {
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		files = filepath.Join(dataHome, "Trash", "files")
		info = filepath.Join(dataHome, "Trash", "info")
	}
	for _, dir := range []string{files, info} {
		if dir != "" {
			if err := os.MkdirAll(dir, 0o700); err != nil {
				return "", err
			}
		}
	}
	name := filepath.Base(path)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for attempt := 0; ; attempt++ {
		candidate := name
		if attempt > 0 {
			candidate = fmt.Sprintf("%s %s-%d%s", stem, now.Format("20060102-150405"), attempt, ext)
		}
		destination := filepath.Join(files, candidate)
		if info != "" {
			// Claiming the info file first is what reserves the name.
			record := filepath.Join(info, candidate+".trashinfo")
			handle, err := os.OpenFile(record, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if errors.Is(err, os.ErrExist) {
				continue
			}
			if err != nil {
				return "", err
			}
			fmt.Fprintf(handle, "[Trash Info]\nPath=%s\nDeletionDate=%s\n", (&url.URL{Path: path}).EscapedPath(), now.Format("2006-01-02T15:04:05"))
			handle.Close()
			if err := move(path, destination); err != nil {
				os.Remove(record)
				return "", err
			}
			return destination, nil
		}
		if _, err := os.Lstat(destination); err == nil {
			continue
		}
		if err := move(path, destination); err != nil {
			return "", err
		}
		return destination, nil
	}
}

func move(from, to string) error {
	err := os.Rename(from, to)
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && errors.Is(linkErr.Err, syscall.EXDEV) {
		return fmt.Errorf("%s is on another volume than the trash; it was not moved", from)
	}
	return err
}
