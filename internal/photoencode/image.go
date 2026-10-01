// Package photoencode encodes photos independently of TUI and configuration.
package photoencode

import (
	"dgs-toolbox/internal/imaging"
	"fmt"
	xdraw "golang.org/x/image/draw"
	"image"
	"image/color"
	"image/draw"
	"strconv"
	"strings"
)

const DefaultQuality = 82
const DefaultSize = 800
const DefaultPPI = 240
const DefaultBackground = "#ffffff"

// DefaultMaxPixels bounds decoding allocations. Jobs run sequentially.
const DefaultMaxPixels = 40000000

// Transform reuses shared sizing and orientation algorithms, then composites
// transparency onto the selected opaque background. It never enlarges or converts colour spaces.
func Transform(src image.Image, orientation, size int, background color.Color) *image.NRGBA {
	picture := imaging.Orient(imaging.Fit(src, size, xdraw.CatmullRom), orientation)
	b := picture.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), picture, b.Min, draw.Over)
	return dst
}

// BackgroundColor accepts an opaque #RRGGBB colour for JPEG alpha compositing.
func BackgroundColor(value string) (color.NRGBA, error) {
	if len(value) != 7 || !strings.HasPrefix(value, "#") {
		return color.NRGBA{}, fmt.Errorf("background must be #RRGGBB")
	}
	n, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return color.NRGBA{}, fmt.Errorf("background must be #RRGGBB")
	}
	return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
}
