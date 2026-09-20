package cred

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/gitrepo"

	"filippo.io/age"
	tea "github.com/charmbracelet/bubbletea"
)

// vaultWith builds the page on a folder holding one file this machine opens,
// with its recipient record beside it.
func vaultWith(t *testing.T, root string) vaultModel {
	t.Helper()
	mine, _ := age.GenerateX25519Identity()
	writeFile(t, filepath.Join(root, "keys", "age.txt"), mine.String()+"\n", 0o600)
	vaultDir := filepath.Join(root, "vault")
	ageFile(t, filepath.Join(vaultDir, "top.age"), mine.Recipient())
	writeFile(t, filepath.Join(vaultDir, "top.age.json"), `{"version":1,"created":"2026-09-15T14:00:00+10:00","recipients":[]}`, 0o644)
	path := filepath.Join(root, "credentials.json")
	writeFile(t, path, `{"identities":["`+filepath.Join(root, "keys")+`"],"vault":"`+vaultDir+`"}`, 0o600)

	m := newVaultModel()
	m.path = path
	return runVault(t, m)
}

func TestVaultDelete_MovesTheFileAndItsRecordToTheTrash(t *testing.T) {
	root := t.TempDir()
	m := vaultWith(t, root)
	var moved []string
	m.trash = func(path string) (string, error) {
		moved = append(moved, path)
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return filepath.Join(root, "Trash", filepath.Base(path)), nil
	}
	m.list.SelectID("file:0")

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if m.del == nil {
		t.Fatal("d opened no confirmation")
	}
	if view := stripVault(m); !strings.Contains(view, "top.age.json") {
		t.Fatalf("dialog = %q, want it to name the record it takes too", view)
	}

	// The dialog starts on Cancel, so the confirmation is chosen first.
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	if len(moved) != 2 {
		t.Fatalf("moved = %v, want the file and its record", moved)
	}
	for _, name := range []string{"top.age", "top.age.json"} {
		if _, err := os.Lstat(filepath.Join(root, "vault", name)); !os.IsNotExist(err) {
			t.Fatalf("%s is still there: %v", name, err)
		}
	}
	if !strings.Contains(m.Status().Center, "Moved top.age and top.age.json") {
		t.Fatalf("status = %q, want it to say what moved", m.Status().Center)
	}
}

func TestVaultDelete_CancelKeepsTheFile(t *testing.T) {
	root := t.TempDir()
	m := vaultWith(t, root)
	m.trash = func(string) (string, error) {
		t.Fatal("cancelling deleted a file")
		return "", nil
	}
	m.list.SelectID("file:0")
	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.del != nil {
		t.Fatal("esc left the dialog open")
	}
	if _, err := os.Lstat(filepath.Join(root, "vault", "top.age")); err != nil {
		t.Fatalf("top.age = %v, want it left alone", err)
	}
}

// TestVaultPublish_NoGitSaysSo covers the page's answer when git is not there:
// dgs is one binary and does not carry one, so the menu says what is missing
// rather than failing at the first command.
func TestVaultPublish_NoGitSaysSo(t *testing.T) {
	root := t.TempDir()
	m := vaultWith(t, root)
	m.gitRunner = gitrepo.Runner{Binary: filepath.Join(root, "no-such-git")}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.git != nil {
		t.Fatal("the flow stayed open with no git to run")
	}
	if !strings.Contains(m.Status().Center, "git is not installed") {
		t.Fatalf("status = %q, want it to say git is missing", m.Status().Center)
	}
}

func TestVaultPublish_NotARepositorySaysSo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	m := vaultWith(t, root)

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.git != nil {
		t.Fatal("the flow stayed open outside a repository")
	}
	if !strings.Contains(m.Status().Center, "not in a git repository") {
		t.Fatalf("status = %q, want it to say the folder is not in one", m.Status().Center)
	}
}

