//go:build !darwin || !cgo

package photoencode

import (
	"errors"
	"image"
	"runtime"
)

// nativeProblem explains why HEIC/HEIF and TIFF cannot be encoded on this
// build. Plan reports it per file, so other inputs still run.
func nativeProblem(kind string) string {
	if runtime.GOOS == "darwin" {
		return kind + " decoding uses macOS ImageIO and needs a dgs build with cgo enabled"
	}
	return kind + " decoding uses macOS ImageIO and is not supported on " + runtime.GOOS + " yet"
}

// toSRGB passes untagged pixels through as sRGB without a profile. Tagged
// pixels need ColorSync, so they are refused with the reason.
func toSRGB(img *image.NRGBA, icc []byte) (*image.NRGBA, []byte, error) {
	if len(icc) == 0 {
		return img, nil, nil
	}
	if runtime.GOOS == "darwin" {
		return nil, nil, errors.New("converting an ICC-tagged image to sRGB uses macOS ColorSync and needs a dgs build with cgo enabled")
	}
	return nil, nil, errors.New("converting an ICC-tagged image to sRGB uses macOS ColorSync and is not supported on " + runtime.GOOS + " yet")
}

func decodeNative(kind string, data []byte, o Options) (image.Image, metadata, error) {
	return nil, metadata{}, errors.New(nativeProblem(kind))
}
