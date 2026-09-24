package imaging

import "encoding/binary"

// JPEGOrientation reads the EXIF Orientation tag of a JPEG, 1 to 8. A JPEG
// with no EXIF, no such tag, or a value out of range is 1: upright, which is
// what a reader shows when the tag is absent.
//
// Only IFD0 is read. That is where the tag lives; a thumbnail's own
// orientation in IFD1 is not the picture's.
func JPEGOrientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for at := 2; at+4 <= len(data); {
		if data[at] != 0xFF {
			return 1
		}
		marker := data[at+1]
		// Start of scan: the metadata segments are all before it.
		if marker == 0xDA || marker == 0xD9 {
			return 1
		}
		length := int(binary.BigEndian.Uint16(data[at+2:]))
		if length < 2 || at+2+length > len(data) {
			return 1
		}
		segment := data[at+4 : at+2+length]
		if marker == 0xE1 && len(segment) > 6 && string(segment[:6]) == "Exif\x00\x00" {
			return tiffOrientation(segment[6:])
		}
		at += 2 + length
	}
	return 1
}

func tiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var order binary.ByteOrder
	switch string(tiff[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(tiff[4:]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 1
	}
	count := int(order.Uint16(tiff[ifd:]))
	for entry := 0; entry < count; entry++ {
		at := ifd + 2 + entry*12
		if at+12 > len(tiff) {
			return 1
		}
		// Tag 0x0112 is Orientation, a SHORT held in the value field itself.
		if order.Uint16(tiff[at:]) != 0x0112 || order.Uint16(tiff[at+2:]) != 3 {
			continue
		}
		value := int(order.Uint16(tiff[at+8:]))
		if value < 1 || value > 8 {
			return 1
		}
		return value
	}
	return 1
}
