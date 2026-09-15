package cred

import (
	"fmt"
	"path/filepath"
	"strings"

	"dgs-toolbox/internal/cred/identities"
	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/tui/confirm"

	tea "github.com/charmbracelet/bubbletea"
)

// deleteFlow is the confirmation in front of deleting or unregistering a key.
type deleteFlow struct {
	// path is the identity file moved to the trash, or empty to unregister only.
	path      string
	publicKey string
	// host is set when a whole host is deleted: its file goes to the trash and
	// it leaves every group.
	host   string
	dialog confirm.Model
}

type deletedMsg struct {
	trashed string
	changed []string
	err     error
}

// managedIdentity reports whether an identity is one dgs may delete: an age key
// in the new identity directory, alone in its file.
func (m keysModel) managedIdentity(identity identities.Identity) (bool, string) {
	if identity.Line == 0 {
		return false, "only age keys in " + tilde(m.snap.settings.NewIdentityDir) + " are deleted; others are only unregistered"
	}
	dir := resolved(m.snap.settings.NewIdentityDir)
	if resolved(filepath.Dir(identity.Path)) != dir {
		return false, "only age keys in " + tilde(m.snap.settings.NewIdentityDir) + " are deleted; others are only unregistered"
	}
	count := 0
	for _, other := range m.snap.scan.Identities {
		if other.Path == identity.Path {
			count++
		}
	}
	if count > 1 {
		return false, "its file holds other keys too, so it is only unregistered"
	}
	return true, ""
}

func (m *keysModel) startDelete() {
	m.notice = ""
	flow := &deleteFlow{}
	var hosts []string
	var subject string
	switch {
	case m.tab == tabIdentities:
		identity, ok := m.selectedIdentity()
		if !ok {
			return
		}
		for _, match := range identities.Matches(identity, m.snap.folder) {
			hosts = append(hosts, match.Host)
		}
		managed, why := m.managedIdentity(identity)
		if !managed && len(hosts) == 0 {
			m.notice = "! Not deleted: " + why + ", and it is not registered"
			return
		}
		if managed {
			flow.path = identity.Path
		} else {
			m.notice = "Note: " + why
		}
		flow.publicKey = identity.Public.Key
		subject = identityLabelFull(identity)
	case m.tab == tabHosts && m.fields.Current() == listField:
		m.startDeleteHost()
		return
	case m.tab == tabHosts && m.fields.Current() == keysField:
		key, ok := m.selectedKey()
		if !ok {
			return
		}
		flow.publicKey = key.Key
		for _, host := range m.snap.folder.Hosts {
			for _, listed := range host.Keys {
				if listed.Key == key.Key {
					hosts = append(hosts, host.Name)
				}
			}
		}
		subject = "“" + key.Description + "”"
	default:
		return
	}

	config := confirm.Config{Title: "UNREGISTER KEY", ConfirmLabel: "Unregister", CancelLabel: "Keep"}
	if flow.path != "" {
		config.Title, config.ConfirmLabel = "DELETE KEY", "Delete"
		config.Message = fmt.Sprintf("Move %s to the trash", subject)
		if len(hosts) > 0 {
			config.Message += " and remove its public key from " + strings.Join(hosts, ", ")
		}
		config.Message += "?"
	} else {
		config.Message = fmt.Sprintf("Remove the public key of %s from %s? No private key is deleted.", subject, strings.Join(hosts, ", "))
	}
	config.Detail = m.usageNote(flow.publicKey, flow.path != "")
	flow.dialog = confirm.New(config)
	m.deleteFlow = flow
}

// usageNote says what the vault's records show about files encrypted to the key.
func (m keysModel) usageNote(publicKey string, deleting bool) string {
	if publicKey == "" {
		return "Its public key is unknown, so the vault was not checked."
	}
	return m.usage([]string{publicKey}, deleting)
}

// usageNoteAny is usageNote for every key of a host.
func (m keysModel) usageNoteAny(keys []string) string {
	if len(keys) == 0 {
		return "It has no keys, so no file can be encrypted to it."
	}
	return m.usage(keys, false)
}