// TestVaultPublish_CommitsAndPushes is the whole menu on a real repository
// with a real remote: `p`, the message, the confirmation, and what the page
// says afterwards.
func TestVaultPublish_CommitsAndPushes(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	m := vaultWith(t, root)
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	git(root, "init", "--initial-branch=main")
	git(root, "config", "user.name", "Test")
	git(root, "config", "user.email", "test@example.com")
	git(root, "config", "commit.gpgsign", "false")
	remote := t.TempDir()
	git(remote, "init", "--bare", "--initial-branch=main")
	git(root, "remote", "add", "origin", remote)

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.git == nil || m.git.stage != gitMessage {
		t.Fatalf("flow = %+v, want the commit message asked for", m.git)
	}
	if got := m.git.input.Value(); !strings.HasPrefix(got, "vault: update") {
		t.Fatalf("default message = %q, want one naming what moved", got)
	}
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.git.stage != gitConfirm {
		t.Fatalf("stage = %v, want the confirmation", m.git.stage)
	}
	view := stripVault(m)
	for _, want := range []string{"PUBLISH", "main", "vault/"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation = %q, want %q in it", view, want)
		}
	}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.git != nil {
		t.Fatalf("flow = %+v, want it finished", m.git)
	}
	if center := m.Status().Center; !strings.Contains(center, "Committed") || !strings.Contains(center, "pushed") {
		t.Fatalf("status = %q, want it to say what was committed and pushed", center)
	}
	out := git(remote, "log", "-1", "--format=%s", "main")
	if !strings.HasPrefix(strings.TrimSpace(out), "vault: update") {
		t.Fatalf("remote log = %q, want the vault commit", out)
	}
}

// TestVaultPublish_NothingToDoDoesNotAsk covers a clean tree: there is no
// message worth typing, so the page says so instead of opening a form.
func TestVaultPublish_NothingToDoDoesNotAsk(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	m := vaultWith(t, root)
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	git(root, "init", "--initial-branch=main")
	git(root, "config", "user.name", "Test")
	git(root, "config", "user.email", "test@example.com")
	git(root, "config", "commit.gpgsign", "false")
	remote := t.TempDir()
	git(remote, "init", "--bare", "--initial-branch=main")
	git(root, "remote", "add", "origin", remote)
	git(root, "add", "-A")
	git(root, "commit", "-m", "everything")
	git(root, "push", "--set-upstream", "origin", "main")

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.git != nil {
		t.Fatalf("flow = %+v, want nothing asked for", m.git)
	}
	if !strings.Contains(m.Status().Center, "Nothing to publish") {
		t.Fatalf("status = %q, want it to say there is nothing to publish", m.Status().Center)
	}
}

