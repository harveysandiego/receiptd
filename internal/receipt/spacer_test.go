package receipt_test

import (
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
