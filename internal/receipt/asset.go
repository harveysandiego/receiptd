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
// assets.Store.Get at layout time: Validate is fast and local, so it
// cannot check whether the asset actually exists (that I/O is deferred to
// render/layout.Build). Align is the closed enum Text.Align uses; Width is
// the requested rendered width in dots, clamped to the printable page
// width by render/layout.Build (docs/adr/0013-text-and-asset-alignment.md).
type Asset struct {
	Name  string `json:"name"`
	Width int    `json:"width,omitempty"`
	Align string `json:"align,omitempty"`
}

// Validate reports whether a is well-formed: Name must be non-empty,
// valid UTF-8, and free of path separators or a bare "." / ".." — Name
// becomes a lookup key an assets.Store may turn into a filesystem path
// (see assets.FilesystemStore), so rejecting a name that could traverse
// outside the store's root is a local, no-I/O invariant of Asset itself.
// Align must be "", "left", "center", or "right"; Width, if set, must not
// be negative.
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

// MarshalJSON encodes a with the "type":"asset" discriminator the registry
// polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
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
