package exif

import (
	"encoding/binary"
	"io"
	"math"
	"testing"
	"time"
)

type entry struct {
	tag, kind uint16
	count     uint32
	value     []byte
}

func short(v uint16) []byte {
	b := make([]byte, 2)
	binary.LittleEndian.PutUint16(b, v)
	return b
}

func long(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

func rational(numerator, denominator uint32) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint32(b, numerator)
	binary.LittleEndian.PutUint32(b[4:], denominator)
	return b
}

func rationals(values ...[2]uint32) []byte {
	b := make([]byte, 0, len(values)*8)
	for _, value := range values {
		b = append(b, rational(value[0], value[1])...)
	}
	return b
}

// tiffBlock lays out the three directories of a picture and the values they
// point at, as a camera writes them.
func tiffBlock() []byte {
	const (
		entries0 = 6
		entries1 = 8
		entries2 = 6
	)
	size0 := 2 + entries0*12 + 4
	size1 := 2 + entries1*12 + 4
	size2 := 2 + entries2*12 + 4
	exifAt := 8 + size0
	gpsAt := exifAt + size1
	valuesAt := gpsAt + size2

	values := []byte{}
	write := func(entries []entry, next int) []byte {
		out := make([]byte, 2)
		binary.LittleEndian.PutUint16(out, uint16(len(entries)))
		for _, e := range entries {
			item := make([]byte, 12)
			binary.LittleEndian.PutUint16(item[0:], e.tag)
			binary.LittleEndian.PutUint16(item[2:], e.kind)
			binary.LittleEndian.PutUint32(item[4:], e.count)
			if len(e.value) <= 4 {
				copy(item[8:], e.value)
			} else {
				binary.LittleEndian.PutUint32(item[8:], uint32(valuesAt+len(values)))
				values = append(values, e.value...)
			}
			out = append(out, item...)
		}
		tail := make([]byte, 4)
		if next > 0 {
			binary.LittleEndian.PutUint32(tail, uint32(next))
		}
		return append(out, tail...)
	}

	ifd0 := write([]entry{
		{tagMake, kindASCII, 6, []byte("Canon\x00")},
		{tagModel, kindASCII, 7, []byte("EOS R5\x00")},
		{tagOrientation, kindShort, 1, short(6)},
		{tagSoftware, kindASCII, 7, []byte("picsel\x00")},
		{tagExifIFD, kindLong, 1, long(uint32(exifAt))},
		{tagGPSIFD, kindLong, 1, long(uint32(gpsAt))},
	}, 0)
	ifd1 := write([]entry{
		{tagISO, kindShort, 1, short(100)},
		{tagFNumber, kindRational, 1, rational(28, 10)},
		{tagExposureTime, kindRational, 1, rational(1, 250)},
		{tagFocalLength, kindRational, 1, rational(50, 1)},
		{tagCaptured, kindASCII, 20, []byte("2024:03:02 10:11:12\x00")},
		{tagCapturedOffset, kindASCII, 7, []byte("-03:00\x00")},
		{tagDigitized, kindASCII, 20, []byte("2024:03:02 10:11:13\x00")},
		{tagLensModel, kindASCII, 8, []byte("RF 50mm\x00")},
	}, 0)
	ifd2 := write([]entry{
		{tagGPSLatitudeRef, kindASCII, 2, []byte("S\x00")},
		{tagGPSLatitude, kindRational, 3, rationals([2]uint32{34, 1}, [2]uint32{36, 1}, [2]uint32{12, 1})},
		{tagGPSLongitudeRef, kindASCII, 2, []byte("W\x00")},
		{tagGPSLongitude, kindRational, 3, rationals([2]uint32{58, 1}, [2]uint32{22, 1}, [2]uint32{48, 1})},
		{tagGPSAltitudeRef, kindByte, 1, []byte{0}},
		{tagGPSAltitude, kindRational, 1, rational(1234, 10)},
	}, 0)

	out := []byte("II*\x00")
	out = append(out, long(8)...)
	out = append(out, ifd0...)
	out = append(out, ifd1...)
	out = append(out, ifd2...)
	return append(out, values...)
}

// jpegWith wraps a TIFF block in the APP1 segment of a JPEG of width×height.
func jpegWith(block []byte, width, height int) []byte {
	out := []byte{0xFF, 0xD8}
	app1 := append([]byte("Exif\x00\x00"), block...)
	segment := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(segment[2:], uint16(len(app1)+2))
	out = append(out, segment...)
	out = append(out, app1...)
	frame := []byte{0xFF, 0xC0, 0, 17, 8, 0, 0, 0, 0, 3, 1, 0x11, 0, 2, 0x11, 1, 3, 0x11, 1}
	binary.BigEndian.PutUint16(frame[5:], uint16(height))
	binary.BigEndian.PutUint16(frame[7:], uint16(width))
	out = append(out, frame...)
	out = append(out, 0xFF, 0xDA, 0, 2, 1, 2, 3)
	return append(out, 0xFF, 0xD9)
}

