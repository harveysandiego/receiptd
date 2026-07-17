package receipt_test

import (
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
