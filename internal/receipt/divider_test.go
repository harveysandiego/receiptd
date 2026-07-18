package receipt_test

import (
	"encoding/json"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestDividerValidate(t *testing.T) {
	tests := []struct {
		name    string
		divider receipt.Divider
		wantErr bool
	}{
		{"zero value, style omitted", receipt.Divider{}, false},
		{"solid", receipt.Divider{Style: "solid"}, false},
		{"dashed", receipt.Divider{Style: "dashed"}, false},
		{"invalid style", receipt.Divider{Style: "dotted"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.divider.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDivider_JSONRoundTrip(t *testing.T) {
	original := receipt.Divider{Style: "dashed"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "divider" {
		t.Errorf(`wire["type"] = %v, want "divider"`, wire["type"])
	}

	var decoded receipt.Divider
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if decoded != original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
