// Package printer defines a printer's capabilities (Profile) and its
// connection details (Connection) as separate types, the Printer
// interface used to send already-encoded bytes to a physical printer,
// and the network (TCP) transport implementation.
//
// Capabilities and transport are deliberately kept in one Go package but
// are never mixed in a single function signature: render/* only ever
// receives a Profile, never a Connection. cmd/receiptd is the only code
// in the module that constructs a Connection. See docs/ARCHITECTURE.md
// §1 for the full reasoning, including why the (currently single)
// network transport lives here rather than in its own subpackage.
package printer
