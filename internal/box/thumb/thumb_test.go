package thumb_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"dgs-toolbox/internal/box/scanmeta"
	"dgs-toolbox/internal/box/thumb"
)

func pngBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := range width {
		for y := range height {
			out.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 0x80, A: 0xff})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, out); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return encoded.Bytes()
}

func size(t *testing.T, data []byte) (int, int) {
	t.Helper()
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return decoded.Bounds().Dx(), decoded.Bounds().Dy()
}

func TestBothSizesFitAndKeepTheirShape(t *testing.T) {
	// A portrait page, as almost everything in a Box is.
	info := scanmeta.Info{Images: []scanmeta.Image{{Page: 1, Rendered: pngBytes(t, 2000, 3000)}}}
	pair, err := thumb.Render(info)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	gridWidth, gridHeight := size(t, pair.Grid)
	if gridHeight != thumb.GridSize {
		t.Errorf("grid longest side: got %d want %d", gridHeight, thumb.GridSize)
	}
	if gridWidth != 200 {
		t.Errorf("grid width: got %d, which is not the 2:3 shape of the page", gridWidth)
	}
	previewWidth, previewHeight := size(t, pair.Preview)
	if previewHeight != thumb.PreviewSize {
		t.Errorf("preview longest side: got %d want %d", previewHeight, thumb.PreviewSize)
	}
	if previewWidth != 1066 && previewWidth != 1067 {
		t.Errorf("preview width: got %d, which is not the page's shape", previewWidth)
	}
}

// A landscape scan is scaled on its own longest side, not blindly on height.
func TestLandscapeScalesOnItsLongSide(t *testing.T) {
	info := scanmeta.Info{Images: []scanmeta.Image{{Page: 1, Rendered: pngBytes(t, 3000, 2000)}}}
	pair, err := thumb.Render(info)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	width, height := size(t, pair.Grid)
	if width != thumb.GridSize || height != 200 {
		t.Errorf("got %dx%d, want %dx200", width, height, thumb.GridSize)
	}
}

// Enlarging a small scan gives a blurred picture of exactly the same
// information in a several times larger file: wrong twice.
func TestNothingIsEnlarged(t *testing.T) {
	info := scanmeta.Info{Images: []scanmeta.Image{{Page: 1, Rendered: pngBytes(t, 80, 120)}}}
	pair, err := thumb.Render(info)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for name, data := range map[string][]byte{"grid": pair.Grid, "preview": pair.Preview} {
		width, height := size(t, data)
		if width != 80 || height != 120 {
			t.Errorf("%s: got %dx%d, want the original 80x120", name, width, height)
		}
	}
}

// needs-render: an outcome, not a failure. The scan is still filed.
func TestNoImageIsNamedNotGuessed(t *testing.T) {
	_, err := thumb.Render(scanmeta.Info{})
	if !errors.Is(err, thumb.ErrNoImage) {
		t.Fatalf("got %v, want ErrNoImage", err)
	}
	// A stream this build cannot decode is the same answer: no picture, and a
	// reason.
	_, err = thumb.Render(scanmeta.Info{Images: []scanmeta.Image{{Page: 1, Rendered: []byte("not a picture")}}})
	if err == nil {
		t.Fatal("an undecodable stream produced a thumbnail")
	}
}

// The thumbnail comes from the rendered form. The raw stream is often in an
// encoding no image library reads, and is reserved for the digest.
func TestRawStreamIsNotDrawnFrom(t *testing.T) {
	info := scanmeta.Info{Images: []scanmeta.Image{{
		Page:     1,
		Bytes:    []byte("a CCITT stream no decoder here reads"),
		Rendered: pngBytes(t, 100, 100),
	}}}
	if _, err := thumb.Render(info); err != nil {
		t.Fatalf("render: %v", err)
	}
}

