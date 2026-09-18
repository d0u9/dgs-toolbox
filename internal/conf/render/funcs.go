package render

import (
	"encoding/json"
	"fmt"
	"reflect"

	"gopkg.in/yaml.v3"
)

// funcs builds the template.FuncMap: the function set
// docs/apps/conf/export.md#the-template-language names, closing over
// defaults and the render context in's other fields carry.
func funcs(defaults map[string]any, in Input) map[string]any {
	return map[string]any{
		"merge":    mergeFunc,
		"omit":     omitFunc,
		"pick":     pickFunc,
		"has":      hasFunc,
		"append":   appendFunc,
		"slice":    sliceFunc,
		"dict":     dictFunc,
		"toYAML":   toYAMLFunc,
		"toJSON":   toJSONFunc,
		"required": requiredFunc,
		"secret":   secretFunc(in.Own),
		"defaults": func() map[string]any { return defaults },
		"node":     func() map[string]any { return in.Node },
		"instance": func() map[string]any { return in.Instance },
		"upstream": func() map[string]any { return in.Upstream },
		"principals": func(port string) []Principal {
			return in.Principals[port]
		},
		"target": func() map[string]string {
			return map[string]string{
				"service":  in.Target.Service,
				"role":     in.Target.Role,
				"instance": in.Target.Instance,
			}
		},
	}
}

// mergeFunc implements merge: the first argument wins, each later one losing
// to every earlier one. Nested maps merge key by key; a list loses wholesale
// to an earlier list rather than being merged into it — see
// docs/apps/conf/export.md#the-template-language.
func mergeFunc(args ...any) (map[string]any, error) {
	acc := map[string]any{}
	for i := len(args) - 1; i >= 0; i-- {
		m, ok := args[i].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("merge: argument %d is not a map, got %T", i, args[i])
		}
		acc = mergeInto(m, acc)
	}
	return acc, nil
}

// mergeInto merges src under dst, dst winning: a key present in both, with
// both values maps, merges recursively; otherwise dst's value stands.
func mergeInto(dst, src map[string]any) map[string]any {
	result := make(map[string]any, len(src)+len(dst))
	for k, v := range src {
		result[k] = v
	}
	for k, v := range dst {
		if existing, ok := result[k]; ok {
			if vMap, ok1 := v.(map[string]any); ok1 {
				if exMap, ok2 := existing.(map[string]any); ok2 {
					result[k] = mergeInto(vMap, exMap)
					continue
				}
			}
		}
		result[k] = v
	}
	return result
}

func omitFunc(m map[string]any, keys ...string) map[string]any {
	drop := make(map[string]bool, len(keys))
	for _, k := range keys {
		drop[k] = true
	}
	result := make(map[string]any, len(m))
	for k, v := range m {
		if !drop[k] {
			result[k] = v
		}
	}
	return result
}

func pickFunc(m map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, k := range keys {
		if v, ok := m[k]; ok {
			result[k] = v
		}
	}
	return result
}

// hasFunc reports whether a map has a key, or a list holds a value.
func hasFunc(collection any, item any) bool {
	switch c := collection.(type) {
	case map[string]any:
		key, ok := item.(string)
		if !ok {
			return false
		}
		_, exists := c[key]
		return exists
	case []any:
		for _, v := range c {
			if reflect.DeepEqual(v, item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// appendFunc returns list with items added, without modifying list.
func appendFunc(list []any, items ...any) []any {
	result := make([]any, 0, len(list)+len(items))
	result = append(result, list...)
	result = append(result, items...)
	return result
}

func sliceFunc(items ...any) []any {
	return append([]any{}, items...)
}

// dictFunc builds a map from alternating key, value arguments. Keys must be
// strings.
func dictFunc(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, fmt.Errorf("dict: odd number of arguments (%d)", len(pairs))
	}
	result := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is not a string, got %T", i/2, pairs[i])
		}
		result[key] = pairs[i+1]
	}
	return result, nil
}

func toYAMLFunc(v any) (string, error) {
	out, err := yaml.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	return string(out), nil
}

func toJSONFunc(v any) (string, error) {
	out, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toJSON: %w", err)
	}
	return string(out), nil
}

// requiredFunc returns value unchanged unless it is nil or the zero value of
// a string, map or slice, in which case it errors naming what was missing.
func requiredFunc(value any, name string) (any, error) {
	if value == nil {
		return nil, fmt.Errorf("required: %s is missing", name)
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.String:
		if v.Len() == 0 {
			return nil, fmt.Errorf("required: %s is missing", name)
		}
	case reflect.Map, reflect.Slice:
		if v.Len() == 0 {
			return nil, fmt.Errorf("required: %s is missing", name)
		}
	}
	return value, nil
}

// secretFunc returns the instance's own secret named name — a TLS key, an
// administrative password — from own, the render context's own datasource.
// See docs/apps/conf/export.md#the-template-language. A name matching
// nothing is an error naming it, rather than an empty value that fails
// further down.
func secretFunc(own map[string]string) func(name string) (string, error) {
	return func(name string) (string, error) {
		v, ok := own[name]
		if !ok {
			return "", fmt.Errorf("secret: no own secret named %q", name)
		}
		return v, nil
	}
}
