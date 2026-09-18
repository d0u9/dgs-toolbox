package render

import (
	"strings"
	"testing"

	"dgs-toolbox/internal/conf/confgen"
)

func TestRender_DocumentDefaults_InstanceWinsOverDefaults(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "hysteria2", Role: "server", Instance: "us-sfo"},
		Template:     "listen: {{ .listen }}\nlog: {{ .log }}\n",
		Defaults:     []byte("listen: :443\nlog: warn\n"),
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     map[string]any{"listen": ":8443"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "listen: :8443\nlog: warn\n"
	if string(out) != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
}

func TestRender_ElementDefaults_InstanceReachesTemplateUnmerged(t *testing.T) {
	// The template does its own element-level merge, as the existing
	// Shadowsocks templates do.
	out, err := Render(Input{
		Target:       Target{Service: "shadowsocks-rust", Role: "client", Instance: "us-sfo"},
		Template:     `{{ $d := index (defaults).servers 0 }}{{ $s := merge (index .servers 0) $d }}{{ $s.port }}/{{ $s.timeout }}`,
		Defaults:     []byte("servers:\n  - timeout: 60\n    port: 8388\n"),
		DefaultsKind: confgen.DefaultsElement,
		Instance:     map[string]any{"servers": []any{map[string]any{"port": 9000}}},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "9000/60" {
		t.Fatalf("out = %q, want 9000/60", out)
	}
}

func TestRender_Secret_ReadsOwnByName(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "shadowsocks-rust", Role: "server", Instance: "us-sfo"},
		Template:     `{{ secret "auth_password" }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Own:          map[string]string{"auth_password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "hunter2" {
		t.Fatalf("out = %q, want hunter2", out)
	}
}

func TestRender_Secret_MissingNameNamesIt(t *testing.T) {
	_, err := Render(Input{
		Target:       Target{Service: "svc", Role: "server", Instance: "inst"},
		Template:     `{{ secret "does-not-exist" }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Own:          map[string]string{"other": "x"},
	})
	if err == nil {
		t.Fatal("Render: want error for missing secret")
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Fatalf("error = %q, want it to name the missing key", err)
	}
	if !strings.Contains(err.Error(), "svc/server/inst") {
		t.Fatalf("error = %q, want it to name the target", err)
	}
}

func TestRender_NodeInstanceUpstreamAndPrincipals(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "shadowsocks-rust", Role: "server", Instance: "ss-sfo01"},
		Template:     `{{ (node).id }} {{ (instance).id }} {{ (upstream).address }}:{{ (upstream).port }} {{ range principals "main" }}{{ .Name }}={{ .Secret }} {{ end }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     map[string]any{"id": "ss-sfo01"},
		Node:         map[string]any{"id": "us-sfo-dgo-linux-01"},
		Upstream:     map[string]any{"address": "203.0.113.1", "port": 443},
		Principals: map[string][]Principal{
			"main": {{Name: "doug-macbook", Secret: "abc"}, {Name: "yak", Secret: "xyz"}},
		},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "us-sfo-dgo-linux-01 ss-sfo01 203.0.113.1:443 doug-macbook=abc yak=xyz "
	if string(out) != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
}

func TestRender_UpstreamNilForATerminalInstance(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "microbin", Role: "server", Instance: "bin-sfo01"},
		Template:     `{{ if upstream }}has upstream{{ else }}terminal{{ end }}`,
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "terminal" {
		t.Fatalf("out = %q, want terminal", out)
	}
}

func TestRender_Required_MissingNamesWhatAndWhere(t *testing.T) {
	_, err := Render(Input{
		Target:       Target{Service: "microbin", Role: "server", Instance: "us-sfo"},
		Template:     `{{ required .auth_password "microbin auth password" }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     map[string]any{"other": 1},
	})
	if err == nil {
		t.Fatal("Render: want error for missing required value")
	}
	if !strings.Contains(err.Error(), "microbin auth password") {
		t.Fatalf("error = %q, want it to name what was missing", err)
	}
}

func TestRender_TargetFunction(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "hysteria2", Role: "server", Instance: "us-sfo"},
		Template:     `{{ (target).service }}/{{ (target).role }}/{{ (target).instance }}`,
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "hysteria2/server/us-sfo" {
		t.Fatalf("out = %q", out)
	}
}

func TestRender_MergeFirstArgumentWins(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "s", Role: "r", Instance: "i"},
		Template:     `{{ $m := merge (dict "a" 1) (dict "a" 2 "b" 3) }}{{ $m.a }}-{{ $m.b }}`,
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "1-3" {
		t.Fatalf("out = %q, want 1-3", out)
	}
}

func TestRender_MergeListsReplaceRatherThanCombine(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "s", Role: "r", Instance: "i"},
		Template:     `{{ $m := merge (dict "xs" (slice 1 2)) (dict "xs" (slice 9 8 7)) }}{{ len $m.xs }}`,
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "2" {
		t.Fatalf("out = %q, want 2 (the winning list, not merged)", out)
	}
}

func TestRender_OmitAndPick(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "s", Role: "r", Instance: "i"},
		Template:     `{{ $m := omit . "b" }}{{ has $m "a" }}-{{ has $m "b" }}`,
		DefaultsKind: confgen.DefaultsDocument,
		Instance:     map[string]any{"a": 1, "b": 2},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "true-false" {
		t.Fatalf("out = %q, want true-false", out)
	}
}

func TestRender_ToYAMLAndToJSON(t *testing.T) {
	out, err := Render(Input{
		Target:       Target{Service: "s", Role: "r", Instance: "i"},
		Template:     `{{ toJSON (dict "a" 1) }}|{{ toYAML (dict "a" 1) }}`,
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if string(out) != "{\"a\":1}|a: 1\n" {
		t.Fatalf("out = %q", out)
	}
}

func TestRender_BrokenDefaultsNamesTargetAndReason(t *testing.T) {
	_, err := Render(Input{
		Target:       Target{Service: "hysteria2", Role: "server", Instance: "broken-one"},
		Template:     `{{ . }}`,
		Defaults:     []byte("not: [valid"),
		DefaultsKind: confgen.DefaultsDocument,
	})
	if err == nil {
		t.Fatal("Render: want error for unparsable defaults")
	}
	if !strings.Contains(err.Error(), "hysteria2/server/broken-one") {
		t.Fatalf("error = %q, want it to name the target", err)
	}
}

func TestRender_UnknownDefaultsKindErrors(t *testing.T) {
	_, err := Render(Input{
		Target:       Target{Service: "s", Role: "r", Instance: "i"},
		Template:     `{{ . }}`,
		DefaultsKind: "bogus",
	})
	if err == nil {
		t.Fatal("Render: want error for unknown defaults kind")
	}
}
