// Package gitrepo reports what a git working tree holds and publishes its
// changes, by running the git on PATH. It is the user's own git on purpose:
// their identity, their signing key, their credential helper and their hooks
// are what a commit from this machine is expected to carry, and reimplementing
// the parts of git that read that configuration would publish something
// subtly different from what the same command in a terminal does.
//
// It holds no TUI and no app state: a directory in, plain values out.
package gitrepo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// DefaultBinary is the git looked up on PATH, and DefaultTimeout how long one
// git command is given. Push talks to a server, so the timeout has to be a
// network one rather than a local-command one.
const (
	DefaultBinary  = "git"
	DefaultTimeout = 2 * time.Minute
)

// ErrNotInstalled is returned when no git is on PATH, and ErrNotARepository
// when the directory is not inside a working tree.
var (
	ErrNotInstalled   = errors.New("git is not installed")
	ErrNotARepository = errors.New("not inside a git repository")
)

// Change is one path git reports as not committed, with its two-letter
// porcelain code: the index column, then the working-tree column.
type Change struct {
	Code string
	Path string
}

// Staged reports whether the index column says this path is already staged.
func (c Change) Staged() bool {
	return len(c.Code) == 2 && c.Code[0] != ' ' && c.Code[0] != '?'
}

// Untracked reports whether git has never seen this path.
func (c Change) Untracked() bool { return c.Code == "??" }

// Snapshot is a working tree as one reading: where it is, what it is on, what
// it is behind, and what is not committed.
type Snapshot struct {
	// Root is the top of the working tree, which is not always the directory
	// asked about.
	Root string
	// Branch is the branch HEAD is on, empty when Detached.
	Branch   string
	Detached bool
	// Unborn is a repository whose first commit is not made yet: it has a
	// branch but no HEAD to compare against.
	Unborn bool
	// Remote is the remote a push goes to: the upstream's, or the only remote
	// when the branch has no upstream. Empty when the repository has none, or
	// has several and no upstream to choose between them.
	Remote string
	// Upstream is the tracking branch, such as "origin/main", empty when the
	// branch has none.
	Upstream string
	// Ahead and Behind count commits, and are meaningful only with an
	// Upstream.
	Ahead, Behind int
	// Changes is every path git reports, in git's order.
	Changes []Change
}

// Clean reports whether nothing is waiting to be committed.
func (s Snapshot) Clean() bool { return len(s.Changes) == 0 }

// Runner runs git. The zero value uses DefaultBinary and DefaultTimeout, so
// gitrepo.Runner{}.Status(dir) is the ordinary call; the fields are here so a
// test can point at another binary and a caller can shorten the wait.
type Runner struct {
	Binary  string
	Timeout time.Duration
}

func (r Runner) binary() string {
	if r.Binary != "" {
		return r.Binary
	}
	return DefaultBinary
}

func (r Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	return DefaultTimeout
}

// run executes one git command in dir and returns its standard output. A
// failing command carries git's own standard error, which says more about why
// than an exit code does.
func (r Runner) run(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, r.binary(), args...)
	cmd.Dir = dir
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	err := cmd.Run()
	if err != nil {
		// No git to run at all: PATH has none (exec.ErrNotFound), or the
		// binary named is not there (a PathError from the exec itself).
		var execErr *exec.Error
		var pathErr *fs.PathError
		if errors.As(err, &execErr) || errors.As(err, &pathErr) {
			return "", ErrNotInstalled
		}
		if ctx.Err() != nil {
			return "", fmt.Errorf("git %s timed out after %s", args[0], r.timeout())
		}
		message := strings.TrimSpace(errs.String())
		if message == "" {
			message = strings.TrimSpace(out.String())
		}
		if message == "" {
			return "", fmt.Errorf("git %s: %w", args[0], err)
		}
		return "", fmt.Errorf("git %s: %s", args[0], firstLines(message, 3))
	}
	return out.String(), nil
}

// firstLines keeps a command's message short enough for one dialog.
func firstLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return strings.Join(lines, "; ")
	}
	return strings.Join(lines[:n], "; ") + "; …"
}

