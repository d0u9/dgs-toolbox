package sidecar_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"dgs-toolbox/internal/box"
	"dgs-toolbox/internal/box/lifecycle"
	"dgs-toolbox/internal/box/money"
	"dgs-toolbox/internal/box/sidecar"
)

func full() sidecar.File {
	return sidecar.File{
		Version:          sidecar.Version,
		Digest:           "sha256:a1b2c3d4e5f60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
		Size:             4812390,
		Kind:             sidecar.KindPDF,
		OriginalFilename: "Scan_0012.pdf",
		IngestedAt:       "2026-09-22T14:30:00+10:00",
		Type:             "travel",
		Reviewed:         false,
		Description:      "Haneda → Sydney boarding pass",
		Tags:             []string{"japan-2019"},
		EventDate:        box.Date{Year: 2019, Month: 3, Day: 11},
		EventZone:        "Asia/Tokyo",
		ExpiresAt:        box.Date{Year: 2019, Month: 6, Day: 9},
		Total:            money.Amount{Minor: 12350, Currency: "AUD"},
		Pages:            1,
		PageSize:         "A4",
		Producer:         "ScanSnap Manager",
		ScanCreatedAt:    "2019-03-11T20:30:00+09:00",
		Group:            "8f2a1c",
	}
}

func TestRoundTripKeepsEveryField(t *testing.T) {
	want := full()
	data, err := sidecar.Encode(want)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := sidecar.Decode(data, "test")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Digest != want.Digest || got.Size != want.Size || got.Kind != want.Kind {
		t.Errorf("bytes fields: got %+v", got)
	}
	if got.OriginalFilename != want.OriginalFilename || got.IngestedAt != want.IngestedAt {
		t.Errorf("intake fields: got %+v", got)
	}
	if got.Type != want.Type || got.Description != want.Description || len(got.Tags) != 1 || got.Tags[0] != "japan-2019" {
		t.Errorf("described fields: got %+v", got)
	}
	if got.EventDate != want.EventDate || got.EventZone != want.EventZone || got.ExpiresAt != want.ExpiresAt {
		t.Errorf("dates: got %+v", got)
	}
	if got.Total != want.Total {
		t.Errorf("total: got %v want %v", got.Total, want.Total)
	}
	if got.Pages != want.Pages || got.PageSize != want.PageSize || got.Producer != want.Producer || got.ScanCreatedAt != want.ScanCreatedAt {
		t.Errorf("derived fields: got %+v", got)
	}
	if got.Group != want.Group {
		t.Errorf("group: got %q", got.Group)
	}
}

// A document with no total has no total, and a scan nobody has dated has no
// date. Neither may come back as a zero that reads like a recorded answer.
func TestAbsentIsNotZero(t *testing.T) {
	data, err := sidecar.Encode(sidecar.File{Digest: "sha256:aa", Type: "unsorted"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	text := string(data)
	for _, key := range []string{"total_minor", "currency", "event_date", "expires_at"} {
		if strings.Contains(text, key) {
			t.Errorf("wrote %s for a scan that has none:\n%s", key, text)
		}
	}
	got, err := sidecar.Decode(data, "test")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Total.Valid() {
		t.Errorf("total came back as %v", got.Total)
	}
	if !got.EventDate.Zero() || !got.ExpiresAt.Zero() {
		t.Errorf("dates came back as %v / %v", got.EventDate, got.ExpiresAt)
	}
}

// An emptied expiry is what makes a scan permanent. If the flag were not
// stored, the type's default lifetime would creep back on the next read.
func TestClearedExpirySurvives(t *testing.T) {
	data, err := sidecar.Encode(sidecar.File{
		Digest:        "sha256:aa",
		Type:          "ticket",
		EventDate:     box.Date{Year: 2019, Month: 3, Day: 11},
		ExpiryCleared: true,
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got, err := sidecar.Decode(data, "test")
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.ExpiryCleared {
		t.Fatal("expiry_cleared was lost")
	}
	if _, has := lifecycle.Expiry(got.Scan()); has {
		t.Error("a cleared expiry still produced one")
	}
}

func TestDecodeRefusals(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"offset zone", "version: 1\nevent_tz: \"+09:00\"\n", "event_tz"},
		{"newer version", "version: 99\n", "newer than this build reads"},
		{"unknown key", "version: 1\nexpiry: 2019-01-01\n", "expiry"},
		{"amount without currency", "version: 1\ntotal_minor: 100\n", "without currency"},
		{"currency without amount", "version: 1\ncurrency: AUD\n", "without total_minor"},
		{"unknown currency", "version: 1\ntotal_minor: 100\ncurrency: XYZ\n", "XYZ"},
		{"date with a time on it", "version: 1\nevent_date: 2019-03-11T10:00:00Z\n", "event_date"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := sidecar.Decode([]byte(testCase.yaml), "test")
			if err == nil {
				t.Fatalf("accepted %q", testCase.yaml)
			}
			if !strings.Contains(err.Error(), testCase.want) {
				t.Errorf("error %q does not mention %q", err, testCase.want)
			}
		})
	}
}

func TestNameComesFromTheDigestAlone(t *testing.T) {
	digest := "sha256:a1b2c3d4e5f6"
	if got := sidecar.NameFor(digest, sidecar.DefaultPrefix); got != "a1b2c3d4.dgs-doc.yaml" {
		t.Errorf("name: got %q", got)
	}
	// A collision inside one directory extends the prefix; the sidecar follows
	// the scan's own name.
	if got := sidecar.NameFor(digest, 10); got != "a1b2c3d4e5.dgs-doc.yaml" {
		t.Errorf("longer prefix: got %q", got)
	}
	if strings.HasPrefix(sidecar.NameFor(digest, 8), ".") {
		t.Error("a sidecar must not be hidden from Finder")
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	want := full()
	path := sidecar.PathFor(dir, want.Digest, sidecar.DefaultPrefix)
	if err := sidecar.Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := sidecar.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Digest != want.Digest || got.Total != want.Total || got.EventDate != want.EventDate {
		t.Errorf("round trip through disk lost data: %+v", got)
	}
	// Nothing is left behind under the temporary name.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Errorf("directory holds %v", names)
	}
}

func TestSaveReplacesInOneStep(t *testing.T) {
	dir := t.TempDir()
	file := full()
	path := sidecar.PathFor(dir, file.Digest, sidecar.DefaultPrefix)
	if err := sidecar.Save(path, file); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// Reclassifying is one sidecar rewrite and no file move.
	file.Type = "invoice"
	file.Reviewed = true
	if err := sidecar.Save(path, file); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, err := sidecar.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Type != "invoice" || !got.Reviewed {
		t.Errorf("rewrite did not take: %+v", got)
	}
}

func TestLoadMissingIsNotFound(t *testing.T) {
	_, err := sidecar.Load(filepath.Join(t.TempDir(), "nothing.dgs-doc.yaml"))
	if !errors.Is(err, sidecar.ErrNotFound) {
		t.Fatalf("got %v, want ErrNotFound", err)
	}
}
