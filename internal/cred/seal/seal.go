// Package seal adds a file or folder to the vault: it reads the plaintext into
// memory, archives a folder, encrypts to chosen recipients, checks the result
// by decrypting it back, publishes it without replacing anything, and writes
// the recipient record. The rules are in docs/apps/cred/vault.md.
package seal

import (
	"archive/tar"
	"bufio"
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
	"filippo.io/age/armor"

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
	// Passphrase encrypts with a passphrase instead of to Recipients; the two
	// cannot be combined. The result is always checked with the passphrase.
	Passphrase string
	// Verify are this machine's identities, used to decrypt the result back.
	// With none, the result is published unverified.
	Verify []age.Identity
	// MaxSize bounds the plaintext; 0 means DefaultMaxSize.
	MaxSize int64
	// Skip are file name patterns not archived when Source is a folder; nil
	// means DefaultSkip.
	Skip []string
	// Replace allows Destination and its record to exist: the checked result
	// takes their place instead of the add being refused. The record keeps its
	// created stamp and gains an updated one.
	Replace bool
	// Now stamps the record; zero means time.Now.
	Now time.Time
}

// Result is what was published.
type Result struct {
	Path       string
	RecordPath string
	Archived   bool
	// Skipped counts the entries Skip left out of the archive.
	Skipped int
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
	recipients, verifyWith, err := encryptTo(request.Recipients, request.Passphrase)
	if err != nil {
		return Result{}, err
	}
	if verifyWith != nil {
		request.Verify = verifyWith
	}
	result := Result{Path: request.Destination, RecordPath: record.PathFor(request.Destination)}
	for _, path := range []string{result.Path, result.RecordPath} {
		if _, err := os.Lstat(path); err == nil {
			if request.Replace {
				continue
			}
			return Result{}, fmt.Errorf("%s already exists", path)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return Result{}, err
		}
	}

	info, err := os.Lstat(request.Source)
	if err != nil {
		return Result{}, err
	}
	var plaintext []byte
	switch {
	case info.IsDir():
		plaintext, result.Skipped, err = Archive(request.Source, request.MaxSize, request.Skip)
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
	if request.Replace {
		if info, err := os.Stat(request.Destination); err == nil {
			os.Chmod(part, info.Mode().Perm())
		}
		if err := os.Rename(part, request.Destination); err != nil {
			return Result{}, err
		}
	} else if err := os.Link(part, request.Destination); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return Result{}, fmt.Errorf("%s appeared while it was being written", request.Destination)
		}
		return Result{}, err
	}
	syncDir(filepath.Dir(request.Destination))

	rec := record.Record{Created: request.Now}
	if result.Archived {
		rec.Archive = record.ArchiveTarGz
	}
	write := record.Create
	if request.Replace {
		write = record.Replace
		if old, err := record.Read(result.RecordPath); err == nil {
			rec.Created, rec.Comment = old.Created, old.Comment
			now := request.Now
			rec.Updated = &now
		}
	}
	setEncryption(&rec, request.Recipients, request.Passphrase)
	if err := write(result.RecordPath, rec); err != nil {
		return result, fmt.Errorf("%s was published, but its record was not written: %w", request.Destination, err)
	}
	return result, nil
}

// encryptTo parses the recipients, or makes the passphrase's recipient and the
// identity that checks it; age does not combine the two.
func encryptTo(keys []Recipient, passphrase string) ([]age.Recipient, []age.Identity, error) {
	switch {
	case passphrase != "" && len(keys) > 0:
		return nil, nil, errors.New("a passphrase cannot be combined with recipients")
	case passphrase != "":
		scrypt, err := age.NewScryptRecipient(passphrase)
		if err != nil {
			return nil, nil, err
		}
		identity, err := age.NewScryptIdentity(passphrase)
		if err != nil {
			return nil, nil, err
		}
		return []age.Recipient{scrypt}, []age.Identity{identity}, nil
	case len(keys) == 0:
		return nil, nil, errors.New("no recipients")
	}
	recipients := make([]age.Recipient, 0, len(keys))
	for _, r := range keys {
		parsed, err := parseRecipient(r.PublicKey)
		if err != nil {
			return nil, nil, fmt.Errorf("recipient %s: %w", r.PublicKey, err)
		}
		recipients = append(recipients, parsed)
	}
	return recipients, nil, nil
}

