package exif

import (
	"bytes"
	"encoding/binary"
	"strings"
	"time"
)

const (
	tagMake             = 0x010F
	tagModel            = 0x0110
	tagOrientation      = 0x0112
	tagSoftware         = 0x0131
	tagExifIFD          = 0x8769
	tagGPSIFD           = 0x8825
	tagISO              = 0x8827
	tagFNumber          = 0x829D
	tagExposureTime     = 0x829A
	tagFocalLength      = 0x920A
	tagLensModel        = 0xA434
	tagCaptured         = 0x9003
	tagCapturedOffset   = 0x9011
	tagDigitized        = 0x9004
	tagGPSLatitudeRef   = 0x0001
	tagGPSLatitude      = 0x0002
	tagGPSLongitudeRef  = 0x0003
	tagGPSLongitude     = 0x0004
	tagGPSAltitudeRef   = 0x0005
	tagGPSAltitude      = 0x0006
	kindByte            = 1
	kindASCII           = 2
	kindShort           = 3
	kindLong            = 4
	kindRational        = 5
	kindUndefined       = 7
	kindSRational       = 10
	timeLayout          = "2006:01:02 15:04:05"
	timeLayoutWithZone  = "2006:01:02 15:04:05 -07:00"
	exifTimeLayoutBytes = 19
)

// tiff reads the TIFF block an EXIF segment holds.
type tiff struct {
	b     []byte
	order binary.ByteOrder
}

