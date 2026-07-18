package layout_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestBuild_EmptyReceipt(t *testing.T) {
	doc, err := layout.Build(receipt.Receipt{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 0 {
		t.Errorf("len(doc.Blocks) = %d, want 0", len(doc.Blocks))
	}
}

func TestBuild_DocumentCarriesFont(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc, err := layout.Build(receipt.Receipt{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.Font != layout.Font(f) {
		t.Errorf("doc.Font = %v, want %v", doc.Font, f)
	}
}

func TestBuild_OneText(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(r, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	if doc.Blocks[0].Element != (receipt.Text{Content: "Milk"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Milk\"}", doc.Blocks[0].Element)
	}
}

func TestBuild_MultipleText_YIncreasesByLineHeight(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Text{Content: "Eggs"},
		receipt.Text{Content: "Bread"},
	}}
	doc, err := layout.Build(r, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	lh := f.LineHeight()
	for i, b := range doc.Blocks {
		want := i * lh
		if b.Y != want {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, b.Y, want)
		}
	}
}

func TestBuild_PreservesElementOrder(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "First"},
		receipt.Text{Content: "Second"},
		receipt.Text{Content: "Third"},
	}}
	doc, err := layout.Build(r, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"First", "Second", "Third"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(receipt.Text)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element is %T, want receipt.Text", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
	}
}

func TestBuild_UnsupportedElementReturnsPermanentError(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Heading{Content: "Shopping List"},
	}}
	_, err := layout.Build(r, layout.EmbeddedFont{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_UnsupportedElementAmongSupportedOnes(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Divider{Style: "solid"},
	}}
	_, err := layout.Build(r, layout.EmbeddedFont{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Text{Content: "Eggs"},
	}}
	f := layout.EmbeddedFont{}

	first, err := layout.Build(r, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(r, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if len(first.Blocks) != len(second.Blocks) {
		t.Fatalf("len(first.Blocks) = %d, len(second.Blocks) = %d, want equal", len(first.Blocks), len(second.Blocks))
	}
	for i := range first.Blocks {
		if first.Blocks[i] != second.Blocks[i] {
			t.Errorf("Blocks[%d] = %v, then %v, want equal", i, first.Blocks[i], second.Blocks[i])
		}
	}
}
