package gitrepo

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRunner is the runner these tests use, or a skip when the machine has no
// git: this package is a wrapper around the real one, and there is nothing to
// check without it.
func gitRunner(t *testing.T) Runner {
	t.Helper()
	if _, err := exec.LookPath(DefaultBinary); err != nil {
		t.Skip("git is not installed")
	}
	return Runner{}
}

// initRepo makes a working tree with one commit, an identity of its own and no
// signing, so the test does not depend on what the machine's git config says.
func initRepo(t *testing.T, r Runner) string {
	t.Helper()
	dir := t.TempDir()
	run(t, r, dir, "init", "--initial-branch=main")
	run(t, r, dir, "config", "user.name", "Test")
	run(t, r, dir, "config", "user.email", "test@example.com")
	run(t, r, dir, "config", "commit.gpgsign", "false")
	write(t, filepath.Join(dir, "README.md"), "vault\n")
	run(t, r, dir, "add", "-A")
	run(t, r, dir, "commit", "-m", "first")
	return dir
}

func run(t *testing.T, r Runner, dir string, args ...string) string {
	t.Helper()
	out, err := r.run(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatus_ReportsBranchAndChanges(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	write(t, filepath.Join(dir, "secrets", "key.age"), "cipher\n")
	write(t, filepath.Join(dir, "README.md"), "vault, changed\n")

	snap, err := r.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branch != "main" || snap.Detached {
		t.Fatalf("branch = %q detached = %v, want main", snap.Branch, snap.Detached)
	}
	if snap.Clean() {
		t.Fatal("Clean() = true, want the two changes")
	}
	var paths []string
	for _, c := range snap.Changes {
		paths = append(paths, c.Path)
	}
	if len(paths) != 2 {
		t.Fatalf("changes = %v, want the changed file and the new folder", paths)
	}
	if snap.Upstream != "" || snap.Remote != "" {
		t.Fatalf("upstream = %q remote = %q, want none", snap.Upstream, snap.Remote)
	}
}

// TestStatus_SubdirectoryReportsTheWorkingTreeRoot covers the vault folder
// being somewhere under the repository rather than at its top.
func TestStatus_SubdirectoryReportsTheWorkingTreeRoot(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	sub := filepath.Join(dir, "vault", "personal")
	write(t, filepath.Join(sub, "k.age"), "cipher\n")

	snap, err := r.Status(sub)
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(snap.Root)
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if root != want {
		t.Fatalf("Root = %q, want %q", root, want)
	}
}

func TestStatus_NotARepository(t *testing.T) {
	r := gitRunner(t)
	if _, err := r.Status(t.TempDir()); err != ErrNotARepository {
		t.Fatalf("Status err = %v, want ErrNotARepository", err)
	}
}

func TestStatus_NoGitBinary(t *testing.T) {
	r := Runner{Binary: filepath.Join(t.TempDir(), "no-such-git")}
	if _, err := r.Status(t.TempDir()); err != ErrNotInstalled {
		t.Fatalf("Status err = %v, want ErrNotInstalled", err)
	}
}

func TestPublish_CommitsAndPushesToItsRemote(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	remote := t.TempDir()
	run(t, r, remote, "init", "--bare", "--initial-branch=main")
	run(t, r, dir, "remote", "add", "origin", remote)
	write(t, filepath.Join(dir, "vault", "k.age"), "cipher\n")

	result, err := r.Publish(PublishRequest{Dir: dir, Message: "vault: add k"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || !result.Pushed {
		t.Fatalf("result = %+v, want a commit and a push", result)
	}
	if result.Staged != 1 || result.Commit == "" {
		t.Fatalf("result = %+v, want one staged file and the new commit", result)
	}
	// The branch is on the remote, and tracking is set for the next push.
	if out := run(t, r, remote, "log", "-1", "--format=%s", "main"); strings.TrimSpace(out) != "vault: add k" {
		t.Fatalf("remote log = %q, want the new commit", out)
	}
	if snap, err := r.Status(dir); err != nil || snap.Upstream != "origin/main" {
		t.Fatalf("upstream = %+v %v, want origin/main", snap.Upstream, err)
	}
}

func TestPublish_NothingToDoSaysSo(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	remote := t.TempDir()
	run(t, r, remote, "init", "--bare", "--initial-branch=main")
	run(t, r, dir, "remote", "add", "origin", remote)
	run(t, r, dir, "push", "--set-upstream", "origin", "main")

	result, err := r.Publish(PublishRequest{Dir: dir, Message: "vault: nothing"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Committed || result.Pushed || result.Note == "" {
		t.Fatalf("result = %+v, want nothing done and a reason", result)
	}
}

// TestPublish_PushesACommitTheRemoteLacks covers a tree with no change of its
// own: the point of the menu is that the vault folder ends up on the server,
// which an earlier commit that was never pushed also decides.
func TestPublish_PushesACommitTheRemoteLacks(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	remote := t.TempDir()
	run(t, r, remote, "init", "--bare", "--initial-branch=main")
	run(t, r, dir, "remote", "add", "origin", remote)
	run(t, r, dir, "push", "--set-upstream", "origin", "main")
	write(t, filepath.Join(dir, "k.age"), "cipher\n")
	run(t, r, dir, "add", "-A")
	run(t, r, dir, "commit", "-m", "second")

	result, err := r.Publish(PublishRequest{Dir: dir, Message: "unused"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Committed || !result.Pushed {
		t.Fatalf("result = %+v, want only a push", result)
	}
}

func TestPublish_NoRemoteCommitsAndSaysWhyItStopped(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	write(t, filepath.Join(dir, "k.age"), "cipher\n")

	result, err := r.Publish(PublishRequest{Dir: dir, Message: "vault: add k"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Committed || result.Pushed {
		t.Fatalf("result = %+v, want a commit and no push", result)
	}
	if !strings.Contains(result.Note, "no remote") {
		t.Fatalf("note = %q, want it to say there is no remote", result.Note)
	}
}

func TestPublish_EmptyMessageIsRefused(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	write(t, filepath.Join(dir, "k.age"), "cipher\n")

	if _, err := r.Publish(PublishRequest{Dir: dir, Message: "  "}); err == nil {
		t.Fatal("Publish with no message succeeded, want it refused")
	}
	snap, err := r.Status(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Clean() {
		t.Fatal("the change was committed anyway")
	}
}

func TestPublish_DetachedHeadIsRefused(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)
	run(t, r, dir, "checkout", "--detach")
	write(t, filepath.Join(dir, "k.age"), "cipher\n")

	if _, err := r.Publish(PublishRequest{Dir: dir, Message: "vault: add k"}); err == nil {
		t.Fatal("Publish on a detached HEAD succeeded, want it refused")
	}
}

func TestParsePorcelain_RenameKeepsOneChange(t *testing.T) {
	changes := parsePorcelain("R  new name.age\x00old name.age\x00?? other.age\x00")
	if len(changes) != 2 {
		t.Fatalf("changes = %+v, want the rename and the untracked file", changes)
	}
	if changes[0].Path != "new name.age" || !changes[0].Staged() {
		t.Fatalf("first = %+v, want the renamed path, staged", changes[0])
	}
	if !changes[1].Untracked() {
		t.Fatalf("second = %+v, want it untracked", changes[1])
	}
}

// clone makes a second working tree of the same remote, which is how a test
// says "another machine committed".
func clone(t *testing.T, r Runner, remote string) string {
	t.Helper()
	dir := t.TempDir()
	run(t, r, dir, "clone", remote, ".")
	run(t, r, dir, "config", "user.name", "Other")
	run(t, r, dir, "config", "user.email", "other@example.com")
	run(t, r, dir, "config", "commit.gpgsign", "false")
	return dir
}

// published is a repository with a remote and its first commit pushed.
func published(t *testing.T, r Runner) (dir, remote string) {
	t.Helper()
	dir = initRepo(t, r)
	remote = t.TempDir()
	run(t, r, remote, "init", "--bare", "--initial-branch=main")
	run(t, r, dir, "remote", "add", "origin", remote)
	run(t, r, dir, "push", "--set-upstream", "origin", "main")
	return dir, remote
}

func TestFetch_ReadsWhatTheRemoteHas(t *testing.T) {
	r := gitRunner(t)
	dir, remote := published(t, r)
	other := clone(t, r, remote)
	write(t, filepath.Join(other, "vault", "k.age"), "cipher\n")
	run(t, r, other, "add", "-A")
	run(t, r, other, "commit", "-m", "vault: add k")
	run(t, r, other, "push")

	result, err := r.Fetch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Behind != 1 || result.Snapshot.Ahead != 0 {
		t.Fatalf("snapshot = %+v, want one commit behind", result.Snapshot)
	}
	if len(result.Commits) != 1 || result.Commits[0].Subject != "vault: add k" {
		t.Fatalf("commits = %+v, want the one waiting", result.Commits)
	}
	if len(result.Files) != 1 || result.Files[0].Path != "vault/k.age" {
		t.Fatalf("files = %+v, want the path it changes", result.Files)
	}
	// Fetching changes nothing on disk.
	if _, err := os.Lstat(filepath.Join(dir, "vault", "k.age")); !os.IsNotExist(err) {
		t.Fatalf("the file arrived on a fetch: %v", err)
	}
}

func TestFetch_UpToDateHasNothingWaiting(t *testing.T) {
	r := gitRunner(t)
	dir, _ := published(t, r)

	result, err := r.Fetch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Behind != 0 || len(result.Commits) != 0 || result.Note != "" {
		t.Fatalf("result = %+v, want nothing waiting", result)
	}
}

func TestFetch_NoRemoteSaysSo(t *testing.T) {
	r := gitRunner(t)
	dir := initRepo(t, r)

	result, err := r.Fetch(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Note, "no remote") {
		t.Fatalf("note = %q, want it to say there is no remote", result.Note)
	}
}

func TestFastForward_BringsTheRemoteCommitsIn(t *testing.T) {
	r := gitRunner(t)
	dir, remote := published(t, r)
	other := clone(t, r, remote)
	write(t, filepath.Join(other, "vault", "k.age"), "cipher\n")
	run(t, r, other, "add", "-A")
	run(t, r, other, "commit", "-m", "vault: add k")
	run(t, r, other, "push")
	if _, err := r.Fetch(dir); err != nil {
		t.Fatal(err)
	}

	snap, err := r.FastForward(dir)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Behind != 0 || snap.Ahead != 0 {
		t.Fatalf("snapshot = %+v, want it level with the upstream", snap)
	}
	if _, err := os.Lstat(filepath.Join(dir, "vault", "k.age")); err != nil {
		t.Fatalf("the file did not arrive: %v", err)
	}
}

// TestFastForward_DivergedIsRefused covers the case this design does not
// resolve: both sides moved, and a vault holds files no one can merge.
func TestFastForward_DivergedIsRefused(t *testing.T) {
	r := gitRunner(t)
	dir, remote := published(t, r)
	other := clone(t, r, remote)
	write(t, filepath.Join(other, "theirs.age"), "cipher\n")
	run(t, r, other, "add", "-A")
	run(t, r, other, "commit", "-m", "theirs")
	run(t, r, other, "push")
	write(t, filepath.Join(dir, "mine.age"), "cipher\n")
	run(t, r, dir, "add", "-A")
	run(t, r, dir, "commit", "-m", "mine")
	if _, err := r.Fetch(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := r.FastForward(dir); err != ErrDiverged {
		t.Fatalf("FastForward err = %v, want ErrDiverged", err)
	}
	// Nothing was merged: the local commit is still the tip.
	out := run(t, r, dir, "log", "-1", "--format=%s")
	if strings.TrimSpace(out) != "mine" {
		t.Fatalf("log = %q, want the local commit untouched", out)
	}
}

// TestFastForward_LocalChangeInTheWayIsGitsAnswer covers a working-tree change
// on a file the update would bring: git refuses, and its reason is reported as
// it stands.
func TestFastForward_LocalChangeInTheWayIsGitsAnswer(t *testing.T) {
	r := gitRunner(t)
	dir, remote := published(t, r)
	other := clone(t, r, remote)
	write(t, filepath.Join(other, "README.md"), "theirs\n")
	run(t, r, other, "add", "-A")
	run(t, r, other, "commit", "-m", "theirs")
	run(t, r, other, "push")
	write(t, filepath.Join(dir, "README.md"), "mine, uncommitted\n")
	if _, err := r.Fetch(dir); err != nil {
		t.Fatal(err)
	}

	if _, err := r.FastForward(dir); err == nil {
		t.Fatal("FastForward over a local change succeeded, want git's refusal")
	}
	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil || string(data) != "mine, uncommitted\n" {
		t.Fatalf("README.md = %q %v, want the local change kept", data, err)
	}
}

func TestParseNameStatus_RenameKeepsTheNewPath(t *testing.T) {
	changes := parseNameStatus("R100\x00old.age\x00new.age\x00M\x00other.age\x00", FetchLimit)
	if len(changes) != 2 {
		t.Fatalf("changes = %+v, want two", changes)
	}
	if changes[0].Path != "new.age" || changes[1].Path != "other.age" {
		t.Fatalf("changes = %+v, want the new path and the modified one", changes)
	}
}
