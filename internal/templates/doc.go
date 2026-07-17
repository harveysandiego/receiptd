// Package templates defines the Template interface and a registration
// registry (Register/Get/List) used by server-side templates such as a
// daily weather receipt. A Template only ever builds a receipt.Receipt —
// it never calls layout, canvas, escpos, or printer directly.
//
// Concrete templates (e.g. a future templates/weather package) are their
// own packages, self-registering via init() and wired in with a blank
// import at the composition root. See docs/adr/0004-extension-model.md.
package templates
