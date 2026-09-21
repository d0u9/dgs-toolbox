package recipients

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckHostName reports why name cannot be a host name, or nil.
func CheckHostName(name string) error {
	switch {
	case !namePattern.MatchString(name):
		return fmt.Errorf("host name %q may use only letters, digits, -, _ and ., and not start with .", name)
	case strings.HasPrefix(strings.ToLower(name), GroupPrefix):
		return fmt.Errorf("host name %q starts with %s, which is kept for groups", name, GroupPrefix)
	}
	return nil
}

// AddKey adds a public key to a host in the recipient folder at root, creating
// the host file when there is none. A host whose name matches ignoring case is
// the same host. It refuses a host file with an error, since rewriting it would
// lose what could not be read, and a key already listed under any host. It
// returns the host file, relative to root.
func AddKey(root, host, publicKey string, meta Meta) (string, error) {
	if err := CheckHostName(host); err != nil {
		return "", err
	}
	meta.Description = strings.TrimSpace(meta.Description)
	if meta.Description == "" {
		return "", errors.New("description is required")
	}
	if err := checkMeta(meta); err != nil {
		return "", err
	}
	key, err := ParsePublicKey(publicKey)
	if err != nil {
		return "", err
	}
	if meta.Comment == "" {
		meta.Comment = sshComment(publicKey)
	}
	folder, err := Load(root)
	if err != nil {
		return "", err
	}
	for _, existing := range folder.Hosts {
		for _, listed := range existing.Keys {
			if listed.Key == key.Key {
				return "", fmt.Errorf("the key is already listed under %s", existing.Name)
			}
		}
	}

	name := host
	comment := ""
	var entries []keyFile
	if existing, ok := folder.Host(host); ok {
		name, comment = existing.Name, existing.Comment
		for _, listed := range existing.Keys {
			entries = append(entries, keyFile{PublicKey: listed.Key, Meta: listed.Meta})
		}
	} else {
		// A host that did not load may still have a file: one with an error.
		for _, problem := range folder.Problems {
			base := strings.TrimSuffix(filepath.Base(problem.File), FileSuffix)
			if problem.Severity == Error && filepath.Dir(problem.File) == HostsDir && strings.EqualFold(base, host) {
				return "", fmt.Errorf("%s has an error; fix it before adding a key: %s", problem.File, problem.Message)
			}
		}
	}
	// Keys are written in canonical form; an SSH key's comment is kept in its
	// comment field instead.
	entries = append(entries, keyFile{PublicKey: key.Key, Meta: meta})

	if err := writeHost(root, name, comment, entries); err != nil {
		return "", err
	}
	return filepath.Join(HostsDir, name+FileSuffix), nil
}

// sshComment is what follows the key on an SSH public key line.
func sshComment(line string) string {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 3 || strings.HasPrefix(fields[0], "age1") {
		return ""
	}
	return strings.Join(fields[2:], " ")
}

// RemoveKey takes a public key out of every host listing it, keeping each host
// file even when it is left with no keys. It returns the host files changed,
// relative to root; none is not an error.
func RemoveKey(root, publicKey string) ([]string, error) {
	key, err := ParsePublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	folder, err := Load(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, host := range folder.Hosts {
		var entries []keyFile
		found := false
		for _, listed := range host.Keys {
			if listed.Key == key.Key {
				found = true
				continue
			}
			entries = append(entries, keyFile{PublicKey: listed.Key, Meta: listed.Meta})
		}
		if !found {
			continue
		}
		if err := writeHost(root, host.Name, host.Comment, entries); err != nil {
			return changed, err
		}
		changed = append(changed, host.File)
	}
	return changed, nil
}

// writeHost replaces a host file through a temporary file renamed into place.
func writeHost(root, name, comment string, entries []keyFile) error {
	if entries == nil {
		entries = []keyFile{}
	}
	return writeJSON(filepath.Join(root, HostsDir), name, hostFile{Comment: comment, Keys: entries})
}

// SetComment writes a host's comment, the owner's note about the machine. An
// empty comment removes the field. It refuses a host that did not load, since
// rewriting a file with an error would lose what could not be read. It returns
// the host file, relative to root.
func SetComment(root, host, comment string) (string, error) {
	folder, err := Load(root)
	if err != nil {
		return "", err
	}
	existing, ok := folder.Host(host)
	if !ok {
		return "", fmt.Errorf("%s is not a loaded host", host)
	}
	comment = strings.TrimSpace(comment)
	if comment == existing.Comment {
		return "", errors.New("the comment is unchanged")
	}
	entries := make([]keyFile, len(existing.Keys))
	for i, listed := range existing.Keys {
		entries[i] = keyFile{PublicKey: listed.Key, Meta: listed.Meta}
	}
	if err := writeHost(root, existing.Name, comment, entries); err != nil {
		return "", err
	}
	return existing.File, nil
}

// writeJSON writes dir/name.json through a temporary file renamed into place.
func writeJSON(dir, name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, "."+name+".*.dgs-part")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(data); err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(temp.Name(), 0o644)
	}
	if err == nil {
		err = os.Rename(temp.Name(), filepath.Join(dir, name+FileSuffix))
	}
	return err
}

