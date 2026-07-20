package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Table is a flat grid of plain-text cells: Headers names each column,
// and each entry in Rows is one row's cell content, in column order. A
// Table carries no styling or nested Elements of its own — see
// docs/ARCHITECTURE.md §3.
type Table struct {
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// Validate reports whether t is well-formed: at least one header, at
// least one row, every row's length matching len(Headers), and every
// header/cell valid UTF-8.
func (t Table) Validate() error {
	if len(t.Headers) == 0 {
		return errors.New("table: headers is required")
	}
	if len(t.Rows) == 0 {
		return errors.New("table: at least one row is required")
	}
	for _, h := range t.Headers {
		if !utf8.ValidString(h) {
			return fmt.Errorf("table: header %q is not valid UTF-8", h)
		}
	}
	for i, row := range t.Rows {
		if len(row) != len(t.Headers) {
			return fmt.Errorf("table: row %d has %d columns, want %d (len(headers))", i, len(row), len(t.Headers))
		}
		for _, cell := range row {
			if !utf8.ValidString(cell) {
				return fmt.Errorf("table: row %d has a cell that is not valid UTF-8", i)
			}
		}
	}
	return nil
}

// MarshalJSON encodes t alongside the "type":"table" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (t Table) MarshalJSON() ([]byte, error) {
	type alias Table
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "table", alias: alias(t)})
}

func init() {
	registerElement("table", func(data []byte, _ int) (Element, error) {
		var t Table
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, err
		}
		return t, nil
	})
}