// gitAt runs git in dir for the update tests, which set up two working trees
// of one remote.
func gitAt(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// publishedVault is the page on a vault folder inside a repository whose first
// commit is pushed, and a second working tree of the same remote standing in
// for another machine.
func publishedVault(t *testing.T) (m vaultModel, root, other string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root = t.TempDir()
	m = vaultWith(t, root)
	gitAt(t, root, "init", "--initial-branch=main")
	gitAt(t, root, "config", "user.name", "Test")
	gitAt(t, root, "config", "user.email", "test@example.com")
	gitAt(t, root, "config", "commit.gpgsign", "false")
	remote := t.TempDir()
	gitAt(t, remote, "init", "--bare", "--initial-branch=main")
	gitAt(t, root, "remote", "add", "origin", remote)
	gitAt(t, root, "add", "-A")
	gitAt(t, root, "commit", "-m", "the vault")
	gitAt(t, root, "push", "--set-upstream", "origin", "main")

	other = t.TempDir()
	gitAt(t, other, "clone", remote, ".")
	gitAt(t, other, "config", "user.name", "Other")
	gitAt(t, other, "config", "user.email", "other@example.com")
	gitAt(t, other, "config", "commit.gpgsign", "false")
	return m, root, other
}

func TestVaultUpdate_BringsAnotherMachinesFileIn(t *testing.T) {
	m, root, other := publishedVault(t)
	writeFile(t, filepath.Join(other, "vault", "nas.age"), "cipher", 0o644)
	gitAt(t, other, "add", "-A")
	gitAt(t, other, "commit", "-m", "vault: add nas")
	gitAt(t, other, "push")

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.update == nil || m.update.stage != updateConfirm {
		t.Fatalf("flow = %+v, want the confirmation", m.update)
	}
	view := stripVault(m)
	for _, want := range []string{"UPDATE", "vault: add nas", "vault/nas.age"} {
		if !strings.Contains(view, want) {
			t.Fatalf("confirmation = %q, want %q in it", view, want)
		}
	}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.update != nil {
		t.Fatalf("flow = %+v, want it finished", m.update)
	}
	if _, err := os.Lstat(filepath.Join(root, "vault", "nas.age")); err != nil {
		t.Fatalf("the file did not arrive: %v", err)
	}
	// The list is read again, so the new file is on the page.
	if view := stripVault(m); !strings.Contains(view, "nas.age") {
		t.Fatalf("view = %q, want the file that arrived listed", view)
	}
}

func TestVaultUpdate_CancelFetchesAndChangesNothing(t *testing.T) {
	m, root, other := publishedVault(t)
	writeFile(t, filepath.Join(other, "vault", "nas.age"), "cipher", 0o644)
	gitAt(t, other, "add", "-A")
	gitAt(t, other, "commit", "-m", "vault: add nas")
	gitAt(t, other, "push")

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = send(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.update != nil {
		t.Fatal("esc left the dialog open")
	}
	if _, err := os.Lstat(filepath.Join(root, "vault", "nas.age")); !os.IsNotExist(err) {
		t.Fatalf("cancelling still brought the file in: %v", err)
	}
	// The fetch itself happened, so the commit is in the object store.
	if out := gitAt(t, root, "log", "-1", "--format=%s", "origin/main"); !strings.Contains(out, "vault: add nas") {
		t.Fatalf("origin/main = %q, want the fetched commit", out)
	}
}

func TestVaultUpdate_DivergedIsReportedNotMerged(t *testing.T) {
	m, root, other := publishedVault(t)
	writeFile(t, filepath.Join(other, "vault", "theirs.age"), "cipher", 0o644)
	gitAt(t, other, "add", "-A")
	gitAt(t, other, "commit", "-m", "theirs")
	gitAt(t, other, "push")
	writeFile(t, filepath.Join(root, "vault", "mine.age"), "cipher", 0o644)
	gitAt(t, root, "add", "-A")
	gitAt(t, root, "commit", "-m", "mine")

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.update != nil {
		t.Fatalf("flow = %+v, want nothing asked for", m.update)
	}
	if center := m.Status().Center; !strings.Contains(center, "Diverged") || !strings.Contains(center, "terminal") {
		t.Fatalf("status = %q, want it to name the divergence and where to fix it", center)
	}
	if out := gitAt(t, root, "log", "-1", "--format=%s"); strings.TrimSpace(out) != "mine" {
		t.Fatalf("log = %q, want the local commit untouched", out)
	}
}

func TestVaultUpdate_UpToDateSaysSo(t *testing.T) {
	m, _, _ := publishedVault(t)

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.update != nil {
		t.Fatalf("flow = %+v, want nothing asked for", m.update)
	}
	if !strings.Contains(m.Status().Center, "Up to date with origin/main") {
		t.Fatalf("status = %q, want it to say the branch is level", m.Status().Center)
	}
}

func TestVaultUpdate_NoGitSaysSo(t *testing.T) {
	root := t.TempDir()
	m := vaultWith(t, root)
	m.gitRunner = gitrepo.Runner{Binary: filepath.Join(root, "no-such-git")}

	m = send(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.update != nil {
		t.Fatal("the flow stayed open with no git to run")
	}
	if !strings.Contains(m.Status().Center, "git is not installed") {
		t.Fatalf("status = %q, want it to say git is missing", m.Status().Center)
	}
}
