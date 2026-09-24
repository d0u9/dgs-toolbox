// Package imaging shrinks, turns and re-encodes pictures. It knows nothing of
// where a picture came from or where it is going: bytes or an image in, bytes or
// an image out, so a Box thumbnail and a Capture attachment are drawn by the
// same code.
package imaging

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

// DefaultMaxSide is the longest side a re-encoded picture is given when a
// caller does not say: large enough to read on a laptop screen at full width,
// small enough that a note full of them stays light to sync.
const DefaultMaxSide = 2048

// DefaultQuality is the JPEG quality a picture is re-encoded at when a caller
// does not say, on the standard library's 1–100 scale.
const DefaultQuality = 80

// ErrNotJPEG is returned when bytes handed to TranscodeJPEG are not a JPEG. The
// format is checked rather than assumed: a PNG with a .jpg name decodes, but a
// caller that promised JPEG has been given something else.
var ErrNotJPEG = errors.New("not a JPEG")

// Fit scales source so its longest side is at most maxSide, keeping its shape.
// A picture already that small is returned untouched: enlarging it produces the
// same information blurred and several times the size.
func Fit(source image.Image, maxSide int, scaler draw.Scaler) image.Image {
	return FitShape(source, source.Bounds(), maxSide, scaler)
}

// FitShape draws source at the size shape would fit into, so a picture drawn
// from an already shrunk copy keeps the original's proportions exactly.
func FitShape(source image.Image, shape image.Rectangle, maxSide int, scaler draw.Scaler) image.Image {
	bounds := source.Bounds()
	width, height := shape.Dx(), shape.Dy()
	if width <= 0 || height <= 0 || maxSide <= 0 {
		return source
	}
	if width <= maxSide && height <= maxSide {
		if shape == bounds {
			return source
		}
	} else if width >= height {
		height = height * maxSide / width
		width = maxSide
	} else {
		width = width * maxSide / height
		height = maxSide
	}
	width, height = max(width, 1), max(height, 1)
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	scaler.Scale(target, target.Bounds(), source, bounds, draw.Over, nil)
	return target
}

// Orient redraws source the way an EXIF Orientation value says it is meant to
// be seen, 1 to 8. 1, and anything outside the range, returns source unchanged.
//
// 6 is a quarter turn clockwise, 3 a half turn and 8 a quarter turn
// anticlockwise; 2, 4, 5 and 7 are the mirrored forms of 1, 3, 6 and 8.
func Orient(source image.Image, orientation int) image.Image {
	if orientation < 2 || orientation > 8 {
		return source
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	shape := image.Rect(0, 0, width, height)
	if orientation >= 5 {
		shape = image.Rect(0, 0, height, width)
	}
	turned := image.NewRGBA(shape)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			var tx, ty int
			switch orientation {
			case 2:
				tx, ty = width-1-x, y
			case 3:
				tx, ty = width-1-x, height-1-y
			case 4:
				tx, ty = x, height-1-y
			case 5:
				tx, ty = y, x
			case 6:
				tx, ty = height-1-y, x
			case 7:
				tx, ty = height-1-y, width-1-x
			case 8:
				tx, ty = y, width-1-x
			}
			turned.Set(tx, ty, source.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return turned
}

// JPEGOptions is how TranscodeJPEG re-encodes. A zero field takes its default.
type JPEGOptions struct {
	MaxSide int
	Quality int
}

// TranscodeJPEG decodes a JPEG, turns it upright, shrinks it to fit
// MaxSide and encodes it again at Quality.
//
// The result carries no metadata: the standard library writes none. Turning
// the pixels upright first is what keeps that from mattering for the one tag a
// reader would notice, since without it a picture the camera meant to be
// turned would arrive on its side.
func TranscodeJPEG(data []byte, options JPEGOptions) ([]byte, error) {
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotJPEG, err)
	} else if format != "jpeg" {
		return nil, fmt.Errorf("%w: it is %s", ErrNotJPEG, format)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode JPEG: %w", err)
	}
	maxSide := options.MaxSide
	if maxSide <= 0 {
		maxSide = DefaultMaxSide
	}
	quality := options.Quality
	if quality <= 0 {
		quality = DefaultQuality
	}
	// Shrunk before it is turned: turning is a pixel at a time, and the
	// longest side is the same whichever way up the picture is.
	picture := Orient(Fit(decoded, maxSide, draw.CatmullRom), JPEGOrientation(data))
	var out bytes.Buffer
	if err := jpeg.Encode(&out, picture, &jpeg.Options{Quality: min(quality, 100)}); err != nil {
		return nil, fmt.Errorf("encode JPEG: %w", err)
	}
	return out.Bytes(), nil
}
