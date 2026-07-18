package receipt_test

import (
	"encoding/json"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestHeadingValidate(t *testing.T) {
	tests := []struct {
		name    string
		heading receipt.Heading
		wantErr bool
	}{
		{"zero value", receipt.Heading{}, true},
		{"empty content", receipt.Heading{Content: ""}, true},
		{"content set", receipt.Heading{Content: "Shopping List"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.heading.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHeading_JSONRoundTrip(t *testing.T) {
	original := receipt.Heading{Content: "Shopping List"}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "heading" {
		t.Errorf(`wire["type"] = %v, want "heading"`, wire["type"])
	}

	var decoded receipt.Heading
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if decoded != original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
