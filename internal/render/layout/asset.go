package layout

// AlignedAsset is a resolved receipt.Asset's pixel data plus its own
// already-declared Width/Align request. Unlike TableLine/ColumnsLine/
// BarcodeCaption, this is not layout-synthesized content with no 1:1
// element counterpart — it is the same one Asset's own fields, carried
// forward past the one resolution step (assets.Store.Get) only Build is
// positioned to perform. See docs/adr/0013-text-and-asset-alignment.md
// for why that resolution step, not "new types carry extra fields," is
// what actually forces this to be a distinct type from receipt.Asset or
// receipt.Image.
type AlignedAsset struct {
	Data  []byte
	Width int    // 0 = no explicit width requested (shrink-to-fit-only behavior, see resolveTargetWidth)
	Align string // "" (left, default) | "left" | "center" | "right"
}

// Validate always succeeds — see TableLine.Validate's identical doc
// comment: AlignedAsset is never part of a client-supplied
// receipt.Receipt, it exists only as a Block.Element Build itself
// produces.
func (AlignedAsset) Validate() error { return nil }

// resolveTargetWidth returns the width, in dots, an Asset with an
// explicitly requested width (0 = none) renders at, given the page's
// printable width maxWidth (0/negative = Build's "no printer configured"
// sentinel). render/layout.Build (to advance Y, via assetHeight) and
// render/canvas's DecodeAlignedAssetBitmap (to actually rasterize) share
// this one function so the two stages can never disagree — the same "one
// resolution function, two call sites" precedent ResolveSize already
// establishes for Text.Size/Divider.Size.
//
// An explicit requestedWidth may request either a smaller or a larger
// rendered size than the image's native pixel dimensions — unlike the
// implicit maxWidth cap (scaledImageSize), which only ever shrinks. This
// mirrors Barcode.Height, which already lets a receipt author request an
// arbitrary explicit dimension independent of the barcode's native size.
// Width is always clamped to the printable page width when one is known,
// the same hard "never wider than the printable area" rule every other
// raster element (Image, QRCode, Barcode) already enforces.
func resolveTargetWidth(requestedWidth, maxWidth int) int {
	if requestedWidth <= 0 {
		return 0
	}
	if maxWidth > 0 && requestedWidth > maxWidth {
		return maxWidth
	}
	return requestedWidth
}

// targetImageSize returns the width and height, in dots, an image of
// origWidth x origHeight paints at, given an explicit requestedWidth
// (0 = none, via resolveTargetWidth) and the Document's printable width
// maxWidth: when requestedWidth resolves to a positive value, the image
// scales to exactly that width, aspect ratio preserved (integer
// division, the same rounding scaledImageSize already uses); otherwise
// it falls back to scaledImageSize's existing shrink-only cap unchanged.
// assetHeight (header-only) and DecodeAlignedAssetBitmap (full pixel
// decode) share this one arithmetic function for the same "can't
// disagree" reason resolveTargetWidth's own doc comment gives.
func targetImageSize(origWidth, origHeight, requestedWidth, maxWidth int) (width, height int) {
	if w := resolveTargetWidth(requestedWidth, maxWidth); w > 0 {
		return w, origHeight * w / origWidth
	}
	return scaledImageSize(origWidth, origHeight, maxWidth)
}

// assetHeight returns the height, in dots, a resolved receipt.Asset's
// data will occupy once painted — the Asset analogue of imageHeight,
// additionally taking requestedWidth (receipt.Asset.Width) into account
// via targetImageSize. It reads only data's header via
// image.DecodeConfig, the same "Build needs a dimension, not pixels"
// reasoning imageHeight's own doc comment gives.
func assetHeight(data []byte, requestedWidth, maxWidth int) (height int, err error) {
	cfg, err := decodeImageConfig(data)
	if err != nil {
		return 0, err
	}
	_, h := targetImageSize(cfg.Width, cfg.Height, requestedWidth, maxWidth)
	return h, nil
}
