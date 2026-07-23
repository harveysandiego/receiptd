package layout

// AlignedAsset is a resolved receipt.Asset's pixel data plus its own
// Width/Align request. Unlike TableLine/ColumnsLine/BarcodeCaption it is not
// layout-synthesized content — it is one Asset's fields carried past the one
// resolution step (assets.Store.Get) only Build can perform. See
// docs/adr/0013-text-and-asset-alignment.md for why that resolution step,
// not "extra fields," forces a distinct type from receipt.Asset/Image.
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

// resolveTargetWidth returns the width, in dots, an Asset with an explicitly
// requested width (0 = none) renders at, given printable width maxWidth
// (0/negative = "no printer configured" sentinel). Build (via assetHeight)
// and DecodeAlignedAssetBitmap share this one function so the two stages
// can't disagree — the "one resolution function, two call sites" precedent
// ResolveSize sets.
//
// An explicit width may request a larger or smaller size than the image's
// native dimensions — unlike the implicit maxWidth cap (scaledImageSize),
// which only shrinks — mirroring Barcode.Height. Width is always clamped to
// the printable page width when known, like every other raster element.
func resolveTargetWidth(requestedWidth, maxWidth int) int {
	if requestedWidth <= 0 {
		return 0
	}
	if maxWidth > 0 && requestedWidth > maxWidth {
		return maxWidth
	}
	return requestedWidth
}

// targetImageSize returns the width and height, in dots, an origWidth x
// origHeight image paints at given an explicit requestedWidth (0 = none, via
// resolveTargetWidth) and printable width maxWidth: a positive resolved
// width scales to exactly that, aspect ratio preserved; otherwise it falls
// back to scaledImageSize's shrink-only cap. assetHeight and
// DecodeAlignedAssetBitmap share it for the same "can't disagree" reason as
// resolveTargetWidth.
func targetImageSize(origWidth, origHeight, requestedWidth, maxWidth int) (width, height int) {
	if w := resolveTargetWidth(requestedWidth, maxWidth); w > 0 {
		return w, origHeight * w / origWidth
	}
	return scaledImageSize(origWidth, origHeight, maxWidth)
}

// assetHeight returns the height, in dots, a resolved receipt.Asset's data
// occupies once painted — the Asset analogue of imageHeight, additionally
// honoring requestedWidth via targetImageSize. Header-only
// (image.DecodeConfig), for the same reason imageHeight gives.
func assetHeight(data []byte, requestedWidth, maxWidth int) (height int, err error) {
	cfg, err := decodeImageConfig(data)
	if err != nil {
		return 0, err
	}
	_, h := targetImageSize(cfg.Width, cfg.Height, requestedWidth, maxWidth)
	return h, nil
}
