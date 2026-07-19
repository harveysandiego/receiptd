package receipt_test

import (
	"encoding/json"
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestFeedValidate(t *testing.T) {
	tests := []struct {
		name    string
		feed    receipt.Feed
		wantErr bool
	}{
		{"zero value, zero lines", receipt.Feed{}, true},
		{"zero lines", receipt.Feed{Lines: 0}, true},
		{"positive lines", receipt.Feed{Lines: 4}, false},
		{"negative lines", receipt.Feed{Lines: -1}, true},
		{"lines at the max", receipt.Feed{Lines: 255}, false},
		{"lines over the max", receipt.Feed{Lines: 256}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.feed.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFeed_JSONRoundTrip(t *testing.T) {
	original := receipt.Feed{Lines: 4}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v, want nil", err)
	}

	var wire map[string]any
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("json.Unmarshal() into map error = %v, want nil", err)
	}
	if wire["type"] != "feed" {
		t.Errorf(`wire["type"] = %v, want "feed"`, wire["type"])
	}

	var decoded receipt.Feed
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v, want nil", err)
	}
	if decoded != original {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}
