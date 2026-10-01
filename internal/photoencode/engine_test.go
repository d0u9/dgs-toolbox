package photoencode

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, e := os.Create(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = png.Encode(f, img); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
}
func TestEncodePublication(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "source.png")
	dst := filepath.Join(root, "out", "result.jpg")
	img := image.NewNRGBA(image.Rect(0, 0, 40, 20))
	for y := 0; y < 20; y++ {
		for x := 0; x < 40; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
		}
	}
	writePNG(t, src, img)
	original, _ := os.ReadFile(src)
	o := DefaultOptions()
	o.Size = 10
	r := Encode(context.Background(), Job{Source: src, Destination: dst}, o)
	if !r.Published || r.Error != "" || r.Width != 10 || r.Height != 5 {
		t.Fatalf("result: %+v", r)
	}
	data, e := os.ReadFile(dst)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = jpeg.Decode(bytes.NewReader(data)); e != nil {
		t.Fatal(e)
	}
	if got, _ := os.ReadFile(src); !bytes.Equal(got, original) {
		t.Fatal("source changed")
	}
	r = Encode(context.Background(), Job{Source: src, Destination: dst}, o)
	if r.Published || r.Error == "" {
		t.Fatalf("overwritten: %+v", r)
	}
	if got, _ := os.ReadFile(dst); !bytes.Equal(got, data) {
		t.Fatal("destination changed")
	}
	parts, _ := filepath.Glob(filepath.Join(root, "out", "*.dgs-part"))
	if len(parts) != 0 {
		t.Fatal(parts)
	}
}
func TestCancellationAndCorruptionPublishNothing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "bad.jpg")
	dst := filepath.Join(root, "out", "result.jpg")
	os.WriteFile(src, []byte("bad image"), 0600)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{ctx, context.Background()} {
		r := Encode(ctx, Job{Source: src, Destination: dst}, DefaultOptions())
		if r.Published || r.Error == "" {
			t.Fatal(r)
		}
		if _, e := os.Stat(filepath.Dir(dst)); !os.IsNotExist(e) {
			t.Fatal("invalid input created output directory")
		}
	}
}
func TestPlanIsReadOnlyAndDetectsCollisions(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	os.Mkdir(src, 0700)
	os.WriteFile(filepath.Join(src, "same.jpg"), nil, 0600)
	os.WriteFile(filepath.Join(src, "same.png"), nil, 0600)
	os.WriteFile(filepath.Join(src, "scan.tiff"), nil, 0600)
	dst := filepath.Join(root, "out")
	jobs, e := Plan(src, dst, DefaultOptions())
	if e != nil {
		t.Fatal(e)
	}
	if len(jobs) != 3 {
		t.Fatal(jobs)
	}
	for _, j := range jobs {
		if j.Problem == "" {
			t.Fatal(j)
		}
	}
	if _, e = os.Stat(dst); !os.IsNotExist(e) {
		t.Fatal("plan wrote destination")
	}
	if !strings.Contains(jobs[2].Problem, "sips '") {
		t.Fatal(jobs)
	}
}
func TestRecursivePlanExcludesDestination(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "nested"), 0700)
	os.Mkdir(filepath.Join(root, "out"), 0700)
	os.WriteFile(filepath.Join(root, "one.png"), nil, 0600)
	os.WriteFile(filepath.Join(root, "nested", "two.png"), nil, 0600)
	os.WriteFile(filepath.Join(root, "out", "ignore.png"), nil, 0600)
	o := DefaultOptions()
	o.Recursive = true
	jobs, e := Plan(root, filepath.Join(root, "out"), o)
	if e != nil || len(jobs) != 2 {
		t.Fatalf("%v %v", jobs, e)
	}
}
func TestTransformOrientationAndWhiteBackground(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetNRGBA(x, y, color.NRGBA{R: uint8(20 + x + 2*y), A: 255})
		}
	}
	expected := [][]uint8{{20, 21, 22, 23, 24, 25}, {21, 20, 23, 22, 25, 24}, {25, 24, 23, 22, 21, 20}, {24, 25, 22, 23, 20, 21}, {20, 22, 24, 21, 23, 25}, {24, 22, 20, 25, 23, 21}, {25, 23, 21, 24, 22, 20}, {21, 23, 25, 20, 22, 24}}
	for o := 1; o <= 8; o++ {
		got := Transform(src, o, 100, color.White)
		var actual []uint8
		for y := 0; y < got.Bounds().Dy(); y++ {
			for x := 0; x < got.Bounds().Dx(); x++ {
				actual = append(actual, got.NRGBAAt(x, y).R)
			}
		}
		if !bytes.Equal(actual, expected[o-1]) {
			t.Fatalf("orientation %d: %v", o, actual)
		}
	}
	transparent := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if c := Transform(transparent, 1, 800, color.White).NRGBAAt(0, 0); c != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatal(c)
	}
}
func TestICCAndCaptureDateRoundTrip(t *testing.T) {
	profile := make([]byte, 70000)
	binary.BigEndian.PutUint32(profile, uint32(len(profile)))
	copy(profile[16:], "RGB ")
	copy(profile[36:], "acsp")
	m := metadata{icc: profile, date: "2026:10:01 12:34:56", orientation: 1}
	var buf bytes.Buffer
	jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 3, 2)), nil)
	data, e := addMetadata(buf.Bytes(), m, false, 240, 3, 2)
	if e != nil {
		t.Fatal(e)
	}
	read, e := jpegMetadata(data)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Equal(read.icc, profile) || read.date != m.date {
		t.Fatalf("metadata differs: %+v", read)
	}
	if data[13] != 1 || binary.BigEndian.Uint16(data[14:]) != 240 || binary.BigEndian.Uint16(data[16:]) != 240 {
		t.Fatal("PPI was not written")
	}
}
func TestMalformedMetadataRejected(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("II"), []byte("II*\x00\xff\xff\xff\xff")} {
		m := metadata{exif: data}
		if len(data) > 0 && m.readExif() == nil {
			t.Fatal("accepted bad EXIF")
		}
	}
	for _, icc := range [][]byte{[]byte("bad"), make([]byte, 128)} {
		if checkICC(icc) == nil {
			t.Fatal("accepted bad ICC")
		}
	}
	if _, e := jpegMetadata([]byte{255, 216, 255, 226, 255, 255}); e == nil {
		t.Fatal("accepted truncated JPEG")
	}
}
func TestTIFFCommandQuotesPaths(t *testing.T) {
	s := TIFFCommand("/tmp/a'b $(secret).tiff", "/tmp/out.jpg", DefaultOptions())
	if !strings.Contains(s, "'\"'\"'") || strings.Contains(s, " -m ") {
		t.Fatal(s)
	}
}

