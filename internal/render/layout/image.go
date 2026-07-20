package layout

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // raster format receipt.Image.Data may decode as — see receipt.IsSupportedImageFormat
	_ "image/jpeg" // raster format receipt.Image.Data may decode as — see receipt.IsSupportedImageFormat
	_ "image/png"  // raster format receipt.Image.Data may decode as — see receipt.IsSupportedImageFormat

	_ "golang.org/x/image/bmp"  // raster format receipt.Image.Data may decode as — see receipt.IsSupportedImageFormat
	_ "golang.org/x/image/webp" // raster format receipt.Image.Data may decode as — see receipt.IsSupportedImageFormat

	"github.com/harveysandiego/receiptd/internal/receipt"
)

// scaledImageSize returns the dimensions a decoded image of origWidth x
// origHeight paints at, given the Document's printable width maxWidth:
// unchanged if it already fits, or maxWidth <= 0 (Build's documented "no
// printer configured" sentinel — the same convention wrapText's own doc
// comment describes for text), otherwise scaled down to exactly maxWidth
// wide, preserving aspect ratio via integer division. Images are only
// ever shrunk to fit the printable width, mirroring the "content must fit
// the printable width" principle text wrapping already established for
// this milestone — never enlarged, since nothing in the frozen
// architecture calls for an upscale mode and upscaling would only soften
// a raster image with no corresponding benefit.
func scaledImageSize(origWidth, origHeight, maxWidth int) (width, height int) {
	if maxWidth <= 0 || origWidth <= maxWidth {
		return origWidth, origHeight
	}
	return maxWidth, origHeight * maxWidth / origWidth
}

// checkSupportedFormat returns an error unless format is one
// receipt.IsSupportedImageFormat accepts — the shared check decodeImage
// and imageHeight both apply to whatever image.Decode/
// image.DecodeConfig reports, rather than trusting that no other decoder
// happens to be registered elsewhere in the binary: a program that links
// in another package's image/tiff (or similar) import for unrelated
// reasons must not silently widen what this accepts. Delegated to
// receipt.IsSupportedImageFormat, rather than this package keeping its
// own separate list, so render/layout's decoding and receipt.Image's own
// Validate() (which independently decodes the same Data, for the same
// reason) can never accept a different set of formats than each other.
func checkSupportedFormat(format string) error {
	if !receipt.IsSupportedImageFormat(format) {
		return fmt.Errorf("image: unsupported format %q (supported: %s)", format, receipt.SupportedImageFormatsList)
	}
	return nil
}

// checkImageDimensions returns an error if cfg's declared pixel count
// exceeds receipt.MaxImagePixels — the same decompression-bomb check
// receipt.Image.Validate applies to a receipt.Image's own Data, applied
// again here since decodeImage and imageHeight independently decode Data
// for their own reasons (see decodeImage's doc comment) and, for a
// receipt.Asset, are the *only* check: an Asset's resolved bytes reach
// here via assets.Store.Get in Build's own receipt.Asset case, never
// through receipt.Image.Validate at all. Multiplies in int64 rather than
// cfg's own int fields directly — see receipt.Image.Validate's identical
// note on its own equivalent check for why.
func checkImageDimensions(cfg image.Config) error {
	if pixels := int64(cfg.Width) * int64(cfg.Height); pixels > receipt.MaxImagePixels {
		return fmt.Errorf("image: %dx%d (%d pixels) exceeds the %d pixel limit", cfg.Width, cfg.Height, pixels, receipt.MaxImagePixels)
	}
	return nil
}

