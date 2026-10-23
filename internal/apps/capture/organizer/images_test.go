package organizer

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/apps/capture/indexschema"
)

// photoCapture is beenHere with pictures in a real directory, since this
// Action reads them.
func photoCapture(t *testing.T, pictures map[string][]byte) Capture {
	t.Helper()
	capture := beenHere()
	capture.Path = t.TempDir()
	capture.Index.Attachments = []indexschema.Attachment{{Kind: "null"}, {Kind: "audio", Name: "memo.wav"}}
	for _, name := range []string{"a.jpg", "b.jpg", "c.png"} {
		data, ok := pictures[name]
		if !ok {
			continue
		}
		if err := os.WriteFile(filepath.Join(capture.Path, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
		// "Image" is how the shortcuts write it; the kind is read in any case.
		capture.Index.Attachments = append(capture.Index.Attachments, indexschema.Attachment{Kind: "Image", Name: name})
	}
	return capture
}

func encodedPicture(t *testing.T, width, height int, asPNG bool) []byte {
	t.Helper()
	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	var out bytes.Buffer
	var err error
	if asPNG {
		err = png.Encode(&out, picture)
	} else {
		err = jpeg.Encode(&out, picture, nil)
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func imagesPlan(t *testing.T, ctx Context) ActionPlan {
	t.Helper()
	recipe := Recipe{ID: "images", Actions: []ActionID{ActionDailyAppend}}
	plans := Build(ctx, recipe, recipe.Actions)
	if len(plans) != 1 {
		t.Fatalf("got %d plans", len(plans))
	}
	return plans[0]
}

func TestDailyImagesAreShrunkBesideTheNoteAndShown(t *testing.T) {
	capture := photoCapture(t, map[string][]byte{
		"a.jpg": encodedPicture(t, 300, 100, false),
		"b.jpg": encodedPicture(t, 50, 40, false),
	})
	ctx, vault := vaultContext(t, capture, "Lunch")
	ctx.Settings.ImageMaxSide = 120
	testEntry := testEntryTemplate + "\n{{- range .Images}}\n  - ![[{{.}}]]\n{{- end}}\n"
	if err := os.WriteFile(filepath.Join(ctx.Settings.TemplateDir, DailyEntryTemplate), []byte(testEntry), 0o600); err != nil {
		t.Fatal(err)
	}
	plan := imagesPlan(t, ctx)
	if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}

	folder := filepath.Join(vault, "00 Daily Log", "assets", "2026-09-09")
	first := filepath.Join(folder, "file-20260909213122900-1.jpg")
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width != 120 || config.Height != 40 {
		t.Fatalf("first picture is %dx%d, want 120x40", config.Width, config.Height)
	}
	if _, err := os.Stat(filepath.Join(folder, "file-20260909213122900-2.jpg")); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(folder); len(entries) != 2 {
		t.Fatalf("folder holds %d files, want the two pictures and nothing else", len(entries))
	}

	note, err := os.ReadFile(filepath.Join(vault, plan.Target))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"  - Lunch\n", "  - ![[file-20260909213122900-1.jpg]]\n", "  - ![[file-20260909213122900-2.jpg]]\n"} {
		if !strings.Contains(string(note), want) {
			t.Fatalf("note lacks %q:\n%s", want, note)
		}
	}

	// A second run finds the entry and writes nothing, pictures included.
	if err := os.Remove(first); err != nil {
		t.Fatal(err)
	}
	results := Execute(ctx, []ActionPlan{plan})
	if !Skipped(results) {
		t.Fatalf("second run = %+v", results)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("a second run wrote a picture again")
	}
}

func TestDailyImagesRefuseAPictureThatIsNotAJPEG(t *testing.T) {
	capture := photoCapture(t, map[string][]byte{
		"a.jpg": encodedPicture(t, 10, 10, false),
		"c.png": encodedPicture(t, 10, 10, true),
	})
	ctx, vault := vaultContext(t, capture, "")
	results := Execute(ctx, []ActionPlan{imagesPlan(t, ctx)})
	if Executed(results) || results[0].Err == nil || !strings.Contains(results[0].Err.Error(), "c.png") {
		t.Fatalf("results = %+v", results)
	}
	// Refused whole: not even the JPEG before it was written.
	if _, err := os.Stat(filepath.Join(vault, "00 Daily Log")); !os.IsNotExist(err) {
		t.Fatal("something was written for a refused Capture")
	}
}

func TestDailyImagesKeepAPictureAnEarlierRunLeft(t *testing.T) {
	capture := photoCapture(t, map[string][]byte{"a.jpg": encodedPicture(t, 10, 10, false)})
	ctx, vault := vaultContext(t, capture, "")
	earlier := filepath.Join(vault, "00 Daily Log", "assets", "2026-09-09", "file-20260909213122900-1.jpg")
	if err := os.MkdirAll(filepath.Dir(earlier), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(earlier, []byte("earlier"), 0o600); err != nil {
		t.Fatal(err)
	}
	if results := Execute(ctx, []ActionPlan{imagesPlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	if data, _ := os.ReadFile(earlier); string(data) != "earlier" {
		t.Fatal("a picture already in place was replaced")
	}
}

func TestDailyEntryWithoutPicturesIgnoresTheFolder(t *testing.T) {
	ctx, vault := vaultContext(t, photoCapture(t, nil), "Just words")
	// A folder that would be refused is never asked about without a picture.
	ctx.Settings.ImageFolder = "../../../outside"
	plan := imagesPlan(t, ctx)
	if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(filepath.Join(vault, "00 Daily Log", "assets")); !os.IsNotExist(err) {
		t.Fatal("a picture folder was made for a Capture with no pictures")
	}
}

func TestImageFolderFollowsItsTemplateAndStaysInTheVault(t *testing.T) {
	ctx := NewContext(beenHere(), nil).WithSettings(testVaultSettings(t, t.TempDir()))
	day, err := captureDay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ctx.Settings.ImageFolder = "../Pictures/{{.Year}}-{{.Month}}"
	folder, err := imageFolder(ctx, "00 Daily Log/2026/2026-09-09.md", day)
	if err != nil || folder != "00 Daily Log/Pictures/2026-09" {
		t.Fatalf("folder = %q, %v", folder, err)
	}
	ctx.Settings.ImageFolder = "../../../outside"
	if _, err := imageFolder(ctx, "00 Daily Log/2026/2026-09-09.md", day); err == nil {
		t.Fatal("a folder outside the vault was accepted")
	}
}

// A timeline shows the Capture's pictures too, written beside its own note the
// way the daily note's are.
func TestTimelineImagesAreWrittenBesideTheNoteAndShown(t *testing.T) {
	vault := t.TempDir()
	capture := photoCapture(t, map[string][]byte{"a.jpg": encodedPicture(t, 50, 40, false)})
	settings := locationSettings(t, vault)
	writeLocationTemplate(t, settings.TemplateDir, "- {{.Content}} {{.ID}}\n{{- range .Images}}\n    - ![[{{.}}]]\n{{- end}}")
	ctx := NewContext(capture, map[FieldID]any{FieldContent: "沙滩"}).WithSettings(settings)
	plan := locationPlan(t, ctx)
	if results := Execute(ctx, []ActionPlan{plan}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(filepath.Join(vault, "88 Inbox", "assets", "06 Locations", "file-20260909213122900-1.jpg")); err != nil {
		t.Fatal(err)
	}
	note, err := os.ReadFile(filepath.Join(vault, plan.Target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(note), "    - ![[file-20260909213122900-1.jpg]]") {
		t.Fatalf("note does not show the picture:\n%s", note)
	}
}

// A year leaving a timeline for its archive takes its pictures along, into the
// folder beside the archive, where a Capture from that year organized later
// puts its own.
func TestTimelineArchiveMovesTheYearsPictures(t *testing.T) {
	vault := t.TempDir()
	settings := locationSettings(t, vault)
	note := filepath.Join(vault, "88 Inbox/06 Locations.md")
	old := filepath.Join(vault, "88 Inbox/assets/06 Locations/file-20251231230000000-1.jpg")
	kept := filepath.Join(vault, "88 Inbox/assets/06 Locations/file-20260102100000000-1.jpg")
	pasted := filepath.Join(vault, "88 Inbox/assets/06 Locations/Pasted image.png")
	for _, file := range []string{old, kept, pasted} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("jpeg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	existing := "- *2026-01-02 周五*\n- `10:00:00 +10` · 今年 ^dgs-a\n    - ![[file-20260102100000000-1.jpg]]\n" +
		"- *2025-12-31 周三*\n- `23:00:00 +10` · 去年 ^dgs-b\n    - ![[file-20251231230000000-1.jpg|200]]\n    - ![[Pasted image.png]]\n"
	if err := os.WriteFile(note, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := NewContext(beenHere(), map[FieldID]any{FieldContent: "今天"}).WithSettings(settings)
	if results := Execute(ctx, []ActionPlan{locationPlan(t, ctx)}); !Executed(results) {
		t.Fatalf("results = %+v", results)
	}
	if _, err := os.Stat(filepath.Join(vault, "88 Inbox/06 Locations/assets/2025/file-20251231230000000-1.jpg")); err != nil {
		t.Fatalf("the year's picture did not move beside its archive: %v", err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatal("the year's picture is still beside the running note")
	}
	// A pasted picture, with no day in its name, goes along with its year.
	if _, err := os.Stat(filepath.Join(vault, "88 Inbox/06 Locations/assets/2025/Pasted image.png")); err != nil {
		t.Fatalf("a pasted picture did not move with its year: %v", err)
	}
	if _, err := os.Stat(kept); err != nil {
		t.Fatalf("a picture of the current year moved: %v", err)
	}
}
