package receipt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/boombuler/barcode/qr"
)

// QRCode renders as a generated QR code bitmap encoding Content — the
// "generation" counterpart to Image's "here are the bytes": QRCode
// carries a string a QR encoder turns into pixels at render time
// (render/layout.GenerateQRCodeBitmap), then flows through the same
// raster pipeline Image uses.
type QRCode struct {
	Content string `json:"content"`

	// Size is the QR code's target width and height in dots (always
	// square). Zero or omitted means DefaultQRCodeSize. Like Image, a
	// QRCode is only ever scaled down to fit the printable width, never
	// enlarged.
	Size int `json:"size,omitempty"`

	// ErrorCorrection selects the QR code's error-recovery level: one of
	// QRCodeErrorCorrectionLevels ("low", "medium", "quartile", "high" —
	// increasing recovery capacity at the cost of a denser code). Empty
	// means "medium".
	ErrorCorrection string `json:"error_correction,omitempty"`
}

// DefaultQRCodeSize is the width and height, in dots, a QRCode renders at
// when Size is omitted or non-positive.
const DefaultQRCodeSize = 200

// QRCodeErrorCorrectionLevels is every value QRCode.ErrorCorrection
// accepts: the QR standard's own L/M/Q/H levels, spelled out. See
// RecoveryLevel for how each maps to this package's QR encoder.
var QRCodeErrorCorrectionLevels = []string{"low", "medium", "quartile", "high"}

var qrCodeErrorCorrectionSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(QRCodeErrorCorrectionLevels))
	for _, l := range QRCodeErrorCorrectionLevels {
		m[l] = struct{}{}
	}
	return m
}()

// IsSupportedQRCodeErrorCorrection reports whether level is one
// QRCode.ErrorCorrection may use.
func IsSupportedQRCodeErrorCorrection(level string) bool {
	_, ok := qrCodeErrorCorrectionSet[level]
	return ok
}

// RecoveryLevel returns the github.com/boombuler/barcode/qr
// error-recovery level q.ErrorCorrection names. "", "medium", and any
// value Validate would reject all resolve to qr.M: this method cannot
// report an error, so it degrades to the default rather than panicking
// (Validate already rejects an unsupported level before rendering).
// Exported so render/layout.GenerateQRCodeBitmap resolves the exact same
// level Validate checked encodability against.
func (q QRCode) RecoveryLevel() qr.ErrorCorrectionLevel {
	switch q.ErrorCorrection {
	case "low":
		return qr.L
	case "quartile":
		return qr.Q
	case "high":
		return qr.H
	default: // "medium", "", or anything Validate would reject
		return qr.M
	}
}

// Validate reports whether q is well-formed: Content must be non-empty,
// valid UTF-8, and encodable as a QR code at q's ErrorCorrection level —
// checked by attempting the real encode (qr.Encode) rather than
// reimplementing its rules. This is local, in-memory CPU work, so it fits
// the package's "Validate stays fast and local" convention.
// ErrorCorrection, if set, must be one of QRCodeErrorCorrectionLevels.
func (q QRCode) Validate() error {
	if q.Content == "" {
		return errors.New("qrcode: content is required")
	}
	if !utf8.ValidString(q.Content) {
		return errors.New("qrcode: content must be valid UTF-8")
	}
	if q.ErrorCorrection != "" && !IsSupportedQRCodeErrorCorrection(q.ErrorCorrection) {
		return fmt.Errorf("qrcode: unsupported error_correction %q (supported: %s)", q.ErrorCorrection, strings.Join(QRCodeErrorCorrectionLevels, ", "))
	}
	if _, err := qr.Encode(q.Content, q.RecoveryLevel(), qr.Auto); err != nil {
		return fmt.Errorf("qrcode: content cannot be encoded: %w", err)
	}
	return nil
}

// MarshalJSON encodes q with the "type":"qrcode" discriminator the
// registry polymorphism decodes it back through (docs/adr/0001-receipt-model.md).
func (q QRCode) MarshalJSON() ([]byte, error) {
	type alias QRCode
	return json.Marshal(struct {
		Type string `json:"type"`
		alias
	}{Type: "qrcode", alias: alias(q)})
}

func init() {
	registerElement("qrcode", func(data []byte, _ int) (Element, error) {
		var q QRCode
		if err := json.Unmarshal(data, &q); err != nil {
			return nil, err
		}
		return q, nil
	})
}
