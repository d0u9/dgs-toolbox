//go:build !darwin || !cgo

package photoencode

import (
	"errors"
	"image"
)

func decodeHEIC(data []byte, o Options) (image.Image, metadata, error) {
	return nil, metadata{}, errors.New("HEIC decoding requires macOS and a dgs build with cgo enabled")
}
