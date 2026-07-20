package receipt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestAssetValidate(t *testing.T) {
	tests := []struct {
		name    string
		asset   receipt.Asset
		wantErr bool
	}{
		{"zero value, no name", receipt.Asset{}, true},
		{"empty name", receipt.Asset{Name: ""}, true},
		{"valid name", receipt.Asset{Name: "logo.png"}, false},
		{"valid name with width and align", receipt.Asset{Name: "logo.png", Width: 100, Align: "center"}, false},
		{"align is not restricted to a fixed set", receipt.Asset{Name: "logo.png", Align: "diagonal"}, false},
		{"name containing a forward slash", receipt.Asset{Name: "logos/logo.png"}, true},
		{"name containing a backslash", receipt.Asset{Name: `logos\logo.png`}, true},
		{"name is a single dot", receipt.Asset{Name: "."}, true},
		{"name is a parent-directory reference", receipt.Asset{Name: ".."}, true},
		{"name containing invalid UTF-8", receipt.Asset{Name: "logo-\xff.png"}, true},
		{"negative width", receipt.Asset{Name: "logo.png", Width: -1}, true},
		{"zero width", receipt.Asset{Name: "logo.png", Width: 0}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.asset.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAssetValidate_Deterministic(t *testing.T) {
	a := receipt.Asset{Name: "logo.png", Width: 100, Align: "center"}
	if err := a.Validate(); err != nil {
		t.Fatalf("first Validate() error = %v, want nil", err)
	}
	if err := a.Validate(); err != nil {
		t.Fatalf("second Validate() error = %v, want nil", err)
	}
}

func TestAsset_JSONRoundTrip(t *testing.T) {
	original := receipt.Asset{Name: "logo.png", Width: 100, Align: "center"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "asset" {
		t.Errorf(`wire["type"] = %v, want "asset"`, wire["type"])
	}

	var decoded receipt.Asset
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestAsset_JSONRoundTrip_MinimalFields(t *testing.T) {
	original := receipt.Asset{Name: "logo.png"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}
	var decoded receipt.Asset
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestReceipt_WithAsset_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.Asset{Name: "logo.png", Width: 100, Align: "center"},
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