func TestReadJPEG(t *testing.T) {
	metadata, err := ReadBytes(jpegWith(tiffBlock(), 6000, 4000))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 6000 || metadata.Height != 4000 {
		t.Fatalf("size %dx%d", metadata.Width, metadata.Height)
	}
	if width, height := metadata.Rotated(); width != 4000 || height != 6000 {
		t.Fatalf("rotated %dx%d", width, height)
	}
	if metadata.Orientation != 6 {
		t.Fatalf("orientation %d", metadata.Orientation)
	}
	if metadata.Make != "Canon" || metadata.Model != "EOS R5" || metadata.Lens != "RF 50mm" || metadata.Software != "picsel" {
		t.Fatalf("camera %+v", metadata)
	}
	if metadata.ISO != 100 {
		t.Fatalf("iso %d", metadata.ISO)
	}
	if math.Abs(metadata.FNumber-2.8) > 1e-9 {
		t.Fatalf("fnumber %v", metadata.FNumber)
	}
	if math.Abs(metadata.Exposure-0.004) > 1e-9 {
		t.Fatalf("exposure %v", metadata.Exposure)
	}
	if math.Abs(metadata.FocalLength-50) > 1e-9 {
		t.Fatalf("focal %v", metadata.FocalLength)
	}
	want := time.Date(2024, 3, 2, 10, 11, 12, 0, time.FixedZone("", -3*3600))
	if !metadata.Captured.Equal(want) {
		t.Fatalf("captured %v, want %v", metadata.Captured, want)
	}
	if !metadata.Digitized.Equal(want.Add(time.Second)) {
		t.Fatalf("digitized %v", metadata.Digitized)
	}
	if metadata.GPS == nil {
		t.Fatal("no gps")
	}
	if math.Abs(metadata.GPS.Latitude+34.603333) > 1e-5 {
		t.Fatalf("latitude %v", metadata.GPS.Latitude)
	}
	if math.Abs(metadata.GPS.Longitude+58.38) > 1e-5 {
		t.Fatalf("longitude %v", metadata.GPS.Longitude)
	}
	if !metadata.GPS.AltitudeSet || math.Abs(metadata.GPS.Altitude-123.4) > 1e-9 {
		t.Fatalf("altitude %v", metadata.GPS.Altitude)
	}
}

func TestReadJPEGWithoutEXIF(t *testing.T) {
	metadata, err := ReadBytes(jpegWith(nil, 800, 600))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 800 || metadata.Height != 600 {
		t.Fatalf("size %dx%d", metadata.Width, metadata.Height)
	}
	if metadata.Make != "" || metadata.GPS != nil {
		t.Fatalf("metadata %+v", metadata)
	}
}

func TestReadPNG(t *testing.T) {
	b := []byte("\x89PNG\r\n\x1a\n")
	ihdr := []byte{0, 0, 0, 13, 'I', 'H', 'D', 'R', 0, 0, 1, 44, 0, 0, 0, 200, 8, 6, 0, 0, 0}
	b = append(b, ihdr...)
	b = append(b, 0, 0, 0, 0)
	metadata, err := ReadBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 300 || metadata.Height != 200 {
		t.Fatalf("size %dx%d", metadata.Width, metadata.Height)
	}
}

func TestReadWebP(t *testing.T) {
	b := make([]byte, 30)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WEBPVP8X")
	width, height := 1024, 768
	b[24] = byte(width - 1)
	b[25] = byte((width - 1) >> 8)
	b[26] = byte((width - 1) >> 16)
	b[27] = byte(height - 1)
	b[28] = byte((height - 1) >> 8)
	b[29] = byte((height - 1) >> 16)
	metadata, err := ReadBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != width || metadata.Height != height {
		t.Fatalf("size %dx%d", metadata.Width, metadata.Height)
	}
}

func TestReadRejectsGarbage(t *testing.T) {
	if _, err := ReadBytes([]byte("not a picture")); err != ErrFormat {
		t.Fatalf("error %v", err)
	}
	if _, err := ReadBytes(nil); err != ErrFormat {
		t.Fatalf("error %v", err)
	}
}

// TestReadTruncated walks every prefix of a JPEG with EXIF: a file the server
// received half of must not panic, and must not read past its end.
func TestReadTruncated(t *testing.T) {
	full := jpegWith(tiffBlock(), 6000, 4000)
	for end := 0; end <= len(full); end++ {
		func() {
			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("prefix of %d bytes panicked: %v", end, recovered)
				}
			}()
			ReadBytes(full[:end])
		}()
	}
}

func TestReadFromReaderAt(t *testing.T) {
	full := jpegWith(tiffBlock(), 6000, 4000)
	reader := readerOf(full)
	metadata, err := Read(t.Context(), reader, int64(len(full)))
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Width != 6000 {
		t.Fatalf("width %d", metadata.Width)
	}
}

type readerOf []byte

func (r readerOf) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r)) {
		return 0, io.EOF
	}
	n := copy(p, r[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