func setEncryption(rec *record.Record, keys []Recipient, passphrase string) {
	rec.Encryption = ""
	if passphrase != "" {
		rec.Encryption = record.EncryptionPassphrase
	}
	rec.Recipients = make([]record.Recipient, len(keys))
	for i, r := range keys {
		rec.Recipients[i] = record.Recipient{PublicKey: r.PublicKey, Host: r.Host, Description: r.Description}
	}
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

// DefaultSkip are the file name patterns Archive leaves out when a caller
// gives none: what an operating system, a file manager or git writes beside
// the files, which nobody means to put in a vault. A dotfile is not junk by
// itself — .ssh and .env are what a folder is added for — so only these names
// are skipped, and the caller can replace the list.
var DefaultSkip = []string{
	// macOS
	".DS_Store", "._*", ".AppleDouble", ".LSOverride", ".Spotlight-V100",
	".Trashes", ".fseventsd", ".TemporaryItems", ".DocumentRevisions-V100",
	".apdisk", "Icon\r",
	// Windows
	"Thumbs.db", "Thumbs.db:encryptable", "ehthumbs.db", "ehthumbs_vista.db",
	"desktop.ini", "$RECYCLE.BIN", "*.stackdump",
	// Linux desktops
	".directory", ".Trash-*",
	// git
	".git",
}

// skipped reports whether the entry name matches one of the patterns, case
// insensitively: Windows writes Thumbs.db and thumbs.db for the same thing.
// An invalid pattern matches nothing rather than failing the archive.
func skipped(name string, patterns []string) bool {
	lower := strings.ToLower(name)
	for _, pattern := range patterns {
		if ok, err := filepath.Match(strings.ToLower(pattern), lower); ok && err == nil {
			return true
		}
	}
	return false
}

// Count returns how many entries in the folder the patterns leave out, so a
// caller can say what will not be archived before archiving it. A skipped
// directory counts as one, and its contents are not walked.
func Count(folder string, patterns []string) (names []string, err error) {
	if patterns == nil {
		patterns = DefaultSkip
	}
	folder = filepath.Clean(folder)
	err = filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == folder || !skipped(entry.Name(), patterns) {
			return nil
		}
		rel, err := filepath.Rel(folder, path)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(rel))
		if entry.IsDir() {
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// Archive returns a gzip-compressed tar of the folder, under the folder's own
// name, with every entry including hidden ones and their permission bits, and
// the number of entries the patterns left out. A nil patterns means
// DefaultSkip; an empty one skips nothing. A symbolic link or other
// non-regular entry refuses the folder.
func Archive(folder string, maxSize int64, patterns []string) ([]byte, int, error) {
	if patterns == nil {
		patterns = DefaultSkip
	}
	folder = filepath.Clean(folder)
	base := filepath.Base(folder)
	var skips int
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	var total int64
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != folder && skipped(entry.Name(), patterns) {
			skips++
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
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
		return nil, 0, err
	}
	if int64(buffer.Len()) > maxSize {
		clear(buffer.Bytes())
		return nil, 0, fmt.Errorf("%s archived is larger than %d MiB", folder, maxSize>>20)
	}
	return buffer.Bytes(), skips, nil
}

func writePart(part string, plaintext []byte, recipients []age.Recipient) error {
	return writePartFormat(part, plaintext, recipients, false)
}

func writePartFormat(part string, plaintext []byte, recipients []age.Recipient, armored bool) error {
	file, err := os.OpenFile(part, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	var dst io.Writer = file
	var armorWriter io.WriteCloser
	if armored {
		armorWriter = armor.NewWriter(file)
		dst = armorWriter
	}
	encrypted, err := age.Encrypt(dst, recipients...)
	if err == nil {
		_, err = encrypted.Write(plaintext)
	}
	if err == nil {
		err = encrypted.Close()
	}
	if err == nil && armorWriter != nil {
		err = armorWriter.Close()
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
	decrypted, err := age.Decrypt(armoredOrNot(file), identities...)
	if err != nil {
		return fmt.Errorf("the written file does not decrypt back: %w", err)
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

const armorIntro = "-----BEGIN AGE ENCRYPTED FILE-----"

// armoredOrNot reads an age file in either format.
func armoredOrNot(src io.Reader) io.Reader {
	buffered := bufio.NewReader(src)
	if peek, _ := buffered.Peek(len(armorIntro)); bytes.Equal(peek, []byte(armorIntro)) {
		return armor.NewReader(buffered)
	}
	return buffered
}

// ResealRequest changes the recipients of a file already in the vault.
type ResealRequest struct {
	// Path is the age file, replaced once the new one has been checked.
	Path string
	// Open are identities that decrypt the file as it is.
	Open       []age.Identity
	Recipients []Recipient
	// Passphrase encrypts the new file with a passphrase instead of to
	// Recipients, and checks it with that passphrase.
	Passphrase string
	// Verify are this machine's identities, used to decrypt the new file back.
	// With none, it replaces the old one unverified.
	Verify  []age.Identity
	MaxSize int64
	Now     time.Time
}

// Reseal decrypts the file, encrypts it again to the new recipients, or with a
// new passphrase, with a new
// file key, checks the result by decrypting it back, and replaces the file and
// its record. The format, binary or armored, is kept. Nothing is kept of the old
// version: a copy of it is exactly what a removed recipient can still open.
func Reseal(request ResealRequest) (Result, error) {
	if request.MaxSize <= 0 {
		request.MaxSize = DefaultMaxSize
	}
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	recipients, verifyWith, err := encryptTo(request.Recipients, request.Passphrase)
	if err != nil {
		return Result{}, err
	}
	if verifyWith != nil {
		request.Verify = verifyWith
	}
	original, err := os.ReadFile(request.Path)
	if err != nil {
		return Result{}, err
	}
	armored := bytes.HasPrefix(original, []byte(armorIntro))
	decrypted, err := age.Decrypt(armoredOrNot(bytes.NewReader(original)), request.Open...)
	if err != nil {
		return Result{}, fmt.Errorf("the file does not decrypt: %w", err)
	}
	plaintext, err := io.ReadAll(io.LimitReader(decrypted, request.MaxSize+1))
	defer clear(plaintext)
	if err != nil {
		return Result{}, fmt.Errorf("the file does not decrypt: %w", err)
	}
	if int64(len(plaintext)) > request.MaxSize {
		return Result{}, fmt.Errorf("%s is larger than %d MiB", request.Path, request.MaxSize>>20)
	}

	result := Result{Path: request.Path, RecordPath: record.PathFor(request.Path), PlaintextSize: int64(len(plaintext)), SHA256: sha256.Sum256(plaintext)}
	part := request.Path + PartSuffix
	if err := writePartFormat(part, plaintext, recipients, armored); err != nil {
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
	if info, err := os.Stat(request.Path); err == nil {
		os.Chmod(part, info.Mode().Perm())
	}
	if err := os.Rename(part, request.Path); err != nil {
		return Result{}, err
	}
	syncDir(filepath.Dir(request.Path))

	rec, err := record.Read(result.RecordPath)
	if err != nil {
		rec = record.Record{Created: request.Now}
		if bytes.HasPrefix(plaintext, []byte{0x1f, 0x8b}) {
			rec.Archive = record.ArchiveTarGz
		}
	}
	now := request.Now
	rec.Updated = &now
	setEncryption(&rec, request.Recipients, request.Passphrase)
	if err := record.Replace(result.RecordPath, rec); err != nil {
		return result, fmt.Errorf("%s was re-encrypted, but its record was not updated: %w", request.Path, err)
	}
	return result, nil
}