// Status reads the working tree holding dir. A directory that is not in one is
// ErrNotARepository, and no git on PATH is ErrNotInstalled; both are told
// apart from a real failure so a caller can say which.
func (r Runner) Status(dir string) (Snapshot, error) {
	root, err := r.run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		if errors.Is(err, ErrNotInstalled) {
			return Snapshot{}, err
		}
		return Snapshot{}, ErrNotARepository
	}
	snap := Snapshot{Root: strings.TrimSpace(root)}

	branch, err := r.run(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		// A repository with no commit yet has a branch but no HEAD to resolve.
		snap.Unborn = true
		if name, nameErr := r.run(dir, "symbolic-ref", "--short", "HEAD"); nameErr == nil {
			snap.Branch = strings.TrimSpace(name)
		}
	} else {
		snap.Branch = strings.TrimSpace(branch)
		if snap.Branch == "HEAD" {
			snap.Branch, snap.Detached = "", true
		}
	}

	if upstream, err := r.run(dir, "rev-parse", "--abbrev-ref", "@{upstream}"); err == nil {
		snap.Upstream = strings.TrimSpace(upstream)
		snap.Remote, _, _ = strings.Cut(snap.Upstream, "/")
		if counts, err := r.run(dir, "rev-list", "--left-right", "--count", "@{upstream}...HEAD"); err == nil {
			behind, ahead, _ := strings.Cut(strings.TrimSpace(counts), "\t")
			snap.Behind, _ = strconv.Atoi(strings.TrimSpace(behind))
			snap.Ahead, _ = strconv.Atoi(strings.TrimSpace(ahead))
		}
	} else if remotes, err := r.run(dir, "remote"); err == nil {
		names := strings.Fields(remotes)
		if len(names) == 1 {
			snap.Remote = names[0]
		}
	}

	porcelain, err := r.run(dir, "status", "--porcelain=v1", "-z")
	if err != nil {
		return snap, err
	}
	snap.Changes = parsePorcelain(porcelain)
	return snap, nil
}

// parsePorcelain reads `status --porcelain=v1 -z`. The NUL form is the one
// that survives a name with a space or a quote in it, and a rename spends a
// second record on the name it came from, which is not a change of its own.
func parsePorcelain(out string) []Change {
	var changes []Change
	records := strings.Split(out, "\x00")
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) < 4 {
			continue
		}
		code, path := record[:2], record[3:]
		if code[0] == 'R' || code[0] == 'C' {
			i++ // the record after a rename or copy is where it came from.
		}
		changes = append(changes, Change{Code: code, Path: path})
	}
	return changes
}

// Incoming is one commit the remote has and this branch does not.
type Incoming struct {
	// Hash is the short hash, and Subject the first line of the message.
	Hash    string
	Subject string
}

// FetchResult is what the remote has, read after fetching: the tree as it
// stands and what is waiting in it.
type FetchResult struct {
	Snapshot Snapshot
	// Commits are the ones the upstream has and this branch does not, newest
	// first, at most Limit of them.
	Commits []Incoming
	// Files are the paths those commits change, at most Limit of them.
	Files []Change
	// Note says why there was nothing to read: no remote, no upstream.
	Note string
}

// FetchLimit is how many incoming commits and changed paths Fetch reads. It is
// what a dialog can show; the counts in Snapshot are the whole truth.
const FetchLimit = 20

// Fetch asks the remote what it has, and reads the branch against it. Nothing
// in the working tree is touched: this is the one git command here that is
// safe whatever state the tree is in, which is why it runs before anything is
// decided.
func (r Runner) Fetch(dir string) (FetchResult, error) {
	snap, err := r.Status(dir)
	if err != nil {
		return FetchResult{}, err
	}
	if snap.Remote == "" {
		return FetchResult{Snapshot: snap, Note: "no remote to fetch from"}, nil
	}
	if _, err := r.run(snap.Root, "fetch", snap.Remote); err != nil {
		return FetchResult{Snapshot: snap}, err
	}
	// The counts are read again: fetching is what makes them true.
	snap, err = r.Status(dir)
	if err != nil {
		return FetchResult{}, err
	}
	result := FetchResult{Snapshot: snap}
	if snap.Upstream == "" {
		result.Note = "this branch has no upstream, so there is nothing to compare against"
		return result, nil
	}
	if snap.Behind == 0 {
		return result, nil
	}
	limit := strconv.Itoa(FetchLimit)
	if out, err := r.run(snap.Root, "log", "--max-count="+limit, "--format=%h%x00%s", "HEAD..@{upstream}"); err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			hash, subject, found := strings.Cut(line, "\x00")
			if found {
				result.Commits = append(result.Commits, Incoming{Hash: hash, Subject: subject})
			}
		}
	}
	if out, err := r.run(snap.Root, "diff", "--name-status", "-z", "HEAD..@{upstream}"); err == nil {
		result.Files = parseNameStatus(out, FetchLimit)
	}
	return result, nil
}

