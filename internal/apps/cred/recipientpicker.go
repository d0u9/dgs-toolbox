package cred

import (
	"fmt"

	"dgs-toolbox/internal/cred/recipients"
	"dgs-toolbox/internal/cred/record"
	"dgs-toolbox/internal/cred/seal"
	"dgs-toolbox/internal/tui/scrolllist"
	"dgs-toolbox/internal/tui/tristate"
)

type rowKind int

const (
	rowGroup rowKind = iota
	rowHost
	rowKey
	// rowOutside is a recorded key that no host in the folder lists.
	rowOutside
)

type recipientRow struct {
	kind    rowKind
	group   recipients.Group
	host    recipients.Host
	key     recipients.Key
	outside record.Recipient
}

// recipientPicker is the checklist of groups, hosts and keys a file is
// encrypted to, used when adding a file and when changing its recipients.
type recipientPicker struct {
	folder  recipients.Folder
	held    map[string]bool
	rows    []recipientRow
	list    scrolllist.Model
	checked map[string]bool
}

// newRecipientPicker lists the folder's groups and hosts, then the recorded
// keys it no longer lists. initial are the keys checked to start with; nil
// checks the keys this machine holds.
func newRecipientPicker(folder recipients.Folder, held map[string]bool, initial map[string]bool, outside []record.Recipient) *recipientPicker {
	p := &recipientPicker{folder: folder, held: held, checked: map[string]bool{}, list: scrolllist.New()}
	if initial == nil {
		initial = held
	}
	for _, group := range folder.Groups {
		p.rows = append(p.rows, recipientRow{kind: rowGroup, group: group})
	}
	for _, host := range folder.Hosts {
		p.rows = append(p.rows, recipientRow{kind: rowHost, host: host})
		for _, key := range host.Keys {
			p.rows = append(p.rows, recipientRow{kind: rowKey, host: host, key: key})
			if initial[key.Key] {
				p.checked[key.Key] = true
			}
		}
	}
	for _, r := range outside {
		p.rows = append(p.rows, recipientRow{kind: rowOutside, outside: r})
		if initial[r.PublicKey] {
			p.checked[r.PublicKey] = true
		}
	}
	p.refresh()
	return p
}

// rowKeys are the public keys a row stands for.
func (p *recipientPicker) rowKeys(row recipientRow) []string {
	switch row.kind {
	case rowKey:
		return []string{row.key.Key}
	case rowOutside:
		return []string{row.outside.PublicKey}
	case rowHost:
		keys := make([]string, len(row.host.Keys))
		for i, key := range row.host.Keys {
			keys[i] = key.Key
		}
		return keys
	}
	var keys []string
	for _, name := range row.group.Hosts {
		if host, ok := p.folder.Host(name); ok {
			for _, key := range host.Keys {
				keys = append(keys, key.Key)
			}
		}
	}
	return keys
}

func (p *recipientPicker) checkbox(row recipientRow) string {
	keys := p.rowKeys(row)
	return tristate.Box(len(keys), tristate.Count(keys, p.checked))
}

func (p *recipientPicker) refresh() {
	items := make([]scrolllist.Item, len(p.rows))
	groups := 0
	for i, row := range p.rows {
		box := p.checkbox(row)
		// Two rows each, so a long description is not cut off by the key.
		var label, detail string
		switch row.kind {
		case rowGroup:
			groups++
			label = fmt.Sprintf("%s %s", box, row.group.Name)
			detail = "    " + plural(len(row.group.Hosts), "host")
		case rowHost:
			label = fmt.Sprintf("%s %s", box, row.host.Name)
			detail = "    " + plural(len(row.host.Keys), "key")
			if len(row.host.Keys) == 0 {
				detail = "    no keys; nothing to check"
			}
		case rowKey:
			label = fmt.Sprintf("    %s %s", box, row.key.Description)
			detail = "        " + shortKey(row.key.PublicKey)
			if p.held[row.key.Key] {
				detail += "  ● this machine"
			}
		case rowOutside:
			name := row.outside.Host
			if row.outside.Description != "" {
				name += " · " + row.outside.Description
			}
			if name == "" {
				name = "recorded key"
			}
			label = fmt.Sprintf("%s %s", box, name)
			detail = "    " + shortenText(row.outside.PublicKey) + "  not in the recipient folder"
		}
		items[i] = scrolllist.Item{ID: fmt.Sprint(i), Label: label, Detail: detail}
	}
	p.list.SetItems(items)
	p.list.SetDivider(groups, "HOSTS")
}

// update handles a key: space checks or unchecks the row, the rest move. It
// reports whether the checks changed.
func (p *recipientPicker) update(key string) bool {
	switch key {
	case " ", "space", "x":
		index := p.list.Cursor()
		if index >= len(p.rows) {
			return false
		}
		all := p.checkbox(p.rows[index]) == "[x]"
		for _, k := range p.rowKeys(p.rows[index]) {
			p.checked[k] = !all
		}
		p.refresh()
		return true
	}
	var pending bool
	moveKey(&p.list, key, &pending)
	return false
}

// chosen are the checked keys, the folder's in its order and then recorded ones.
func (p *recipientPicker) chosen() []seal.Recipient {
	var chosen []seal.Recipient
	seen := map[string]bool{}
	for _, row := range p.rows {
		switch {
		case row.kind == rowKey && p.checked[row.key.Key] && !seen[row.key.Key]:
			seen[row.key.Key] = true
			chosen = append(chosen, seal.Recipient{PublicKey: row.key.Key, Host: row.host.Name, Description: row.key.Description})
		case row.kind == rowOutside && p.checked[row.outside.PublicKey] && !seen[row.outside.PublicKey]:
			seen[row.outside.PublicKey] = true
			chosen = append(chosen, seal.Recipient{PublicKey: row.outside.PublicKey, Host: row.outside.Host, Description: row.outside.Description})
		}
	}
	return chosen
}

// mine reports whether a chosen key belongs to this machine.
func (p *recipientPicker) mine() bool {
	for _, r := range p.chosen() {
		if p.held[r.PublicKey] {
			return true
		}
	}
	return false
}

func (p *recipientPicker) view(width, height int) string {
	if len(p.rows) == 0 {
		return mutedStyle.Render("· The recipient folder has no hosts.")
	}
	list := p.list
	list.SetSize(width, max(1, height))
	return list.View(true, titleStyle, mutedStyle)
}
