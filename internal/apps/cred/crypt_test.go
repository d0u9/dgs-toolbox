package cred

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/cred/opened"
	"dgs-toolbox/internal/cred/record"
	"filippo.io/age"
)

func TestCryptStandaloneRoundTrip(t *testing.T) {
	root := t.TempDir()
	write := func(name string, data []byte) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	write("identities/key.txt", []byte(id.String()))
	write("recipients/hosts/host-a.json", []byte(`{"keys":[{"public_key":"`+id.Recipient().String()+`","description":"Test"}]}`))
	write("recipients/groups/g-team.json", []byte(`{"hosts":["host-a"]}`))
	settings, _ := json.Marshal(map[string]any{"identities": []string{filepath.Join(root, "identities")}, "recipients": filepath.Join(root, "recipients")})
	configPath := write("config.json", settings)
	plain := []byte("synthetic secret\x00\xff\n")
	source := write("notes.txt", plain)
	encrypted := filepath.Join(root, "notes.age")
	var out bytes.Buffer
	flags := map[string]string{"output": encrypted, "host": `["HOST-A"]`, "group": `["G-TEAM"]`}
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "verified by decryption") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(record.PathFor(encrypted)); !os.IsNotExist(err) {
		t.Fatalf("unexpected record: %v", err)
	}
	restored := filepath.Join(root, "restored.txt")
	if err := runCrypt(&out, configPath, "decrypt", encrypted, map[string]string{"output": restored}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(restored)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("round trip: %q, %v", got, err)
	}
	info, _ := os.Stat(restored)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode())
	}
	for _, kind := range []string{"encrypt", "decrypt"} {
		if err := runCrypt(&out, configPath, kind, source, map[string]string{"output": " "}); err == nil {
			t.Fatal("blank output accepted")
		}
	}
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err == nil {
		t.Fatal("overwrote ciphertext")
	}
	if err := runCrypt(&out, configPath, "decrypt", encrypted, map[string]string{"output": restored}); err == nil {
		t.Fatal("overwrote plaintext")
	}
	flags["output"] = filepath.Join(root, "unknown.age")
	flags["host"] = `["missing"]`
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err == nil {
		t.Fatal("unknown host accepted")
	}
	if _, err := os.Stat(flags["output"]); !os.IsNotExist(err) {
		t.Fatal("output created on failure")
	}
	flags["host"] = `[]`
	flags["output"] = filepath.Join(root, "folder.age")
	write("folder/item.txt", plain)
	if err := runCrypt(&out, configPath, "encrypt", filepath.Join(root, "folder"), flags); err != nil {
		t.Fatal(err)
	}
	expanded, err := opened.Open(flags["output"], []age.Identity{id}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer expanded.Close()
	if expanded.Archive != opened.ArchiveTarGz {
		t.Fatal("folder was not archived")
	}
	// A pre-existing part file belongs to another operation and must survive.
	flags["output"] = filepath.Join(root, "busy.age")
	part := write("busy.age.dgs-part", []byte("existing operation"))
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err == nil {
		t.Fatal("existing part accepted")
	}
	if got, err := os.ReadFile(part); err != nil || string(got) != "existing operation" {
		t.Fatalf("existing part changed: %q, %v", got, err)
	}
	foreign, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	write("recipients/hosts/host-b.json", []byte(`{"keys":[{"public_key":"`+foreign.Recipient().String()+`","description":"Other"}]}`))
	flags["output"] = filepath.Join(root, "foreign.age")
	flags["host"], flags["group"] = `["host-b"]`, `[]`
	out.Reset()
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "not verified by decryption") {
		t.Fatal(out.String())
	}
	if err := runCrypt(&out, configPath, "decrypt", flags["output"], map[string]string{"output": filepath.Join(root, "wrong-key")}); err == nil {
		t.Fatal("wrong identity accepted")
	}
	flags["group"] = `["missing"]`
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err == nil {
		t.Fatal("unknown group accepted")
	}
	flags["host"], flags["group"] = `[]`, `[]`
	if err := runCrypt(&out, configPath, "encrypt", source, flags); err == nil {
		t.Fatal("empty selection accepted")
	}
	// Corrupt an otherwise valid ciphertext; authenticated decryption must not publish.
	cipher, err := os.ReadFile(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	cipher[len(cipher)-1] ^= 1
	damaged := write("damaged.age", cipher)
	damagedOutput := filepath.Join(root, "damaged-output")
	if err := runCrypt(&out, configPath, "decrypt", damaged, map[string]string{"output": damagedOutput}); err == nil {
		t.Fatal("tampered ciphertext accepted")
	}
	if _, err := os.Stat(damagedOutput); !os.IsNotExist(err) {
		t.Fatal("tampered ciphertext published plaintext")
	}
	missingSettings, err := json.Marshal(map[string]any{"identities": []string{filepath.Join(root, "missing-identities")}})
	if err != nil {
		t.Fatal(err)
	}
	missingConfig := write("missing-config.json", missingSettings)
	if err := runCrypt(&out, missingConfig, "decrypt", encrypted, map[string]string{"output": damagedOutput}); err == nil || !strings.Contains(err.Error(), "missing-identities") {
		t.Fatalf("missing identity diagnostics: %v", err)
	}
	// Authentication failure must not publish any plaintext.
	bad := write("bad.age", []byte("not an age file"))
	dest := filepath.Join(root, "bad-output")
	if err := runCrypt(&out, configPath, "decrypt", bad, map[string]string{"output": dest}); err == nil {
		t.Fatal("invalid ciphertext accepted")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatal("plaintext published on failure")
	}
}

func TestCryptPathValidation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(source, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(root, "broken")
	if err := os.Symlink(filepath.Join(root, "absent"), broken); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ kind, source, output, want string }{
		{"encrypt", source, "", source + ".age"},
		{"encrypt", root, "", root + ".tar.gz.age"},
	} {
		got, err := cryptPaths(tc.kind, tc.source, tc.output)
		parent, resolveErr := filepath.EvalSymlinks(filepath.Dir(tc.want))
		if resolveErr != nil {
			t.Fatal(resolveErr)
		}
		want := filepath.Join(parent, filepath.Base(tc.want))
		if err != nil || got != want {
			t.Fatalf("%q, %v; want %q", got, err, tc.want)
		}
	}
	for _, tc := range []struct{ kind, source, output string }{
		{"encrypt", link, ""}, {"decrypt", link, ""},
		{"encrypt", link + "/", ""}, {"encrypt", link + "/.", ""}, {"encrypt", link + "/./", ""},
		{"encrypt", source, broken}, {"decrypt", source, broken},
		{"encrypt", source, root}, {"encrypt", source, source},
		{"decrypt", root, ""}, {"decrypt", source, ""},
		{"encrypt", source, filepath.Join(root, "absent", "output")},
		{"encrypt", source, "\x00"}, {"encrypt", " ", ""},
	} {
		if _, err := cryptPaths(tc.kind, tc.source, tc.output); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
}
