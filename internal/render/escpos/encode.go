package escpos

import (
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
)

// initSequence is ESC @, resetting the printer to its power-on defaults
// before anything else is sent.
var initSequence = []byte{0x1B, 0x40}

// rasterCommandFixed is the fixed portion of GS v 0, the raster-image
// print command: GS 'v' '0' m, where m selects normal (unscaled) mode.
// The width/height fields that vary per Canvas follow it in Encode.
var rasterCommandFixed = []byte{0x1D, 0x76, 0x30, 0x00}

// defaultFeedLines is how far Encode feeds the paper before cutting, so
// the cut falls below the last printed row rather than through it. It has
// no printer.Profile field because feed here is a fixed mechanical part of
// the cut sequence, not an independent knob (docs/adr/0002-raster-rendering.md).
const defaultFeedLines = 4

// Encode turns c into an ESC/POS byte stream: init, c's pixels as one or
// more GS v 0 raster bands (chunked to profile.MaxImageHeightDots rows
// each), and — when profile.SupportsCut — a trailing feed+cut
// (docs/adr/0002-raster-rendering.md). c.Controls are interleaved at their
// Y, splitting the bands around them; an explicit trailing receipt.Cut
// suppresses the automatic one (docs/adr/0010-printer-control-elements-via-canvas-controls.md).
//
// profile.SupportsCut gates any cut, explicit or automatic, but not
// receipt.Feed, which needs no cutter hardware (see controlCommand).
//
// Returns apperr.KindPermanent for an empty Canvas, a Bits length not
// matching Width x Height, or a DefaultCut that isn't "full" or "partial".
func Encode(c *canvas.Canvas, profile printer.Profile) ([]byte, error) {
	if c.Width == 0 || c.Height == 0 {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas has no content (%dx%d)", c.Width, c.Height))
	}

	rowBytes := (c.Width + 7) / 8
	if want := rowBytes * c.Height; len(c.Bits) != want {
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("canvas Bits length %d does not match %dx%d dimensions (want %d)", len(c.Bits), c.Width, c.Height, want))
	}

	var autoFeed, autoCut []byte
	if profile.SupportsCut && !endsWithExplicitCut(c.Controls) {
		var err error
		autoCut, err = cutCommand(profile.DefaultCut)
		if err != nil {
			return nil, err
		}
		autoFeed = feedCommand(defaultFeedLines)
	}

	band := bandHeight(c.Height, profile.MaxImageHeightDots)
	out := make([]byte, 0, len(initSequence)+len(c.Bits)+(len(c.Controls)+1)*(len(rasterCommandFixed)+4)+len(c.Controls)*3+len(autoFeed)+len(autoCut))
	out = append(out, initSequence...)

	pos := 0
	for _, ctrl := range c.Controls {
		out = appendRasterBands(out, c, rowBytes, band, pos, ctrl.Y)
		pos = ctrl.Y

		cmd, err := controlCommand(ctrl.Element, profile)
		if err != nil {
			return nil, err
		}
		out = append(out, cmd...)
	}
	out = appendRasterBands(out, c, rowBytes, band, pos, c.Height)

	out = append(out, autoFeed...)
	out = append(out, autoCut...)

	return out, nil
}

// endsWithExplicitCut reports whether controls ends in a Terminal
// receipt.Cut (docs/ARCHITECTURE.md §4 step 8d). A Cut that isn't last, or
// a trailing receipt.Feed, doesn't count.
func endsWithExplicitCut(controls []canvas.Control) bool {
	if len(controls) == 0 {
		return false
	}
	last := controls[len(controls)-1]
	if !last.Terminal {
		return false
	}
	_, ok := last.Element.(receipt.Cut)
	return ok
}

// controlCommand returns the ESC/POS bytes for el (a receipt.Feed or
// receipt.Cut carried by a canvas.Control — nothing else is ever placed
// in Canvas.Controls, see canvas.Paint). A Cut with an empty Mode falls
// back to profile.DefaultCut, and is skipped entirely when
// profile.SupportsCut is false.
func controlCommand(el receipt.Element, profile printer.Profile) ([]byte, error) {
	switch e := el.(type) {
	case receipt.Feed:
		return feedCommand(e.Lines), nil
	case receipt.Cut:
		if !profile.SupportsCut {
			return nil, nil
		}
		mode := e.Mode
		if mode == "" {
			mode = profile.DefaultCut
		}
		return cutCommand(mode)
	default:
		return nil, apperr.Wrap(apperr.KindPermanent, "escpos.Encode", fmt.Errorf("unsupported control element type %T", el))
	}
}

// bandHeight returns how many rows each raster command should carry.
// maxImageHeightDots <= 0 is printer.Profile's documented "0 = no
// chunking"; that and any value >= canvasHeight return canvasHeight, so the
// whole image fits in one band.
func bandHeight(canvasHeight, maxImageHeightDots int) int {
	if maxImageHeightDots <= 0 || maxImageHeightDots >= canvasHeight {
		return canvasHeight
	}
	return maxImageHeightDots
}

// appendRasterBands appends one GS v 0 command per band-tall slice of c's
// rows within [from, to); from == to appends nothing. Encode calls it once
// per gap between c.Controls rather than once for the whole Canvas because
// a raster command can't have a control's bytes spliced into its data.
func appendRasterBands(out []byte, c *canvas.Canvas, rowBytes, band, from, to int) []byte {
	for start := from; start < to; start += band {
		height := band
		if start+height > to {
			height = to - start
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
