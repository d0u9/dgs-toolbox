package cred

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"dgs-toolbox/internal/cred/seal"
	"filippo.io/age"
)

func safetyFixture(t *testing.T) (root, settings, source, keyFile string, id *age.X25519Identity) {
	t.Helper()
	root = t.TempDir()
	var err error
	id, err = age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	write := func(name string, data []byte) string {
		t.Helper()
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	keyFile = write("keys/key.txt", []byte(id.String()))
	cfg, err := json.Marshal(map[string]any{"identities": []string{filepath.Join(root, "keys")}, "recipients": filepath.Join(root, "recipients")})
	if err != nil {
		t.Fatal(err)
	}
	settings = write("config.json", cfg)
	write("recipients/hosts/host-a.json", []byte(`{"keys":[{"public_key":"`+id.Recipient().String()+`","description":"Synthetic"}]}`))
	source = write("notes.txt", []byte("synthetic plaintext"))
	return
}

func TestSharedKeyWarningsDoNotBlockCLI(t *testing.T) {
	root, settings, source, _, _ := safetyFixture(t)
	data, err := os.ReadFile(filepath.Join(root, "recipients", "hosts", "host-a.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "recipients", "hosts", "host-b.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := listRecipients(&out, settings, "hosts"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warning:") {
		t.Fatal("warning missing")
	}
	out.Reset()
	if err := runCrypt(&out, settings, "encrypt", source, map[string]string{"host": `["host-a","host-b"]`}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "warning:") || !strings.Contains(out.String(), "verified by decryption") {
		t.Fatal(out.String())
	}
}

func TestCryptDoesNotWriteExtraKeys(t *testing.T) {
	root, settings, source, keyFile, id := safetyFixture(t)
	initial, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	// The CLI must work with read-only identity input and no writable key directory.
	if err := os.Chmod(keyFile, 0400); err != nil {
		t.Fatal(err)
	}
	keyDir := filepath.Dir(keyFile)
	if err := os.Chmod(keyDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(keyDir, 0700)
	before := map[string]bool{}
	walk := func(paths map[string]bool) {
		t.Helper()
		if err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				paths[p] = true
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	walk(before)
	var out bytes.Buffer
	cipher := filepath.Join(root, "output.age")
	restored := filepath.Join(root, "restored")
	if err := runCrypt(&out, settings, "encrypt", source, map[string]string{"host": `["host-a"]`, "output": cipher}); err != nil {
		t.Fatal(err)
	}
	if err := runCrypt(&out, settings, "decrypt", cipher, map[string]string{"output": restored}); err != nil {
		t.Fatal(err)
	}
	after := map[string]bool{}
	walk(after)
	for p := range after {
		if !before[p] && p != cipher && p != restored {
			t.Fatal("unexpected file created")
		}
	}
	got, err := os.ReadFile(keyFile)
	if err != nil || !bytes.Equal(got, initial) {
		t.Fatal("identity input changed")
	}
	output, err := os.ReadFile(restored)
	if err != nil || strings.Contains(string(output), id.String()) || string(output) != "synthetic plaintext" {
		t.Fatal("unexpected plaintext output")
	}
}

func TestCredInterruptHelper(t *testing.T) {
	if os.Getenv("DGS_CRED_INTERRUPT_CHILD") != "1" {
		return
	}
	err := runCrypt(os.Stdout, os.Getenv("DGS_CRED_TEST_CONFIG"), "decrypt", os.Getenv("DGS_CRED_TEST_CIPHER"), map[string]string{"output": os.Getenv("DGS_CRED_TEST_OUTPUT")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestCredInterruptCleansPlaintext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Process.Signal cannot send an interrupt on Windows")
	}
	root, settings, source, _, id := safetyFixture(t)
	if err := os.Truncate(source, 128<<20); err != nil {
		t.Fatal(err)
	}
	cipher := filepath.Join(root, "cipher.age")
	if _, err := seal.Encrypt(context.Background(), source, cipher, []seal.Recipient{{PublicKey: id.Recipient().String()}}, nil, nil); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "restored")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCredInterruptHelper$")
	cmd.Env = append(os.Environ(), "DGS_CRED_INTERRUPT_CHILD=1", "DGS_CRED_TEST_CONFIG="+settings, "DGS_CRED_TEST_CIPHER="+cipher, "DGS_CRED_TEST_OUTPUT="+destination)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	part := destination + ".dgs-part"
	for {
		if info, err := os.Stat(part); err == nil && info.Size() > 1<<20 {
			break
		}
		if ctx.Err() != nil {
			_ = cmd.Wait()
			t.Fatal("child did not start writing plaintext")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%v: %s", err, output.String())
	}
	for _, p := range []string{part, destination} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Fatal("interrupted operation left plaintext")
		}
	}
	// A cancelled run must not block a clean retry.
	var retry bytes.Buffer
	if err := runCrypt(&retry, settings, "decrypt", cipher, map[string]string{"output": destination}); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultOutputPreservesSymlinkParentSemantics(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(real, "child"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(real, "child"), filepath.Join(root, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "notes.txt"), []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	raw := root + "/link/../notes.txt"
	got, err := cryptPaths("encrypt", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(resolved, "notes.txt.age") {
		t.Fatalf("wrong default destination: %q", got)
	}
}
