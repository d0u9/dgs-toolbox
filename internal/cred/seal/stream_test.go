package seal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"filippo.io/age/armor"
)

func streamHash(t *testing.T, r io.Reader) []byte {
	t.Helper()
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		t.Fatal(err)
	}
	return h.Sum(nil)
}

func TestStreamingBeyondVaultLimit(t *testing.T) {
	const size = DefaultMaxSize + 1
	root := t.TempDir()
	folder := filepath.Join(root, "folder")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(folder, "large.bin")
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("synthetic end marker"), size-20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	ids := []age.Identity{id}
	keys := []Recipient{{PublicKey: id.Recipient().String()}}
	original, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	want := streamHash(t, original)
	original.Close()
	for _, archived := range []bool{false, true} {
		name, src := "file", source
		if archived {
			name, src = "folder", folder
		}
		t.Run(name, func(t *testing.T) {
			dest := filepath.Join(root, name+".age")
			result, err := Encrypt(context.Background(), src, dest, keys, ids, nil)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Verified || result.Archived != archived {
				t.Fatalf("%+v", result)
			}
			restored := filepath.Join(root, name+".restored")
			if err := Decrypt(context.Background(), dest, restored, ids); err != nil {
				t.Fatal(err)
			}
			f, err := os.Open(restored)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			var reader io.Reader = f
			if archived {
				gz, err := gzip.NewReader(f)
				if err != nil {
					t.Fatal(err)
				}
				defer gz.Close()
				tr := tar.NewReader(gz)
				for {
					header, err := tr.Next()
					if err != nil {
						t.Fatal(err)
					}
					if header.Typeflag == tar.TypeReg {
						if header.Size != size {
							t.Fatalf("size %d", header.Size)
						}
						break
					}
				}
				reader = tr
			} else {
				info, err := f.Stat()
				if err != nil || info.Size() != size || info.Mode().Perm() != 0600 {
					t.Fatalf("info %v, error %v", info, err)
				}
			}
			if got := streamHash(t, reader); !bytes.Equal(got, want) {
				t.Fatal("large-file digest mismatch")
			}
		})
	}
	// The TUI path retains its original bound.
	if _, err := Seal(Request{Source: source, Destination: filepath.Join(root, "vault.age"), Recipients: keys, Verify: ids}); err == nil {
		t.Fatal("vault limit removed")
	}
	inside := filepath.Join(folder, "inside.age")
	if _, err := Encrypt(context.Background(), folder, inside, keys, ids, nil); err == nil {
		t.Fatal("output inside source accepted")
	}
	if _, err := os.Stat(inside + PartSuffix); !os.IsNotExist(err) {
		t.Fatal("part created inside source")
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(folder, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := Encrypt(context.Background(), folder, filepath.Join(alias, "inside.age"), keys, ids, nil); err == nil {
		t.Fatal("symlinked output inside source accepted")
	}
}

func TestStreamDecryptArmor(t *testing.T) {
	root := t.TempDir()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	var cipher bytes.Buffer
	arm := armor.NewWriter(&cipher)
	writer, err := age.Encrypt(arm, id.Recipient())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "synthetic plaintext"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := arm.Close(); err != nil {
		t.Fatal(err)
	}
	source, dest := filepath.Join(root, "input.age"), filepath.Join(root, "output")
	if err := os.WriteFile(source, cipher.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Decrypt(context.Background(), source, dest, []age.Identity{id}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || string(got) != "synthetic plaintext" {
		t.Fatalf("%q %v", got, err)
	}
}
