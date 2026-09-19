package conf

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/conf/secretstore"
	"dgs-toolbox/internal/config"
)

func checkConfig(root, secrets string) config.Config {
	var c config.Config
	c.Conf.Root = root
	c.Conf.Secrets = secrets
	return c
}

// TestCheckReport_CleanInventory covers the ordinary case. A report that
// printed the whole inventory when nothing is wrong is a report nobody reads
// the top of, so success is one line and no error.
func TestCheckReport_CleanInventory(t *testing.T) {
	root, secrets := buildInspectRoot(t), t.TempDir()
	// buildInspectRoot carries a node file that will not parse, on purpose;
	// this case is the inventory without it.
	if err := os.Remove(filepath.Join(root, "nodes", "bad.yaml")); err != nil {
		t.Fatal(err)
	}
	// Generate everything the inventory implies, so the store is in step.
	if err := writeImplied(t, root, secrets); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := writeCheckReport(&out, checkConfig(root, secrets)); err != nil {
		t.Fatalf("writeCheckReport: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "no problem found") {
		t.Fatalf("report = %q, want it to say so", out.String())
	}
	if strings.Count(out.String(), "\n") != 1 {
		t.Fatalf("report = %q, want one line when there is nothing to say", out.String())
	}
}

// TestCheckReport_NamesEveryKindOfProblem covers the report's reason to
// exist: one command that finds what four tabs would have shown. Each of
// these is silent in a different way — a node file that will not parse drops
// its instances, a route that does not exist renders nothing, a missing
// secret fails mid-export, and an orphaned one is never regenerated and
// never deleted.
func TestCheckReport_NamesEveryKindOfProblem(t *testing.T) {
	root, secrets := buildInspectRoot(t), t.TempDir()
	if err := writeImplied(t, root, secrets); err != nil {
		t.Fatal(err)
	}

	// A node file that will not parse is already in buildInspectRoot.
	// Add an access list naming a route that does not exist.
	users := filepath.Join(root, "users.yaml")
	body, err := os.ReadFile(users)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(users, []byte(strings.Replace(string(body), "access: [sfo]", "access: [sfo, nosuchroute]", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	// A credential the inventory implies and the store does not hold.
	if err := os.Remove(filepath.Join(secrets, "ss-srv", "self", "psk", "main")); err != nil {
		t.Fatal(err)
	}
	// A file nothing implies.
	writeSecret(t, secrets, "ss-srv/main/user/nobody", "orphan")
	// A rotation nobody finished.
	stale := writeSecret(t, secrets, "ss-srv/main/user/yak.previous", "old")
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	err = writeCheckReport(&out, checkConfig(root, secrets))

	var problems ErrProblems
	if !errors.As(err, &problems) {
		t.Fatalf("writeCheckReport err = %v, want ErrProblems so the shell exits non-zero", err)
	}
	report := out.String()
	for _, want := range []string{
		"bad.yaml",
		"nosuchroute",
		"secret missing: ss-srv/self/psk",
		"secret orphaned: ss-srv/main/user/nobody",
		"days old",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("report = %q, want it to name %q", report, want)
		}
	}
	// The rotation leftover is validate's rule and the store's comparison
	// both; it must be reported once.
	if strings.Count(report, ".previous") != 1 {
		t.Fatalf("report = %q, want the stale rotation named once", report)
	}
	// The count is the error's message, so printing it in the report too
	// would put the same line on stdout and on stderr.
	if strings.Contains(report, "problems found") {
		t.Fatalf("report = %q, want the count left to the error", report)
	}
}

// TestCheckReport_WithoutASecretsStore covers conf.secrets unset. Every path
// the inventory implies is still known and none can be checked, which is
// worth one line rather than a clean bill of health.
func TestCheckReport_WithoutASecretsStore(t *testing.T) {
	var out bytes.Buffer
	err := writeCheckReport(&out, checkConfig(buildInspectRoot(t), ""))
	if err == nil {
		t.Fatalf("report = %q, want an unset conf.secrets to count as a problem", out.String())
	}
	if !strings.Contains(out.String(), "conf.secrets is not configured") {
		t.Fatalf("report = %q, want it to name what is unset", out.String())
	}
}

// TestCheckReport_WithoutARoot covers conf.root unset, which is not a
// problem with an inventory but the absence of one to check.
func TestCheckReport_WithoutARoot(t *testing.T) {
	var out bytes.Buffer
	err := writeCheckReport(&out, checkConfig("", ""))
	if err == nil || !strings.Contains(err.Error(), "conf.root") {
		t.Fatalf("err = %v, want it to name conf.root", err)
	}
}

// TestTargetReport lists what the root holds without rendering anything,
// which is what docs/apps/conf/export.md#targets-and-selectors asks of it.
func TestTargetReport(t *testing.T) {
	var out bytes.Buffer
	if err := writeTargetReport(&out, checkConfig(buildInspectRoot(t), "")); err != nil {
		t.Fatalf("writeTargetReport: %v", err)
	}
	report := out.String()
	for _, want := range []string{"srv", "ss-srv", "ssserver", "yak (unmanaged user)", "broken"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report = %q, want it to name %q", report, want)
		}
	}
}

func writeSecret(t *testing.T, root, rel, value string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeImplied fills a store with every path root's inventory implies, so a
// test starts from one that is in step.
func writeImplied(t *testing.T, root, secrets string) error {
	t.Helper()
	l, err := load(root)
	if err != nil {
		return err
	}
	for _, p := range secretstore.ImpliedPaths(l.inv, l.manifests, l.derived) {
		writeSecret(t, secrets, p.String(), "value")
	}
	return nil
}
