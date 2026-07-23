package receipt_test

import (
	"errors"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// stubElement is a minimal test double for receipt.Element, used to
// exercise Receipt.Validate()'s aggregation logic independent of any
// concrete Element type's own validation rules.
type stubElement struct{ err error }

func (s stubElement) Validate() error { return s.err }

func TestReceiptValidate(t *testing.T) {
	tests := []struct {
		name    string
		r       receipt.Receipt
		wantErr bool
	}{
		{"zero value", receipt.Receipt{}, false},
		{"no elements, version set", receipt.Receipt{Version: 1}, false},
		{"empty elements slice", receipt.Receipt{Version: 1, Elements: []receipt.Element{}}, false},
		{
			"all valid elements",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{
				receipt.Text{Content: "Milk"},
				receipt.Heading{Content: "Shopping List"},
				receipt.Divider{},
				receipt.Spacer{Height: 10},
			}},
			false,
		},
		{
			"single invalid element",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{
				receipt.Text{},
			}},
			true,
		},
		{
			"multiple invalid elements",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{
				receipt.Text{},
				receipt.Heading{},
			}},
			true,
		},
		{
			"one valid, one invalid",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{
				receipt.Text{Content: "Milk"},
				receipt.Spacer{Height: -1},
			}},
			true,
		},
		{
			"nil element",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{nil}},
			true,
		},
		{
			"nil element among otherwise-valid elements",
			receipt.Receipt{Version: 1, Elements: []receipt.Element{
				receipt.Text{Content: "Milk"}, nil,
			}},
			true,
		},
		{"zero copies", receipt.Receipt{Version: 1, Copies: 0}, false},
		{"positive copies", receipt.Receipt{Version: 1, Copies: 3}, false},
		{"negative copies", receipt.Receipt{Version: 1, Copies: -1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.r.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestReceiptValidate_WrapsAsApperrValidation(t *testing.T) {
	r := receipt.Receipt{Version: 1, Elements: []receipt.Element{receipt.Text{}}}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if !apperr.Is(err, apperr.KindValidation) {
		t.Error("apperr.Is(err, KindValidation) = false, want true")
	}

	var target *apperr.Error
	if !errors.As(err, &target) {
		t.Fatal("errors.As failed to find *apperr.Error in the chain")
	}
	if target.Op != "receipt.Validate" {
		t.Errorf("Op = %q, want %q", target.Op, "receipt.Validate")
	}
}

func TestReceiptValidate_NegativeCopies_ReturnsValidationError(t *testing.T) {
	r := receipt.Receipt{Version: 1, Copies: -1}

	err := r.Validate()
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Validate() error = %v, want apperr.KindValidation", err)
	}
}

func TestReceipt_EffectiveCopies(t *testing.T) {
	tests := []struct {
		name   string
		copies int
		want   int
	}{
		{"zero treated as one", 0, 1},
		{"one stays one", 1, 1},
		{"positive passes through", 3, 3},
		{"negative treated as one", -1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := receipt.Receipt{Copies: tt.copies}
			if got := r.EffectiveCopies(); got != tt.want {
				t.Errorf("EffectiveCopies() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestReceiptValidate_ValidReturnsNilNotApperr(t *testing.T) {
	r := receipt.Receipt{Version: 1, Elements: []receipt.Element{receipt.Text{Content: "Milk"}}}

	if err := r.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestReceiptValidate_AggregatesAllElementErrorsViaJoin(t *testing.T) {
	e1 := errors.New("boom one")
	e2 := errors.New("boom two")
	r := receipt.Receipt{Elements: []receipt.Element{
		stubElement{err: e1},
		stubElement{err: nil},
		stubElement{err: e2},
	}}

	err := r.Validate()
	if err == nil {
		t.Fatal("Validate() = nil, want error")
	}
	if !errors.Is(err, e1) {
		t.Error("aggregated error does not contain e1")
	}
	if !errors.Is(err, e2) {
		t.Error("aggregated error does not contain e2")
	}
}
