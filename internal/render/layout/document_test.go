package layout_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestDocument_ZeroValue(t *testing.T) {
	var doc layout.Document
	if doc.Blocks != nil {
		t.Errorf("zero-value Document.Blocks = %v, want nil", doc.Blocks)
	}
}

func TestBlock_ZeroValue(t *testing.T) {
	var b layout.Block
	if b.Y != 0 {
		t.Errorf("zero-value Block.Y = %d, want 0", b.Y)
	}
	if b.Element != nil {
		t.Errorf("zero-value Block.Element = %v, want nil", b.Element)
	}
}

func TestDocument_HoldsBlocksInOrder(t *testing.T) {
	blocks := []layout.Block{
		{Y: 0, Element: receipt.Heading{Content: "Shopping List"}},
		{Y: 10, Element: receipt.Divider{Style: "solid"}},
		{Y: 12, Element: receipt.Text{Content: "Milk"}},
		{Y: 22, Element: receipt.Spacer{Height: 5}},
	}
	doc := layout.Document{Blocks: blocks}

	if len(doc.Blocks) != len(blocks) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(blocks))
	}
	for i, want := range blocks {
		got := doc.Blocks[i]
		if got.Y != want.Y {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, got.Y, want.Y)
		}
		if got.Element != want.Element {
			t.Errorf("doc.Blocks[%d].Element = %v, want %v", i, got.Element, want.Element)
		}
	}
}
