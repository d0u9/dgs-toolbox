// Package target lists the targets a generator root holds and matches
// selectors against them.
//
// The rules are in docs/apps/conf/export.md#targets-and-selectors.
package target

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"dgs-toolbox/internal/conf/confgen"
)

// Target is one <service>/<role>/<instance>.
type Target struct {
	Service  string
	Role     string
	Instance string
}

// String is service/role/instance, the form a selector and --targets write.
func (t Target) String() string {
	return t.Service + "/" + t.Role + "/" + t.Instance
}

// Status is one target as List finds it: either ready to render, or broken
// with the reason, carried over from the service's manifest or the
// instance's own file.
type Status struct {
	Target Target
	Broken string
}

// List is every target a discovered root holds, sorted by service, role and
// instance. A service whose manifest is broken contributes no targets, since
// it declares no roles; an instance that is itself broken still becomes a
// target, so a selector can name it and a report can say why it cannot
// render.
func List(root *confgen.Root) []Status {
	var statuses []Status
	for _, svc := range root.Services {
		for _, role := range svc.Roles {
			for _, inst := range role.Instances {
				statuses = append(statuses, Status{
					Target: Target{Service: svc.Name, Role: role.Name, Instance: inst.Name},
					Broken: inst.Broken,
				})
			}
		}
	}
	sort.Slice(statuses, func(i, j int) bool {
		a, b := statuses[i].Target, statuses[j].Target
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		return a.Instance < b.Instance
	})
	return statuses
}

// Match returns every target a selector matches, sorted the same way List
// sorts. A selector matching nothing is an error naming the selector, not an
// empty result — see docs/apps/conf/export.md#targets-and-selectors.
func Match(selector string, targets []Target) ([]Target, error) {
	m, err := compile(selector)
	if err != nil {
		return nil, fmt.Errorf("selector %q: %w", selector, err)
	}
	var matched []Target
	for _, t := range targets {
		if m.matches(t) {
			matched = append(matched, t)
		}
	}
	if len(matched) == 0 {
		return nil, fmt.Errorf("selector %q matches nothing", selector)
	}
	sort.Slice(matched, func(i, j int) bool {
		a, b := matched[i], matched[j]
		if a.Service != b.Service {
			return a.Service < b.Service
		}
		if a.Role != b.Role {
			return a.Role < b.Role
		}
		return a.Instance < b.Instance
	})
	return matched, nil
}

// matcher is a compiled selector: literal segments, each matched with `*`
// glob within its own position, and where "**" sits — -1 when the selector
// has none.
type matcher struct {
	segments []string
	starStar int
}

func compile(selector string) (matcher, error) {
	if selector == "" {
		return matcher{}, fmt.Errorf("empty selector")
	}
	segments := strings.Split(selector, "/")
	starStar := -1
	for i, seg := range segments {
		if seg == "" {
			return matcher{}, fmt.Errorf("empty segment")
		}
		if seg == "**" {
			if starStar != -1 {
				return matcher{}, fmt.Errorf("more than one **")
			}
			starStar = i
		}
	}
	return matcher{segments: segments, starStar: starStar}, nil
}

// matches checks t's three segments against m. Without "**", the selector
// must have exactly three segments, matched positionally. With "**", it
// consumes however many of t's segments are left over once the literal
// segments before and after it are accounted for.
func (m matcher) matches(t Target) bool {
	parts := []string{t.Service, t.Role, t.Instance}

	if m.starStar == -1 {
		if len(m.segments) != len(parts) {
			return false
		}
		for i, seg := range m.segments {
			if ok, _ := filepath.Match(seg, parts[i]); !ok {
				return false
			}
		}
		return true
	}

	before := m.segments[:m.starStar]
	after := m.segments[m.starStar+1:]
	consumed := len(before) + len(after)
	if consumed > len(parts) {
		return false
	}
	for i, seg := range before {
		if ok, _ := filepath.Match(seg, parts[i]); !ok {
			return false
		}
	}
	tailStart := len(parts) - len(after)
	for i, seg := range after {
		if ok, _ := filepath.Match(seg, parts[tailStart+i]); !ok {
			return false
		}
	}
	return true
}
