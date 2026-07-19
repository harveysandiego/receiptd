package escpos

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
)

// initSequence is ESC @, resetting the printer to its power-on defaults
// before anything else is sent.
var initSequence = []byte{0x1B, 0x40}

// rasterCommandFixed is the fixed portion of GS v 0, the raster-image
// print command: GS 'v' '0' m, where m selects normal (unscaled) mode.
// The width/height fields that vary per Canvas follow it in Encode.
var rasterCommandFixed = []byte{0x1D, 0x76, 0x30, 0x00}

// defaultFeedLines is how far Encode feeds the paper, in print lines,
// before cutting — clearance so the cut falls below the last printed row
// rather than through it. printer.Profile has no separate feed-distance
// field for this: ADR-0002 groups "initialization, feed, and cut" as the
// three genuine ESC/POS commands this design uses, and feed here is a
// fixed mechanical part of the cut sequence, not an independent knob.
const defaultFeedLines = 4

// Encode turns c into the ESC/POS byte stream needed to print it:
// initialization, c painted as one or more GS v 0 raster bands (see
// docs/adr/0002-raster-rendering.md), then — when profile.SupportsCut —
// a feed and a cut command, per docs/ARCHITECTURE.md §4 step 8e ("init
// bytes, raster commands ... feed, cut") and ADR-0002 ("`escpos.Encode
// (canvas, profile)` is the one place printer-specific byte sequences are
// produced"). profile is the single, authoritative source of
// printer-specific behavior; Encode has no other configuration surface.
//
// When profile.SupportsCut is false, Encode emits no feed or cut at all —
// there's nothing to clear a cutter for. When it's true, profile.DefaultCut
// selects "full" or "partial"; any other value (including "") is a
// misconfigured Profile and Encode returns apperr.KindPermanent. Resolving
// *which* Profile applies to a given Job is a decision made above Encode,
// not inside it (docs/ARCHITECTURE.md §4 step 8a).
//
// Encode is agnostic to what c contains — text, an image, a QR code — it
// only ever sees painted pixels, per ADR-0002. A Canvas with zero Width or
// Height has no content to print and returns apperr.KindPermanent,
// mirroring canvas.EncodePNG's contract for the same input. Encode also
// rejects a Canvas whose Bits length doesn't match Width x Height — a
// package-boundary check, since a malformed Bits slice would otherwise
// silently produce a raster command whose declared dimensions don't match
// the data that follows it.
//
// c is split into consecutive raster bands, each at most
// profile.MaxImageHeightDots rows tall (docs/ARCHITECTURE.md §4 step 8e,
// §11) — a printer-compatibility concern only: chunking changes how the
// image is transmitted, never what it depicts. A zero MaxImageHeightDots,
// or one at least c.Height, needs no splitting and produces the same
// single raster command Encode has always emitted.
func Encode(c *canvas.Canvas, profile printer.Profile) ([]byte, error) {
	if c.Width == 0 || c.Height == 0 {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas has no content (%dx%d)", c.Width, c.Height))
	}

	rowBytes := (c.Width + 7) / 8
	if want := rowBytes * c.Height; len(c.Bits) != want {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas Bits length %d does not match %dx%d dimensions (want %d)", len(c.Bits), c.Width, c.Height, want))
	}

	var feed, cut []byte
	if profile.SupportsCut {
		var err error
		cut, err = cutCommand(profile.DefaultCut)
		if err != nil {
			return nil, err
		}
		feed = feedCommand(defaultFeedLines)
	}

	band := bandHeight(c.Height, profile.MaxImageHeightDots)
	numBands := (c.Height + band - 1) / band
	out := make([]byte, 0, len(initSequence)+numBands*(len(rasterCommandFixed)+4)+len(c.Bits)+len(feed)+len(cut))
	out = append(out, initSequence...)
	out = appendRasterBands(out, c, rowBytes, band)
	out = append(out, feed...)
	out = append(out, cut...)

	return out, nil
}

// bandHeight returns how many rows each raster command emitted for a
// canvasHeight-tall Canvas should carry. maxImageHeightDots <= 0 means the
// printer needs no chunking at all (printer.Profile's documented "0 = no
// chunking"); a value at least canvasHeight needs no splitting either —
// both cases return canvasHeight, so the whole image fits in one band.
func bandHeight(canvasHeight, maxImageHeightDots int) int {
	if maxImageHeightDots <= 0 || maxImageHeightDots >= canvasHeight {
		return canvasHeight
	}
	return maxImageHeightDots
}

// appendRasterBands appends one GS v 0 command per band-tall slice of
// c's rows, top to bottom, until all of c.Height is covered — the last
// band shorter than band when c.Height isn't a whole multiple of it. Each
// band's data is an unmodified, contiguous slice of c.Bits, so row order
// and pixel content are preserved exactly across bands.
func appendRasterBands(out []byte, c *canvas.Canvas, rowBytes, band int) []byte {
	for start := 0; start < c.Height; start += band {
		height := band
		if start+height > c.Height {
			height = c.Height - start
		}
		out = append(out, rasterCommandFixed...)
		out = append(out, loHi(rowBytes)...)
		out = append(out, loHi(height)...)
		out = append(out, c.Bits[start*rowBytes:(start+height)*rowBytes]...)
	}
	return out
}

// feedCommand returns the ESC d n bytes requesting lines be fed.
func feedCommand(lines int) []byte {
	return []byte{0x1B, 0x64, byte(lines)}
}

// cutCommand returns the GS V m bytes for mode ("full" or "partial"). m
// selects an immediate cut with no automatic feed of its own — Encode's
// own feedCommand call already covers feeding.
func cutCommand(mode string) ([]byte, error) {
	switch mode {
	case "full":
		return []byte{0x1D, 0x56, 0x00}, nil
	case "partial":
		return []byte{0x1D, 0x56, 0x01}, nil
	default:
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("printer.Profile.DefaultCut %q is invalid (want \"full\" or \"partial\")", mode))
	}
}

// loHi returns n as the little-endian 16-bit pair (low byte, high byte)
// GS v 0's width/height fields expect.
func loHi(n int) []byte {
	return []byte{byte(n), byte(n >> 8)}
}
