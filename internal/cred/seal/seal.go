// Package seal adds a file or folder to the vault: it reads the plaintext into
// memory, archives a folder, encrypts to chosen recipients, checks the result
// by decrypting it back, publishes it without replacing anything, and writes
// the recipient record. The rules are in docs/apps/cred/vault.md.
package seal

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
	"filippo.io/age/agessh"

	"dgs-toolbox/internal/cred/record"
)

// DefaultMaxSize is the largest plaintext sealed: a file larger than this could
// not be opened again, since opening holds the plaintext in memory.
const DefaultMaxSize = 64 << 20

// PartSuffix names the file being written before it is published.
const PartSuffix = ".dgs-part"

// ArchiveSuffix is added to a folder's name before .age.
const ArchiveSuffix = ".tar.gz"

// Recipient is a public key to encrypt to, with its names for the record.
type Recipient struct {
	PublicKey   string
	Host        string
	Description string
}

// Request is one file or folder to add.
type Request struct {
	// Source is the file or folder to encrypt. It is only read.
	Source string
	// Destination is the .age file to create.
	Destination string
	Recipients  []Recipient
	// Verify are this machine's identities, used to decrypt the result back.
	// With none, the result is published unverified.
	Verify []age.Identity
	// MaxSize bounds the plaintext; 0 means DefaultMaxSize.
	MaxSize int64
	// Now stamps the record; zero means time.Now.
	Now time.Time
}

// Result is what was published.
type Result struct {
	Path       string
	RecordPath string
	Archived   bool
	// PlaintextSize and SHA256 describe the plaintext sealed.
	PlaintextSize int64
	SHA256        [32]byte
	Verified      bool
}

// DestinationName is the default file name for a source: name.age for a file,
// name.tar.gz.age for a folder.
func DestinationName(source string, folder bool) string {
	name := filepath.Base(filepath.Clean(source))
	if folder {
		return name + ArchiveSuffix + ".age"
	}
	return name + ".age"
}

// Seal adds the source to the vault.
func Seal(request Request) (Result, error) {
	if request.MaxSize <= 0 {
		request.MaxSize = DefaultMaxSize
	}
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	if len(request.Recipients) == 0 {
		return Result{}, errors.New("no recipients")
	}
	result := Result{Path: request.Destination, RecordPath: record.PathFor(request.Destination)}
	for _, path := range []string{result.Path, result.RecordPath} {
		if _, err := os.Lstat(path); err == nil {
			return Result{}, fmt.Errorf("%s already exists", path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return Result{}, err
		}
	}
	recipients := make([]age.Recipient, 0, len(request.Recipients))
	for _, r := range request.Recipients {
		parsed, err := parseRecipient(r.PublicKey)
		if err != nil {
			return Result{}, fmt.Errorf("recipient %s: %w", r.PublicKey, err)
		}
		recipients = append(recipients, parsed)
	}

	info, err := os.Lstat(request.Source)
	if err != nil {
		return Result{}, err
	}
	var plaintext []byte
	switch {
	case info.IsDir():
		plaintext, err = Archive(request.Source, request.MaxSize)
		result.Archived = true
	case info.Mode().IsRegular():
		plaintext, err = readLimited(request.Source, request.MaxSize)
	default:
		err = fmt.Errorf("%s is not a regular file or a folder", request.Source)
	}
	if err != nil {
		return Result{}, err
	}
	defer clear(plaintext)
	result.PlaintextSize = int64(len(plaintext))
	result.SHA256 = sha256.Sum256(plaintext)

	part := request.Destination + PartSuffix
	if err := writePart(part, plaintext, recipients); err != nil {
		os.Remove(part)
		return Result{}, err
	}
	defer os.Remove(part)

	if len(request.Verify) > 0 {
		if err := verify(part, result.SHA256, request.Verify); err != nil {
			return Result{}, err
		}
		result.Verified = true
	}
	if err := os.Link(part, request.Destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Result{}, fmt.Errorf("%s appeared while it was being written", request.Destination)
		}
		return Result{}, err
	}
	syncDir(filepath.Dir(request.Destination))

	rec := record.Record{Created: request.Now, Recipients: make([]record.Recipient, len(request.Recipients))}
	if result.Archived {
		rec.Archive = record.ArchiveTarGz
	}
	for i, r := range request.Recipients {
		rec.Recipients[i] = record.Recipient{PublicKey: r.PublicKey, Host: r.Host, Description: r.Description}
	}
	if err := record.Create(result.RecordPath, rec); err != nil {
		return result, fmt.Errorf("%s was published, but its record was not written: %w", request.Destination, err)
	}
	return result, nil
}

func parseRecipient(key string) (age.Recipient, error) {
	if strings.HasPrefix(key, "age1") {
		return age.ParseX25519Recipient(key)
	}
	return agessh.ParseRecipient(key)
}

func readLimited(path string, maxSize int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxSize {
		clear(data)
		return nil, fmt.Errorf("%s is larger than %d MiB", path, maxSize>>20)
	}
	return data, nil
}

// Archive returns a gzip-compressed tar of the folder, under the folder's own
// name, with every entry including hidden ones and their permission bits. A
// symbolic link or other non-regular entry refuses the folder.
func Archive(folder string, maxSize int64) ([]byte, error) {
	folder = filepath.Clean(folder)
	base := filepath.Base(folder)
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	var total int64
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(filepath.Join(base, rel))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name + "/", Mode: int64(info.Mode().Perm()), ModTime: info.ModTime()})
		case info.Mode().IsRegular():
		default:
			return fmt.Errorf("%s is not a regular file or folder (%s); it is not archived", path, info.Mode().Type())
		}
		total += info.Size()
		if total > maxSize {
			return fmt.Errorf("%s is larger than %d MiB", folder, maxSize>>20)
		}
		if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: int64(info.Mode().Perm()), Size: info.Size(), ModTime: info.ModTime()}); err != nil {
			return err
		}
		data, err := readLimited(path, info.Size())
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		clear(data)
		return err
	})
	if err == nil {
		err = tw.Close()
	}
	if err == nil {
		err = gz.Close()
	}
	if err != nil {
		clear(buffer.Bytes())
		return nil, err
	}
	if int64(buffer.Len()) > maxSize {
		clear(buffer.Bytes())
		return nil, fmt.Errorf("%s archived is larger than %d MiB", folder, maxSize>>20)
	}
	return buffer.Bytes(), nil
}

func writePart(part string, plaintext []byte, recipients []age.Recipient) error {
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	encrypted, err := age.Encrypt(file, recipients...)
	if err == nil {
		_, err = encrypted.Write(plaintext)
	}
	if err == nil {
		err = encrypted.Close()
	}
	if err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

// verify decrypts the part file independently of what was written and compares
// the plaintext's hash.
func verify(part string, want [32]byte, identities []age.Identity) error {
	file, err := os.Open(part)
	if err != nil {
		return err
	}
	defer file.Close()
	decrypted, err := age.Decrypt(file, identities...)
	if err != nil {
		return fmt.Errorf("the written file does not decrypt with this machine's identities: %w", err)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, decrypted); err != nil {
		return fmt.Errorf("the written file does not decrypt: %w", err)
	}
	var got [32]byte
	copy(got[:], hash.Sum(nil))
	if got != want {
		return errors.New("the written file decrypts to different content")
	}
	return nil
}

func syncDir(dir string) {
	if handle, err := os.Open(dir); err == nil {
		handle.Sync()
		handle.Close()
	}
}
