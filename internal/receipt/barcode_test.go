package receipt_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestBarcodeValidate(t *testing.T) {
	tests := []struct {
		name    string
		b       receipt.Barcode
		wantErr bool
	}{
		{"zero value, no content, no symbology", receipt.Barcode{}, true},
		{"empty content", receipt.Barcode{Symbology: "code128"}, true},
		{"empty symbology", receipt.Barcode{Content: "HELLO"}, true},
		{"unsupported symbology", receipt.Barcode{Content: "HELLO", Symbology: "codabar"}, true},
		{"invalid UTF-8", receipt.Barcode{Content: "\xff\xfe", Symbology: "code128"}, true},

		{"valid code128", receipt.Barcode{Content: "HELLO-128", Symbology: "code128"}, false},
		{"code128 content too long", receipt.Barcode{Content: strings.Repeat("A", 81), Symbology: "code128"}, true},

		{"valid ean13, no check digit", receipt.Barcode{Content: "400638133393", Symbology: "ean13"}, false},
		{"valid ean13, with check digit", receipt.Barcode{Content: "4006381333931", Symbology: "ean13"}, false},
		{"ean13 wrong length", receipt.Barcode{Content: "12345", Symbology: "ean13"}, true},
		{"ean13 non-digit", receipt.Barcode{Content: "40063813339X", Symbology: "ean13"}, true},
		{"ean13 given ean8-length content", receipt.Barcode{Content: "7351353", Symbology: "ean13"}, true},

		{"valid ean8, no check digit", receipt.Barcode{Content: "7351353", Symbology: "ean8"}, false},
		{"valid ean8, with check digit", receipt.Barcode{Content: "73513537", Symbology: "ean8"}, false},
		{"ean8 wrong length", receipt.Barcode{Content: "123", Symbology: "ean8"}, true},
		{"ean8 non-digit", receipt.Barcode{Content: "735135X", Symbology: "ean8"}, true},

		{"valid upca, no check digit", receipt.Barcode{Content: "12345678901", Symbology: "upca"}, false},
		{"valid upca, with check digit", receipt.Barcode{Content: "123456789012", Symbology: "upca"}, false},
		{"upca wrong length", receipt.Barcode{Content: "123456789", Symbology: "upca"}, true},
		{"upca non-digit", receipt.Barcode{Content: "1234567890X", Symbology: "upca"}, true},

		{"valid code39", receipt.Barcode{Content: "CODE-39", Symbology: "code39"}, false},
		{"code39 lowercase unsupported", receipt.Barcode{Content: "lowercase", Symbology: "code39"}, true},

		{"valid itf", receipt.Barcode{Content: "12345678", Symbology: "itf"}, false},
		{"itf odd digit count", receipt.Barcode{Content: "12345", Symbology: "itf"}, true},
		{"itf non-digit", receipt.Barcode{Content: "1234567X", Symbology: "itf"}, true},

		{"omitted Height (zero value) is valid", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 0}, false},
		{"positive Height is valid", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 200}, false},
		{"negative Height is valid (treated as omitted, see barcodeHeight)", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: -1}, false},
		{"Height at the upper bound is valid", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 10000}, false},
		{"Height just over the upper bound is invalid", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 10001}, true},
		{"Height far over the upper bound is invalid", receipt.Barcode{Content: "HELLO-128", Symbology: "code128", Height: 1 << 30}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.b.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBarcode_JSONRoundTrip(t *testing.T) {
	original := receipt.Barcode{Content: "400638133393", Symbology: "ean13", Height: 100, ShowText: true}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "barcode" {
		t.Errorf(`wire["type"] = %v, want "barcode"`, wire["type"])
	}

	var decoded receipt.Barcode
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestReceipt_WithBarcode_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.Barcode{Content: "HELLO-128", Symbology: "code128"},
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

func TestIsSupportedBarcodeSymbology(t *testing.T) {
	tests := []struct {
		symbology string
		want      bool
	}{
		{"code128", true},
		{"ean13", true},
		{"ean8", true},
		{"upca", true},
		{"code39", true},
		{"itf", true},
		{"codabar", false},
		{"qrcode", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := receipt.IsSupportedBarcodeSymbology(tt.symbology); got != tt.want {
			t.Errorf("IsSupportedBarcodeSymbology(%q) = %v, want %v", tt.symbology, got, tt.want)
		}
	}
}

func TestBarcodeSymbologies_MatchesADR0009(t *testing.T) {
	want := []string{"code128", "ean13", "ean8", "upca", "code39", "itf"}
	if !reflect.DeepEqual(receipt.BarcodeSymbologies, want) {
		t.Errorf("BarcodeSymbologies = %v, want %v", receipt.BarcodeSymbologies, want)
	}
}
