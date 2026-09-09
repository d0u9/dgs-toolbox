package indexschema

import (
	"strings"
	"testing"
)

const validIndex = `{
  "schema": "v1",
  "source": {
    "app": "Shortcut",
    "device": {
      "os": "iOS",
      "systemVesion": "26.4.2",
      "name": "Yak’s iPhone 15 Pro"
    }
  },
  "position": {
    "altitude": 93.2716060213753,
    "longitude": 151.0819795403514,
    "latitude": -33.76910836568016,
    "address": "28 Cambridge St\nEpping NSW 2121\nAustralia"
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
  "createdAt": "2026-09-09T16:34:35.556+10:00",
  "type": "photo_note",
  "dir": "001 Inbox/20260909_163435_556_+1000-photo_note"
}`

func TestValidateAcceptsV1Example(t *testing.T) {
	if err := Validate(strings.NewReader(validIndex)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAcceptsStructuredLocationWithoutPayload(t *testing.T) {
	index := `{"schema":"v1","source":{"app":"Shortcut","device":{"os":"iOS","systemVesion":"26.4.2","name":"Phone"}},"position":{"city":"Epping","longitude":151.0819795403514,"country":"Australia","region ":"NSW","locality":"28 Cambridge St","latitude":-33.76910836568016,"altitude":95.2430086517334},"id":"capture-1","isDone":"true","attachments":[{"kind":"null"}],"createdAt":"2026-09-09T18:47:10.932+10:00","type":"been_here","dir":"001 Inbox/been_here"}`
	if err := Validate(strings.NewReader(index)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsSchemaViolations(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
	}{
		{name: "wrong schema", old: `"schema": "v1"`, new: `"schema": "v2"`},
		{name: "boolean isDone", old: `"isDone": "true"`, new: `"isDone": true`},
		{name: "payload is not object", old: `"payload": {`, new: `"payload": [`},
		{name: "latitude outside range", old: `"latitude": -33.76910836568016`, new: `"latitude": -133.0`},
		{name: "address is not string", old: `"address": "28 Cambridge St\nEpping NSW 2121\nAustralia"`, new: `"address": 28`},
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
