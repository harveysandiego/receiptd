package receipt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestTableValidate(t *testing.T) {
	tests := []struct {
		name    string
		table   receipt.Table
		wantErr bool
	}{
		{"empty table", receipt.Table{}, true},
		{"empty headers, valid row", receipt.Table{Rows: [][]string{{"Milk", "1"}}}, true},
		{"headers, empty rows", receipt.Table{Headers: []string{"Item", "Qty"}}, true},
		{"row length mismatch, too few columns", receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows:    [][]string{{"Milk"}},
		}, true},
		{"row length mismatch, too many columns", receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows:    [][]string{{"Milk", "1", "extra"}},
		}, true},
		{"one header, valid single-column rows", receipt.Table{
			Headers: []string{"Item"},
			Rows:    [][]string{{"Milk"}, {"Eggs"}},
		}, false},
		{"valid table, multiple rows", receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows: [][]string{
				{"Milk", "1"},
				{"Eggs", "12"},
			},
		}, false},
		{"empty string cells are valid content", receipt.Table{
			Headers: []string{"Item", "Note"},
			Rows:    [][]string{{"Milk", ""}},
		}, false},
		{"valid UTF-8 content", receipt.Table{
			Headers: []string{"Item", "Price"},
			Rows:    [][]string{{"Café", "£1.50"}},
		}, false},
		{"invalid UTF-8 in header", receipt.Table{
			Headers: []string{"Item", "\xff\xfe"},
			Rows:    [][]string{{"Milk", "1"}},
		}, true},
		{"invalid UTF-8 in cell", receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows:    [][]string{{"Milk", "\xff\xfe"}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.table.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTableValidate_Deterministic(t *testing.T) {
	tbl := receipt.Table{
		Headers: []string{"Item"},
		Rows:    [][]string{{"Milk"}},
	}
	first := tbl.Validate()
	second := tbl.Validate()
	if (first == nil) != (second == nil) {
		t.Fatalf("Validate() = %v, then %v, want the same result both times", first, second)
	}

	invalid := receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk", "extra"}}}
	first = invalid.Validate()
	second = invalid.Validate()
	if first == nil || second == nil {
		t.Fatalf("Validate() = %v, then %v, want both non-nil", first, second)
	}
	if first.Error() != second.Error() {
		t.Errorf("Validate() error = %q, then %q, want identical messages", first.Error(), second.Error())
	}
}

func TestTable_JSONRoundTrip(t *testing.T) {
	original := receipt.Table{
		Headers: []string{"Item", "Qty"},
		Rows: [][]string{
			{"Milk", "1"},
			{"Eggs", "12"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "table" {
		t.Errorf(`wire["type"] = %v, want "table"`, wire["type"])
	}

	var decoded receipt.Table
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestReceipt_WithTable_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.Table{
				Headers: []string{"Item", "Qty"},
				Rows:    [][]string{{"Milk", "1"}},
			},
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
