package layout_test

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestBuild_EmptyReceipt(t *testing.T) {
	doc, err := layout.Build(receipt.Receipt{}, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 0 {
		t.Errorf("len(doc.Blocks) = %d, want 0", len(doc.Blocks))
	}
}

func TestBuild_DocumentCarriesFont(t *testing.T) {
	f := layout.EmbeddedFont{}
	doc, err := layout.Build(receipt.Receipt{}, printer.Profile{}, f)
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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
	doc, err := layout.Build(r, printer.Profile{}, f)
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
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
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
		receipt.Divider{Style: "solid"},
	}}
	_, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_OneHeading(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Heading{Content: "Shopping List"},
	}}
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	if doc.Blocks[0].Element != (receipt.Heading{Content: "Shopping List"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Heading{Content: \"Shopping List\"}", doc.Blocks[0].Element)
	}
}

func TestBuild_HeadingAndText_PreservesOrderAndAdvancesY(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Heading{Content: "Shopping List"},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(r, printer.Profile{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}

	lh := f.LineHeight()
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	if doc.Blocks[0].Element != (receipt.Heading{Content: "Shopping List"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Heading{Content: \"Shopping List\"}", doc.Blocks[0].Element)
	}
	if doc.Blocks[1].Y != lh {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, lh)
	}
	if doc.Blocks[1].Element != (receipt.Text{Content: "Milk"}) {
		t.Errorf("doc.Blocks[1].Element = %v, want Text{Content: \"Milk\"}", doc.Blocks[1].Element)
	}
}

func TestBuild_UnsupportedElementAmongSupportedOnes(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Divider{Style: "solid"},
	}}
	_, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_OneSpacer(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Spacer{Height: 20},
	}}
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	if doc.Blocks[0].Element != (receipt.Spacer{Height: 20}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Spacer{Height: 20}", doc.Blocks[0].Element)
	}
}

func TestBuild_SpacerAdvancesYByOwnHeight_NotLineHeight(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Spacer{Height: 20},
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(r, printer.Profile{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != 20 {
		t.Errorf("doc.Blocks[1].Y = %d, want 20 (Spacer's own Height, not f.LineHeight() = %d)", doc.Blocks[1].Y, f.LineHeight())
	}
}

func TestBuild_SpacerAndText_PreservesOrder(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Spacer{Height: 20},
		receipt.Text{Content: "Eggs"},
	}}
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	if doc.Blocks[0].Element != (receipt.Text{Content: "Milk"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Milk\"}", doc.Blocks[0].Element)
	}
	if doc.Blocks[1].Element != (receipt.Spacer{Height: 20}) {
		t.Errorf("doc.Blocks[1].Element = %v, want Spacer{Height: 20}", doc.Blocks[1].Element)
	}
	if doc.Blocks[2].Element != (receipt.Text{Content: "Eggs"}) {
		t.Errorf("doc.Blocks[2].Element = %v, want Text{Content: \"Eggs\"}", doc.Blocks[2].Element)
	}
}

func TestBuild_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
		receipt.Text{Content: "Eggs"},
	}}
	f := layout.EmbeddedFont{}

	first, err := layout.Build(r, printer.Profile{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(r, printer.Profile{}, f)
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
	if first.WidthDots != second.WidthDots {
		t.Errorf("WidthDots = %d, then %d, want equal", first.WidthDots, second.WidthDots)
	}
}

func TestBuild_WidthDotsFromProfile(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: 384}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.WidthDots != 384 {
		t.Errorf("doc.WidthDots = %d, want 384 (profile.WidthDots)", doc.WidthDots)
	}
}

func TestBuild_DifferentProfileWidths_ProduceDifferentDocumentWidths(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "Milk"}}}

	narrow, err := layout.Build(r, printer.Profile{WidthDots: 200}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	wide, err := layout.Build(r, printer.Profile{WidthDots: 400}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	if narrow.WidthDots != 200 || wide.WidthDots != 400 {
		t.Errorf("WidthDots = %d, %d, want 200, 400 (each Document reflects its own Profile)", narrow.WidthDots, wide.WidthDots)
	}
}

func TestBuild_ZeroProfileWidthDots_DocumentWidthDotsIsZero(t *testing.T) {
	// printer.Profile{} is what cmd/receipt's offline `render` command
	// passes, having no daemon or config to resolve a real printer from.
	// Build must not invent a width for it — Document.WidthDots stays 0,
	// the documented "no constraint" sentinel render/canvas.Paint falls
	// back to content-fit sizing for.
	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "Milk"}}}
	doc, err := layout.Build(r, printer.Profile{}, layout.EmbeddedFont{})
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if doc.WidthDots != 0 {
		t.Errorf("doc.WidthDots = %d, want 0", doc.WidthDots)
	}
}
