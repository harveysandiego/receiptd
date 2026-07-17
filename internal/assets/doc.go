// Package assets provides named asset storage (Get/Put/Delete/List) used
// to resolve receipt.Asset elements — logos and other reusable images
// referenced by name rather than embedded inline as a receipt.Image. See
// docs/adr/0001-receipt-model.md for why Asset is kept as a distinct
// concept from an inline Image, and what that buys for future asset
// resolution strategies (SVG, generated, themed).
package assets
