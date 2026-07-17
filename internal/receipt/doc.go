// Package receipt defines Receiptd's printer-agnostic document model:
// the Receipt type, its ordered Elements (text, headings, images, named
// assets, QR codes, barcodes, tables, and so on), and the registry-based
// JSON polymorphism used to serialize, deserialize, and validate them.
//
// No client of Receiptd ever needs to know it's talking to an ESC/POS
// printer; that is precisely what this package's document model exists
// to guarantee. See docs/ARCHITECTURE.md §3 and
// docs/adr/0001-receipt-model.md for the full design and rationale,
// including why Image and Asset are kept as distinct Element types.
package receipt
