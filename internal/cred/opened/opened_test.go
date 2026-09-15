package opened

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/seal"
)

func encrypt(t *testing.T, plaintext []byte, armored bool, recipient age.Recipient) []byte {
	t.Helper()
	var out bytes.Buffer
	var dst io.Writer = &out
	var aw io.WriteCloser
	if armored {
		aw = armor.NewWriter(&out)
		dst = aw
	}
	w, err := age.Encrypt(dst, recipient)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(plaintext)
	w.Close()
	if aw != nil {
		aw.Close()
	}
	return out.Bytes()
}

func writeAge(t *testing.T, name string, plaintext []byte, recipient age.Recipient) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, encrypt(t, plaintext, false, recipient), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func entry(t *testing.T, o *Opened, path string) Entry {
	t.Helper()
	for _, e := range o.Entries {
		if e.Path == path {
			return e
		}
	}
	var paths []string
	for _, e := range o.Entries {
		paths = append(paths, e.Path)
	}
	t.Fatalf("no entry %s in %v", path, paths)
	return Entry{}
}

func TestOpenSingleFile(t *testing.T) {
	identity, _ := age.GenerateX25519Identity()
	path := writeAge(t, "notes.txt.age", []byte("hello\n"), identity.Recipient())
	o, err := Open(path, []age.Identity{identity}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if o.Name != "notes.txt" || o.Archive != ArchiveNone || len(o.Entries) != 1 {
		t.Fatalf("opened %+v", o)
	}
	e := o.Entries[0]
	if e.Path != "notes.txt" || e.Kind != KindText || string(e.Data) != "hello\n" || e.Size != 6 {
		t.Errorf("entry %+v", e)
	}
	data := e.Data
	o.Close()
	if !bytes.Equal(data, make([]byte, 6)) || o.Entries != nil {
		t.Errorf("close left %q", data)
	}
}

func TestOpenArmoredAndWrongIdentity(t *testing.T) {
	identity, _ := age.GenerateX25519Identity()
	other, _ := age.GenerateX25519Identity()
	data := encrypt(t, []byte("x"), true, identity.Recipient())
	if plain, err := Decrypt(bytes.NewReader(data), []age.Identity{identity}, 0); err != nil || string(plain) != "x" {
		t.Errorf("armored: %q %v", plain, err)
	}
	var noMatch *age.NoIdentityMatchError
	if _, err := Decrypt(bytes.NewReader(data), []age.Identity{other}, 0); !errors.As(err, &noMatch) {
		t.Errorf("wrong identity: %v", err)
	}
	if _, err := Decrypt(bytes.NewReader(encrypt(t, make([]byte, 100), false, identity.Recipient())), []age.Identity{identity}, 99); !errors.Is(err, ErrTooLarge) {
		t.Errorf("too large: %v", err)
	}
}

func TestOpenSealedFolder(t *testing.T) {
	identity, _ := age.GenerateX25519Identity()
	folder := filepath.Join(t.TempDir(), "nas-keys")
	_, private, _ := ed25519.GenerateKey(rand.Reader)
	block, _ := ssh.MarshalPrivateKey(private, "")
	signer, _ := ssh.NewSignerFromKey(private)
	files := map[string][]byte{
		"id_ed25519":     pem.EncodeToMemory(block),
		"id_ed25519.pub": ssh.MarshalAuthorizedKey(signer.PublicKey()),
		".ssh/config":    []byte("Host nas\n"),
		"logo.bin":       {0, 1, 2, 3},
	}
	for name, content := range files {
		full := filepath.Join(folder, name)
		os.MkdirAll(filepath.Dir(full), 0o755)
		mode := os.FileMode(0o644)
		if name == "id_ed25519" {
			mode = 0o600
		}
		os.WriteFile(full, content, mode)
		os.Chmod(full, mode)
	}
	archive, err := seal.Archive(folder, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	path := writeAge(t, "nas-keys.tar.gz.age", archive, identity.Recipient())
	o, err := Open(path, []age.Identity{identity}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer o.Close()
	if o.Name != "nas-keys.tar.gz" || o.Archive != ArchiveTarGz {
		t.Errorf("opened %s %s", o.Name, o.Archive)
	}
	for name, kind := range map[string]Kind{
		"nas-keys":                KindDirectory,
		"nas-keys/.ssh":           KindDirectory,
		"nas-keys/.ssh/config":    KindText,
		"nas-keys/id_ed25519":     KindSSHPrivate,
		"nas-keys/id_ed25519.pub": KindSSHPublic,
		"nas-keys/logo.bin":       KindOther,
	} {
		if got := entry(t, o, name).Kind; got != kind {
			t.Errorf("%s: %s, want %s", name, got, kind)
		}
	}
	if mode := entry(t, o, "nas-keys/id_ed25519").Mode; mode != 0o600 {
		t.Errorf("mode %v", mode)
	}
}

func TestUnsafeEntries(t *testing.T) {
	var buffer bytes.Buffer
	tw := tar.NewWriter(&buffer)
	add := func(header *tar.Header, body string) {
		header.Size = int64(len(body))
		if header.Mode == 0 {
			header.Mode = 0o644
		}
		tw.WriteHeader(header)
		tw.Write([]byte(body))
	}
	add(&tar.Header{Typeflag: tar.TypeReg, Name: "ok/file"}, "fine")
	add(&tar.Header{Typeflag: tar.TypeReg, Name: "../escape"}, "bad")
	add(&tar.Header{Typeflag: tar.TypeReg, Name: "/etc/passwd"}, "bad")
	add(&tar.Header{Typeflag: tar.TypeSymlink, Name: "ok/link", Linkname: "/etc/shadow"}, "")
	add(&tar.Header{Typeflag: tar.TypeChar, Name: "ok/dev"}, "")
	tw.Close()

	o, err := Expand("x.tar", buffer.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if o.Archive != ArchiveTar {
		t.Errorf("archive %q", o.Archive)
	}
	if e := entry(t, o, "ok/file"); e.Kind != KindText || string(e.Data) != "fine" {
		t.Errorf("ok/file %+v", e)
	}
	for name, why := range map[string]string{"../escape": "climbs", "/etc/passwd": "absolute", "ok/link": "link to /etc/shadow", "ok/dev": "not a regular file"} {
		e := entry(t, o, name)
		if e.Kind != KindUnsafe || e.Data != nil || !strings.Contains(e.Unsafe, why) {
			t.Errorf("%s: %+v", name, e)
		}
	}
	entry(t, o, "ok")
	for _, e := range o.Entries {
		if e.Path == ".." || e.Path == "/etc" || e.Path == "/" {
			t.Errorf("unsafe name implied directory %q", e.Path)
		}
	}
}

func TestZip(t *testing.T) {
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	w, _ := zw.Create("docs/readme.md")
	w.Write([]byte("# hi"))
	zw.Create("../evil")
	header := &zip.FileHeader{Name: "docs/link"}
	header.SetMode(fs.ModeSymlink | 0o777)
	w, _ = zw.CreateHeader(header)
	w.Write([]byte("/etc"))
	zw.Close()

	o, err := Expand("bundle.zip", buffer.Bytes(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if o.Archive != ArchiveZip || entry(t, o, "docs/readme.md").Kind != KindText || entry(t, o, "docs").Kind != KindDirectory {
		t.Errorf("zip %+v", o)
	}
	if entry(t, o, "../evil").Kind != KindUnsafe || entry(t, o, "docs/link").Kind != KindUnsafe {
		t.Error("unsafe zip entries extracted")
	}
}

func TestArchiveBomb(t *testing.T) {
	var buffer bytes.Buffer
	gz, _ := gzipWriter(&buffer)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "big", Size: 10000, Mode: 0o644})
	tw.Write(make([]byte, 10000))
	tw.Close()
	gz.Close()
	if buffer.Len() > 1000 {
		t.Fatalf("archive did not compress: %d", buffer.Len())
	}
	if _, err := Expand("bomb.tar.gz", buffer.Bytes(), 5000); !errors.Is(err, ErrTooLarge) {
		t.Errorf("bomb: %v", err)
	}
}

func TestClassify(t *testing.T) {
	identity, _ := age.GenerateX25519Identity()
	for name, test := range map[string]struct {
		data string
		kind Kind
	}{
		"age identity": {identity.String() + "\n", KindAgeKey},
		"mention":      {"use AGE-SECRET-KEY-1 lines\n", KindText},
		"empty":        {"", KindText},
		"binary":       {"a\x00b", KindOther},
		"invalid utf8": {"\xff\xfe", KindOther},
	} {
		if got := Classify([]byte(test.data)); got != test.kind {
			t.Errorf("%s: %s, want %s", name, got, test.kind)
		}
	}
	if !KindSSHPrivate.Sensitive() || !KindAgeKey.Sensitive() || KindText.Sensitive() {
		t.Error("sensitive kinds")
	}
}

func gzipWriter(w io.Writer) (io.WriteCloser, error) { return gzip.NewWriter(w), nil }
