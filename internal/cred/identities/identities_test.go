package identities

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/recipients"
)

func pemBytes(t *testing.T, block *pem.Block, err error) []byte {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

func publicOf(t *testing.T, key any) string {
	t.Helper()
	public, err := ssh.NewPublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(public)))
}

func write(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestParseSSH(t *testing.T) {
	edPublic, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	rsa2048, _ := rsa.GenerateKey(rand.Reader, 2048)
	rsa1024, _ := rsa.GenerateKey(rand.Reader, 1024)
	ec, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)

	openssh, err := ssh.MarshalPrivateKey(edPrivate, "")
	protected, err2 := ssh.MarshalPrivateKeyWithPassphrase(edPrivate, "", []byte("secret"))
	if err2 != nil {
		t.Fatal(err2)
	}
	// Legacy encrypted PEM is deprecated, and still what old keys on disk are.
	legacy, err3 := x509.EncryptPEMBlock(rand.Reader, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsa2048), []byte("secret"), x509.PEMCipherAES256)
	pkcs8, err4 := x509.MarshalPKCS8PrivateKey(rsa2048)
	ecOpenSSH, err5 := ssh.MarshalPrivateKey(ec, "")
	if err3 != nil || err4 != nil || err5 != nil {
		t.Fatal(err3, err4, err5)
	}

	for name, test := range map[string]struct {
		data    []byte
		status  Status
		kind    string
		public  string
		message string
	}{
		"openssh ed25519":            {data: pemBytes(t, openssh, err), status: Usable, kind: "ssh-ed25519", public: publicOf(t, edPublic)},
		"protected keeps public key": {data: pem.EncodeToMemory(protected), status: Protected, kind: "ssh-ed25519", public: publicOf(t, edPublic), message: "passphrase"},
		"pkcs1 rsa":                  {data: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsa2048)}), status: Usable, kind: "ssh-rsa", public: publicOf(t, &rsa2048.PublicKey)},
		"pkcs8 rsa":                  {data: pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}), status: Usable, kind: "ssh-rsa", public: publicOf(t, &rsa2048.PublicKey)},
		"legacy encrypted":           {data: pem.EncodeToMemory(legacy), status: Protected, kind: "ssh", message: "passphrase"},
		"short rsa":                  {data: pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rsa1024)}), status: Unsupported, kind: "ssh-rsa", message: "shorter than 2048"},
		"ecdsa":                      {data: pem.EncodeToMemory(ecOpenSSH), status: Unsupported, kind: "ecdsa-sha2-nistp256", message: "age cannot decrypt"},
		"corrupt":                    {data: []byte("-----BEGIN OPENSSH PRIVATE KEY-----\nAAAA\n-----END OPENSSH PRIVATE KEY-----\n"), status: Invalid, kind: "ssh"},
	} {
		found := Parse(test.data)
		if len(found) != 1 {
			t.Errorf("%s: %d identities", name, len(found))
			continue
		}
		identity := found[0]
		if identity.Status != test.status || identity.Kind != test.kind || identity.Public.Key != test.public {
			t.Errorf("%s: got %s %q %q (%s)", name, identity.Status, identity.Kind, identity.Public.Key, identity.Message)
		}
		if !strings.Contains(identity.Message, test.message) {
			t.Errorf("%s: message %q, want %q", name, identity.Message, test.message)
		}
	}
}

func TestParseAge(t *testing.T) {
	first, _ := age.GenerateX25519Identity()
	second, _ := age.GenerateX25519Identity()
	data := strings.Join([]string{
		"# created: 2026-09-15",
		"# public key: " + first.Recipient().String(),
		first.String(),
		"",
		second.String(),
		"AGE-PLUGIN-YUBIKEY-1QQQQ",
		"AGE-SECRET-KEY-PQ-1QQQQ",
		"AGE-SECRET-KEY-1NOTAKEY",
		"something else",
	}, "\n")
	found := Parse([]byte(data))
	want := []struct {
		line   int
		status Status
		public string
	}{
		{3, Usable, first.Recipient().String()},
		{5, Usable, second.Recipient().String()},
		{6, Unsupported, ""},
		{7, Unsupported, ""},
		{8, Invalid, ""},
		{9, Invalid, ""},
	}
	if len(found) != len(want) {
		t.Fatalf("found %+v", found)
	}
	for i, w := range want {
		if found[i].Line != w.line || found[i].Status != w.status || found[i].Public.Key != w.public {
			t.Errorf("%d: got line %d %s %q (%s)", i, found[i].Line, found[i].Status, found[i].Public.Key, found[i].Message)
		}
	}
}

func TestParseIgnoresOtherFiles(t *testing.T) {
	edPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	for name, data := range map[string]string{
		"public key":  publicOf(t, edPublic) + " me@host\n",
		"known_hosts": "github.com ssh-ed25519 AAAAC3Nza...\n",
		"ssh config":  "Host *\n  IdentityFile ~/.ssh/id_ed25519\n",
		"certificate": "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
		"mentions":    "see AGE-SECRET-KEY-1 in the docs\n",
		"empty":       "",
	} {
		if found := Parse([]byte(data)); len(found) != 0 {
			t.Errorf("%s: %+v", name, found)
		}
	}
}

