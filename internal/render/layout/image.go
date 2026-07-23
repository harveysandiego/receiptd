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

// scaledImageSize returns the dimensions an origWidth x origHeight image
// paints at within printable width maxWidth: unchanged if it fits or
// maxWidth <= 0 (Build's "no printer configured" sentinel, see wrapText),
// otherwise scaled down to maxWidth wide, aspect ratio preserved via
// integer division. Shrink-only, never enlarged: upscaling would only
// soften a raster image with no benefit.
func scaledImageSize(origWidth, origHeight, maxWidth int) (width, height int) {
	if maxWidth <= 0 || origWidth <= maxWidth {
		return origWidth, origHeight
	}
	return maxWidth, origHeight * maxWidth / origWidth
}

// checkSupportedFormat returns an error unless format is one
// receipt.IsSupportedImageFormat accepts. The check guards against another
// package's blank import (image/tiff or similar) silently widening what
// decodes here. Delegated to receipt rather than keeping a separate list,
// so this package's decoding and receipt.Image.Validate can never accept
// different format sets.
func checkSupportedFormat(format string) error {
	if !receipt.IsSupportedImageFormat(format) {
		return fmt.Errorf("image: unsupported format %q (supported: %s)", format, receipt.SupportedImageFormatsList)
	}
	return nil
}

// checkImageDimensions returns an error if cfg's declared pixel count
// exceeds receipt.MaxImagePixels — the decompression-bomb guard
// receipt.Image.Validate applies to Data, repeated here because for a
// receipt.Asset (resolved via assets.Store.Get, never through
// receipt.Image.Validate) this is the only such check. Multiplies in
// int64 to avoid overflow, matching receipt.Image.Validate.
func checkImageDimensions(cfg image.Config) error {
	if pixels := int64(cfg.Width) * int64(cfg.Height); pixels > receipt.MaxImagePixels {
		return fmt.Errorf("image: %dx%d (%d pixels) exceeds the %d pixel limit", cfg.Width, cfg.Height, pixels, receipt.MaxImagePixels)
	}
	return nil
}

// decodeImage decodes data as any supported raster format into a
// format-agnostic image.Image: everything downstream (scaledImageSize,
// rasterizeImage, darkOverWhite) is format-agnostic, so a new format needs
// only a decoder registered (see this file's blank imports) and a
// receipt.supportedImageFormats entry. The cheap image.DecodeConfig header
// checks run before the full image.Decode — checking after would already
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

// decodeImageConfig reads data's header via image.DecodeConfig (dimensions
// and format, no pixels) and applies the same checks decodeImage does
// before its full decode. Shared by imageHeight and assetHeight, which
// need a dimension, not pixels, to advance Y. An animated GIF reports its
// first frame's dimensions, matching what DecodeImageBitmap/
// DecodeAlignedAssetBitmap paint (see receipt.Image.Validate).
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

// imageHeight returns the height, in dots, a receipt.Image's Data occupies
// once painted, scaled to fit maxWidth (scaledImageSize). Only height is
// returned: Build needs it to advance Y, while render/canvas.Paint
// recomputes both dimensions via DecodeImageBitmap when it paints.
func imageHeight(data []byte, maxWidth int) (height int, err error) {
	cfg, err := decodeImageConfig(data)
	if err != nil {
		return 0, err
	}
	_, h := scaledImageSize(cfg.Width, cfg.Height, maxWidth)
	return h, nil
}

// DecodeImageBitmap decodes data as any supported raster format (see
// decodeImage), scales it to fit maxWidth (scaledImageSize), and converts
// it to a GlyphBitmap — the same 1bpp representation glyphs use, so
// render/canvas.Paint paints an Image Block with the one paintGlyph
// primitive (docs/ARCHITECTURE.md §4). Exported because Paint calls it
// directly; Build needs only imageHeight to advance Y. Build and Paint
// each decode Data once rather than threading a decoded bitmap through
// Block: that keeps Block the {Y, Element, Style} shape §2 documents and —
// since a GlyphBitmap holds a []byte — keeps Block comparable, which tests
// like TestBuild_Deterministic rely on.
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
// maxWidth) or, if a.Width is 0, to maxWidth (both via targetImageSize,
// shared with assetHeight so the stages can't disagree), then for a.Align
// "center"/"right" left-pads via alignBitmap so it paints at the right
// offset when blitted from x=0. a.Width == 0 and a.Align == "" reproduces
// DecodeImageBitmap's bitmap exactly — see
// docs/adr/0013-text-and-asset-alignment.md.
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

// alignBitmap is alignPad's pixel-domain sibling: for align "center" or
// "right" it left-pads bmp with blank columns (maxWidth-bmp.Width for
// "right", half that for "center") into a maxWidth-wide bitmap so it paints
// at the right offset from x=0. align "left"/"", maxWidth <= 0, or bmp
// already >= maxWidth returns bmp unchanged — the same fallback alignPad
// applies.
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

// rasterizeImage converts img into a targetWidth x targetHeight
// GlyphBitmap, sampling via nearest-neighbour (the integer-ratio
// convention embedded_font.go's upscale uses, here downscaling) and
// thresholding each pixel as packMask thresholds glyph alpha: composited
// over opaque white and set only if darker than half white (see
// darkOverWhite).
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

// darkOverWhite reports whether c, composited over opaque white, is darker
// than half white — the threshold packMask applies to glyph alpha,
// generalized to full colour. c's components are alpha-premultiplied (the
// color.Color contract), so compositing over white is c + (0xffff-a) per
// channel: a fully transparent pixel composites to white and is never set,
// like unprinted paper. This is how transparency is handled, since
// docs/ARCHITECTURE.md does not define it.
func darkOverWhite(c color.Color) bool {
	r, g, b, a := c.RGBA()
	white := 0xffff - a
	lum := (r+white)*299/1000 + (g+white)*587/1000 + (b+white)*114/1000
	return lum < 0x8000
}
