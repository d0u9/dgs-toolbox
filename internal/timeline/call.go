package timeline

import (
	"encoding/json"
	"fmt"
	"time"
)

// APIVersion names the shape of Call: its operations, their arguments and
// their results. A caller built against another version must not use this
// one, so any change to that shape bumps it.
const APIVersion = 2

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
	}
	return nil, fmt.Errorf("unknown operation %q", op)
}

func decode(args []byte, into any) error {
	if err := json.Unmarshal(args, into); err != nil {
		return fmt.Errorf("arguments: %w", err)
	}
	return nil
}
