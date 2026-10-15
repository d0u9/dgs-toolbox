package conf

import (
	"maps"
	"slices"
	"strings"

	rcli "github.com/d0u9/rhumb/cli"
	"github.com/d0u9/rhumb/inventory"
)

func hostMigrationRows(host string, h inventory.Host) []migrationRow {
	var rows []migrationRow
	for _, name := range slices.Sorted(maps.Keys(h.Interfaces)) {
		i := h.Interfaces[name]
		mac := i.MAC
		if mac == "" {
			mac = "(none)"
		}
		rows = append(rows, migrationRow{kind: "host-mac", label: "MAC " + name, source: inventory.HostSource(host) + ".interfaces." + name + ".mac", before: mac, after: mac, host: host, iface: name})
		for _, aid := range slices.Sorted(maps.Keys(i.Addresses)) {
			a := i.Addresses[aid]
			rows = append(rows, migrationRow{kind: "host-ip", label: "IP " + name + "/" + aid + " (" + a.Network + ")", source: inventory.AddressSource(host, name, aid), before: a.IP, after: a.IP, host: host, iface: name, address: aid})
		}
	}
	for _, network := range slices.Sorted(maps.Keys(h.Endpoints)) {
		e := h.Endpoints[network]
		kind, value := "host-endpoint", e.Address
		field := ".address"
		if e.From != nil {
			kind, value, field = "host-target", e.From.Interface+"/"+e.From.Address, ".from"
		}
		rows = append(rows, migrationRow{kind: kind, label: "ENDPOINT " + network, source: inventory.HostSource(host) + ".endpoints." + network + field, before: value, after: value, host: host, network: network})
	}
	return rows
}

func hostChangeFromRow(row migrationRow) rcli.HostChange {
	return rcli.HostChange{Kind: strings.TrimPrefix(row.kind, "host-"), Interface: row.iface, Address: row.address, Network: row.network, From: row.before, To: row.after}
}