// parseNameStatus reads `diff --name-status -z`, where a status and its path
// are separate NUL-terminated records and a rename carries two paths.
func parseNameStatus(out string, limit int) []Change {
	records := strings.FieldsFunc(out, func(r rune) bool { return r == '\x00' })
	var changes []Change
	for i := 0; i+1 < len(records); i += 2 {
		code, path := records[i], records[i+1]
		if code != "" && (code[0] == 'R' || code[0] == 'C') {
			// A rename's record is "R100", the old path, then the new one.
			if i+2 < len(records) {
				i++
				path = records[i+1]
			}
		}
		changes = append(changes, Change{Code: code, Path: path})
		if len(changes) == limit {
			break
		}
	}
	return changes
}

// ErrDiverged is returned by FastForward when both sides have commits the
// other does not: merging or rebasing them is a decision this package does not
// make.
var ErrDiverged = errors.New("the branch and its upstream have both moved")

// FastForward moves the branch to its upstream, and only when that is a
// fast-forward: no merge commit, no rebase, no stash. A vault holds encrypted
// files, which no one can resolve a conflict in, so the one update that cannot
// leave the working tree half-merged is the only one offered.
//
// It does not fetch. Call Fetch first; what this decides on is what that read.
func (r Runner) FastForward(dir string) (Snapshot, error) {
	snap, err := r.Status(dir)
	if err != nil {
		return Snapshot{}, err
	}
	switch {
	case snap.Upstream == "":
		return snap, errors.New("this branch has no upstream")
	case snap.Behind == 0:
		return snap, nil
	case snap.Ahead > 0:
		return snap, ErrDiverged
	}
	if _, err := r.run(snap.Root, "merge", "--ff-only", "@{upstream}"); err != nil {
		return snap, err
	}
	return r.Status(dir)
}

// PublishRequest is one "put what is here on the server": stage everything
// under the working tree, commit it, and push the branch.
type PublishRequest struct {
	// Dir is any directory inside the working tree.
	Dir string
	// Message is the commit message. Required when there is something to
	// commit.
	Message string
}

// Result is what Publish did, step by step, so the caller reports what
// happened rather than that it finished.
type Result struct {
	// Staged is how many paths the commit holds.
	Staged    int
	Committed bool
	// Commit is the short hash of the new commit.
	Commit string
	Pushed bool
	// Note says why a step was skipped: nothing to commit, no remote.
	Note string
}

// Publish stages every change in the working tree, commits them and pushes the
// branch. Steps that have nothing to do are skipped rather than failed: a tree
// with no change but a commit the remote lacks is pushed, and a tree with
// neither does nothing and says so.
//
// A repository with no remote is committed and not pushed, with a Note saying
// so; a detached HEAD is refused before anything is staged, since there is no
// branch to push and a commit there is easy to lose.
func (r Runner) Publish(req PublishRequest) (Result, error) {
	snap, err := r.Status(req.Dir)
	if err != nil {
		return Result{}, err
	}
	if snap.Detached {
		return Result{}, errors.New("HEAD is detached: check out a branch first")
	}
	var result Result
	if !snap.Clean() {
		if strings.TrimSpace(req.Message) == "" {
			return result, errors.New("a commit message is required")
		}
		if _, err := r.run(snap.Root, "add", "-A", "--", "."); err != nil {
			return result, err
		}
		// Staging can find nothing after all — every change may be ignored by
		// a gitignore rule that `status` still listed as untracked directory
		// content — so what is committed is counted from the index.
		staged, err := r.run(snap.Root, "diff", "--cached", "--name-only", "-z")
		if err != nil {
			return result, err
		}
		names := strings.FieldsFunc(staged, func(r rune) bool { return r == '\x00' })
		result.Staged = len(names)
		if result.Staged > 0 {
			if _, err := r.run(snap.Root, "commit", "-m", req.Message); err != nil {
				return result, err
			}
			result.Committed = true
			if hash, err := r.run(snap.Root, "rev-parse", "--short", "HEAD"); err == nil {
				result.Commit = strings.TrimSpace(hash)
			}
		}
	}
	if !result.Committed && snap.Ahead == 0 && snap.Upstream != "" {
		result.Note = "nothing to commit or push"
		return result, nil
	}
	switch {
	case snap.Remote == "":
		result.Note = "no remote to push to"
		return result, nil
	case snap.Branch == "":
		result.Note = "no branch to push"
		return result, nil
	}
	args := []string{"push", snap.Remote, snap.Branch}
	if snap.Upstream == "" {
		args = []string{"push", "--set-upstream", snap.Remote, snap.Branch}
	}
	if _, err := r.run(snap.Root, args...); err != nil {
		return result, err
	}
	result.Pushed = true
	return result, nil
}
