package receipt_test

import (
	"encoding/json"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestSpacerValidate(t *testing.T) {
	tests := []struct {
		name    string
		spacer  receipt.Spacer
		wantErr bool
	}{
		{"zero value", receipt.Spacer{}, false},
		{"positive height", receipt.Spacer{Height: 20}, false},
		{"negative height", receipt.Spacer{Height: -1}, true},
		{"height at the max", receipt.Spacer{Height: 10000}, false},
		{"height over the max", receipt.Spacer{Height: 10001}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.spacer.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpacer_JSONRoundTrip(t *testing.T) {
	original := receipt.Spacer{Height: 20}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "spacer" {
		t.Errorf(`wire["type"] = %v, want "spacer"`, wire["type"])
	}

	var decoded receipt.Spacer
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if decoded != original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
