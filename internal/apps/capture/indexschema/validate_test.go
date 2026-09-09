package indexschema

import (
	"strings"
	"testing"
)

const validIndex = `{
  "schema": "v1",
  "source": {
    "app": "Shortcut",
    "workflow": "photo_note",
    "device": {
      "os": "iOS",
      "systemVersion": "26.4.2",
      "name": "Yak’s iPhone 15 Pro"
    }
  },
  "coordinates": {
    "altitude": 93.2716060213753,
    "longitude": 151.0819795403514,
    "latitude": -33.76910836568016
  },
  "place": {
    "address": "28 Cambridge St\nEpping NSW 2121\nAustralia",
    "city": "Sydney",
    "region": "NSW",
    "country": "Australia"
  },
  "id": "20260909163435556-2376",
  "isDone": "true",
  "payload": {
    "address": "28 Cambridge St\nEpping NSW 2121\nAustralia",
    "text": "棒",
    "img": "163440087-6979.jpg"
  },
  "attachments": [
    {"kind": "null"},
    {
      "kind": "Image",
      "sha256": "252dde498a743ef7c3611a3c4e5571a953ed1f1f63832676536613728556966c",
      "name": "163440087-6979.jpg"
    }
  ],
  "createdAt": "2026-09-09T16:34:35.556+10:00"
}`

func TestValidateAcceptsV1Example(t *testing.T) {
	if err := Validate(strings.NewReader(validIndex)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptsStructuredLocationWithoutPayload(t *testing.T) {
	index := `{"schema":"v1","source":{"app":"Shortcut","workflow":"note","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"coordinates":{"longitude":151.0819795403514,"latitude":-33.76910836568016,"altitude":95.2430086517334},"place":{"city":"Epping","country":"Australia","region":"NSW","locality":"28 Cambridge St"},"id":"capture-1","isDone":"true","attachments":[{"kind":"null"}],"createdAt":"2026-09-09T18:47:10.932+10:00"}`
	if err := Validate(strings.NewReader(index)); err != nil {
		t.Fatal(err)
	}
}

// The schema describes what the Capture sessions read. Producers may write
// other fields, such as isDone or dir, and those are neither required nor
// rejected.
func TestValidateIgnoresUndescribedFields(t *testing.T) {
	extra := strings.Replace(validIndex, `  "id":`, `  "isDone": true,
  "dir": "001 Inbox/been_here",
  "id":`, 1)
	if err := Validate(strings.NewReader(extra)); err != nil {
		t.Fatalf("undescribed producer fields were rejected: %v", err)
	}
	if err := Validate(strings.NewReader(validIndex)); err != nil {
		t.Fatalf("an index without them was rejected: %v", err)
	}
}

// A Capture may have coordinates, a place, both, or neither.
func TestValidateAcceptsCapturesWithoutLocation(t *testing.T) {
	base := `{"schema":"v1","source":{"app":"Shortcut","workflow":"note","device":{"os":"iOS","systemVersion":"26.4.2","name":"Phone"}},"id":"capture-1","createdAt":"2026-09-09T18:47:10.932+10:00","type":"note"`
	cases := map[string]string{
		"neither":       base + `}`,
		"position only": base + `,"coordinates":{"altitude":12,"longitude":151.2093,"latitude":-33.8688}}`,
		"place only":    base + `,"place":{"city":"Sydney","country":"Australia"}}`,
		"both":          base + `,"coordinates":{"altitude":12,"longitude":151.2093,"latitude":-33.8688},"place":{"city":"Sydney","region":"NSW","country":"Australia"}}`,
	}
	for name, index := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(strings.NewReader(index)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidateRejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "wrong schema", old: `"schema": "v1"`, new: `"schema": "v2"`},
		{name: "payload is not object", old: `"payload": {`, new: `"payload": [`},
		{name: "latitude outside range", old: `"latitude": -33.76910836568016`, new: `"latitude": -133.0`},
		{name: "place address is not string", old: `"address": "28 Cambridge St\nEpping NSW 2121\nAustralia"`, new: `"address": 28`},
		{name: "place is not object", old: `"place": {`, new: `"place": [`},
		{name: "descriptive field inside coordinates", old: `"latitude": -33.76910836568016`, new: `"latitude": -33.76910836568016, "city": "Sydney"`},
		{name: "source without workflow", old: `"workflow": "photo_note",`, new: ``},
		{name: "device with the old systemVesion spelling", old: `"systemVersion": "26.4.2"`, new: `"systemVesion": "26.4.2"`},
		{name: "unknown place field", old: `"city": "Sydney"`, new: `"suburb": "Sydney"`},
		{name: "coordinates without latitude", old: `"latitude": -33.76910836568016`, new: `"elevation": -33.76910836568016`},
		{name: "attachment missing kind", old: `{"kind": "null"}`, new: `{}`},
		{name: "invalid date-time", old: `"createdAt": "2026-09-09T16:34:35.556+10:00"`, new: `"createdAt": "yesterday"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := strings.Replace(validIndex, test.old, test.new, 1)
			if err := Validate(strings.NewReader(candidate)); err == nil {
				t.Fatal("invalid index was accepted")
			}
		})
	}
}
