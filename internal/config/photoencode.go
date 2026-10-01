package config

import (
	"fmt"

	"dgs-toolbox/internal/photoencode"
)

// PhotoEncode provides initial values for each fresh Encode session.
type PhotoEncode struct {
	Background  string `json:"background"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Quality     int    `json:"quality"`
	MaxSide     int    `json:"max_side"`
	PPI         int    `json:"ppi"`
	Recursive   bool   `json:"recursive"`
	PreserveGPS bool   `json:"preserve_gps"`
	MaxPixels   int    `json:"max_pixels"`
}

func DefaultPhotoEncode() PhotoEncode {
	return PhotoEncode{Quality: photoencode.DefaultQuality, MaxSide: photoencode.DefaultSize, PPI: photoencode.DefaultPPI, MaxPixels: photoencode.DefaultMaxPixels, Background: photoencode.DefaultBackground}
}
func (c PhotoEncode) Options() photoencode.Options {
	return photoencode.Options{Quality: c.Quality, Size: c.MaxSide, PPI: c.PPI, Recursive: c.Recursive, PreserveGPS: c.PreserveGPS, MaxPixels: c.MaxPixels, Background: c.Background}
}
func (c PhotoEncode) validate() error {
	if err := c.Options().Validate(); err != nil {
		return fmt.Errorf("encode.%w", err)
	}
	return nil
}