// RemoveFromGroups takes a host out of every group naming it, ignoring case. It
// rewrites each group from its file rather than from the loaded group, so
// members that did not resolve are kept. It returns the group files changed.
func RemoveFromGroups(root, host string) ([]string, error) {
	folder, err := Load(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, group := range folder.Groups {
		data, err := os.ReadFile(filepath.Join(root, group.File))
		if err != nil {
			return changed, err
		}
		var file groupFile
		if err := json.Unmarshal(data, &file); err != nil {
			return changed, fmt.Errorf("%s: %w", group.File, err)
		}
		kept := []string{}
		for _, member := range file.Hosts {
			if !strings.EqualFold(member, host) {
				kept = append(kept, member)
			}
		}
		if len(kept) == len(file.Hosts) {
			continue
		}
		if err := writeJSON(filepath.Join(root, GroupsDir), group.Name, groupFile{Hosts: kept}); err != nil {
			return changed, err
		}
		changed = append(changed, group.File)
	}
	return changed, nil
}

// RenameHost gives a host a new name: its file becomes <new>.json and every
// group naming it names the new name. The keys and their fields are untouched,
// so the file is renamed rather than rewritten. It refuses a name already taken
// by another host, including one whose file has an error, since the two would
// then be the same name. Changing only the case of a name is allowed. It
// returns the new host file and the group files changed, both relative to root.
func RenameHost(root, oldName, newName string) (string, []string, error) {
	if err := CheckHostName(newName); err != nil {
		return "", nil, err
	}
	folder, err := Load(root)
	if err != nil {
		return "", nil, err
	}
	host, ok := folder.Host(oldName)
	if !ok {
		return "", nil, fmt.Errorf("%s is not a loaded host", oldName)
	}
	if host.Name == newName {
		return "", nil, fmt.Errorf("%s is already its name", newName)
	}
	sameName := strings.EqualFold(host.Name, newName)
	if !sameName {
		if existing, ok := folder.Host(newName); ok {
			return "", nil, fmt.Errorf("%s is already a host, in %s", existing.Name, existing.File)
		}
		for _, problem := range folder.Problems {
			base := strings.TrimSuffix(filepath.Base(problem.File), FileSuffix)
			if problem.Severity == Error && filepath.Dir(problem.File) == HostsDir && strings.EqualFold(base, newName) {
				return "", nil, fmt.Errorf("%s already exists, with an error: %s", problem.File, problem.Message)
			}
		}
	}
	from := filepath.Join(root, host.File)
	to := filepath.Join(root, HostsDir, newName+FileSuffix)
	// A name that differs only in case is the same file on a case-insensitive
	// filesystem, so the check is skipped there rather than refusing itself.
	if !sameName {
		if _, err := os.Lstat(to); err == nil {
			return "", nil, fmt.Errorf("%s already exists", filepath.Join(HostsDir, newName+FileSuffix))
		}
	}
	if err := os.Rename(from, to); err != nil {
		return "", nil, err
	}
	newFile := filepath.Join(HostsDir, newName+FileSuffix)
	changed, err := renameInGroups(root, folder, oldName, newName)
	return newFile, changed, err
}

// renameInGroups renames a member in every group naming it, ignoring case. As
// in RemoveFromGroups, each group is rewritten from its file, so members that
// did not resolve are kept.
func renameInGroups(root string, folder Folder, oldName, newName string) ([]string, error) {
	var changed []string
	for _, group := range folder.Groups {
		data, err := os.ReadFile(filepath.Join(root, group.File))
		if err != nil {
			return changed, err
		}
		var file groupFile
		if err := json.Unmarshal(data, &file); err != nil {
			return changed, fmt.Errorf("%s: %w", group.File, err)
		}
		found := false
		members := make([]string, len(file.Hosts))
		for i, member := range file.Hosts {
			if strings.EqualFold(member, oldName) {
				member, found = newName, true
			}
			members[i] = member
		}
		if !found {
			continue
		}
		if err := writeJSON(filepath.Join(root, GroupsDir), group.Name, groupFile{Hosts: members}); err != nil {
			return changed, err
		}
		changed = append(changed, group.File)
	}
	return changed, nil
}
