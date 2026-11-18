// Package exif reads what a picture says of itself before it goes to the
// server: the size it has, the way it must be turned to stand upright, the
// camera and the lens, the exposure, when it was taken and where.
//
// Read parses the headers of a JPEG, PNG or WebP file, and reads no more of it
// than it has to: a picture of 40MiB goes to the server without being decoded.
package exif

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrFormat is what Read answers for a file that is none of the formats it
// reads.
var ErrFormat = errors.New("exif: not a JPEG, PNG or WebP file")

// HeaderSize is how much of a file the headers of every format it reads need.
// Read reads that much, and a file hands Head that much to ReadBytes.
const HeaderSize = 256 << 10

// Metadata is what a picture says of itself. A field the picture says nothing
// of is the zero value, and GPS is nil when it says nothing of where it was
// taken.
type Metadata struct {
	// Width and Height are the size of the picture in pixels, as its header
	// gives them, before Orientation turns it.
	Width, Height int
	// Orientation is how the picture must be turned to stand upright, as EXIF
	// numbers it: 1 is as it is, 3 turns it 180°, 6 turns it 90° clockwise,
	// and 8 turns it 90° counterclockwise.
	Orientation int
	Make        string
	Model       string
	Lens        string
	Software    string
	ISO         int
	FNumber     float64
	// Exposure is how long the shutter was open, in seconds: 1/250 is 0.004.
	Exposure    float64
	FocalLength float64
	// Captured is when the shutter was pressed, and Digitized when the picture
	// was made of it; zero when the picture does not say.
	Captured  time.Time
	Digitized time.Time
	GPS       *Point
}

// Point is where a picture was taken.
type Point struct {
	Latitude  float64
	Longitude float64
	// Altitude is in meters above sea level.
	Altitude    float64
	AltitudeSet bool
}

// Rotated returns the size of the picture after Orientation turns it, which is
// the size a view shows.
func (m Metadata) Rotated() (width, height int) {
	if m.Orientation == 5 || m.Orientation == 6 || m.Orientation == 7 || m.Orientation == 8 {
		return m.Height, m.Width
	}
	return m.Width, m.Height
}

// Read reads the metadata of the file reader holds, of size bytes. It reads the
// first HeaderSize of it.
func Read(ctx context.Context, reader io.ReaderAt, size int64) (Metadata, error) {
	if err := ctx.Err(); err != nil {
		return Metadata{}, err
	}
	if size < 4 {
		return Metadata{}, ErrFormat
	}
	length := min(size, HeaderSize)
	head := make([]byte, length)
	read, err := reader.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return Metadata{}, err
	}
	return ReadBytes(head[:read])
}

// ReadBytes reads the metadata of the bytes of the start of a file.
func ReadBytes(b []byte) (Metadata, error) {
	switch {
	case isJPEG(b):
		return readJPEG(b)
	case isPNG(b):
		return readPNG(b)
	case isWebP(b):
		return readWebP(b)
	case isTIFF(b):
		var metadata Metadata
		parseTIFF(b, &metadata)
		return metadata, nil
	}
	return Metadata{}, ErrFormat
}

func isJPEG(b []byte) bool { return len(b) > 2 && b[0] == 0xFF && b[1] == 0xD8 }

func isPNG(b []byte) bool {
	return len(b) > 8 && string(b[1:4]) == "PNG" && b[0] == 0x89
}

func isWebP(b []byte) bool {
	return len(b) > 12 && string(b[0:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

func isTIFF(b []byte) bool {
	return len(b) > 8 && (string(b[0:4]) == "II*\x00" || string(b[0:4]) == "MM\x00*")
}
