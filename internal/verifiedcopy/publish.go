package verifiedcopy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// Publish gives the verified temporary file its final name without ever
// replacing a file already there.
//
// A plain rename replaces silently, and a check before it leaves a window in
// which another file can appear and be destroyed. So publication asks the
// filesystem itself to refuse: an exclusive rename where the platform has one,
// otherwise a hard link, which fails when the name exists. A filesystem that
// supports neither (an SMB mount on macOS) is refused, unless the user opted
// that root in with AllowCheckRename: check-then-rename can overwrite a file
// another program writes to the same name in between.
func Publish(temporary, final string) error {
	return publish(temporary, final, renameExclusive, os.Link)
}

func publish(temporary, final string, rename, link func(string, string) error) error {
	err := rename(temporary, final)
	if err == nil || errors.Is(err, os.ErrExist) || !unsupported(err) {
		return err
	}
	if err := link(temporary, final); err == nil {
		// The final name now holds the verified file. A temporary name left
		// behind is removed as stale by the next copy of this name.
		_ = os.Remove(temporary)
		return nil
	} else if errors.Is(err, os.ErrExist) || !unsupported(err) {
		return err
	}
	if !checkRenameAllowed(final) {
		return fmt.Errorf("safe publication is unsupported: %w", errNoExclusiveRename)
	}
	if _, err := os.Lstat(final); err == nil {
		return fmt.Errorf("%w: %s", os.ErrExist, final)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(temporary, final)
}

// LockName is the file AllowCheckRename holds in a root while it is opted in.
const LockName = "_dgs-lock"

var checkRenameRoots = struct {
	sync.Mutex
	roots map[string]bool
}{roots: map[string]bool{}}

// AllowCheckRename lets Publish fall back to check-then-rename under root,
// where neither exclusive rename nor hard links exist. It takes LockName in
// root exclusively, so two dgs runs never publish into it at once and the
// remaining risk is another program writing the same name. Release undoes
// both. The lock names its host and process: one left by a process of this
// host that is no longer running is taken over; any other is reported.
func AllowCheckRename(root string) (release func(), err error) {
	if root, err = filepath.Abs(root); err != nil {
		return nil, err
	}
	lock := filepath.Join(root, LockName)
	host, _ := os.Hostname()
	owner := fmt.Sprintf("%s %d\n", host, os.Getpid())
	if err := takeLock(lock, owner); err != nil {
		return nil, err
	}
	// Publication may name the root as given or as resolved.
	roots := []string{root}
	if resolved, err := filepath.EvalSymlinks(root); err == nil && resolved != root {
		roots = append(roots, resolved)
	}
	checkRenameRoots.Lock()
	for _, one := range roots {
		checkRenameRoots.roots[one] = true
	}
	checkRenameRoots.Unlock()
	return func() {
		checkRenameRoots.Lock()
		for _, one := range roots {
			delete(checkRenameRoots.roots, one)
		}
		checkRenameRoots.Unlock()
		// Only our own lock is removed, never one taken over meanwhile.
		if held, _ := os.ReadFile(lock); string(held) == owner {
			_ = os.Remove(lock)
		}
	}, nil
}

// takeLock creates lock holding owner, taking over once a lock whose process
// of this host is gone.
func takeLock(lock, owner string) error {
	for attempt := 0; ; attempt++ {
		file, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, err = file.WriteString(owner)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			if err != nil {
				_ = os.Remove(lock)
			}
			return err
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		held, _ := os.ReadFile(lock)
		if attempt > 0 || !staleOwner(string(held)) {
			return fmt.Errorf("%s is held by another import (%s); remove it if none is running", lock, strings.TrimSpace(string(held)))
		}
		// Unchanged since read: removing it does not remove a newer owner's.
		if again, _ := os.ReadFile(lock); string(again) != string(held) {
			continue
		}
		_ = os.Remove(lock)
	}
}

// staleOwner says whether a lock names a process of this host that is gone.
// An unreadable owner, another host, or a live process is never stale.
func staleOwner(held string) bool {
	host, _ := os.Hostname()
	var lockHost string
	var pid int
	if _, err := fmt.Sscanf(held, "%s %d", &lockHost, &pid); err != nil || lockHost != host || pid <= 0 {
		return false
	}
	return !processAlive(pid)
}

func checkRenameAllowed(final string) bool {
	checkRenameRoots.Lock()
	defer checkRenameRoots.Unlock()
	for dir := filepath.Dir(filepath.Clean(final)); ; dir = filepath.Dir(dir) {
		if checkRenameRoots.roots[dir] {
			return true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return false
		}
	}
}

// unsupported reports an error that means "this filesystem cannot do that",
// as opposed to a failure of the operation itself.
//
// EPERM is among them because FAT and exFAT volumes, and some FUSE and SMB
// mounts, answer link(2) with it. A real permission failure costs nothing by
// trying the hard-link fallback; a failure there still refuses publication.
func unsupported(err error) bool {
	return errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EOPNOTSUPP) ||
		errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EXDEV) ||
		errors.Is(err, errNoExclusiveRename)
}

var errNoExclusiveRename = errors.New("exclusive rename is not available on this platform")
