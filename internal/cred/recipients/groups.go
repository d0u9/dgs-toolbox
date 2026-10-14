package recipients

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"dgs-toolbox/internal/cred/publish"
)

// GroupDraft retains the original members, including unresolved and repeated
// names, and the bytes used to detect edits made outside this form.
type GroupDraft struct {
	Name     string
	Hosts    []string
	original []byte
}

func CheckGroupName(name string) error {
	if !namePattern.MatchString(name) || !strings.HasPrefix(strings.ToLower(name), GroupPrefix) {
		return fmt.Errorf("group name must start with %s and use only letters, digits, -, _ and .", GroupPrefix)
	}
	return nil
}

// ReadGroup reads a valid group without filtering its members. Symlink files
// are refused for mutation; the recipient folder itself may be a symlink.
func ReadGroup(root, name string) (GroupDraft, error) {
	if err := CheckGroupName(name); err != nil {
		return GroupDraft{}, err
	}
	folder, err := Load(root)
	if err != nil {
		return GroupDraft{}, err
	}
	for _, group := range folder.Groups {
		if !strings.EqualFold(group.Name, name) {
			continue
		}
		path := filepath.Join(root, group.File)
		info, err := os.Lstat(path)
		if err != nil {
			return GroupDraft{}, err
		}
		if !info.Mode().IsRegular() {
			return GroupDraft{}, errors.New("group must be a regular file to edit, rename or delete")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return GroupDraft{}, err
		}
		var file groupFile
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&file); err != nil {
			return GroupDraft{}, err
		}
		if _, err := decoder.Token(); err != io.EOF {
			return GroupDraft{}, errors.New("content after the group object")
		}
		return GroupDraft{Name: group.Name, Hosts: file.Hosts, original: data}, nil
	}
	return GroupDraft{}, fmt.Errorf("%s is not a valid loaded group; fix its file and reload first", name)
}

// CheckGroupUnchanged is a preflight check, not a lock against concurrent
// writers between this check and publication, rename or trash.
// shortcut: preflight checks do not serialize writers; add a shared write protocol when concurrent editing is required.
func CheckGroupUnchanged(root string, draft GroupDraft) error {
	if err := CheckGroupName(draft.Name); err != nil {
		return err
	}
	if draft.original == nil {
		return errors.New("group was not read before editing")
	}
	current, err := ReadGroup(root, draft.Name)
	if err != nil {
		return fmt.Errorf("group changed or disappeared; cancel and reload: %w", err)
	}
	if current.Name != draft.Name || !bytes.Equal(current.original, draft.original) {
		return errors.New("group changed outside this form; cancel and reload before saving")
	}
	return nil
}

func groupNameAvailable(root, name, except string) error {
	if err := CheckGroupName(name); err != nil {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(root, GroupsDir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != except+FileSuffix && strings.EqualFold(entry.Name(), name+FileSuffix) {
			return fmt.Errorf("%s already exists; it will not be replaced", entry.Name())
		}
	}
	return nil
}

func checkGroupMembers(root string, hosts, original []string) error {
	folder, err := Load(root)
	if err != nil {
		return err
	}
	for _, name := range hosts {
		if _, ok := folder.Host(name); !ok && !slices.Contains(original, name) {
			return fmt.Errorf("host %q is unavailable; reload before adding it", name)
		}
	}
	return nil
}

// CreateGroup publishes a complete group without replacing an existing path.
func CreateGroup(root, name string, hosts []string) (string, error) {
	if err := groupNameAvailable(root, name, ""); err != nil {
		return "", err
	}
	if err := checkGroupMembers(root, hosts, nil); err != nil {
		return "", err
	}
	if hosts == nil {
		hosts = []string{}
	}
	data, err := json.MarshalIndent(groupFile{Hosts: hosts}, "", "  ")
	if err != nil {
		return "", err
	}
	file := filepath.Join(GroupsDir, name+FileSuffix)
	err = publish.Create(filepath.Join(root, file), append(data, '\n'), 0o644, 0o755)
	return file, err
}

// SaveGroup preserves unresolved names only when they were in the original.
func SaveGroup(root string, draft GroupDraft, hosts []string) (string, error) {
	if err := checkGroupMembers(root, hosts, draft.Hosts); err != nil {
		return "", err
	}
	if err := CheckGroupUnchanged(root, draft); err != nil {
		return "", err
	}
	if hosts == nil {
		hosts = []string{}
	}
	file := filepath.Join(GroupsDir, draft.Name+FileSuffix)
	return file, writeJSON(filepath.Join(root, GroupsDir), draft.Name, groupFile{Hosts: hosts})
}

// RenameGroup keeps the file bytes intact. A different name is linked first
// so an existing target is never overwritten. Case-only changes use rename.
func RenameGroup(root string, draft GroupDraft, name string) (string, error) {
	if name == draft.Name {
		return "", errors.New("that is already its name")
	}
	if err := groupNameAvailable(root, name, draft.Name); err != nil {
		return "", err
	}
	if err := CheckGroupUnchanged(root, draft); err != nil {
		return "", err
	}
	from := filepath.Join(root, GroupsDir, draft.Name+FileSuffix)
	file := filepath.Join(GroupsDir, name+FileSuffix)
	to := filepath.Join(root, file)
	if err := os.Link(from, to); err != nil {
		if strings.EqualFold(draft.Name, name) && errors.Is(err, os.ErrExist) {
			source, sourceErr := os.Lstat(from)
			target, targetErr := os.Lstat(to)
			if sourceErr == nil && targetErr == nil && os.SameFile(source, target) {
				return file, os.Rename(from, to)
			}
		}
		return "", err
	}
	data, err := os.ReadFile(from)
	if err == nil && !bytes.Equal(data, draft.original) {
		err = errors.New("group changed outside this form")
	}
	if err != nil {
		return file, fmt.Errorf("new name %s exists, but the old name was kept: %w", file, err)
	}
	if err := os.Remove(from); err != nil {
		return file, fmt.Errorf("new name %s exists, but removing the old name failed: %w", file, err)
	}
	return file, nil
}
