package target

import (
	"strings"
	"testing"
)

func targets(strs ...string) []Target {
	var out []Target
	for _, s := range strs {
		parts := strings.Split(s, "/")
		out = append(out, Target{Service: parts[0], Role: parts[1], Instance: parts[2]})
	}
	return out
}

func names(ts []Target) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.String())
	}
	return out
}

func TestMatch_ServiceEveryRoleAndInstance(t *testing.T) {
	all := targets(
		"hysteria2/server/u-node-group-10-01",
		"hysteria2/client/u-node-group-10-01",
		"shadowsocks-rust/client/u-node-group-10-01",
	)
	got, err := Match("hysteria2/**", all)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	want := []string{"hysteria2/client/u-node-group-10-01", "hysteria2/server/u-node-group-10-01"}
	if got := names(got); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatch_ServerSideAcrossServices(t *testing.T) {
	all := targets(
		"hysteria2/server/u-node-group-10-01",
		"hysteria2/server/j-node-group-06-01",
		"shadowsocks-rust/server/u-node-group-10-01",
		"shadowsocks-rust/client/u-node-group-10-01",
	)
	got, err := Match("*/server/u-node-group-10-*", all)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	want := []string{"hysteria2/server/u-node-group-10-01", "shadowsocks-rust/server/u-node-group-10-01"}
	if got := names(got); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatch_EveryInstanceOfThatNameWhateverRole(t *testing.T) {
	all := targets(
		"hysteria2/server/u-node-group-10-01",
		"shadowsocks-rust/client/u-node-group-10-01",
		"shadowsocks-rust/client/j-node-group-06-01",
	)
	got, err := Match("**/u-node-group-10-01", all)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	want := []string{"hysteria2/server/u-node-group-10-01", "shadowsocks-rust/client/u-node-group-10-01"}
	if got := names(got); !equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMatch_ExactTarget(t *testing.T) {
	all := targets("hysteria2/server/u-node-group-10-01", "hysteria2/server/j-node-group-06-01")
	got, err := Match("hysteria2/server/u-node-group-10-01", all)
	if err != nil {
		t.Fatalf("Match: %v", err)
	}
	if len(got) != 1 || got[0].Instance != "u-node-group-10-01" {
		t.Fatalf("got %v", got)
	}
}

func TestMatch_NoMatchIsAnErrorNamingTheSelector(t *testing.T) {
	all := targets("hysteria2/server/u-node-group-10-01")
	_, err := Match("microbin/**", all)
	if err == nil {
		t.Fatal("Match: want error")
	}
	if !strings.Contains(err.Error(), "microbin/**") {
		t.Fatalf("error = %q, want it to name the selector", err)
	}
}

func TestMatch_EmptySelectorIsAnError(t *testing.T) {
	_, err := Match("", targets("a/b/c"))
	if err == nil {
		t.Fatal("Match: want error for empty selector")
	}
}

func TestMatch_MultipleStarStarIsAnError(t *testing.T) {
	_, err := Match("**/server/**", targets("a/server/c"))
	if err == nil {
		t.Fatal("Match: want error for more than one **")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
