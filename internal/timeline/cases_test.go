package timeline

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// update rewrites each case's want from what Call returns. Read the diff
// before committing it: the cases are the contract, not a record of output.
var update = flag.Bool("update", false, "rewrite the wanted results of testdata/cases")

// The cases under testdata/cases are the contract every build of this package
// answers to: this test runs them natively and, under GOOS=js GOARCH=wasm, in
// the WebAssembly build; the Obsidian plugin runs the same files through its
// JavaScript bridge. Each case is a folder holding case.json —
//
//	{"op": "insert", "args": {...}, "want": ...}   or   {..., "error": "text"}
//
// — where any string "@name" stands for the file of that name beside it, so a
// note is kept as Markdown rather than escaped into JSON.
func TestCases(t *testing.T) {
	dirs, err := filepath.Glob(filepath.Join("testdata", "cases", "*", "case.json"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no cases found: %v", err)
	}
	for _, path := range dirs {
		dir := filepath.Dir(path)
		t.Run(filepath.Base(dir), func(t *testing.T) {
			var spec struct {
				Op    string `json:"op"`
				Args  any    `json:"args"`
				Want  any    `json:"want"`
				Error string `json:"error"`
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &spec); err != nil {
				t.Fatalf("case.json: %v", err)
			}
			args, err := json.Marshal(resolve(t, dir, spec.Args))
			if err != nil {
				t.Fatal(err)
			}
			result, err := Call(spec.Op, args)
			if spec.Error != "" {
				if err == nil || !strings.Contains(err.Error(), spec.Error) {
					t.Fatalf("error = %v, want one containing %q", err, spec.Error)
				}
				return
			}
			if err != nil {
				t.Fatalf("Call: %v", err)
			}
			var got any
			if err := json.Unmarshal(result, &got); err != nil {
				t.Fatal(err)
			}
			if *update {
				write(t, dir, path, data, got)
				return
			}
			want := resolve(t, dir, spec.Want)
			if !reflect.DeepEqual(got, roundTrip(t, want)) {
				gotText, _ := json.MarshalIndent(got, "", "  ")
				wantText, _ := json.MarshalIndent(want, "", "  ")
				t.Fatalf("got\n%s\nwant\n%s", gotText, wantText)
			}
		})
	}
}

// resolve replaces every "@name" string with the content of that file.
func resolve(t *testing.T, dir string, value any) any {
	switch value := value.(type) {
	case string:
		if name, ok := strings.CutPrefix(value, "@"); ok {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			return string(data)
		}
		return value
	case map[string]any:
		out := map[string]any{}
		for key, item := range value {
			out[key] = resolve(t, dir, item)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for index, item := range value {
			out[index] = resolve(t, dir, item)
		}
		return out
	}
	return value
}

// roundTrip gives a value the shapes json.Unmarshal produces, numbers included.
func roundTrip(t *testing.T, value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// write stores a result as the case's want. A note — any string of more
// than one line, the whole result or a field of it — goes into a Markdown file
// beside case.json, so a change to it reads as a diff of the note.
func write(t *testing.T, dir, path string, data []byte, got any) {
	got = toFiles(t, dir, "want", got)
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	spec["want"] = encoded
	out, err := json.MarshalIndent(orderedCase(spec), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func toFiles(t *testing.T, dir, name string, value any) any {
	switch value := value.(type) {
	case string:
		if !strings.Contains(value, "\n") {
			return value
		}
		if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
		return "@" + name + ".md"
	case map[string]any:
		out := map[string]any{}
		for key, item := range value {
			out[key] = toFiles(t, dir, name+"-"+key, item)
		}
		return out
	}
	return value
}

// orderedCase keeps op, args and want in reading order.
func orderedCase(spec map[string]json.RawMessage) any {
	type ordered struct {
		Op    json.RawMessage `json:"op"`
		Args  json.RawMessage `json:"args"`
		Want  json.RawMessage `json:"want,omitempty"`
		Error json.RawMessage `json:"error,omitempty"`
	}
	return ordered{Op: spec["op"], Args: spec["args"], Want: spec["want"], Error: spec["error"]}
}
