package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CredentialsFilename is dgs cred's own settings file. It is kept apart from
// dgs-config.json: the identity directories are this machine's, and the file
// sits beside the main one rather than inside it.
const CredentialsFilename = "credentials.json"

// Credentials are dgs cred's settings, with every path expanded.
type Credentials struct {
	// Identities are directories searched for private keys.
	Identities []string `json:"identities"`
	// Recipients is the recipient folder; empty means none.
	Recipients string `json:"recipients"`
	// Vault is the folder of age files the vault page opens at; empty means
	// none.
	Vault string `json:"vault"`
}

// CredentialsPath is <XDG config home>/dgs-toolbox/credentials.json.
func CredentialsPath() (string, error) {
	path, err := DefaultPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), CredentialsFilename), nil
}

// LoadCredentials reads the file at path. A missing file is not an error:
// found is false and the settings are empty, so the command can say where the
// file is looked for.
func LoadCredentials(path string) (credentials Credentials, found bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Credentials{}, false, nil
	}
	if err != nil {
		return Credentials{}, false, fmt.Errorf("open %s: %w", path, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&credentials); err != nil {
		return Credentials{}, true, fmt.Errorf("decode %s: %w", path, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Credentials{}, true, fmt.Errorf("decode %s: content after the object", path)
	}
	home, _ := os.UserHomeDir()
	for i, dir := range credentials.Identities {
		expanded, err := ExpandPath(dir, os.LookupEnv, home)
		if err != nil {
			return Credentials{}, true, fmt.Errorf("decode %s: identities[%d]: %w", path, i, err)
		}
		credentials.Identities[i] = expanded
	}
	if credentials.Recipients != "" {
		expanded, err := ExpandPath(credentials.Recipients, os.LookupEnv, home)
		if err != nil {
			return Credentials{}, true, fmt.Errorf("decode %s: recipients: %w", path, err)
		}
		credentials.Recipients = expanded
	}
	if credentials.Vault != "" {
		expanded, err := ExpandPath(credentials.Vault, os.LookupEnv, home)
		if err != nil {
			return Credentials{}, true, fmt.Errorf("decode %s: vault: %w", path, err)
		}
		credentials.Vault = expanded
	}
	return credentials, true, nil
}

// ExpandPath expands a leading ~ to home and $NAME or ${NAME} to the
// environment variable, and requires the result to be absolute. A variable
// that is unset or empty is an error naming it: expanding it to nothing would
// turn $DOT_CONF_DIR/recipients into /recipients without a word.
func ExpandPath(path string, lookup func(string) (string, bool), home string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home == "" {
			return "", errors.New("no home directory to expand ~")
		}
		path = home + path[1:]
	}
	var out strings.Builder
	for i := 0; i < len(path); i++ {
		if path[i] != '$' {
			out.WriteByte(path[i])
			continue
		}
		var name string
		if i+1 < len(path) && path[i+1] == '{' {
			end := strings.IndexByte(path[i+2:], '}')
			if end < 0 {
				return "", fmt.Errorf("%q: unclosed ${", path)
			}
			name = path[i+2 : i+2+end]
			i += end + 2
		} else {
			j := i + 1
			for j < len(path) && isNameByte(path[j], j == i+1) {
				j++
			}
			name = path[i+1 : j]
			i = j - 1
		}
		if name == "" || !validName(name) {
			return "", fmt.Errorf("%q: $ must be followed by a variable name", path)
		}
		value, ok := lookup(name)
		if !ok || value == "" {
			return "", fmt.Errorf("%q: environment variable %s is not set", path, name)
		}
		out.WriteString(value)
	}
	expanded := filepath.Clean(out.String())
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("%q is not an absolute path; start it with /, ~ or a variable", path)
	}
	return expanded, nil
}

func isNameByte(c byte, first bool) bool {
	return c == '_' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || !first && c >= '0' && c <= '9'
}

func validName(name string) bool {
	for i := 0; i < len(name); i++ {
		if !isNameByte(name[i], i == 0) {
			return false
		}
	}
	return true
}
