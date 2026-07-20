package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Asset is a reference to a named, previously-stored image — "look this
// name up," as opposed to Image's "here are the bytes"
// (docs/ARCHITECTURE.md §3 "Image vs. Asset"). Name is resolved via
// assets.Store.Get at layout time, not here: Validate stays fast and
// local, so it cannot tell whether an asset actually exists (that's I/O,
// deferred to render/layout.Build). Align is the same closed enum
// Text.Align uses; Width is the requested rendered width in dots, clamped
// to the printable page width by render/layout.Build (see
// docs/adr/0013-text-and-asset-alignment.md).
type Asset struct {
	Name  string `json:"name"`
	Width int    `json:"width,omitempty"`
	Align string `json:"align,omitempty"`
}

// Validate reports whether a is well-formed: Name must be non-empty,
// valid UTF-8, and free of path separators or a bare "." / ".." — Name
// ultimately becomes a lookup key an assets.Store implementation may use
// to build a filesystem path (see assets.FilesystemStore), so rejecting a
// name that could traverse outside the store's root is a local,
// no-I/O-required invariant of Asset itself, the same way Table.Validate()
// already rejects malformed header/cell content. Align must be "", "left",
// "center", or "right" (docs/adr/0013-text-and-asset-alignment.md). Width,
// if set, must not be negative, the same convention Text.Size and
// Divider.Size already use.
func (a Asset) Validate() error {
	if a.Name == "" {
		return errors.New("asset: name is required")
	}
	if !utf8.ValidString(a.Name) {
		return fmt.Errorf("asset: name %q is not valid UTF-8", a.Name)
	}
	if strings.ContainsAny(a.Name, `/\`) || a.Name == "." || a.Name == ".." {
		return fmt.Errorf("asset: invalid name %q", a.Name)
	}
	switch a.Align {
	case "", "left", "center", "right":
	default:
		return fmt.Errorf("asset: invalid align %q", a.Align)
	}
	if a.Width < 0 {
		return errors.New("asset: width must not be negative")
	}
	return nil
}

// MarshalJSON encodes a alongside the "type":"asset" discriminator the
// registry-based polymorphism in docs/adr/0001-receipt-model.md relies on
// to decode it back.
func (a Asset) MarshalJSON() ([]byte, error) {
	type alias Asset
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "asset", alias: alias(a)})
}

func init() {
	registerElement("asset", func(data []byte, _ int) (Element, error) {
		var a Asset
		if err := json.Unmarshal(data, &a); err != nil {
			return nil, err
		}
		return a, nil
	})
}
