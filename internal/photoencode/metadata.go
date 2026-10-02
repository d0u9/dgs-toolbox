package photoencode

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
)

const maxMetadata = 4 << 20

type metadata struct {
	icc, exif   []byte
	orientation int
	date        string
}

// Read only container framing here, not ICC internals. Profiles remain opaque.
func jpegMetadata(data []byte) (metadata, error) {
	m := metadata{orientation: 1}
	chunks := map[int][]byte{}
	total := 0
	for p := 2; p < len(data); {
		if data[p] != 255 {
			return m, errors.New("invalid JPEG marker")
		}
		p++
		for p < len(data) && data[p] == 255 {
			p++
		}
		if p >= len(data) {
			break
		}
		marker := data[p]
		p++
		if marker == 0xda || marker == 0xd9 {
			break
		}
		if marker == 0x01 || marker >= 0xd0 && marker <= 0xd7 {
			continue
		}
		if p+2 > len(data) {
			return m, io.ErrUnexpectedEOF
		}
		n := int(binary.BigEndian.Uint16(data[p:]))
		if n < 2 || p+n > len(data) {
			return m, io.ErrUnexpectedEOF
		}
		body := data[p+2 : p+n]
		p += n
		if marker == 0xe1 && bytes.HasPrefix(body, []byte("Exif\x00\x00")) {
			if m.exif != nil {
				return m, errors.New("multiple EXIF blocks")
			}
			m.exif = bytes.Clone(body[6:])
		}
		if marker == 0xe2 && bytes.HasPrefix(body, []byte("ICC_PROFILE\x00")) {
			if len(body) < 14 || body[12] == 0 || body[13] == 0 {
				return m, errors.New("invalid ICC chunks")
			}
			if total != 0 && total != int(body[13]) {
				return m, errors.New("inconsistent ICC chunks")
			}
			total = int(body[13])
			index := int(body[12])
			if chunks[index] != nil || index > total {
				return m, errors.New("duplicate ICC chunk")
			}
			chunks[index] = body[14:]
		}
	}
	for i := 1; i <= total; i++ {
		chunk, ok := chunks[i]
		if !ok {
			return m, errors.New("missing ICC chunk")
		}
		m.icc = append(m.icc, chunk...)
		if len(m.icc) > maxMetadata {
			return m, errors.New("ICC profile too large")
		}
	}
	if total > 0 && len(m.icc) == 0 {
		return m, errors.New("empty ICC profile")
	}
	if err := m.readExif(); err != nil {
		return m, err
	}
	return m, nil
}

func pngMetadata(data []byte) (metadata, error) {
	m := metadata{orientation: 1}
	// PNG gives iCCP, then sRGB, precedence over gAMA/cHRM; those are refused
	// only when they are the image's sole colour description.
	srgb := false
	var signalling error
	for p := 8; p+12 <= len(data); {
		n := int(binary.BigEndian.Uint32(data[p:]))
		if n > len(data)-p-12 {
			return m, io.ErrUnexpectedEOF
		}
		kind := string(data[p+4 : p+8])
		body := data[p+8 : p+8+n]
		if crc32.ChecksumIEEE(data[p+4:p+8+n]) != binary.BigEndian.Uint32(data[p+8+n:]) {
			return m, errors.New("invalid PNG checksum")
		}
		switch kind {
		case "iCCP":
			if m.icc != nil {
				return m, errors.New("multiple ICC profiles")
			}
			i := bytes.IndexByte(body, 0)
			if i < 1 || i+2 > len(body) || body[i+1] != 0 {
				return m, errors.New("invalid PNG ICC profile")
			}
			r, err := zlib.NewReader(bytes.NewReader(body[i+2:]))
			if err != nil {
				return m, err
			}
			m.icc, err = io.ReadAll(io.LimitReader(r, maxMetadata+1))
			r.Close()
			if err != nil || len(m.icc) > maxMetadata || len(m.icc) < 128 {
				return m, errors.New("invalid or oversized ICC profile")
			}
		case "eXIf":
			m.exif = bytes.Clone(body)
		case "cICP":
			return m, errors.New("PNG cICP colour description is not supported without colour conversion")
		case "sRGB":
			srgb = true
		case "gAMA":
			if n != 4 {
				return m, errors.New("invalid PNG gamma")
			}
			if binary.BigEndian.Uint32(body) != 45455 {
				signalling = errors.New("non-sRGB PNG gamma is not supported")
			}
		case "cHRM":
			expected := []uint32{31270, 32900, 64000, 33000, 30000, 60000, 15000, 6000}
			if n != 32 {
				return m, errors.New("invalid PNG chromaticities")
			}
			for i, v := range expected {
				if binary.BigEndian.Uint32(body[4*i:]) != v {
					signalling = errors.New("non-sRGB PNG chromaticities require a supported ICC profile")
				}
			}
		}
		p += n + 12
		if kind == "IEND" {
			break
		}
	}
	if m.icc == nil && !srgb && signalling != nil {
		return m, signalling
	}
	if err := m.readExif(); err != nil {
		return m, err
	}
	return m, nil
}