// JPEG, not PNG: the content is photographic and a PNG of a scanned page is
// several times larger for no visible gain.
func TestOutputIsJPEG(t *testing.T) {
	pair, err := thumb.Render(scanmeta.Info{Images: []scanmeta.Image{{Page: 1, Rendered: pngBytes(t, 900, 900)}}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for name, data := range map[string][]byte{"grid": pair.Grid, "preview": pair.Preview} {
		if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != "jpeg" {
			t.Errorf("%s is %q (%v), want jpeg", name, format, err)
		}
	}
}

func TestRenderPageDrawsThatPage(t *testing.T) {
	info := scanmeta.Info{Pages: 3, Images: []scanmeta.Image{
		{Page: 1, Rendered: pngBytes(t, 300, 400)},
		{Page: 2, Rendered: pngBytes(t, 400, 300)},
		{Page: 3, Rendered: pngBytes(t, 100, 100)},
	}}
	pair, err := thumb.RenderPage(info, 2)
	if err != nil {
		t.Fatalf("render page 2: %v", err)
	}
	// Page 2 is the only landscape one, so its shape says which page was drawn.
	if width, height := size(t, pair.Grid); width <= height {
		t.Errorf("page 2 came out %dx%d, which is not page 2", width, height)
	}
	if _, err := thumb.RenderPage(info, 4); !errors.Is(err, thumb.ErrNoImage) {
		t.Errorf("a page that does not exist: got %v, want ErrNoImage", err)
	}
}

func TestFirstPageFallsBackToTheOnlyImage(t *testing.T) {
	// A PDF whose one picture sits on page 3 still has a thumbnail.
	info := scanmeta.Info{Pages: 3, Images: []scanmeta.Image{{Page: 3, Rendered: pngBytes(t, 200, 300)}}}
	if _, err := thumb.Render(info); err != nil {
		t.Errorf("render: %v", err)
	}
	if _, err := thumb.RenderPage(info, 1); !errors.Is(err, thumb.ErrNoImage) {
		t.Errorf("page 1 asked for by number: got %v, want ErrNoImage", err)
	}
}

// A page turned by /Rotate is drawn turned: a wide picture on a page turned a
// quarter becomes tall, and its top-left corner moves to the top right.
func TestTurnFollowsThePage(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 4, 2))
	red := color.RGBA{R: 255, A: 255}
	source.Set(0, 0, red)
	cases := []struct {
		degrees    int
		width      int
		height     int
		redX, redY int
	}{
		{0, 4, 2, 0, 0},
		{90, 2, 4, 1, 0},
		{180, 4, 2, 3, 1},
		{270, 2, 4, 0, 3},
		{45, 4, 2, 0, 0},
	}
	for _, c := range cases {
		turned := thumb.Turn(source, c.degrees)
		if turned.Bounds().Dx() != c.width || turned.Bounds().Dy() != c.height {
			t.Errorf("%d°: %v", c.degrees, turned.Bounds())
			continue
		}
		if got := color.RGBAModel.Convert(turned.At(c.redX, c.redY)); got != red {
			t.Errorf("%d°: corner went elsewhere, (%d,%d) is %v", c.degrees, c.redX, c.redY, got)
		}
	}
}

// RenderPicture draws a page turned a quarter tall, as a PDF reader shows it.
func TestRenderPictureTurnsThePage(t *testing.T) {
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, image.NewGray(image.Rect(0, 0, 400, 200))); err != nil {
		t.Fatal(err)
	}
	pair, err := thumb.RenderPicture(scanmeta.Image{Rendered: buffer.Bytes(), Rotate: 90})
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"grid": pair.Grid, "preview": pair.Preview} {
		config, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if config.Width >= config.Height {
			t.Errorf("%s is %dx%d, not tall", name, config.Width, config.Height)
		}
	}
}

func TestRenderPictureAtFitsAndTurns(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewGray(image.Rect(0, 0, 400, 200))); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		size, rotate int
		want         image.Point
	}{{100, 0, image.Pt(100, 50)}, {100, 90, image.Pt(50, 100)}, {1000, 0, image.Pt(400, 200)}} {
		out, err := thumb.RenderPictureAt(scanmeta.Image{Rendered: encoded.Bytes(), Rotate: c.rotate}, c.size)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		if got := decoded.Bounds().Size(); got != c.want {
			t.Errorf("size %d rotate %d: %v, want %v", c.size, c.rotate, got, c.want)
		}
	}
}
