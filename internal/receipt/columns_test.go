package receipt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestColumnsValidate(t *testing.T) {
	tests := []struct {
		name    string
		columns receipt.Columns
		wantErr bool
	}{
		{
			name:    "zero value: no columns",
			columns: receipt.Columns{},
			wantErr: true,
		},
		{
			name:    "single column, no elements",
			columns: receipt.Columns{Columns: []receipt.Column{{}}},
			wantErr: false,
		},
		{
			name: "two columns with text, no weight set",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
				{Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
			}},
			wantErr: false,
		},
		{
			name: "weight set on every column",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Weight: 2, Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
				{Weight: 1, Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
			}},
			wantErr: false,
		},
		{
			name: "negative weight",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Weight: -1, Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
			}},
			wantErr: true,
		},
		{
			name: "nested element invalid",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Elements: []receipt.Element{receipt.Asset{Name: ""}}},
			}},
			wantErr: true,
		},
		{
			name: "nil element inside a column",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Elements: []receipt.Element{nil}},
			}},
			wantErr: true,
		},
		{
			name: "nested element's own Validate() actually runs, not just checked for presence",
			columns: receipt.Columns{Columns: []receipt.Column{
				{Elements: []receipt.Element{receipt.Image{Data: []byte("not a decodable image")}}},
			}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.columns.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestColumnsValidate_Deterministic(t *testing.T) {
	c := receipt.Columns{Columns: []receipt.Column{
		{Elements: []receipt.Element{receipt.Asset{Name: ""}}},
	}}
	first := c.Validate()
	second := c.Validate()
	if (first == nil) != (second == nil) {
		t.Fatalf("Validate() = %v, then %v, want same nilness", first, second)
	}
	if first != nil && first.Error() != second.Error() {
		t.Errorf("Validate() = %q, then %q, want equal", first, second)
	}
}

func TestColumns_JSONRoundTrip(t *testing.T) {
	original := receipt.Columns{Columns: []receipt.Column{
		{Weight: 2, Elements: []receipt.Element{receipt.Text{Content: "Item"}, receipt.Text{Content: "Milk"}}},
		{Weight: 1, Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
	}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "columns" {
		t.Errorf(`wire["type"] = %v, want "columns"`, wire["type"])
	}

	var decoded receipt.Columns
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestColumns_JSONRoundTrip_MinimalFields(t *testing.T) {
	data := []byte(`{"type":"columns","columns":[{"elements":[{"type":"text","content":"Milk"}]}]}`)
	var decoded receipt.Columns
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if len(decoded.Columns) != 1 || decoded.Columns[0].Weight != 0 {
		t.Fatalf("decoded = %+v, want one column with zero-value Weight", decoded)
	}
	content, ok := decoded.Columns[0].Elements[0].(receipt.Text)
	if !ok || content.Content != "Milk" {
		t.Errorf("decoded.Columns[0].Elements[0] = %v, want Text{Content: \"Milk\"}", decoded.Columns[0].Elements[0])
	}
}

func TestColumns_UnmarshalJSON_UnknownNestedElementType(t *testing.T) {
	data := []byte(`{"type":"columns","columns":[{"elements":[{"type":"not-a-real-type"}]}]}`)
	var decoded receipt.Columns
	if err := json.Unmarshal(data, &decoded); err == nil {
		t.Fatalf("json.Unmarshal() error = nil, want error for unknown nested element type")
	}
}

func TestColumns_UnmarshalJSON_MalformedJSON(t *testing.T) {
	var decoded receipt.Columns
	if err := json.Unmarshal([]byte(`{not valid json`), &decoded); err == nil {
		t.Fatalf("json.Unmarshal() error = nil, want error for malformed JSON")
	}
}

func TestReceipt_WithColumns_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.Columns{Columns: []receipt.Column{
				{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
				{Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
			}},
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
