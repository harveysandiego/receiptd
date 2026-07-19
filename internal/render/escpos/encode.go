package escpos

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
)

// initSequence is ESC @, resetting the printer to its power-on defaults
// before anything else is sent.
var initSequence = []byte{0x1B, 0x40}

// rasterCommandFixed is the fixed portion of GS v 0, the raster-image
// print command: GS 'v' '0' m, where m selects normal (unscaled) mode.
// The width/height fields that vary per Canvas follow it in Encode.
var rasterCommandFixed = []byte{0x1D, 0x76, 0x30, 0x00}

// Encode turns c into the ESC/POS byte stream needed to print it:
// initialization, followed by c painted as a single GS v 0 raster image —
// see docs/adr/0002-raster-rendering.md. Encode never chunks the image
// (docs/ARCHITECTURE.md §11: Profile-driven chunking ships as a no-op
// until real hardware testing proves it necessary) and does not feed or
// cut. Those are the only behaviors this design ties to a printer.Profile,
// so — like render/layout.Build, which for the same reason doesn't accept
// one yet either — Encode has nothing Profile-dependent to do in this
// slice and doesn't carry the parameter until it does.
//
// Encode is agnostic to what c contains — text, an image, a QR code — it
// only ever sees painted pixels, per ADR-0002. A Canvas with zero Width or
// Height has no content to print and returns apperr.KindPermanent,
// mirroring canvas.EncodePNG's contract for the same input. Encode also
// rejects a Canvas whose Bits length doesn't match Width x Height — a
// package-boundary check, since a malformed Bits slice would otherwise
// silently produce a raster command whose declared dimensions don't match
// the data that follows it.
func Encode(c *canvas.Canvas) ([]byte, error) {
	if c.Width == 0 || c.Height == 0 {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas has no content (%dx%d)", c.Width, c.Height))
	}

	rowBytes := (c.Width + 7) / 8
	if want := rowBytes * c.Height; len(c.Bits) != want {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas Bits length %d does not match %dx%d dimensions (want %d)", len(c.Bits), c.Width, c.Height, want))
	}

	out := make([]byte, 0, len(initSequence)+len(rasterCommandFixed)+4+len(c.Bits))
	out = append(out, initSequence...)
	out = append(out, rasterCommandFixed...)
	out = append(out, loHi(rowBytes)...)
	out = append(out, loHi(c.Height)...)
	out = append(out, c.Bits...)

	return out, nil
}

// loHi returns n as the little-endian 16-bit pair (low byte, high byte)
// GS v 0's width/height fields expect.
func loHi(n int) []byte {
	return []byte{byte(n), byte(n >> 8)}
}
