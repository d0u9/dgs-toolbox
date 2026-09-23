package scanmeta

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"strings"
	"time"

	// Registered for image.DecodeConfig, which reads a header rather than the
	// pixels: the size and colour model of a scan come out of a few hundred
	// bytes, not out of decoding a 40-megapixel photograph.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

// ReadImage reads a camera or scanner image — the remaining 1% of a Box.
//
// An image is never converted to PDF. A conversion rewrites the bytes, which
// throws away the original and changes the digest that identifies it.
//
// It holds one page, has no paper size, and is its own single image stream: the
// file's bytes are what the second digest and both thumbnails are made from.
func ReadImage(data []byte) (Info, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		// A format this build cannot decode is still a file worth filing. It
		// keeps its metadata from EXIF, is marked as having no render, and the
		// inbox stays drainable.
		info := Info{Kind: KindImage, Pages: 1, ImageError: fmt.Sprintf("read image header: %v", err)}
		applyEXIF(&info, data)
		return info, nil
	}
	info := Info{
		Kind:  KindImage,
		Pages: 1,
		Images: []Image{{
			Page:       1,
			Width:      config.Width,
			Height:     config.Height,
			Bits:       colorModelBits(config),
			ColorSpace: colorModelName(config),
			Format:     strings.ToLower(format),
			Bytes:      data,
			// An image is its own render: the file already holds a picture an
			// image library reads, so there is nothing to decode it into.
			Rendered:       data,
			RenderedFormat: strings.ToLower(format),
		}},
	}
	applyEXIF(&info, data)
	return info, nil
}

// applyEXIF fills in what the camera or scanner recorded. A file with no EXIF
// at all is ordinary — a screenshot, a re-saved scan — and leaves the fields
// empty rather than failing the read.
func applyEXIF(info *Info, data []byte) {
	decoded, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		return
	}
	info.Producer = strings.TrimSpace(strings.Join(nonEmpty(
		stringTag(decoded, exif.Make),
		stringTag(decoded, exif.Model),
	), " "))
	info.CreatedAt = exifInstant(decoded)
}

// exifInstant is when the picture was taken, in RFC 3339.
//
// EXIF writes the time with no zone on it, so the offset comes from
// OffsetTimeOriginal when the camera wrote one. When it did not, the time is
// recorded as it stands with no offset invented: a guessed zone is a wrong
// answer that looks like a right one, and the day — which is what anyone sorting
// paper cares about — is correct either way.
func exifInstant(decoded *exif.Exif) string {
	text := stringTag(decoded, exif.DateTimeOriginal)
	if text == "" {
		text = stringTag(decoded, exif.DateTime)
	}
	if text == "" {
		return ""
	}
	instant, err := time.Parse("2006:01:02 15:04:05", strings.TrimSpace(text))
	if err != nil {
		return strings.TrimSpace(text)
	}
	offset := stringTag(decoded, exif.FieldName("OffsetTimeOriginal"))
	if offset == "" {
		offset = stringTag(decoded, exif.FieldName("OffsetTime"))
	}
	if offset != "" {
		if withZone, err := time.Parse("2006:01:02 15:04:05-07:00", strings.TrimSpace(text)+strings.TrimSpace(offset)); err == nil {
			return withZone.Format(time.RFC3339)
		}
	}
	return instant.Format("2006-01-02T15:04:05")
}

func stringTag(decoded *exif.Exif, name exif.FieldName) string {
	tag, err := decoded.Get(name)
	if err != nil {
		return ""
	}
	if tag.Format() == tiff.StringVal {
		value, err := tag.StringVal()
		if err != nil {
			return ""
		}
		return strings.TrimRight(strings.TrimSpace(value), "\x00")
	}
	return strings.Trim(tag.String(), `"`)
}

func nonEmpty(values ...string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			kept = append(kept, strings.TrimSpace(value))
		}
	}
	return kept
}

func colorModelName(config image.Config) string {
	switch config.ColorModel {
	case color.GrayModel, color.Gray16Model:
		return "DeviceGray"
	case color.CMYKModel:
		return "DeviceCMYK"
	default:
		return "DeviceRGB"
	}
}

func colorModelBits(config image.Config) int {
	switch config.ColorModel {
	case color.Gray16Model, color.RGBA64Model, color.NRGBA64Model:
		return 16
	default:
		return 8
	}
}
