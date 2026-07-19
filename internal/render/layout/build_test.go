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

func TestBuild_ShortText_FitsWithinWidth_RemainsOneBlockUnchanged(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: 1000}, f)
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
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Milk\"} unchanged", doc.Blocks[0].Element)
	}
}

func TestBuild_LongText_WrapsAcrossMultipleBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello World") // fits exactly two words; "Foo" must wrap
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World Foo"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[0].Y != 0 {
		t.Errorf("doc.Blocks[0].Y = %d, want 0", doc.Blocks[0].Y)
	}
	if doc.Blocks[0].Element != (receipt.Text{Content: "Hello World"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Hello World\"}", doc.Blocks[0].Element)
	}
	if doc.Blocks[1].Y != f.LineHeight() {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, f.LineHeight())
	}
	if doc.Blocks[1].Element != (receipt.Text{Content: "Foo"}) {
		t.Errorf("doc.Blocks[1].Element = %v, want Text{Content: \"Foo\"}", doc.Blocks[1].Element)
	}
}

func TestBuild_WordsAlreadyFitting_AreNeverSplit(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Alpha") // forces one word per line
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Alpha Beta Gamma"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"Alpha", "Beta", "Gamma"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		if doc.Blocks[i].Element != (receipt.Text{Content: w}) {
			t.Errorf("doc.Blocks[%d].Element = %v, want Text{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

func TestBuild_OverlongWord_NotSplit_StaysOnOwnLine(t *testing.T) {
	f := layout.EmbeddedFont{}
	const word = "Supercalifragilisticexpialidocious"
	width := f.Measure("Super") // much narrower than the whole word
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: word},
		receipt.Text{Content: "Next"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (the overlong word stays whole, on its own line)", len(doc.Blocks))
	}
	if doc.Blocks[0].Element != (receipt.Text{Content: word}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: %q} (not split into characters)", doc.Blocks[0].Element, word)
	}
	if doc.Blocks[1].Y != f.LineHeight() {
		t.Errorf("doc.Blocks[1].Y = %d, want %d (overlong word advanced exactly one line)", doc.Blocks[1].Y, f.LineHeight())
	}
}

func TestBuild_ExplicitNewline_SplitsIntoSeparateBlocks(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Line1\nLine2"},
	}}
	// No width constraint: only the explicit newline should split lines.
	doc, err := layout.Build(r, printer.Profile{}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	if doc.Blocks[0].Element != (receipt.Text{Content: "Line1"}) {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Line1\"}", doc.Blocks[0].Element)
	}
	if doc.Blocks[1].Y != f.LineHeight() {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, f.LineHeight())
	}
	if doc.Blocks[1].Element != (receipt.Text{Content: "Line2"}) {
		t.Errorf("doc.Blocks[1].Element = %v, want Text{Content: \"Line2\"}", doc.Blocks[1].Element)
	}
}

func TestBuild_ExplicitNewlineCombinedWithWrapping(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello") // forces "Hello World" to wrap into two lines
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World\nFoo"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"Hello", "World", "Foo"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		if doc.Blocks[i].Element != (receipt.Text{Content: w}) {
			t.Errorf("doc.Blocks[%d].Element = %v, want Text{Content: %q}", i, doc.Blocks[i].Element, w)
		}
		wantY := i * f.LineHeight()
		if doc.Blocks[i].Y != wantY {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, doc.Blocks[i].Y, wantY)
		}
	}
}

func TestBuild_WrappingRespectsDifferentPrinterWidths(t *testing.T) {
	f := layout.EmbeddedFont{}
	content := "Alpha Beta Gamma"
	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: content}}}

	wide, err := layout.Build(r, printer.Profile{WidthDots: f.Measure(content)}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(wide.Blocks) != 1 {
		t.Errorf("len(wide.Blocks) = %d, want 1 (wide enough for one line)", len(wide.Blocks))
	}

	narrow, err := layout.Build(r, printer.Profile{WidthDots: f.Measure("Alpha")}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(narrow.Blocks) != 3 {
		t.Errorf("len(narrow.Blocks) = %d, want 3 (one word per line)", len(narrow.Blocks))
	}
}

func TestBuild_WrappedText_SubsequentElementYAccountsForExtraLines(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello World") // "Foo" wraps to a second line
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World Foo"},
		receipt.Text{Content: "Bar"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	if want := 2 * f.LineHeight(); doc.Blocks[2].Y != want {
		t.Errorf("doc.Blocks[2].Y = %d, want %d (document height grew by the wrapped line)", doc.Blocks[2].Y, want)
	}
	if doc.Blocks[2].Element != (receipt.Text{Content: "Bar"}) {
		t.Errorf("doc.Blocks[2].Element = %v, want Text{Content: \"Bar\"}", doc.Blocks[2].Element)
	}
}

func TestBuild_HeadingAlsoWraps(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello World")
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Heading{Content: "Hello World Foo"},
	}}
	doc, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"Hello World", "Foo"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		if doc.Blocks[i].Element != (receipt.Heading{Content: w}) {
			t.Errorf("doc.Blocks[%d].Element = %v, want Heading{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

func TestBuild_WrappedOutput_Deterministic(t *testing.T) {
	f := layout.EmbeddedFont{}
	width := f.Measure("Hello World")
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hello World Foo Bar Baz"},
	}}

	first, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(r, printer.Profile{WidthDots: width}, f)
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
