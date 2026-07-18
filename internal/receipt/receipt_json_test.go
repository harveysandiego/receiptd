package receipt_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestReceipt_JSONRoundTrip(t *testing.T) {
	original := receipt.Receipt{
		Version: 1,
		Copies:  2,
		Elements: []receipt.Element{
			receipt.Heading{Content: "Shopping List"},
			receipt.Divider{},
			receipt.Text{Content: "Milk"},
			receipt.Spacer{Height: 10},
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

func TestReceipt_JSONRoundTrip_ElementOrderPreserved(t *testing.T) {
	original := receipt.Receipt{
		Elements: []receipt.Element{
			receipt.Text{Content: "first"},
			receipt.Text{Content: "second"},
			receipt.Text{Content: "third"},
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

	if len(decoded.Elements) != len(original.Elements) {
		t.Fatalf("len(decoded.Elements) = %d, want %d", len(decoded.Elements), len(original.Elements))
	}
	for i, want := range []string{"first", "second", "third"} {
		got, ok := decoded.Elements[i].(receipt.Text)
		if !ok {
			t.Fatalf("decoded.Elements[%d] = %T, want receipt.Text", i, decoded.Elements[i])
		}
		if got.Content != want {
			t.Errorf("decoded.Elements[%d].Content = %q, want %q", i, got.Content, want)
		}
	}
}

func TestReceipt_UnmarshalJSON_UnknownElementType(t *testing.T) {
	data := []byte(`{"version":1,"elements":[{"type":"nonexistent"}]}`)

	var r receipt.Receipt
	err := json.Unmarshal(data, &r)
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Unmarshal() error = %v, want apperr.KindValidation", err)
	}
}

func TestReceipt_UnmarshalJSON_MalformedElement(t *testing.T) {
	// The document is syntactically valid JSON, but "content" doesn't fit
	// Text.Content (a string) — this is the malformed-input case that
	// actually reaches receipt's decoding, as opposed to a syntactically
	// broken top-level document, which encoding/json rejects before ever
	// calling Receipt.UnmarshalJSON (see the architecture-questions note
	// in the slice's output).
	data := []byte(`{"version":1,"elements":[{"type":"text","content":123}]}`)

	var r receipt.Receipt
	err := json.Unmarshal(data, &r)
	if !apperr.Is(err, apperr.KindValidation) {
		t.Fatalf("Unmarshal() error = %v, want apperr.KindValidation", err)
	}
}
