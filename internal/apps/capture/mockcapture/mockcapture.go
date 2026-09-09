// Package mockcapture generates a Capture root of mock v1 Captures. The
// generated tree is a fixture for Capture Scan and Route; testdata is not
// tracked, so regenerate it with `go run ./cmd/mockcapture`.
package mockcapture

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Write replaces root with a freshly generated set of mock Captures.
func Write(root string) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("generate mock captures: %v", recovered)
		}
	}()

	must(os.RemoveAll(root))
	must(os.MkdirAll(root, 0o755))
	write(filepath.Join(root, "README.txt"), []byte("Mock Capture root for the Capture Scan and Route TUIs.\nEvery directory here is generated; see docs/apps/capture/index-v1.schema.json.\n"))

	// 1 been_here with a text attachment: the workflow Route organizes first
	note := capture{Dir: "2026-09-01-note-osaka", Workflow: "been_here", App: "Shortcut", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-01-osaka", Created: "2026-09-01T09:12:04.118+09:00",
		Lat: 34.66939, Lon: 135.50122, Alt: 12.4,
		Place:   map[string]string{"locality": "Chuo", "city": "Osaka", "region": "Osaka Prefecture", "country": "Japan"},
		Payload: map[string]any{"note": "Coffee stand under the bridge. Ask about the single-origin.", "words": 10}}
	note.attach("text", "note.txt", []byte("Coffee stand under the bridge.\nAsk about the single-origin.\n"))
	note.build(root)

	// 2 photo with jpeg + png thumbnail
	photo := capture{Dir: "2026-09-02-photo-shinjuku", Workflow: "photo_note", App: "Camera", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-02-shinjuku", Created: "2026-09-02T19:44:51.902+09:00",
		Lat: 35.69384, Lon: 139.70355, Alt: 41,
		Place:   map[string]string{"locality": "Shinjuku", "city": "Tokyo", "region": "Tokyo", "country": "Japan"},
		Payload: map[string]any{"lens": "26mm", "iso": 400}}
	photo.attach("image", "street.jpg", jpegBytes(640, 400))
	photo.attach("image", "street-thumb.png", pngBytes(160, 100))
	photo.build(root)

	// 3 voice memo with a real WAV
	voice := capture{Dir: "2026-09-03-voice-memo", Workflow: "voice_memo", App: "Voice Memos", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-03-memo", Created: "2026-09-03T07:31:22.004+10:00",
		Lat: -33.86785, Lon: 151.20732, Alt: 8,
		Place:   map[string]string{"locality": "Sydney CBD", "city": "Sydney", "region": "New South Wales", "country": "Australia"},
		Payload: map[string]any{"transcript": "Remember to renew the toolbox certificate."}}
	voice.attach("audio", "memo.wav", wavBytes(1.5, 440))
	voice.build(root)

	// 4 the only Capture carrying undescribed producer fields
	extra := capture{Dir: "2026-09-04-extra-fields", Workflow: "been_here", App: "Shortcut", OS: "iOS", System: "18.6", Device: "Doug's iPad",
		ID: "cap-2026-09-04-extra-fields", Created: "2026-09-04T15:02:00.000+10:00",
		Lat: -37.81422, Lon: 144.96316, Alt: 31,
		Extra:   true,
		Place:   map[string]string{"locality": "Flinders Street", "city": "Melbourne", "region": "Victoria", "country": "Australia"},
		Payload: map[string]any{"note": "Producer fields the schema does not describe are ignored, not rejected."}}
	extra.build(root)

	// 5 quick_mark, and a place without a locality
	partial := capture{Dir: "2026-09-05-partial-place", Workflow: "quick_mark", App: "Shortcut", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-05-partial", Created: "2026-09-05T11:48:36.271+08:00",
		Lat: 22.31931, Lon: 114.16936, Alt: 19,
		Place:   map[string]string{"city": "Hong Kong", "region": "Kowloon", "country": "China"},
		Payload: map[string]any{"mark": "A place may fill only some of its fields."}}
	partial.build(root)

	// 6 ignored attachment kind plus a real one
	mixed := capture{Dir: "2026-09-06-null-attachment", Workflow: "photo_note", App: "Camera", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-06-mixed", Created: "2026-09-06T16:20:10.500+10:00",
		Lat: -27.46977, Lon: 153.02513, Alt: 27,
		Place:   map[string]string{"city": "Brisbane", "region": "Queensland", "country": "Australia"},
		Payload: map[string]any{"note": "The null attachment must not appear in the tree."}}
	mixed.attach("null", "ignored.bin", nil)
	mixed.attach("image", "river.png", pngBytes(320, 200))
	mixed.build(root)

	// 7 place without coordinates
	indoors := capture{Dir: "2026-09-07-place-only", Workflow: "been_here", App: "Shortcut", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-07-place-only", Created: "2026-09-07T13:15:44.000+10:00", NoCoordinates: true,
		Place:   map[string]string{"city": "Canberra", "region": "Australian Capital Territory", "country": "Australia"},
		Payload: map[string]any{"note": "Indoors, so the Shortcut recorded no coordinates."}}
	indoors.build(root)

	// 8 required fields only: no position, no place
	minimal := capture{Dir: "2026-09-08-minimal", Workflow: "note", App: "Shortcut", OS: "iOS", System: "26.4.2", Device: "Doug's iPhone",
		ID: "cap-2026-09-08-minimal", Created: "2026-09-08T21:05:00.000+10:00", NoCoordinates: true}
	minimal.build(root)

	// Rejected neighbours: Scan and Route must skip all three.
	must(os.MkdirAll(filepath.Join(root, "not-a-capture"), 0o755))
	write(filepath.Join(root, "not-a-capture", "notes.txt"), []byte("No index file, so this directory is not a Capture.\n"))
	must(os.MkdirAll(filepath.Join(root, "invalid-schema"), 0o755))
	write(filepath.Join(root, "invalid-schema", "index.json"), []byte("{\n  \"schema\": \"v2\",\n  \"id\": \"cap-invalid-schema\"\n}\n"))
	must(os.MkdirAll(filepath.Join(root, "invalid-json"), 0o755))
	write(filepath.Join(root, "invalid-json", "index.json"), []byte("{ \"schema\": \"v1\", truncated\n"))
	return nil
}

