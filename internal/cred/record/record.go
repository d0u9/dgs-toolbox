// Package record reads and writes the recipient record kept beside an age file
// in the vault: which public keys it was encrypted to, since an age header does
// not say. The format is in docs/apps/cred/vault.md.
package record

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Suffix is appended to the age file's name.
const Suffix = ".json"

// Version is the format this package writes.
const Version = 1

// ArchiveTarGz marks a file holding a gzip-compressed tar of a folder.
const ArchiveTarGz = "tar.gz"

// Record is the content of a record file.
type Record struct {
	Version    int         `json:"version"`
	Created    time.Time   `json:"created"`
	Archive    string      `json:"archive,omitempty"`
	Recipients []Recipient `json:"recipients"`
}

// Recipient is one public key a file was encrypted to, with the names the
// recipient folder gave it at the time.
type Recipient struct {
	PublicKey   string `json:"public_key"`
	Host        string `json:"host,omitempty"`
	Description string `json:"description,omitempty"`
}

// PathFor is the record path of an age file.
func PathFor(ageFile string) string { return ageFile + Suffix }

// Read loads a record. A record of a newer version, or with unknown fields, is
// refused rather than half understood.
func Read(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var r Record
	if err := decoder.Decode(&r); err != nil {
		return Record{}, fmt.Errorf("record %s: %w", path, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Record{}, fmt.Errorf("record %s: content after the object", path)
	}
	if r.Version != Version {
		return Record{}, fmt.Errorf("record %s: version %d, want %d", path, r.Version, Version)
	}
	return r, nil
}

// Create writes a record that must not already exist. It is written to a
// temporary name beside it and linked into place, so a reader never sees half
// a record and an existing one is never replaced.
func Create(path string, r Record) error {
	r.Version = Version
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.dgs-part")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tempPath, 0o644); err != nil {
		return err
	}
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("record %s already exists", path)
		}
		return err
	}
	return nil
}

// Usage is which files in a vault were encrypted to a key, by their records.
type Usage struct {
	// Files are the age files whose record lists the key, relative to the
	// vault; Only are those listing no other key.
	Files []string
	Only  []string
	// Unreadable are records that could not be read, so the answer may be
	// incomplete.
	Unreadable []string
}

// Using reads every record under vault, skipping hidden entries as the vault
// listing does, and reports the files encrypted to publicKey. Files without a
// record are not known about.
func Using(vault, publicKey string) (Usage, error) {
	return UsingAny(vault, []string{publicKey})
}

// UsingAny is Using for a set of keys, such as every key of a host: Files list
// any of them, and Only list no key outside the set.
func UsingAny(vault string, publicKeys []string) (Usage, error) {
	set := map[string]bool{}
	for _, key := range publicKeys {
		set[key] = true
	}
	var usage Usage
	err := filepath.WalkDir(vault, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path != vault && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".age"+Suffix) {
			return nil
		}
		rel, _ := filepath.Rel(vault, strings.TrimSuffix(path, Suffix))
		rel = filepath.ToSlash(rel)
		r, err := Read(path)
		if err != nil {
			usage.Unreadable = append(usage.Unreadable, rel)
			return nil
		}
		matched, outside := false, false
		for _, recipient := range r.Recipients {
			if set[recipient.PublicKey] {
				matched = true
			} else {
				outside = true
			}
		}
		if matched {
			usage.Files = append(usage.Files, rel)
			if !outside {
				usage.Only = append(usage.Only, rel)
			}
		}
		return nil
	})
	sort.Strings(usage.Files)
	sort.Strings(usage.Only)
	return usage, err
}
