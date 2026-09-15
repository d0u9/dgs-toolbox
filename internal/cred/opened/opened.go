// Package opened decrypts a vault file into memory and says what is inside:
// a single file, or the entries of a tar, gzip-compressed tar or zip archive,
// each recognised by content. It writes nothing to disk. The rules are in
// docs/apps/cred/vault.md.
package opened

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"filippo.io/age"
	"filippo.io/age/armor"
	"golang.org/x/crypto/ssh"

	"dgs-toolbox/internal/cred/identities"
)

// DefaultMaxSize bounds the plaintext of a file and the expanded size of an
// archive, the same limit a file can be added with.
const DefaultMaxSize = 64 << 20

// Kind is what an entry is recognised as.
type Kind string

const (
	KindDirectory  Kind = "directory"
	KindSSHPrivate Kind = "SSH private key"
	KindSSHPublic  Kind = "SSH public key"
	KindAgeKey     Kind = "age identity"
	KindText       Kind = "text"
	KindOther      Kind = "other"
	// KindUnsafe is an archive entry that is not extracted: an absolute or
	// climbing name, a link or a device.
	KindUnsafe Kind = "unsafe"
)

// Sensitive reports whether an entry's content is hidden until asked for.
func (k Kind) Sensitive() bool { return k == KindSSHPrivate || k == KindAgeKey }

// Archive formats recognised.
const (
	ArchiveNone  = ""
	ArchiveTarGz = "tar.gz"
	ArchiveTar   = "tar"
	ArchiveZip   = "zip"
)

// Entry is one file or directory inside an opened file.
type Entry struct {
	// Path uses forward slashes; for a single file it is the file's name.
	Path string
	Kind Kind
	Mode fs.FileMode
	Size int64
	// Data is the content of a file entry; nil for directories and unsafe
	// entries.
	Data   []byte
	SHA256 [32]byte
	// Unsafe explains a KindUnsafe entry.
	Unsafe string
}

// Opened is a decrypted file held in memory.
type Opened struct {
	// Name is the file's name without .age.
	Name    string
	Archive string
	Entries []Entry
	// Size is the plaintext size before expansion.
	Size int64
}

// Close overwrites every entry's content with zeros and drops it. That is as far
// as a Go program can go: copies the runtime made, or memory swapped to disk, are
// beyond it.
func (o *Opened) Close() {
	if o == nil {
		return
	}
	for i := range o.Entries {
		clear(o.Entries[i].Data)
		o.Entries[i].Data = nil
	}
	o.Entries = nil
}

// ErrTooLarge is returned when plaintext or an archive's content exceeds the
// limit.
var ErrTooLarge = errors.New("larger than the limit a vault file can be opened with")

// Open decrypts the file at path with identities and expands it.
func Open(path string, ids []age.Identity, maxSize int64) (*Opened, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	plaintext, err := Decrypt(file, ids, maxSize)
	if err != nil {
		return nil, err
	}
	defer clear(plaintext)
	return Expand(strings.TrimSuffix(filepath.Base(path), ".age"), plaintext, maxSize)
}

// Decrypt reads an age file, binary or armored, into memory.
func Decrypt(src io.Reader, ids []age.Identity, maxSize int64) ([]byte, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	buffered := bufio.NewReader(src)
	var reader io.Reader = buffered
	if peek, _ := buffered.Peek(len("-----BEGIN AGE ENCRYPTED FILE-----")); bytes.Equal(peek, []byte("-----BEGIN AGE ENCRYPTED FILE-----")) {
		reader = armor.NewReader(buffered)
	}
	decrypted, err := age.Decrypt(reader, ids...)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(decrypted, maxSize+1))
	if err != nil {
		clear(data)
		return nil, err
	}
	if int64(len(data)) > maxSize {
		clear(data)
		return nil, ErrTooLarge
	}
	return data, nil
}

// Expand recognises plaintext as an archive or a single file. The entries own
// copies of their content, so the caller may clear plaintext afterwards.
func Expand(name string, plaintext []byte, maxSize int64) (*Opened, error) {
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	o := &Opened{Name: name, Size: int64(len(plaintext))}
	var err error
	switch {
	case bytes.HasPrefix(plaintext, []byte{0x1f, 0x8b}):
		var unpacked []byte
		unpacked, err = gunzip(plaintext, maxSize)
		if err == nil && isTar(unpacked) {
			o.Archive = ArchiveTarGz
			o.Entries, err = readTar(unpacked, maxSize)
		} else if err == nil {
			o.Entries = []Entry{fileEntry(name, 0o600, plaintext)}
		}
		clear(unpacked)
	case isTar(plaintext):
		o.Archive = ArchiveTar
		o.Entries, err = readTar(plaintext, maxSize)
	case bytes.HasPrefix(plaintext, []byte("PK\x03\x04")) || bytes.HasPrefix(plaintext, []byte("PK\x05\x06")):
		o.Archive = ArchiveZip
		o.Entries, err = readZip(plaintext, maxSize)
	default:
		o.Entries = []Entry{fileEntry(name, 0o600, plaintext)}
	}
	if err != nil {
		o.Close()
		return nil, err
	}
	if o.Archive != ArchiveNone {
		o.Entries = withParents(o.Entries)
	}
	return o, nil
}

func gunzip(data []byte, maxSize int64) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("gzip: %w", err)
	}
	out, err := io.ReadAll(io.LimitReader(reader, maxSize+1))
	if err != nil {
		clear(out)
		return nil, fmt.Errorf("gzip: %w", err)
	}
	if int64(len(out)) > maxSize {
		clear(out)
		return nil, ErrTooLarge
	}
	return out, nil
}

