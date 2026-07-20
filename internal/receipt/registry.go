package receipt

import (
	"encoding/json"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// elementDecoder decodes the raw JSON object for a single Element
// (including its "type" field, which a decoder is free to ignore) into a
// concrete Element value. depth is the element's own nesting level within
// the Receipt (0 for a top-level Receipt.Elements entry) — every decoder
// accepts it, even the ten that never recurse and simply ignore it,
// so that Columns (the one type that does recurse, via Column.Elements)
// can pass it on to decodeElement for its own nested elements without a
// second, Columns-specific registry mechanism.
type elementDecoder func(data []byte, depth int) (Element, error)

// registry maps a JSON "type" discriminator to the decoder for the
// concrete Element type it names. Populated by each Element type's own
// init() — the same pattern as image.RegisterFormat or
// database/sql.Register — so adding an Element type never requires
// editing this file. See docs/adr/0001-receipt-model.md.
var registry = map[string]elementDecoder{}

// registerElement adds typ to the registry. Called from each Element
// type's own file via init(). A duplicate registration is a programming
// error, not a runtime condition a caller can trigger, so it panics at
// package-init time rather than silently overwriting the earlier entry.
func registerElement(typ string, decode elementDecoder) {
	if _, exists := registry[typ]; exists {
		panic(fmt.Sprintf("receipt: element type %q registered twice", typ))
	}
	registry[typ] = decode
}

// maxElementDepth bounds how deeply a Columns may nest another Columns
// inside its own Column.Elements. Decoding recurses on the Go call stack
// for each nesting level (decodeElement -> Columns' registered decoder ->
// Columns.unmarshalJSON -> decodeElement -> ...), and a Go stack overflow
// is a fatal, unrecoverable process crash — not a panic net/http's
// ServeMux can recover from — so this has to be enforced structurally
// during decode itself, rather than left to Validate() or the API's
// request-body size limit to catch indirectly (a deeply nested payload
// can be tiny). 32 is far beyond any legitimate receipt layout — Columns
// nested even two or three deep is already unusual — while leaving huge
// headroom under the default goroutine stack.
const maxElementDepth = 32

// decodeElement decodes a single element's raw JSON object by resolving
// its "type" field through the registry. depth is this element's own
// nesting level (see elementDecoder); callers decoding a Receipt's
// top-level Elements always pass 0.
func decodeElement(data []byte, depth int) (Element, error) {
	if depth > maxElementDepth {
		return nil, apperr.Wrap(apperr.KindValidation, "receipt.decodeElement", fmt.Errorf("element nesting exceeds the maximum depth of %d", maxElementDepth))
	}

	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, apperr.Wrap(apperr.KindValidation, "receipt.decodeElement", err)
	}

	decode, ok := registry[head.Type]
	if !ok {
		return nil, apperr.Wrap(apperr.KindValidation, "receipt.decodeElement", fmt.Errorf("unknown element type %q", head.Type))
	}

	el, err := decode(data, depth)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindValidation, "receipt.decodeElement", err)
	}
	return el, nil
}
