package receipt_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestQRCodeValidate(t *testing.T) {
	tests := []struct {
		name    string
		qr      receipt.QRCode
		wantErr bool
	}{
		{"zero value, no content", receipt.QRCode{}, true},
		{"valid content", receipt.QRCode{Content: "https://example.com"}, false},
		{"invalid UTF-8", receipt.QRCode{Content: "\xff\xfe"}, true},
		{"unsupported error_correction", receipt.QRCode{Content: "hello", ErrorCorrection: "extreme"}, true},
		{"low error_correction", receipt.QRCode{Content: "hello", ErrorCorrection: "low"}, false},
		{"medium error_correction", receipt.QRCode{Content: "hello", ErrorCorrection: "medium"}, false},
		{"quartile error_correction", receipt.QRCode{Content: "hello", ErrorCorrection: "quartile"}, false},
		{"high error_correction", receipt.QRCode{Content: "hello", ErrorCorrection: "high"}, false},
		{"payload too large for QR capacity", receipt.QRCode{Content: strings.Repeat("A", 10000)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.qr.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestQRCode_JSONRoundTrip(t *testing.T) {
	original := receipt.QRCode{Content: "https://example.com/list/42", Size: 300, ErrorCorrection: "high"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "qrcode" {
		t.Errorf(`wire["type"] = %v, want "qrcode"`, wire["type"])
	}

	var decoded receipt.QRCode
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestReceipt_WithQRCode_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.QRCode{Content: "https://example.com"},
			receipt.Text{Content: "After"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	var decoded receipt.Receipt
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestQRCode_RecoveryLevel_MapsErrorCorrectionToLibraryConstant(t *testing.T) {
	// Asserts the four levels map to pairwise distinct qr.ErrorCorrectionLevel
	// values, and that an omitted ErrorCorrection resolves to the same
	// value as an explicit "medium" — without pinning to the underlying
	// library's own constant names, which RecoveryLevel's doc comment
	// notes may not always agree with QRCodeErrorCorrectionLevels' naming.
	levels := map[int]struct{}{}
	for _, ec := range receipt.QRCodeErrorCorrectionLevels {
		lvl := receipt.QRCode{Content: "x", ErrorCorrection: ec}.RecoveryLevel()
		key := int(lvl)
		if _, dup := levels[key]; dup {
			t.Errorf("ErrorCorrection %q maps to a level already used by another level", ec)
		}
		levels[key] = struct{}{}
	}
	omitted := receipt.QRCode{Content: "x"}
	explicitMedium := receipt.QRCode{Content: "x", ErrorCorrection: "medium"}
	if got, want := omitted.RecoveryLevel(), explicitMedium.RecoveryLevel(); got != want {
		t.Errorf("omitted ErrorCorrection = %v, want same as explicit %q (%v)", got, "medium", want)
	}
}

func TestIsSupportedQRCodeErrorCorrection(t *testing.T) {
	tests := []struct {
		level string
		want  bool
	}{
		{"low", true},
		{"medium", true},
		{"quartile", true},
		{"high", true},
		{"extreme", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := receipt.IsSupportedQRCodeErrorCorrection(tt.level); got != tt.want {
			t.Errorf("IsSupportedQRCodeErrorCorrection(%q) = %v, want %v", tt.level, got, tt.want)
		}
	}
}