func (m keysModel) usage(keys []string, deleting bool) string {
	vault := m.snap.settings.Vault
	if vault == "" {
		return "No vault is configured, so files encrypted to it were not looked for."
	}
	usage, err := record.UsingAny(vault, keys)
	if err != nil {
		return "The vault could not be checked: " + err.Error()
	}
	var notes []string
	switch {
	case len(usage.Files) == 0:
		notes = append(notes, "No file in the vault is recorded as encrypted to it.")
	case len(usage.Only) == 0:
		notes = append(notes, fmt.Sprintf("%s in the vault %s encrypted to it and to other keys too.", plural(len(usage.Files), "file"), isOrAre(len(usage.Files))))
	default:
		lost := "can no longer be opened by a key this change leaves registered"
		if deleting {
			lost = "cannot be opened again"
		}
		notes = append(notes, fmt.Sprintf("%s in the vault %s encrypted only to it and %s: %s.", plural(len(usage.Only), "file"), isOrAre(len(usage.Only)), lost, listSome(usage.Only, 3)))
	}
	if len(usage.Unreadable) > 0 {
		notes = append(notes, fmt.Sprintf("%s could not be read, so this may be incomplete.", plural(len(usage.Unreadable), "record")))
	}
	return strings.Join(notes, " ")
}

func (m *keysModel) startDeleteHost() {
	host, ok := m.selectedHost()
	if !ok {
		return
	}
	var groups []string
	for _, group := range m.snap.folder.Groups {
		for _, member := range group.Hosts {
			if member == host.Name {
				groups = append(groups, group.Name)
			}
		}
	}
	message := fmt.Sprintf("Move %s to the trash, with its %s", filepath.ToSlash(host.File), plural(len(host.Keys), "public key"))
	if len(groups) > 0 {
		message += ", and remove " + host.Name + " from " + strings.Join(groups, ", ")
	}
	var keys []string
	held := false
	for _, key := range host.Keys {
		keys = append(keys, key.Key)
		held = held || m.snap.held[key.Key]
	}
	detail := m.usageNoteAny(keys)
	if held {
		detail = "This machine's private keys for it are not deleted. " + detail
	}
	m.deleteFlow = &deleteFlow{
		path: filepath.Join(m.snap.settings.Recipients, host.File),
		host: host.Name,
		dialog: confirm.New(confirm.Config{
			Title:        "DELETE HOST",
			Message:      message + "?",
			Detail:       detail,
			ConfirmLabel: "Delete",
			CancelLabel:  "Keep",
		}),
	}
}

func isOrAre(n int) string {
	if n == 1 {
		return "is"
	}
	return "are"
}

func listSome(items []string, limit int) string {
	if len(items) <= limit {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:limit], ", ") + fmt.Sprintf(" and %d more", len(items)-limit)
}

func (m keysModel) updateDelete(key string) (tea.Model, tea.Cmd) {
	dialog, decision := m.deleteFlow.dialog.Update(key)
	m.deleteFlow.dialog = dialog
	switch decision {
	case confirm.Cancelled:
		m.deleteFlow = nil
	case confirm.Confirmed:
		flow := *m.deleteFlow
		m.deleteFlow = nil
		root, trash := m.snap.settings.Recipients, m.trash
		if flow.host != "" {
			return m, func() tea.Msg {
				var msg deletedMsg
				// Leave the groups first, while the host still resolves; a
				// failure then leaves its file in place.
				if msg.changed, msg.err = recipients.RemoveFromGroups(root, flow.host); msg.err != nil {
					return msg
				}
				msg.trashed, msg.err = trash(flow.path)
				return msg
			}
		}
		return m, func() tea.Msg {
			var msg deletedMsg
			if flow.path != "" {
				if msg.trashed, msg.err = trash(flow.path); msg.err != nil {
					return msg
				}
			}
			if flow.publicKey != "" && root != "" {
				msg.changed, msg.err = recipients.RemoveKey(root, flow.publicKey)
			}
			return msg
		}
	}
	return m, nil
}

func (m keysModel) finishDelete(msg deletedMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.err != nil && msg.trashed != "":
		m.notice = fmt.Sprintf("! Moved to %s, but removing the public key failed: %v", tilde(msg.trashed), msg.err)
	case msg.err != nil:
		m.notice = "! " + msg.err.Error()
	case msg.trashed != "":
		m.notice = fmt.Sprintf("Moved to %s", tilde(msg.trashed))
		if len(msg.changed) > 0 {
			m.notice += " · updated " + strings.Join(msg.changed, ", ")
		}
	default:
		m.notice = "Removed from " + strings.Join(msg.changed, ", ")
	}
	return m, m.reload()
}
