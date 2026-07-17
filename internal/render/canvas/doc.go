// Package canvas paints a layout.Document onto a monochrome bitmap
// (Canvas) and encodes it as PNG for preview. PNG encoding is kept as a
// direct method on Canvas rather than behind a separate "Output
// interface" — see docs/ARCHITECTURE.md §11 for why that abstraction is
// deferred until a second output format actually exists.
package canvas