type attachment struct {
	Kind   string `json:"kind"`
	Name   string `json:"name,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	data   []byte
}

type capture struct {
	Dir, Workflow, App, OS, System, Device, ID, Created string
	Lat, Lon, Alt                                       float64
	NoCoordinates                                       bool
	Extra                                               bool
	Place                                               map[string]string
	Payload                                             map[string]any
	attachments                                         []attachment
}

func (c *capture) attach(kind, name string, data []byte) {
	item := attachment{Kind: kind, Name: name, data: data}
	if data != nil {
		sum := sha256.Sum256(data)
		item.SHA256 = hex.EncodeToString(sum[:])
	}
	c.attachments = append(c.attachments, item)
}

func (c capture) build(root string) {
	dir := filepath.Join(root, c.Dir)
	must(os.MkdirAll(dir, 0o755))
	index := map[string]any{
		"schema":    "v1",
		"source":    map[string]any{"app": c.App, "workflow": c.Workflow, "device": map[string]any{"os": c.OS, "systemVersion": c.System, "name": c.Device}},
		"id":        c.ID,
		"createdAt": c.Created,
	}
	if !c.NoCoordinates {
		index["coordinates"] = map[string]any{"altitude": c.Alt, "longitude": c.Lon, "latitude": c.Lat}
	}
	if len(c.Place) > 0 {
		place := map[string]any{}
		for key, value := range c.Place {
			place[key] = value
		}
		index["place"] = place
	}
	// One Capture carries extra producer fields the schema does not describe,
	// proving the Capture sessions accept and ignore them.
	if c.Extra {
		index["isDone"] = "true"
		index["dir"] = c.Dir
	}
	if c.Payload != nil {
		index["payload"] = c.Payload
	}
	if len(c.attachments) > 0 {
		index["attachments"] = c.attachments
	}
	data, err := json.MarshalIndent(index, "", "  ")
	must(err)
	write(filepath.Join(dir, "index.json"), append(data, '\n'))
	for _, item := range c.attachments {
		if item.data == nil {
			continue
		}
		write(filepath.Join(dir, item.Name), item.data)
	}
}

func pngBytes(width, height int) []byte {
	var buffer bytes.Buffer
	must(png.Encode(&buffer, gradient(width, height)))
	return buffer.Bytes()
}

func jpegBytes(width, height int) []byte {
	var buffer bytes.Buffer
	must(jpeg.Encode(&buffer, gradient(width, height), &jpeg.Options{Quality: 85}))
	return buffer.Bytes()
}

func gradient(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(255 * x / width),
				G: uint8(255 * y / height),
				B: uint8(160 - 120*((x+y)%2)),
				A: 255,
			})
		}
	}
	return img
}

func wavBytes(seconds float64, hertz float64) []byte {
	const rate = 8000
	samples := int(seconds * rate)
	var body bytes.Buffer
	for i := 0; i < samples; i++ {
		value := int16(6000 * math.Sin(2*math.Pi*hertz*float64(i)/rate))
		must(binary.Write(&body, binary.LittleEndian, value))
	}
	var file bytes.Buffer
	file.WriteString("RIFF")
	must(binary.Write(&file, binary.LittleEndian, uint32(36+body.Len())))
	file.WriteString("WAVEfmt ")
	must(binary.Write(&file, binary.LittleEndian, uint32(16)))
	must(binary.Write(&file, binary.LittleEndian, uint16(1)))
	must(binary.Write(&file, binary.LittleEndian, uint16(1)))
	must(binary.Write(&file, binary.LittleEndian, uint32(rate)))
	must(binary.Write(&file, binary.LittleEndian, uint32(rate*2)))
	must(binary.Write(&file, binary.LittleEndian, uint16(2)))
	must(binary.Write(&file, binary.LittleEndian, uint16(16)))
	file.WriteString("data")
	must(binary.Write(&file, binary.LittleEndian, uint32(body.Len())))
	file.Write(body.Bytes())
	return file.Bytes()
}

func write(path string, data []byte) { must(os.WriteFile(path, data, 0o644)) }

func must(err error) {
	if err != nil {
		panic(err)
	}
}