// parseTIFF reads the tags of the TIFF block b, as EXIF, GPS and the size of a
// TIFF file itself give them. A block it cannot read leaves the metadata as it
// was, and it never reads past b.
func parseTIFF(b []byte, metadata *Metadata) {
	if len(b) < 8 {
		return
	}
	var order binary.ByteOrder
	switch string(b[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return
	}
	t := tiff{b: b, order: order}
	if t.u16(2) != 42 {
		return
	}
	t.readIFD(int(t.u32(4)), metadata)
}

// readIFD reads the tags of one directory, and the ones of the EXIF and GPS
// directories it points at.
func (t tiff) readIFD(ifd int, metadata *Metadata) {
	if ifd <= 0 || ifd+2 > len(t.b) {
		return
	}
	exifIFD, gpsIFD := 0, 0
	t.each(ifd, func(tag, kind, count int, data []byte) {
		switch tag {
		case tagMake:
			metadata.Make = ascii(data)
		case tagModel:
			metadata.Model = ascii(data)
		case tagSoftware:
			metadata.Software = ascii(data)
		case tagOrientation:
			metadata.Orientation = int(t.u16Of(data))
		case tagExifIFD:
			exifIFD = int(t.u32Of(data))
		case tagGPSIFD:
			gpsIFD = int(t.u32Of(data))
		}
	})
	if exifIFD > 0 {
		t.readExifIFD(exifIFD, metadata)
	}
	if gpsIFD > 0 {
		metadata.GPS = t.readGPS(gpsIFD)
	}
}

func (t tiff) readExifIFD(ifd int, metadata *Metadata) {
	offset := ""
	t.each(ifd, func(tag, kind, count int, data []byte) {
		switch tag {
		case tagISO:
			metadata.ISO = int(t.u16Of(data))
		case tagFNumber:
			metadata.FNumber = t.rational(data, 0)
		case tagExposureTime:
			metadata.Exposure = t.rational(data, 0)
		case tagFocalLength:
			metadata.FocalLength = t.rational(data, 0)
		case tagLensModel:
			metadata.Lens = ascii(data)
		case tagCaptured:
			metadata.Captured = parseTime(ascii(data), "")
		case tagDigitized:
			metadata.Digitized = parseTime(ascii(data), "")
		case tagCapturedOffset:
			offset = ascii(data)
		}
	})
	if offset != "" && !metadata.Captured.IsZero() {
		metadata.Captured = atOffset(metadata.Captured, offset)
	}
	if offset != "" && !metadata.Digitized.IsZero() {
		metadata.Digitized = atOffset(metadata.Digitized, offset)
	}
}

func (t tiff) readGPS(ifd int) *Point {
	point := Point{}
	latitude, longitude := false, false
	refs := map[int]string{}
	t.each(ifd, func(tag, kind, count int, data []byte) {
		switch tag {
		case tagGPSLatitudeRef, tagGPSLongitudeRef:
			refs[tag] = ascii(data)
		case tagGPSLatitude:
			point.Latitude, latitude = t.degrees(data), true
		case tagGPSLongitude:
			point.Longitude, longitude = t.degrees(data), true
		case tagGPSAltitude:
			point.Altitude, point.AltitudeSet = t.rational(data, 0), true
		case tagGPSAltitudeRef:
			if len(data) > 0 && data[0] == 1 {
				point.Altitude = -point.Altitude
			}
		}
	})
	if !latitude || !longitude {
		return nil
	}
	if strings.EqualFold(refs[tagGPSLatitudeRef], "S") {
		point.Latitude = -point.Latitude
	}
	if strings.EqualFold(refs[tagGPSLongitudeRef], "W") {
		point.Longitude = -point.Longitude
	}
	return &point
}

// each calls fn for the entries of the directory at ifd, with the bytes of the
// value of each and how many values it holds.
func (t tiff) each(ifd int, fn func(tag, kind, count int, data []byte)) {
	count := int(t.u16(ifd))
	for i := 0; i < count; i++ {
		at := ifd + 2 + i*12
		if at+12 > len(t.b) {
			return
		}
		tag := int(t.u16(at))
		kind := int(t.u16(at + 2))
		values := int(t.u32(at + 4))
		size := valueSize(kind)
		if size == 0 || values < 0 {
			continue
		}
		total := size * values
		if total <= 4 {
			fn(tag, kind, values, t.b[at+8:at+12])
			continue
		}
		offset := int(t.u32(at + 8))
		if offset < 0 || offset > len(t.b) || total > len(t.b)-offset {
			continue
		}
		fn(tag, kind, values, t.b[offset:offset+total])
	}
}

func valueSize(kind int) int {
	switch kind {
	case kindByte, kindASCII, kindUndefined:
		return 1
	case kindShort:
		return 2
	case kindLong:
		return 4
	case kindSRational, kindRational:
		return 8
	default:
		return 0
	}
}

func (t tiff) u16(at int) uint16 {
	if at < 0 || at+2 > len(t.b) {
		return 0
	}
	return t.order.Uint16(t.b[at:])
}

func (t tiff) u32(at int) uint32 {
	if at < 0 || at+4 > len(t.b) {
		return 0
	}
	return t.order.Uint32(t.b[at:])
}

func (t tiff) u16Of(data []byte) uint16 {
	if len(data) < 2 {
		return 0
	}
	return t.order.Uint16(data)
}

func (t tiff) u32Of(data []byte) uint32 {
	if len(data) < 4 {
		return 0
	}
	return t.order.Uint32(data)
}

// rational returns the value of the index-th fraction of data, or 0 when it is
// not there or its denominator is 0.
func (t tiff) rational(data []byte, index int) float64 {
	if index < 0 || len(data) < (index+1)*8 {
		return 0
	}
	numerator := t.order.Uint32(data[index*8:])
	denominator := t.order.Uint32(data[index*8+4:])
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// degrees reads a coordinate as EXIF gives it: degrees, minutes and seconds.
func (t tiff) degrees(data []byte) float64 {
	return t.rational(data, 0) + t.rational(data, 1)/60 + t.rational(data, 2)/3600
}

func ascii(data []byte) string {
	if end := bytes.IndexByte(data, 0); end >= 0 {
		data = data[:end]
	}
	return strings.TrimSpace(string(data))
}

// parseTime reads the time EXIF writes, "2006:01:02 15:04:05", and the offset
// that may go with it, as "+02:00"; a time with no offset is UTC.
func parseTime(text, offset string) time.Time {
	if len(text) < exifTimeLayoutBytes {
		return time.Time{}
	}
	text = text[:exifTimeLayoutBytes]
	if offset != "" {
		if at, err := time.Parse(timeLayoutWithZone, text+" "+offset); err == nil {
			return at
		}
	}
	at, err := time.Parse(timeLayout, text)
	if err != nil {
		return time.Time{}
	}
	return at
}

// atOffset moves the wall clock of at to the zone offset names, as EXIF's
// OffsetTimeOriginal does.
func atOffset(at time.Time, offset string) time.Time {
	moved, err := time.Parse(timeLayoutWithZone, at.Format(timeLayout)+" "+offset)
	if err != nil {
		return at
	}
	return moved
}
