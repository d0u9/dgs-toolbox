package conf

import (
	"testing"

	"github.com/d0u9/rhumb/derive"
	"github.com/d0u9/rhumb/inventory"
	"github.com/d0u9/rhumb/target"
)

func treeLabels(nodes []*nodeGroup) (holders, labels map[string]string) {
	holders, labels = map[string]string{}, map[string]string{}
	for _, n := range nodes {
		holders[n.key] = n.name
		for _, inst := range n.instances {
			labels[inst.name] = inst.label
		}
	}
	return holders, labels
}

func TestBuildTree_ExportLabelsDistinguishCredentialsProfilesAndFormats(t *testing.T) {
	targets := []target.Target{
		{User: "alice", Instance: "file-a", Service: "proxy", Export: "json", Routes: []string{"route-a"}},
		{User: "alice", Instance: "file-b", Service: "proxy", Export: "json", Routes: []string{"route-a"}},
		{User: "alice", Instance: "file-c", Service: "proxy", Export: "link", Routes: []string{"route-a"}},
		{Node: "node-1", Instance: "file-d", Service: "proxy", Export: "json", Profile: "browser", Routes: []string{"route-a"}},
		{Node: "node-1", Instance: "file-e", Service: "proxy", Export: "json", Profile: "desktop", Routes: []string{"route-a"}},
		{Node: "node-1", Instance: "file-f", Service: "proxy", Export: "json", Routes: []string{"route-a"}},
	}
	exports := []derive.ExportInstance{
		{ID: "file-a", Credential: "default"}, {ID: "file-b", Credential: "work"}, {ID: "file-c", Credential: "work"},
	}
	nodes := buildTree(targets, exports)
	holders, labels := treeLabels(nodes)
	want := map[string]string{
		"file-a": "default / route-a / json",
		"file-b": "work / route-a / json",
		"file-c": "work / route-a / link",
		"file-d": "browser / route-a / json",
		"file-e": "desktop / route-a / json",
		"file-f": "route-a / json",
	}
	for id, label := range want {
		if labels[id] != label {
			t.Errorf("%s label = %q, want %q", id, labels[id], label)
		}
	}
	if holders["alice"] != "credentials" {
		t.Errorf("holder with several credentials = %q, want credentials", holders["alice"])
	}
	for _, n := range nodes {
		for _, inst := range n.instances {
			if inst.detail != "proxy" {
				t.Errorf("%s detail = %q, want the service", inst.name, inst.detail)
			}
			if got := inst.units(); len(got) != 1 || got[0] != inst.name {
				t.Errorf("%s units = %v, want its own instance ID", inst.name, got)
			}
		}
	}
}

func TestBuildTree_UserHolderNames(t *testing.T) {
	targets := []target.Target{
		{User: "alice", Instance: "file-a", Service: "proxy", Export: "json", Routes: []string{"route-a"}},
		{User: "bob", Instance: "file-b", Service: "proxy", Export: "json", Routes: []string{"route-a"}},
	}
	holders, _ := treeLabels(buildTree(targets, []derive.ExportInstance{{ID: "file-a", Credential: "work"}}))
	if holders["alice"] != "work" {
		t.Errorf("single-credential holder = %q, want work", holders["alice"])
	}
	if holders["bob"] != inventory.DefaultCredential {
		t.Errorf("holder without credentials = %q, want %q", holders["bob"], inventory.DefaultCredential)
	}
}

func TestBuildTree_SeveralRoutesKeepInstanceName(t *testing.T) {
	targets := []target.Target{
		{User: "alice", Instance: "alice" + inventory.QualifiedSep + "multi-json", Service: "proxy", Export: "json", Routes: []string{"route-a", "route-b"}},
		{Node: "node-1", Instance: "node-1" + inventory.QualifiedSep + "multi-link", Service: "proxy", Export: "link", Profile: "browser", Routes: []string{"route-a", "route-b"}},
	}
	exports := []derive.ExportInstance{{ID: targets[0].Instance, Credential: "work"}}
	_, labels := treeLabels(buildTree(targets, exports))
	if got := labels[targets[0].Instance]; got != "work / multi-json" {
		t.Errorf("user label = %q, want work / multi-json", got)
	}
	if got := labels[targets[1].Instance]; got != "browser / multi-link" {
		t.Errorf("node label = %q, want browser / multi-link", got)
	}
}
