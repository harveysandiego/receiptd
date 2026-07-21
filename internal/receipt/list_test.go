package receipt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestListValidate(t *testing.T) {
	tests := []struct {
		name    string
		list    receipt.List
		wantErr bool
	}{
		{"zero value, no items", receipt.List{}, true},
		{"invalid kind", receipt.List{Kind: "roman", Items: []receipt.ListItem{{Content: "Milk"}}}, true},
		{"empty content", receipt.List{Items: []receipt.ListItem{{Content: ""}}}, true},
		{"invalid UTF-8 content", receipt.List{Items: []receipt.ListItem{{Content: "\xff\xfe"}}}, true},
		{"negative indent", receipt.List{Items: []receipt.ListItem{{Content: "Milk", Indent: -1}}}, true},
		{"indent over the maximum", receipt.List{Items: []receipt.ListItem{{Content: "Milk", Indent: 9}}}, true},
		{"indent at the maximum", receipt.List{Items: []receipt.ListItem{{Content: "Milk", Indent: 8}}}, false},
		{"checked on a bullet list", receipt.List{Kind: "bullet", Items: []receipt.ListItem{{Content: "Milk", Checked: true}}}, true},
		{"checked on default (bullet) kind", receipt.List{Items: []receipt.ListItem{{Content: "Milk", Checked: true}}}, true},
		{"checked on a numbered list", receipt.List{Kind: "number", Items: []receipt.ListItem{{Content: "Milk", Checked: true}}}, true},

		{"default kind, single item", receipt.List{Items: []receipt.ListItem{{Content: "Milk"}}}, false},
		{"explicit bullet kind", receipt.List{Kind: "bullet", Items: []receipt.ListItem{{Content: "Milk"}}}, false},
		{"multiple bullet items", receipt.List{Items: []receipt.ListItem{{Content: "Milk"}, {Content: "Eggs"}}}, false},
		{"numbered list", receipt.List{Kind: "number", Items: []receipt.ListItem{{Content: "Step 1"}, {Content: "Step 2"}}}, false},
		{"checkbox list, checked and unchecked", receipt.List{Kind: "checkbox", Items: []receipt.ListItem{
			{Content: "Done", Checked: true},
			{Content: "Not done", Checked: false},
		}}, false},
		{"indented items", receipt.List{Items: []receipt.ListItem{{Content: "Parent"}, {Content: "Child", Indent: 1}}}, false},
		{"valid UTF-8 content", receipt.List{Items: []receipt.ListItem{{Content: "Café"}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.list.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListValidate_Deterministic(t *testing.T) {
	l := receipt.List{Items: []receipt.ListItem{{Content: "Milk"}}}
	first := l.Validate()
	second := l.Validate()
	if (first == nil) != (second == nil) {
		t.Fatalf("Validate() = %v, then %v, want the same result both times", first, second)
	}

	invalid := receipt.List{Items: []receipt.ListItem{{Content: "Milk", Indent: -1}}}
	first = invalid.Validate()
	second = invalid.Validate()
	if first == nil || second == nil {
		t.Fatalf("Validate() = %v, then %v, want both non-nil", first, second)
	}
	if first.Error() != second.Error() {
		t.Errorf("Validate() error = %q, then %q, want identical messages", first.Error(), second.Error())
	}
}

func TestList_JSONRoundTrip(t *testing.T) {
	original := receipt.List{
		Kind: "checkbox",
		Items: []receipt.ListItem{
			{Content: "Done", Checked: true},
			{Content: "Not done", Indent: 1},
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
	if wire["type"] != "list" {
		t.Errorf(`wire["type"] = %v, want "list"`, wire["type"])
	}

	var decoded receipt.List
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestList_JSONRoundTrip_DefaultKindOmitted(t *testing.T) {
	original := receipt.List{Items: []receipt.ListItem{{Content: "Milk"}}}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if _, present := wire["kind"]; present {
		t.Errorf(`wire["kind"] present = %v, want omitted for the default kind`, wire["kind"])
	}

	var decoded receipt.List
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if !reflect.DeepEqual(decoded, original) {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestReceipt_WithList_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "Before"},
			receipt.List{Kind: "number", Items: []receipt.ListItem{{Content: "Step 1"}, {Content: "Step 2"}}},
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
