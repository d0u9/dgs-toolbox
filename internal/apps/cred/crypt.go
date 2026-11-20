package cred

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"dgs-toolbox/internal/config"
	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/cred/seal"
	"dgs-toolbox/internal/tui"
	"filippo.io/age"
)

func cryptAction(kind string) tui.Action {
	action := tui.Action{
		ID: map[string]string{"encrypt": "e", "decrypt": "d"}[kind], Aliases: []string{kind}, Usage: "<source> [-o <destination>]", Args: 1,
		Description: map[string]string{"encrypt": "Encrypt", "decrypt": "Decrypt"}[kind] + " a file outside the vault without replacing files.",
		Help:        map[string]string{"encrypt": encryptHelp, "decrypt": decryptHelp}[kind],
		Flags:       []tui.ActionFlag{{Name: "output", Shorthand: "o", Usage: "destination file (default: add/remove .age; must not exist)"}},
		RunWithConfig: func(_ io.Reader, out io.Writer, args []string, flags map[string]string, global config.Config) error {
			return runCrypt(out, global.CredentialsPath(), kind, args[0], flags)
		},
	}
	if kind == "encrypt" {
		action.Flags = append(action.Flags,
			tui.ActionFlag{Name: "host", Shorthand: "H", Repeatable: true, Usage: "recipient host; repeat -H for multiple hosts", Complete: completeHosts},
			tui.ActionFlag{Name: "group", Shorthand: "G", Repeatable: true, Usage: "recipient group (full g- name); repeat -G for multiple groups", Complete: completeGroups})
	}
	return action
}

func runCrypt(out io.Writer, configPath, kind, source string, flags map[string]string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runCryptContext(ctx, out, configPath, kind, source, flags)
}

func runCryptContext(ctx context.Context, out io.Writer, configPath, kind, source string, flags map[string]string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	var given []string
	if raw := flags[tui.GivenFlags]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &given); err != nil {
			return fmt.Errorf("invalid flag metadata: %w", err)
		}
	}
	for _, name := range given {
		switch name {
		case "output":
			if flags[name] == "" {
				return fmt.Errorf("--output (-o) requires a nonempty file path")
			}
		case "host", "group":
			if flags[name] == "[]" || flags[name] == "" {
				return fmt.Errorf("--%s requires a nonempty %s name", name, name)
			}
		}
	}
	destination, err := cryptPaths(kind, source, flags["output"])
	if err != nil {
		return err
	}
	settings, _, err := config.LoadCredentials(configPath)
	if err != nil {
		return fmt.Errorf("load credential settings: %w", err)
	}
	var selected []seal.Recipient
	if kind == "encrypt" {
		if settings.Recipients == "" {
			return fmt.Errorf("no recipient folder configured: set recipients in cred/config.json; see dgs cred key hosts")
		}
		folder, err := recipients.Load(settings.Recipients)
		if err != nil {
			return err
		}
		if err := recipientErrors(folder); err != nil {
			return err
		}
		if err := writeRecipientWarnings(out, folder); err != nil {
			return err
		}
		selected, err = selectRecipients(folder, flags)
		if err != nil {
			return err
		}
	}
	var ids []age.Identity
	scan := identities.Discover(settings.Identities, identities.Options{})
	for _, identity := range scan.Identities {
		if identity.Status != identities.Usable {
			continue
		}
		if kind == "encrypt" {
			matches := false
			for _, recipient := range selected {
				if recipient.PublicKey == identity.Public.Key {
					matches = true
					break
				}
			}
			if !matches {
				continue
			}
		}
		id, err := identities.Open(identity)
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if kind == "encrypt" {
		result, err := seal.Encrypt(ctx, source, destination, selected, ids, settings.Skip())
		if err != nil {
			return err
		}
		state := "verified by decryption"
		if !result.Verified {
			state = "not verified by decryption: no matching unlocked local identity"
		}
		_, err = fmt.Fprintf(out, "Encrypted: %s (%s; %d archive entries skipped)\n", result.Path, state, result.Skipped)
		return err
	}
	if len(ids) == 0 {
		var problems []string
		for _, p := range scan.Problems {
			problems = append(problems, fmt.Sprintf("%s: %s", p.Path, p.Message))
		}
		for _, id := range scan.Identities {
			if id.Status != identities.Usable {
				problems = append(problems, fmt.Sprintf("%s: %s", id.Path, id.Status))
			}
		}
		detail := ""
		if len(problems) > 0 {
			detail = "\n" + strings.Join(problems, "\n")
		}
		return fmt.Errorf("no usable unlocked identities; check cred/config.json and dgs cred keys%s", detail)
	}
	if err := seal.Decrypt(ctx, source, destination, ids); err != nil {
		return fmt.Errorf("decrypt %q: %w (check the file, output path and configured identities; passphrase files and protected identities require the vault TUI)", source, err)
	}
	_, err = fmt.Fprintf(out, "Decrypted: %s\n", destination)
	return err
}

