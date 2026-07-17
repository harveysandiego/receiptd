// Package layout turns a receipt.Receipt plus a printer.Profile into a
// Document: a fully resolved, positioned set of draw instructions (pure
// data — no pixels yet). It measures text, wraps lines, sizes
// columns/tables, and resolves Image/Asset elements into decoded pixel
// content.
//
// layout is the only stage in the rendering pipeline that performs I/O
// (resolving named assets via an assets.Store) — after Build returns,
// nothing downstream ever touches receipt.Receipt, an assets.Store, or
// any provider again. It also owns the Font interface: the one
// interface in this codebase kept despite having a single
// implementation, as a deliberate, documented exception — see
// docs/ARCHITECTURE.md §2 and §11.
package layout