func isTar(data []byte) bool {
	return len(data) >= 262 && bytes.Equal(data[257:262], []byte("ustar"))
}

// unsafeName explains why an archive name is not extracted, or is empty.
func unsafeName(name string) string {
	slashed := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(name) || (len(slashed) > 1 && slashed[1] == ':') {
		return "absolute name"
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return "name climbs out with .."
		}
	}
	return ""
}

func cleanName(name string) string {
	return strings.TrimSuffix(path.Clean(strings.ReplaceAll(name, `\`, "/")), "/")
}

func readTar(data []byte, maxSize int64) ([]Entry, error) {
	reader := tar.NewReader(bytes.NewReader(data))
	var entries []Entry
	var total int64
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			closeEntries(entries)
			return nil, fmt.Errorf("tar: %w", err)
		}
		name := cleanName(header.Name)
		mode := fs.FileMode(header.Mode).Perm()
		if why := unsafeName(header.Name); why != "" {
			entries = append(entries, Entry{Path: name, Kind: KindUnsafe, Mode: mode, Unsafe: why})
			continue
		}
		switch header.Typeflag {
		case tar.TypeDir:
			entries = append(entries, Entry{Path: name, Kind: KindDirectory, Mode: mode | fs.ModeDir})
		case tar.TypeReg:
			total += header.Size
			if total > maxSize {
				closeEntries(entries)
				return nil, ErrTooLarge
			}
			content, err := io.ReadAll(io.LimitReader(reader, header.Size))
			if err != nil {
				clear(content)
				closeEntries(entries)
				return nil, fmt.Errorf("tar: %s: %w", name, err)
			}
			entries = append(entries, fileEntry(name, mode, content))
			clear(content)
		case tar.TypeSymlink, tar.TypeLink:
			entries = append(entries, Entry{Path: name, Kind: KindUnsafe, Mode: mode, Unsafe: "link to " + header.Linkname})
		default:
			entries = append(entries, Entry{Path: name, Kind: KindUnsafe, Mode: mode, Unsafe: fmt.Sprintf("not a regular file (type %q)", header.Typeflag)})
		}
	}
}

func readZip(data []byte, maxSize int64) ([]Entry, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("zip: %w", err)
	}
	var entries []Entry
	var total int64
	for _, file := range reader.File {
		name := cleanName(file.Name)
		mode := file.Mode()
		if why := unsafeName(file.Name); why != "" {
			entries = append(entries, Entry{Path: name, Kind: KindUnsafe, Mode: mode.Perm(), Unsafe: why})
			continue
		}
		switch {
		case mode.IsDir():
			entries = append(entries, Entry{Path: name, Kind: KindDirectory, Mode: mode})
			continue
		case !mode.IsRegular():
			entries = append(entries, Entry{Path: name, Kind: KindUnsafe, Mode: mode.Perm(), Unsafe: "not a regular file (" + mode.Type().String() + ")"})
			continue
		}
		// The declared size is only a claim; the limit is enforced on what is
		// actually read.
		if file.UncompressedSize64 > uint64(maxSize) || total+int64(file.UncompressedSize64) > maxSize {
			closeEntries(entries)
			return nil, ErrTooLarge
		}
		opened, err := file.Open()
		if err != nil {
			closeEntries(entries)
			return nil, fmt.Errorf("zip: %s: %w", name, err)
		}
		content, err := io.ReadAll(io.LimitReader(opened, maxSize-total+1))
		opened.Close()
		if err != nil {
			clear(content)
			closeEntries(entries)
			return nil, fmt.Errorf("zip: %s: %w", name, err)
		}
		total += int64(len(content))
		if total > maxSize {
			clear(content)
			closeEntries(entries)
			return nil, ErrTooLarge
		}
		perm := mode.Perm()
		if perm == 0 {
			perm = 0o644
		}
		entries = append(entries, fileEntry(name, perm, content))
		clear(content)
	}
	return entries, nil
}

func closeEntries(entries []Entry) {
	for i := range entries {
		clear(entries[i].Data)
	}
}

// fileEntry copies content into a new entry and recognises it.
func fileEntry(name string, mode fs.FileMode, content []byte) Entry {
	data := append([]byte(nil), content...)
	return Entry{Path: name, Kind: Classify(data), Mode: mode, Size: int64(len(data)), Data: data, SHA256: sha256.Sum256(data)}
}

// withParents adds the directories an archive's names imply but it does not
// list, and sorts entries by path.
func withParents(entries []Entry) []Entry {
	present := map[string]bool{}
	for _, entry := range entries {
		present[entry.Path] = true
	}
	for _, entry := range entries {
		// An unsafe name implies no directory worth listing, such as .. or /etc.
		if entry.Kind == KindUnsafe {
			continue
		}
		for dir := path.Dir(entry.Path); dir != "." && dir != "/" && !present[dir]; dir = path.Dir(dir) {
			present[dir] = true
			entries = append(entries, Entry{Path: dir, Kind: KindDirectory, Mode: fs.ModeDir | 0o755})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}

// Classify recognises content as a key, text or something else.
func Classify(data []byte) Kind {
	for _, identity := range identities.Parse(data) {
		if identity.Line == 0 {
			return KindSSHPrivate
		}
		// A note that merely mentions AGE-SECRET-KEY-1 is text, not a key.
		if identity.Status != identities.Invalid {
			return KindAgeKey
		}
	}
	if _, _, _, _, err := ssh.ParseAuthorizedKey(data); err == nil {
		return KindSSHPublic
	}
	if utf8.Valid(data) && !bytes.ContainsRune(data, 0) {
		return KindText
	}
	return KindOther
}