func selectRecipients(folder recipients.Folder, flags map[string]string) ([]seal.Recipient, error) {
	var hosts []string
	if raw := flags["host"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &hosts); err != nil {
			return nil, err
		}
	}
	var groups []string
	if raw := flags["group"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &groups); err != nil {
			return nil, err
		}
	}
	for _, name := range groups {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("--group (-G) requires a nonempty group name")
		}
		found := false
		for _, group := range folder.Groups {
			if strings.EqualFold(name, group.Name) {
				if len(group.Hosts) == 0 {
					return nil, fmt.Errorf("group %q has no hosts", name)
				}
				hosts = append(hosts, group.Hosts...)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown group %q; list names with dgs cred key groups (include the g- prefix)", name)
		}
	}
	var selected []seal.Recipient
	seen := map[string]bool{}
	for _, name := range hosts {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("--host (-H) requires a nonempty host name")
		}
		host, ok := folder.Host(name)
		if !ok {
			return nil, fmt.Errorf("unknown host %q; list names with dgs cred key hosts", name)
		}
		if len(host.Keys) == 0 {
			return nil, fmt.Errorf("host %q has no keys", name)
		}
		for _, key := range host.Keys {
			if seen[key.Key] {
				continue
			}
			seen[key.Key] = true
			selected = append(selected, seal.Recipient{PublicKey: key.Key, Host: host.Name, Description: key.Description})
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("select at least one --host or --group")
	}
	return selected, nil
}

// cryptPaths rejects invalid paths before configuration discovery or decryption.
// Publication still checks for racing destination creation.
func cryptPaths(kind, source, output string) (string, error) {
	if kind != "encrypt" && kind != "decrypt" {
		return "", fmt.Errorf("unknown operation %q", kind)
	}
	if strings.TrimSpace(source) == "" {
		return "", fmt.Errorf("source path must not be empty")
	}
	info, err := seal.SourceInfo(source, kind == "encrypt")
	if err != nil {
		return "", err
	}
	if output == "" {
		source, err = filepath.EvalSymlinks(source)
		if err != nil {
			return "", err
		}
		if info.IsDir() {
			source, err = filepath.Abs(source)
			if err != nil {
				return "", err
			}
			if filepath.Dir(source) == source {
				return "", fmt.Errorf("cannot infer output for filesystem root; use -o <destination>")
			}
		}
		if kind == "encrypt" {
			output = source + ".age"
			if info.IsDir() {
				output = source + seal.ArchiveSuffix + ".age"
			}
		} else {
			if !strings.HasSuffix(source, ".age") || filepath.Base(source) == ".age" {
				return "", fmt.Errorf("cannot infer output from %q: use -o <destination> or a .age source name", source)
			}
			output = strings.TrimSuffix(source, ".age")
		}
	}
	if strings.TrimSpace(output) == "" || strings.ContainsRune(output, '\x00') {
		return "", fmt.Errorf("output path must not be empty or contain NUL")
	}
	if output == "-" {
		return "", fmt.Errorf("stdout output (-o -) is not supported; choose a destination file")
	}
	if _, err := os.Lstat(output); err == nil {
		return "", fmt.Errorf("output %q already exists; choose another path with -o (nothing was overwritten)", output)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check output %q: %w", output, err)
	}
	if kind == "encrypt" {
		parent, err := os.Stat(filepath.Dir(output))
		if err != nil {
			return "", fmt.Errorf("output directory: %w (create it first or choose another path with -o)", err)
		}
		if !parent.IsDir() {
			return "", fmt.Errorf("output parent must be a directory")
		}
	}
	return output, nil
}
