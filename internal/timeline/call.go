package timeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dgs-toolbox/internal/entrytemplate"
)

// APIVersion names the shape of Call: its operations, their arguments and
// their results. A caller built against another version must not use this
// one, so a change to an operation that is there, or its removal, bumps it.
// A new operation does not: a caller that does not know it never calls it,
// and a plugin and the module it reads are installed together.
const APIVersion = 1

// Call runs one operation by name, with its arguments and result as JSON. It
// is the whole surface the WebAssembly build exposes, and what the shared test
// cases drive, so a plugin and dgs are checked against the same answers.
//
//	marker  {date, weekday}                 → string
//	insert  {content, date, marker, entry}  → {content, inserted}
//	archive {content, current}              → {remaining, moved: [{year, text}]}
//	merge   {archive, header, year}         → string
//	header  {header, year}                  → string, a new archive's start
//	parse   {content}                       → Document
//	definitions {text}                      → [Definition], the timelines file read
//	tidy    TidyInput                       → TidyResult
//	pictures {content}                      → [string], the pictures it embeds
//	builtin {name}                          → string, a compiled-in template, or ""
//	note    {name, template, input, mappings} → string, a day's note rendered;
//	                                          no template is the compiled-in one
//	entry   {name, template, input, mappings} → string, one entry rendered;
//	                                          no template is the compiled-in one
//	section {content, section, entry}       → string, a daily note with the entry
//	                                          at the end of its section
func Call(op string, args []byte) (result []byte, err error) {
	switch op {
	case "marker":
		var in struct{ Date, Weekday string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		day, err := time.Parse(DateLayout, in.Date)
		if err != nil {
			return nil, fmt.Errorf("marker: %w", err)
		}
		return json.Marshal(Marker(day, in.Weekday))
	case "insert":
		var in struct{ Content, Date, Marker, Entry string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		if _, err := time.Parse(DateLayout, in.Date); err != nil {
			return nil, fmt.Errorf("insert: %w", err)
		}
		content, inserted := Insert(in.Content, in.Date, in.Marker, in.Entry)
		return json.Marshal(map[string]any{"content": content, "inserted": inserted})
	case "archive":
		var in struct{ Content, Current string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		remaining, moved := Archive(in.Content, in.Current)
		if moved == nil {
			moved = []Year{}
		}
		return json.Marshal(map[string]any{"remaining": remaining, "moved": moved})
	case "merge":
		var in struct {
			Archive string
			Header  Header
			Year    Year
		}
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		return json.Marshal(Merge(in.Archive, in.Header, in.Year))
	case "header":
		var in struct {
			Header Header
			Year   string
		}
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		return json.Marshal(in.Header.Render(in.Year))
	case "parse":
		var in struct{ Content string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		return json.Marshal(Parse(in.Content))
	case "definitions":
		var in struct{ Text string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		definitions, err := ParseDefinitions([]byte(in.Text))
		if err != nil {
			return nil, err
		}
		return json.Marshal(definitions)
	case "tidy":
		var in TidyInput
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		result, err := Tidy(in)
		if err != nil {
			return nil, err
		}
		return json.Marshal(result)
	case "pictures":
		var in struct{ Content string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		return json.Marshal(Pictures(in.Content))
	case "entry":
		var in struct {
			Name     string
			Template string
			Input    entrytemplate.Input
			Mappings entrytemplate.Mappings
		}
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Template) == "" {
			// A vault without a template of this name gets the compiled-in
			// one, as dgs capture does.
			builtin, ok := entrytemplate.Builtin(in.Name)
			if !ok {
				return nil, fmt.Errorf("entry: no template named %q", in.Name)
			}
			in.Template = builtin
		}
		lines, err := entrytemplate.Render(in.Name, in.Template, in.Input, in.Mappings)
		if err != nil {
			return nil, fmt.Errorf("entry: %w", err)
		}
		return json.Marshal(strings.Join(lines, "\n"))
	case "note":
		var in struct {
			Name     string
			Template string
			Input    entrytemplate.NoteInput
			Mappings entrytemplate.Mappings
		}
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		if in.Name == "" {
			in.Name = entrytemplate.DailyNote
		}
		if strings.TrimSpace(in.Template) == "" {
			builtin, ok := entrytemplate.Builtin(in.Name)
			if !ok {
				return nil, fmt.Errorf("note: no template named %q", in.Name)
			}
			in.Template = builtin
		}
		rendered, err := entrytemplate.RenderNote(in.Name, in.Template, in.Input, in.Mappings)
		if err != nil {
			return nil, fmt.Errorf("note: %w", err)
		}
		return json.Marshal(rendered)
	case "builtin":
		var in struct{ Name string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		text, _ := entrytemplate.Builtin(in.Name)
		return json.Marshal(text)
	case "section":
		var in struct{ Content, Section, Entry string }
		if err := decode(args, &in); err != nil {
			return nil, err
		}
		if strings.TrimSpace(in.Section) == "" {
			return nil, errors.New("section: no section heading")
		}
		entry := strings.Split(strings.Trim(in.Entry, "\n"), "\n")
		if strings.TrimSpace(in.Entry) == "" {
			return nil, errors.New("section: nothing to insert")
		}
		return json.Marshal(InsertUnderSection(in.Content, in.Section, entry))
	}
	return nil, fmt.Errorf("unknown operation %q", op)
}

func decode(args []byte, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	return nil
}
