package receipt_test

import (
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
