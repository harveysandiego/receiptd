package receipt

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"strings"

	// Blank imports register the decoders for every format Image.Data may
	// decode as — see supportedImageFormatNames.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// Image is a bitmap embedded directly in the Receipt as inline bytes —
// "here are the bytes," as opposed to Asset's "look this name up"
// (docs/ARCHITECTURE.md §3 "Image vs. Asset"). Data marshals to a base64
// string via encoding/json's standard []byte handling.
type Image struct {
	Data []byte `json:"data"`
}

// supportedImageFormatNames is the single source both
// IsSupportedImageFormat and SupportedImageFormatsList derive from, so
// they can never drift apart. The set is checked explicitly (rather than
// trusting only these decoders are registered) so that some unrelated
// package linking in e.g. image/tiff cannot silently widen what Image
// accepts. SVG is deliberately absent: it is not raster and this project
// has no rasterizer (docs/adr/0002-raster-rendering.md), so SVG bytes
// simply fail to decode like any other unsupported input.
var supportedImageFormatNames = []string{"png", "jpeg", "gif", "bmp", "webp"}

// supportedImageFormats is supportedImageFormatNames as a set, for
// IsSupportedImageFormat's O(1) lookup.
var supportedImageFormats = func() map[string]struct{} {
	m := make(map[string]struct{}, len(supportedImageFormatNames))
	for _, name := range supportedImageFormatNames {
		m[name] = struct{}{}
	}
	return m
}()

// SupportedImageFormatsList is supportedImageFormatNames joined for
// human-readable error messages (e.g. "png, jpeg, gif, bmp, webp").
var SupportedImageFormatsList = strings.Join(supportedImageFormatNames, ", ")

// IsSupportedImageFormat reports whether format — an image.Decode or
// image.DecodeConfig format name — is one Image.Data may use.
func IsSupportedImageFormat(format string) bool {
	_, ok := supportedImageFormats[format]
	return ok
}

// MaxImagePixels bounds the total pixel count (width * height) Image.Data
// may declare, guarding against a decompression bomb: a tiny, highly
// compressible file whose header claims e.g. 40000x40000 would force a
// multi-gigabyte bitmap allocation once decoded. 20,000,000 (a 5000x4000
// photo) covers any legitimate receipt image while bounding the decoded
// worst case to roughly 80MB. render/layout.checkImageDimensions applies
// the same bound to an Asset's resolved bytes.
const MaxImagePixels = 20_000_000

// Validate reports whether i is well-formed: Data must be non-empty,
// decode as one of supportedImageFormats, and declare no more than
// MaxImagePixels. It reads only Data's header via image.DecodeConfig:
// decoding full pixel data before the MaxImagePixels check would defeat
// the check's purpose, and the format name and dimensions are all
// Validate needs. This is local, in-memory work, so it fits the package's
// "Validate stays fast and local" convention (docs/ARCHITECTURE.md §3).
// The pixel-count check multiplies in int64 because Go's int is 32 bits
// on some target architectures, where cfg.Width*cfg.Height could overflow
// before reaching MaxImagePixels and silently defeat the check.
func (i Image) Validate() error {
	if len(i.Data) == 0 {
		return errors.New("image: data is required")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(i.Data))
	if err != nil {
		return fmt.Errorf("image: data does not decode as a supported image format: %w", err)
	}
	if !IsSupportedImageFormat(format) {
		return fmt.Errorf("image: unsupported format %q (supported: %s)", format, SupportedImageFormatsList)
	}
	if pixels := int64(cfg.Width) * int64(cfg.Height); pixels > MaxImagePixels {
		return fmt.Errorf("image: %dx%d (%d pixels) exceeds the %d pixel limit", cfg.Width, cfg.Height, pixels, MaxImagePixels)
	}
	return nil
}

// MarshalJSON encodes i with the "type":"image" discriminator the
// registry polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
func (i Image) MarshalJSON() ([]byte, error) {
	type alias Image
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "image", alias: alias(i)})
}

func init() {
	registerElement("image", func(data []byte, _ int) (Element, error) {
		var i Image
		if err := json.Unmarshal(data, &i); err != nil {
			return nil, err
		}
		return i, nil
	})
}