// decodeImage decodes data as any receipt.IsSupportedImageFormat raster
// format (checkSupportedFormat) into the same image.Image representation
// regardless of source format — everything downstream of this call
// (scaledImageSize, rasterizeImage, darkOverWhite) is entirely
// format-agnostic, so a new supported format only ever needs a decoder
// registered (see this file's blank imports) and an entry in
// receipt.supportedImageFormats, never a change here or below. It checks
// the format and declared dimensions via the cheap image.DecodeConfig
// header read (checkSupportedFormat, checkImageDimensions) before ever
// calling the full image.Decode below it — checking after would already
// have paid the cost checkImageDimensions exists to avoid.
func decodeImage(data []byte) (image.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if err := checkSupportedFormat(format); err != nil {
		return nil, err
	}
	if err := checkImageDimensions(cfg); err != nil {
		return nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// decodeImageConfig reads data's header via image.DecodeConfig — width,
// height, and format, with no pixel data decoded — and applies the same
// checkSupportedFormat/checkImageDimensions checks decodeImage applies
// before its own full image.Decode. Shared by imageHeight and assetHeight,
// the two Build-side callers that need only a dimension, not pixels, to
// advance Y. Animated GIFs report their first frame's dimensions like any
// other GIF: an animated GIF's frames may in principle differ in size, but
// image.DecodeConfig (like image/gif.Decode — see receipt.Image.Validate's
// doc comment) only ever reports the first frame's, keeping this in
// agreement with what DecodeImageBitmap/DecodeAlignedAssetBitmap actually
// paint.
func decodeImageConfig(data []byte) (image.Config, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return image.Config{}, err
	}
	if err := checkSupportedFormat(format); err != nil {
		return image.Config{}, err
	}
	if err := checkImageDimensions(cfg); err != nil {
		return image.Config{}, err
	}
	return cfg, nil
}

// imageHeight returns the height, in dots, a receipt.Image's Data will
// occupy once painted, scaled to fit maxWidth per scaledImageSize — the
// only dimension Build itself needs, to advance Y (the scaled width
// matters only to render/canvas.Paint, which recomputes both via
// DecodeImageBitmap when it actually paints the Block, so returning width
// here as well would be a result no caller uses).
func imageHeight(data []byte, maxWidth int) (height int, err error) {
	cfg, err := decodeImageConfig(data)
	if err != nil {
		return 0, err
	}
	_, h := scaledImageSize(cfg.Width, cfg.Height, maxWidth)
	return h, nil
}

// DecodeImageBitmap decodes data as any supported raster format (see
// decodeImage), scales it to fit maxWidth (see scaledImageSize), and
// converts it to a GlyphBitmap — the same 1bpp pixel representation
// glyphs already use (Font.Glyph), so render/canvas.Paint paints an Image
// Block with the exact same paintGlyph primitive it already paints text
// with: there is exactly one bitmap-painting path, not a parallel one for
// images (docs/ARCHITECTURE.md §4), and exactly one image-decoding path,
// not one per format. Exported because render/canvas.Paint calls it
// directly against the receipt.Image.Data its Block carries — Build only
// needs imageHeight (a dimension, not pixels) to advance Y, so the two
// stages never redundantly decode full pixel data in the same place, but
// Build and Paint still independently decode Data once each: deferring
// pixel decoding to Paint, rather than resolving it once in Build and
// threading a decoded bitmap through Block, keeps Block exactly the
// {Y, Element, Style} shape docs/ARCHITECTURE.md §2 already documents,
// and — since a GlyphBitmap holds a []byte — avoids making Block itself
// uncomparable, which several existing tests rely on (e.g.
// TestBuild_Deterministic's Blocks[i] != Blocks[i] check).
func DecodeImageBitmap(data []byte, maxWidth int) (GlyphBitmap, error) {
	img, err := decodeImage(data)
	if err != nil {
		return GlyphBitmap{}, err
	}
	bounds := img.Bounds()
	width, height := targetImageSize(bounds.Dx(), bounds.Dy(), 0, maxWidth)
	return rasterizeImage(img, width, height), nil
}

// DecodeAlignedAssetBitmap is the AlignedAsset analogue of
// DecodeImageBitmap: decodes a.Data, scales it to a.Width (clamped to
// maxWidth) or, if a.Width is 0, to maxWidth per scaledImageSize's
// existing shrink-only cap (both via targetImageSize, the same function
// assetHeight used to advance Y, so the two stages can never disagree),
// then — if a.Align is "center" or "right" — left-pads the rasterized
// bitmap with blank pixel columns via alignBitmap so it paints at the
// correct horizontal offset once render/canvas.Paint blits it starting at
// x=0. An AlignedAsset with a.Width == 0 and a.Align == "" produces
// exactly the bitmap DecodeImageBitmap already would for the same Data —
// see docs/adr/0013-text-and-asset-alignment.md.
func DecodeAlignedAssetBitmap(a AlignedAsset, maxWidth int) (GlyphBitmap, error) {
	img, err := decodeImage(a.Data)
	if err != nil {
		return GlyphBitmap{}, err
	}
	bounds := img.Bounds()
	width, height := targetImageSize(bounds.Dx(), bounds.Dy(), a.Width, maxWidth)
	bmp := rasterizeImage(img, width, height)
	return alignBitmap(bmp, a.Align, maxWidth), nil
}

// alignBitmap is alignPad's pixel-domain sibling: for a.Align "center" or
// "right", it left-pads bmp with blank (unset) pixel columns so it sits
// at the correct horizontal offset once painted at x=0 — maxWidth minus
// bmp.Width of them for "right", half that for "center" — by allocating a
// maxWidth-wide GlyphBitmap and copying bmp's set bits in at the computed
// column offset, leaving every other bit unset (blank/white). align ""
// or "left", or maxWidth <= 0 or bmp already as wide as or wider than
// maxWidth (no room to move it), returns bmp unchanged — the same
// fallback alignPad itself applies.
func alignBitmap(bmp GlyphBitmap, align string, maxWidth int) GlyphBitmap {
	if align != alignCenter && align != alignRight {
		return bmp
	}
	if maxWidth <= 0 || bmp.Width >= maxWidth {
		return bmp
	}
	offset := maxWidth - bmp.Width
	if align == alignCenter {
		offset /= 2
	}
	rowBytes := (maxWidth + 7) / 8
	srcRowBytes := (bmp.Width + 7) / 8
	bits := make([]byte, rowBytes*bmp.Height)
	for y := 0; y < bmp.Height; y++ {
		for x := 0; x < bmp.Width; x++ {
			if bmp.Bits[y*srcRowBytes+x/8]&(0x80>>uint(x%8)) == 0 {
				continue
			}
			px := x + offset
			bits[y*rowBytes+px/8] |= 0x80 >> uint(px%8)
		}
	}
	return GlyphBitmap{Width: maxWidth, Height: bmp.Height, Bits: bits}
}

// rasterizeImage converts img into a GlyphBitmap sized targetWidth x
// targetHeight, sampling img via nearest-neighbour (the same
// integer-ratio sampling convention render/layout/embedded_font.go's
// upscale already uses for glyphs, applied here in the opposite direction
// — downscaling rather than upscaling) and thresholding each sampled
// pixel exactly as packMask there already thresholds glyph alpha:
// composited over an opaque white background (the receipt paper's own
// colour) and set only if the result is darker than half white — the
// same "half of fully covered" convention packMask established for glyph
// masks, generalised here from alpha-only to full RGBA (see darkOverWhite).
func rasterizeImage(img image.Image, targetWidth, targetHeight int) GlyphBitmap {
	bounds := img.Bounds()
	srcWidth, srcHeight := bounds.Dx(), bounds.Dy()
	rowBytes := (targetWidth + 7) / 8
	bits := make([]byte, rowBytes*targetHeight)
	for y := 0; y < targetHeight; y++ {
		sy := bounds.Min.Y + y*srcHeight/targetHeight
		for x := 0; x < targetWidth; x++ {
			sx := bounds.Min.X + x*srcWidth/targetWidth
			if darkOverWhite(img.At(sx, sy)) {
				bits[y*rowBytes+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	return GlyphBitmap{Width: targetWidth, Height: targetHeight, Bits: bits}
}

// darkOverWhite reports whether c, composited over an opaque white
// background, is darker than half white — the same threshold
// render/layout/embedded_font.go's packMask already applies to glyph
// alpha, generalised here to full colour. c's components are
// alpha-premultiplied (the color.Color contract), so compositing over
// white is c + (0xffff - a) per channel: a fully transparent pixel
// (a == 0) composites to pure white regardless of its nominal colour and
// is therefore never set, the same as unprinted paper — this is how
// transparency is handled, since docs/ARCHITECTURE.md does not define it
// explicitly.
func darkOverWhite(c color.Color) bool {
	r, g, b, a := c.RGBA()
	white := 0xffff - a
	lum := (r+white)*299/1000 + (g+white)*587/1000 + (b+white)*114/1000
	return lum < 0x8000
}
