package receipt

import (
	"encoding/json"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// elementDecoder decodes the raw JSON object for a single Element
// (including its "type" field, which a decoder is free to ignore) into a
// concrete Element value.
type elementDecoder func(data []byte) (Element, error)

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

// decodeElement decodes a single element's raw JSON object by resolving
// its "type" field through the registry.
func decodeElement(data []byte) (Element, error) {
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

	el, err := decode(data)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindValidation, "receipt.decodeElement", err)
	}
	return el, nil
}
