package receipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // raster format Image.Data may decode as — see supportedImageFormatNames
	_ "image/jpeg" // raster format Image.Data may decode as — see supportedImageFormatNames
	_ "image/png"  // raster format Image.Data may decode as — see supportedImageFormatNames
	"strings"

	_ "golang.org/x/image/bmp"  // raster format Image.Data may decode as — see supportedImageFormatNames
	_ "golang.org/x/image/webp" // raster format Image.Data may decode as — see supportedImageFormatNames
)

// Image is a bitmap embedded directly in the Receipt as inline bytes —
// "here are the bytes," as opposed to Asset's "look this name up"
// (docs/ARCHITECTURE.md §3 "Image vs. Asset"). Data is JSON-encoded as a
// base64 string automatically by encoding/json's standard []byte
// handling — the "data (inline base64)" the element table describes is
// exactly what a []byte field already gives for free, with no custom
// marshaling needed for that part.
type Image struct {
	Data []byte `json:"data"`
}

// supportedImageFormatNames is every image.Decode/image.DecodeConfig
// format name Image.Data may decode as, checked explicitly against the
// format image.Decode itself reports rather than trusting that no other
// decoder happens to be registered elsewhere in the binary — a program
// that links in some other package's image/tiff (or similar) import for
// unrelated reasons must not silently widen what Image accepts. This is
// the single source both IsSupportedImageFormat (the set membership
// check) and SupportedImageFormatsList (the same names, formatted for
// error messages) derive from, so the two can never list a different set
// of formats than each other. Exported behaviour (IsSupportedImageFormat)
// so render/layout's decoding — which independently decodes the same
// Data to paint it, for the same reason explained there — checks against
// this exact same set, rather than each package maintaining its own list
// that could silently drift apart.
//
// SVG is deliberately absent: it is not a raster format (no
// image.RegisterFormat entry decodes it) and this project has no SVG
// rasterizer (docs/adr/0002-raster-rendering.md) — SVG bytes simply fail
// to decode at all via image.Decode, the same "does not decode as a
// supported image format" error any other unsupported input produces.
var supportedImageFormatNames = []string{"png", "jpeg", "gif", "bmp", "webp"}

// supportedImageFormats is supportedImageFormatNames as a set, for
// IsSupportedImageFormat's O(1) lookup — the conventional Go
// representation of a set (an empty-struct value costs nothing per entry,
// unlike map[string]bool's unused bool).
var supportedImageFormats = func() map[string]struct{} {
	m := make(map[string]struct{}, len(supportedImageFormatNames))
	for _, name := range supportedImageFormatNames {
		m[name] = struct{}{}
	}
	return m
}()

// SupportedImageFormatsList is supportedImageFormatNames joined for
// human-readable error messages (e.g. "png, jpeg, gif, bmp, webp") —
// generated from the same slice IsSupportedImageFormat checks against, so
// an error message can never fall out of sync with what's actually
// accepted as formats are added or removed.
var SupportedImageFormatsList = strings.Join(supportedImageFormatNames, ", ")

// IsSupportedImageFormat reports whether format — an image.Decode or
// image.DecodeConfig format name — is one Image.Data may use.
func IsSupportedImageFormat(format string) bool {
	_, ok := supportedImageFormats[format]
	return ok
}

// Validate reports whether i is well-formed: Data must be non-empty and
// decode as one of supportedImageFormats. Decoding here is local,
// in-memory work against bytes the caller already holds, not I/O, so it
// fits this package's "Validate stays fast and local" convention
// (docs/ARCHITECTURE.md §3: "Image.Validate() checks Data decodes as a
// supported image format"). Animated GIFs decode as their first frame
// only: image.RegisterFormat("gif", ...) registers image/gif's Decode
// (single image.Image), not DecodeAll, so this falls out of using the
// same image.Decode entry point every format uses — no animation-specific
// code needed here.
func (i Image) Validate() error {
	if len(i.Data) == 0 {
		return errors.New("image: data is required")
	}
	_, format, err := image.Decode(bytes.NewReader(i.Data))
	if err != nil {
		return fmt.Errorf("image: data does not decode as a supported image format: %w", err)
	}
	if !IsSupportedImageFormat(format) {
		return fmt.Errorf("image: unsupported format %q (supported: %s)", format, SupportedImageFormatsList)
	}
	return nil
}

// MarshalJSON encodes i alongside the "type":"image" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (i Image) MarshalJSON() ([]byte, error) {
	type alias Image
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "image", alias: alias(i)})
}

func init() {
	registerElement("image", func(data []byte) (Element, error) {
		var i Image
		if err := json.Unmarshal(data, &i); err != nil {
			return nil, err
		}
		return i, nil
	})
}