func TestEXIFExportCorrectsDirectionDimensionsDensityAndRemovesGPS(t *testing.T) {
	ex := make([]byte, 150)
	copy(ex, "II")
	o := binary.LittleEndian
	o.PutUint16(ex[2:], 42)
	o.PutUint32(ex[4:], 8)
	o.PutUint16(ex[8:], 5)
	entry := func(at int, tag, typ uint16, count, value uint32) {
		o.PutUint16(ex[at:], tag)
		o.PutUint16(ex[at+2:], typ)
		o.PutUint32(ex[at+4:], count)
		o.PutUint32(ex[at+8:], value)
	}
	entry(10, 0x112, 3, 1, 6)
	entry(22, 0x100, 4, 1, 2)
	entry(34, 0x8769, 4, 1, 74)
	entry(46, 0x8825, 4, 1, 92)
	entry(58, 0x11a, 5, 1, 132)
	o.PutUint16(ex[74:], 1)
	entry(76, 0x9003, 2, 20, 112)
	o.PutUint16(ex[92:], 1)
	entry(94, 1, 2, 2, uint32('N'))
	copy(ex[112:], "2026:10:01 12:34:56\x00")
	o.PutUint32(ex[132:], 72)
	o.PutUint32(ex[136:], 1)
	copy(ex[140:], "PRIVATEGPS")
	m := metadata{exif: ex}
	if e := m.readExif(); e != nil {
		t.Fatal(e)
	}
	kept, e := m.outputExif(true, 42, 24, 240)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(kept, []byte("PRIVATEGPS")) {
		t.Fatal("preserved EXIF missing")
	}
	result := metadata{exif: kept[6:]}
	if e = result.readExif(); e != nil {
		t.Fatal(e)
	}
	if result.orientation != 1 || result.date != m.date {
		t.Fatal(result)
	}
	if o.Uint32(kept[6+30:]) != 42 || o.Uint32(kept[6+132:]) != 240 {
		t.Fatal("dimensions/density not corrected")
	}
	stripped, e := m.outputExif(false, 42, 24, 240)
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(stripped, []byte("PRIVATEGPS")) {
		t.Fatal("private GPS bytes leaked")
	}
	result = metadata{exif: stripped[6:]}
	if e = result.readExif(); e != nil || result.date != m.date {
		t.Fatalf("date lost: %v %+v", e, result)
	}
}
func FuzzMetadataFraming(f *testing.F) {
	f.Add([]byte("II*\x00\x08\x00\x00\x00\x00\x00\x00\x00\x00\x00"))
	f.Add([]byte{255, 216, 255, 226, 0, 2, 255, 217})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 100000 {
			t.Skip()
		}
		m := metadata{exif: bytes.Clone(data)}
		if m.readExif() == nil {
			_, _ = m.outputExif(true, 100, 50, 240)
		}
		_, _ = jpegMetadata(data)
		_, _ = pngMetadata(data)
	})
}

func TestConfigurableBackground(t *testing.T) {
	background, e := BackgroundColor("#123456")
	if e != nil {
		t.Fatal(e)
	}
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	got := Transform(src, 1, 100, background).NRGBAAt(0, 0)
	if got != background {
		t.Fatal(got)
	}
	o := DefaultOptions()
	o.Background = "bad"
	if o.Validate() == nil {
		t.Fatal("invalid background accepted")
	}
}