func (m *metadata) readExif() error {
	if len(m.exif) == 0 {
		return nil
	}
	_, err := walkExif(m.exif, func(order binary.ByteOrder, e []byte) error {
		tag := order.Uint16(e)
		typ := order.Uint16(e[2:])
		count := order.Uint32(e[4:])
		if tag == 0x112 {
			if typ != 3 || count != 1 {
				return errors.New("invalid EXIF orientation")
			}
			m.orientation = int(order.Uint16(e[8:]))
			if m.orientation < 1 || m.orientation > 8 {
				return errors.New("invalid EXIF orientation")
			}
		}
		if tag == 0x9003 && typ == 2 && count >= 19 && count <= 64 {
			off := int(order.Uint32(e[8:]))
			if off < 0 || off+int(count) > len(m.exif) {
				return errors.New("invalid EXIF date")
			}
			m.date = string(bytes.TrimRight(m.exif[off:off+int(count)], "\x00"))
		}
		return nil
	})
	return err
}

// walkExif validates directory bounds and follows only EXIF/GPS/interop pointers.
func walkExif(data []byte, visit func(binary.ByteOrder, []byte) error) (binary.ByteOrder, error) {
	if len(data) < 8 {
		return nil, errors.New("truncated EXIF")
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, errors.New("invalid EXIF byte order")
	}
	if order.Uint16(data[2:]) != 42 {
		return nil, errors.New("invalid EXIF header")
	}
	if order.Uint32(data[4:]) == 0 {
		return nil, errors.New("missing EXIF root directory")
	}
	seen := map[uint32]bool{}
	var walk func(uint32) error
	walk = func(off uint32) error {
		if off == 0 {
			return nil
		}
		if seen[off] || len(seen) > 32 {
			return errors.New("cyclic or excessive EXIF directories")
		}
		seen[off] = true
		p := int(off)
		if p < 8 || p > len(data)-2 {
			return errors.New("invalid EXIF directory")
		}
		n := int(order.Uint16(data[p:]))
		end := p + 2 + 12*n
		if end > len(data)-4 {
			return errors.New("truncated EXIF directory")
		}
		for i := 0; i < n; i++ {
			e := data[p+2+12*i : p+14+12*i]
			sizes := map[uint16]uint64{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8, 13: 4}
			size, ok := sizes[order.Uint16(e[2:])]
			if !ok {
				return errors.New("unsupported EXIF value type")
			}
			length := size * uint64(order.Uint32(e[4:]))
			if length > 4 {
				off := uint64(order.Uint32(e[8:]))
				if off < 8 || off+length > uint64(len(data)) {
					return errors.New("EXIF value outside block")
				}
			}
			if err := visit(order, e); err != nil {
				return err
			}
			tag := order.Uint16(e)
			if tag == 0x8769 || tag == 0x8825 || tag == 0xa005 {
				if order.Uint16(e[2:]) != 4 || order.Uint32(e[4:]) != 1 {
					return errors.New("invalid EXIF pointer")
				}
				if err := walk(order.Uint32(e[8:])); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return order, walk(order.Uint32(data[4:]))
}

func (m metadata) outputExif(gps bool, w, h, ppi int) ([]byte, error) {
	if gps && len(m.exif) > 0 {
		out := bytes.Clone(m.exif)
		order, err := walkExif(out, func(o binary.ByteOrder, e []byte) error {
			tag := o.Uint16(e)
			if tag == 0x11a || tag == 0x11b {
				if o.Uint16(e[2:]) != 5 || o.Uint32(e[4:]) != 1 {
					return errors.New("invalid EXIF density")
				}
				off := int(o.Uint32(e[8:]))
				o.PutUint32(out[off:], uint32(ppi))
				o.PutUint32(out[off+4:], 1)
			}
			if tag == 0x128 {
				if o.Uint16(e[2:]) != 3 || o.Uint32(e[4:]) != 1 {
					return errors.New("invalid EXIF density unit")
				}
				o.PutUint16(e[8:], 2)
			}
			if tag == 0x112 {
				o.PutUint16(e[8:], 1)
			}
			if tag == 0xa001 && o.Uint16(e[2:]) == 3 && o.Uint32(e[4:]) == 1 {
				o.PutUint16(e[8:], 1) // ColorSpace: the pixels are now sRGB.
			}
			if tag == 0x100 || tag == 0xa002 || tag == 0x101 || tag == 0xa003 {
				if o.Uint32(e[4:]) != 1 {
					return errors.New("invalid EXIF dimension count")
				}
				v := w
				if tag == 0x101 || tag == 0xa003 {
					v = h
				}
				switch o.Uint16(e[2:]) {
				case 3:
					if v > 65535 {
						return errors.New("EXIF dimensions overflow")
					}
					o.PutUint16(e[8:], uint16(v))
				case 4:
					o.PutUint32(e[8:], uint32(v))
				default:
					return errors.New("invalid EXIF dimensions")
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		// Remove the referenced IFD1 thumbnail, which describes the old pixels.
		p := int(order.Uint32(out[4:]))
		end := p + 2 + 12*int(order.Uint16(out[p:]))
		order.PutUint32(out[end:], 0)
		if len(out)+6 > maxSegment {
			// Large maker notes cannot fit one APP1 segment; keep what was asked for.
			return compactExif(order, m.date, ifdEntries(out, order, order.Uint32(out[4:]), 0x8825)), nil
		}
		return append([]byte("Exif\x00\x00"), out...), nil
	}
	// With GPS disabled, rebuild only capture date. No discarded GPS bytes remain.
	if m.date == "" {
		return nil, nil
	}
	return compactExif(binary.LittleEndian, m.date, nil), nil
}

const maxSegment = 65533

type exifEntry struct {
	tag, typ uint16
	count    uint32
	value    []byte
}

// ifdEntries copies the directory that the pointer tag in directory off names.
// The block has already been bounds-checked by walkExif.
func ifdEntries(data []byte, order binary.ByteOrder, off uint32, pointer uint16) []exifEntry {
	sizes := map[uint16]int{1: 1, 2: 1, 3: 2, 4: 4, 5: 8, 6: 1, 7: 1, 8: 2, 9: 4, 10: 8, 11: 4, 12: 8, 13: 4}
	read := func(off uint32) (entries []exifEntry) {
		p := int(off)
		for i := 0; i < int(order.Uint16(data[p:])); i++ {
			e := data[p+2+12*i : p+14+12*i]
			x := exifEntry{tag: order.Uint16(e), typ: order.Uint16(e[2:]), count: order.Uint32(e[4:])}
			n := sizes[x.typ] * int(x.count)
			if n <= 4 {
				x.value = bytes.Clone(e[8 : 8+n])
			} else {
				v := int(order.Uint32(e[8:]))
				x.value = bytes.Clone(data[v : v+n])
			}
			entries = append(entries, x)
		}
		return entries
	}
	for _, e := range read(off) {
		if e.tag == pointer {
			return read(order.Uint32(e.value))
		}
	}
	return nil
}

// compactExif builds IFD0 → Exif IFD (capture date) and GPS IFD, in order.
func compactExif(order binary.ByteOrder, date string, gps []exifEntry) []byte {
	var a binary.AppendByteOrder = binary.LittleEndian
	if order == binary.BigEndian {
		a = binary.BigEndian
	}
	var exif []exifEntry
	if date != "" {
		v := append([]byte(date), 0)
		exif = append(exif, exifEntry{0x9003, 2, uint32(len(v)), v})
	}
	if exif == nil && gps == nil {
		return nil
	}
	size := func(entries []exifEntry) int {
		n := 2 + 12*len(entries) + 4
		for _, e := range entries {
			if len(e.value) > 4 {
				n += len(e.value) + len(e.value)%2
			}
		}
		return n
	}
	var root []exifEntry
	if exif != nil {
		root = append(root, exifEntry{0x8769, 4, 1, nil})
	}
	if gps != nil {
		root = append(root, exifEntry{0x8825, 4, 1, nil})
	}
	next := 8 + size(root)
	for i := range root {
		root[i].value = a.AppendUint32(nil, uint32(next))
		if root[i].tag == 0x8769 {
			next += size(exif)
		}
	}
	out := append([]byte{}, "II\x2a\x00\x08\x00\x00\x00"...)
	if order == binary.BigEndian {
		out = append([]byte{}, "MM\x00\x2a\x00\x00\x00\x08"...)
	}
	for _, entries := range [][]exifEntry{root, exif, gps} {
		if entries == nil {
			continue
		}
		start := len(out)
		data := start + 2 + 12*len(entries) + 4
		out = a.AppendUint16(out, uint16(len(entries)))
		var tail []byte
		for _, e := range entries {
			out = a.AppendUint16(out, e.tag)
			out = a.AppendUint16(out, e.typ)
			out = a.AppendUint32(out, e.count)
			if len(e.value) > 4 {
				out = a.AppendUint32(out, uint32(data+len(tail)))
				tail = append(tail, e.value...)
				if len(e.value)%2 == 1 {
					tail = append(tail, 0)
				}
			} else {
				out = append(out, e.value...)
				out = append(out, make([]byte, 4-len(e.value))...)
			}
		}
		out = a.AppendUint32(out, 0)
		out = append(out, tail...)
	}
	return append([]byte("Exif\x00\x00"), out...)
}

func addMetadata(jpeg []byte, m metadata, gps bool, ppi, w, h int) ([]byte, error) {
	var out bytes.Buffer
	out.Write(jpeg[:2])
	segment := func(marker byte, body []byte) error {
		if len(body) > maxSegment {
			return errors.New("metadata block too large")
		}
		out.Write([]byte{255, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)})
		out.Write(body)
		return nil
	}
	jfif := []byte{'J', 'F', 'I', 'F', 0, 1, 1, 1, byte(ppi >> 8), byte(ppi), byte(ppi >> 8), byte(ppi), 0, 0}
	if err := segment(0xe0, jfif); err != nil {
		return nil, err
	}
	ex, err := m.outputExif(gps, w, h, ppi)
	if err != nil {
		return nil, err
	}
	if len(ex) > 0 {
		if err := segment(0xe1, ex); err != nil {
			return nil, err
		}
	}
	const chunkSize = 65519
	total := (len(m.icc) + chunkSize - 1) / chunkSize
	for i := 0; i < total; i++ {
		body := append([]byte("ICC_PROFILE\x00"), byte(i+1), byte(total))
		body = append(body, m.icc[i*chunkSize:min(len(m.icc), (i+1)*chunkSize)]...)
		if err := segment(0xe2, body); err != nil {
			return nil, err
		}
	}
	out.Write(jpeg[2:])
	return out.Bytes(), nil
}

// checkICC accepts an RGB profile, or a GRAY profile on a grayscale image.
func checkICC(icc []byte, gray bool) error {
	if len(icc) == 0 {
		return nil
	}
	if len(icc) < 128 || string(icc[36:40]) != "acsp" || int(binary.BigEndian.Uint32(icc)) != len(icc) {
		return errors.New("invalid ICC profile")
	}
	if gray && string(icc[16:20]) == "GRAY" {
		return nil
	}
	if string(icc[16:20]) != "RGB " {
		return fmt.Errorf("ICC colour space %q is not supported without conversion", icc[16:20])
	}
	return nil
}
