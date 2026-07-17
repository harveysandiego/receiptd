package receipt_test

import (
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
			"content with align, bold, size",
			receipt.Text{Content: "Milk", Align: "center", Bold: true, Size: "large"},
			false,
		},
		{
			"align and size are not restricted to a fixed set",
			receipt.Text{Content: "Milk", Align: "diagonal", Size: "enormous"},
			false,
		},
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
