package receipt_test

import (
	"encoding/json"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestTextValidate(t *testing.T) {
	tests := []struct {
		name    string
		text    receipt.Text
		wantErr bool
	}{
		{"zero value", receipt.Text{}, true},
		{"empty content", receipt.Text{Content: ""}, true},
		{"content only", receipt.Text{Content: "Milk"}, false},
		{
			"content with align and every style field set",
			receipt.Text{Content: "Milk", Align: "center", Bold: true, Italic: true, Underline: true, Strikethrough: true, Size: 2},
			false,
		},
		{
			"align is not restricted to a fixed set",
			receipt.Text{Content: "Milk", Align: "diagonal"},
			false,
		},
		{"omitted Size (zero value) is valid", receipt.Text{Content: "Milk", Size: 0}, false},
		{"positive Size is valid", receipt.Text{Content: "Milk", Size: 3}, false},
		{"negative Size is invalid", receipt.Text{Content: "Milk", Size: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.text.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestText_JSONRoundTrip(t *testing.T) {
	original := receipt.Text{Content: "Milk", Align: "center", Bold: true, Italic: true, Underline: true, Strikethrough: true, Size: 2}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "text" {
		t.Errorf(`wire["type"] = %v, want "text"`, wire["type"])
	}

	var decoded receipt.Text
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if decoded != original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
