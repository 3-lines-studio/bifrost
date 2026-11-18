package qr

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Quiet is how many light modules a code carries around it, as readers need to
// find it: the quiet zone of the specification.
const Quiet = 4

// module is how many pixels a module of the bitmap of a code takes, enough to
// draw it small and sharp.
const module = 8

// image returns the code as a picture of scale pixels a module, quiet zone
// included, dark modules black on white.
func (c *Code) image(scale int) *image.NRGBA {
	if scale < 1 {
		scale = 1
	}
	side := (c.size + 2*Quiet) * scale
	picture := image.NewNRGBA(image.Rect(0, 0, side, side))
	white := color.NRGBA{255, 255, 255, 255}
	black := color.NRGBA{0, 0, 0, 255}
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			row := y/scale - Quiet
			column := x/scale - Quiet
			if c.Dark(row, column) {
				picture.SetNRGBA(x, y, black)
				continue
			}
			picture.SetNRGBA(x, y, white)
		}
	}
	return picture
}

// PNG returns the code as a PNG file of scale pixels a module, quiet zone
// included: 8 draws a code to read on a screen and to print small.
func (c *Code) PNG(scale int) ([]byte, error) {
	var out bytes.Buffer
	if err := png.Encode(&out, c.image(scale)); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// SVG returns the code as an SVG document of its modules, quiet zone included:
// it scales to any size, as the one of a page that prints.
func (c *Code) SVG() string {
	side := c.size + 2*Quiet
	var path strings.Builder
	for row := 0; row < c.size; row++ {
		for column := 0; column < c.size; column++ {
			if !c.Dark(row, column) {
				continue
			}
			x, y := column+Quiet, row+Quiet
			path.WriteString("M")
			path.WriteString(strconv.Itoa(x))
			path.WriteString(" ")
			path.WriteString(strconv.Itoa(y))
			path.WriteString("h1v1h-1z")
		}
	}
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %[1]d %[1]d" shape-rendering="crispEdges"><rect width="%[1]d" height="%[1]d" fill="#fff"/><path fill="#000" d="%[2]s"/></svg>`, side, path.String())
}

// picture is the bitmap of a code, made once.
type picture struct {
	once   sync.Once
	bitmap *ui.Bitmap
}

// Element draws the code in a view, size DIPs a side, quiet zone included. It
// converts the code to a bitmap once, and draws that bitmap on every frame.
func Element(c *ui.Context, code *Code, size float32) ui.Element {
	if code == nil {
		return ui.Box(c).Width(size).Height(size)
	}
	code.picture.once.Do(func() {
		code.picture.bitmap = ui.NewBitmap(code.image(module))
	})
	return ui.Image(c, code.picture.bitmap).Width(size).Height(size)
}