func TestDiscover(t *testing.T) {
	base := t.TempDir()
	keys := filepath.Join(base, "keys")
	outside := filepath.Join(base, "outside")
	identity, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	openssh, err := ssh.MarshalPrivateKey(edPrivate, "")
	if err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(keys, "age", "keys.txt"), []byte(identity.String()+"\n"), 0o600)
	write(t, filepath.Join(keys, "deep", "er", "id"), pem.EncodeToMemory(openssh), 0o644)
	write(t, filepath.Join(keys, "notes.md"), []byte("hello"), 0o644)
	big := append([]byte(identity.String()+"\n"), make([]byte, 2000)...)
	write(t, filepath.Join(keys, "big"), big, 0o600)
	write(t, filepath.Join(outside, "other.txt"), []byte(other.String()+"\n"), 0o600)
	if err := os.Symlink(outside, filepath.Join(keys, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "other.txt"), filepath.Join(keys, "linked-file")); err != nil {
		t.Fatal(err)
	}
	linkedRoot := filepath.Join(base, "root-link")
	if err := os.Symlink(keys, linkedRoot); err != nil {
		t.Fatal(err)
	}

	scan := Discover([]string{linkedRoot, keys, filepath.Join(base, "absent")}, Options{MaxSize: 1024})

	// keys.txt and deep/er/id once each despite two roots reaching them, the
	// linked file read, the linked directory and the oversized file not.
	realKeys, _ := filepath.EvalSymlinks(keys)
	var paths []string
	for _, found := range scan.Identities {
		paths = append(paths, strings.TrimPrefix(found.Path, realKeys+string(filepath.Separator)))
	}
	if strings.Join(paths, ",") != "age/keys.txt,deep/er/id,linked-file" {
		t.Fatalf("paths %q", paths)
	}
	for _, found := range scan.Identities {
		wantWarning := strings.HasSuffix(found.Path, "id")
		if (len(found.Warnings) > 0) != wantWarning {
			t.Errorf("%s warnings %q", found.Path, found.Warnings)
		}
	}
	if len(scan.Problems) != 1 || !strings.Contains(scan.Problems[0].Message, "does not exist") {
		t.Errorf("problems %+v", scan.Problems)
	}
}

func TestDiscoverReportsUnreadableFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads everything")
	}
	dir, _ := filepath.EvalSymlinks(t.TempDir())
	path := filepath.Join(dir, "locked")
	write(t, path, []byte("x"), 0o000)
	scan := Discover([]string{dir}, Options{})
	if len(scan.Problems) != 1 || scan.Problems[0].Path != path {
		t.Errorf("problems %+v", scan.Problems)
	}
}

func TestMatches(t *testing.T) {
	identity, _ := age.GenerateX25519Identity()
	found := Parse([]byte(identity.String()))[0]
	public := found.Public
	folder := recipients.Folder{Hosts: []recipients.Host{
		{Name: "nas", Keys: []recipients.Key{{PublicKey: public, Description: "main"}}},
		{Name: "copy", Keys: []recipients.Key{{PublicKey: public, Description: "same"}}},
		{Name: "other"},
	}}
	matches := Matches(found, folder)
	if len(matches) != 2 || matches[0].Host != "nas" || matches[1].Key.Description != "same" {
		t.Errorf("matches %+v", matches)
	}
	if Matches(Identity{Status: Protected}, folder) != nil {
		t.Error("identity without a public key matched")
	}
	stranger, _ := age.GenerateX25519Identity()
	if Matches(Parse([]byte(stranger.String()))[0], folder) != nil {
		t.Error("unregistered identity matched")
	}
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	first, _ := age.GenerateX25519Identity()
	second, _ := age.GenerateX25519Identity()
	_, edPrivate, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(edPrivate, "")
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "keys.txt"), []byte("# comment\n"+first.String()+"\n"+second.String()+"\n"), 0o600)
	write(t, filepath.Join(dir, "id"), pem.EncodeToMemory(block), 0o600)

	scan := Discover([]string{dir}, Options{})
	if len(scan.Identities) != 3 {
		t.Fatalf("identities %+v", scan.Identities)
	}
	for _, found := range scan.Identities {
		opened, err := Open(found)
		if err != nil {
			t.Fatalf("%s:%d: %v", found.Path, found.Line, err)
		}
		// The opened identity decrypts what its derived public key encrypts to.
		var recipient age.Recipient
		if found.Line > 0 {
			recipient, err = age.ParseX25519Recipient(found.Public.Key)
		} else {
			recipient, err = agessh.ParseRecipient(found.Public.Key)
		}
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		w, err := age.Encrypt(&out, recipient)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte("x"))
		w.Close()
		if _, err := age.Decrypt(strings.NewReader(out.String()), opened); err != nil {
			t.Errorf("%s:%d does not decrypt for its own key: %v", found.Path, found.Line, err)
		}
	}
	if _, err := Open(Identity{Status: Protected}); err == nil {
		t.Error("opened a protected identity")
	}
}
