package canvas

import (
	"bytes"
	"image"
	"image/color"
	"image/png"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// EncodePNG renders the raster Canvas as a PNG image for debugging,
// testing and visual inspection — it is not part of the printer pipeline
// itself (see escpos.Encode for that). Each set bit becomes a black
// pixel, each unset bit a white one. It never mutates c.
//
// EncodePNG requires a non-empty Canvas (Width and Height both greater
// than zero); an empty Canvas (e.g. Paint on an empty Document) returns
// apperr.KindPermanent rather than corrupt or placeholder bytes.
func (c *Canvas) EncodePNG() ([]byte, error) {
	img := image.NewGray(image.Rect(0, 0, c.Width, c.Height))
	rowBytes := (c.Width + 7) / 8
	for y := 0; y < c.Height; y++ {
		for x := 0; x < c.Width; x++ {
			v := color.Gray{Y: 0xff}
			if c.Bits[y*rowBytes+x/8]&(0x80>>uint(x%8)) != 0 {
				v = color.Gray{Y: 0}
			}
			img.SetGray(x, y, v)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, apperr.Wrap(apperr.KindPermanent, "canvas.EncodePNG", err)
	}
	return buf.Bytes(), nil
}
