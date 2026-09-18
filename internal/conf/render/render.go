// Package render turns one target into rendered bytes: a role's defaults,
// the render context docs/apps/conf/inventory.md#the-render-context pins,
// and the target itself, run through a template. It knows nothing of the
// TUI or the filesystem layout — Input carries everything it needs as
// already-decoded values and bytes.
//
// The rules are in docs/apps/conf/export.md#rendering.
package render

import (
	"bytes"
	"fmt"
	"text/template"

	"gopkg.in/yaml.v3"

	"dgs-toolbox/internal/conf/confgen"
)

// Target names what is being rendered, for error messages and for the
// "target" template function.
type Target struct {
	Service  string
	Role     string
	Instance string
}

// String is service/role/instance, as docs/apps/conf/export.md#targets-and-selectors
// writes it.
func (t Target) String() string {
	return t.Service + "/" + t.Role + "/" + t.Instance
}

// Principal is one account a per-principal port grants, with its secret —
// what docs/apps/conf/inventory.md#the-render-context's principals
// datasource holds per entry.
type Principal struct {
	Name   string
	Secret string
}

// Input is one target's render context, plus the template it renders. Every
// field but Template, Defaults and DefaultsKind corresponds to one of
// docs/apps/conf/inventory.md#the-render-context's datasources; node,
// instance, upstream, own and target are read-only template functions
// returning them.
type Input struct {
	Target Target

	// Template is the role's template text.
	Template string

	// Defaults is the role's defaults.yaml content. Nil or empty means no
	// defaults.
	Defaults []byte
	// DefaultsKind is confgen.DefaultsDocument or confgen.DefaultsElement,
	// and decides how Defaults and Instance combine. See
	// docs/apps/conf/export.md#two-kinds-of-defaults.
	DefaultsKind string

	// Instance is the render context's instance datasource: id, service,
	// role, ports, bind. It is also what Defaults merges with or hands to
	// the template, replacing what used to be a separate values file.
	Instance map[string]any
	// Node is the node this instance runs on: id, networks. Nil for an
	// unmanaged user's derived instance, who has no node.
	Node map[string]any
	// Upstream is the next hop, resolved: address, port and secret. Nil for
	// a terminal instance.
	Upstream map[string]any
	// Principals is every port with auth: per-principal, each to the
	// accounts and secrets of everything holding a grant on it.
	Principals map[string][]Principal
	// Own is the instance's own secrets, by name — what the secret template
	// function reads from.
	Own map[string]string
}

// Render runs Template over Input and returns the rendered bytes. Every
// error — a template that will not parse or execute, a missing secret — is
// wrapped with Input.Target so a failure among many targets names which one
// it was.
func Render(in Input) ([]byte, error) {
	out, err := render(in)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", in.Target, err)
	}
	return out, nil
}

func render(in Input) ([]byte, error) {
	defaults, err := decodeMapping(in.Defaults, "defaults")
	if err != nil {
		return nil, err
	}

	instance := in.Instance
	if instance == nil {
		instance = map[string]any{}
	}

	var root map[string]any
	switch in.DefaultsKind {
	case confgen.DefaultsDocument:
		// The instance's own values win over the whole document.
		root = mergeInto(instance, defaults)
	case confgen.DefaultsElement:
		// The defaults are one entry of a list, and it is for the template to
		// apply to each entry of whichever list that is — see the open
		// question in docs/apps/conf/export.md#open-questions. The instance
		// reaches the template unmerged, and defaults() hands over the raw
		// defaults.
		root = instance
	default:
		return nil, fmt.Errorf("unknown defaults kind %q", in.DefaultsKind)
	}

	tmpl, err := template.New(in.Target.String()).Funcs(funcs(defaults, in)).Parse(in.Template)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, root); err != nil {
		return nil, fmt.Errorf("rendering: %w", err)
	}
	return buf.Bytes(), nil
}

// decodeMapping parses YAML that must be a mapping, or nothing when data is
// empty.
func decodeMapping(data []byte, what string) (map[string]any, error) {
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", what, err)
	}
	if m == nil {
		return nil, fmt.Errorf("%s: not a mapping", what)
	}
	return m, nil
}
