package layout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// A Columns becomes one layout.ColumnsLine Block per composed output
// line — the same technique receipt.Table already uses (see table.go,
// TableLine), generalized from a Table's plain-string cells to each
// Column's own []receipt.Element content. render/canvas.Paint still
// paints a ColumnsLine via the exact same glyph-painting path Text and
// TableLine use, but a Block derived from a Columns element is never
// mistaken for one a caller wrote directly.

func TestBuild_Columns_TwoEqualWidthColumns_ColumnsStayAlignedAcrossRows(t *testing.T) {
	// Mirrors TestBuild_Table_ColumnsStayAlignedAcrossRows exactly: with no
	// Weight set, both columns default to equal width (ResolveSize floors
	// 0 to 1), so the column-width math is identical to Table's own.
	const widthDots = 154 // 2*(5 content chars * 14) + 1 gap char * 14
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ColumnsLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ColumnsLine", doc.Blocks[0].Element)
	}
	want := "Item " + " Qty" // "Item" (4 chars) + 1 pad space to reach 5 + 1 gap space
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
	if doc.Blocks[0].Style != (layout.Style{Size: 1}) {
		t.Errorf("doc.Blocks[0].Style = %+v, want normalized Style{Size: 1}", doc.Blocks[0].Style)
	}
}

func TestBuild_Columns_WeightedColumns_AllocatesProportionalWidth(t *testing.T) {
	// gap = 14 dots (1 char); budget = 126 - 14 = 112 dots (8 chars), split
	// 3:1 across weights 3 and 1 -> column 0 gets 84 dots (6 chars), column
	// 1 gets 28 dots (2 chars). Both cells exactly fill their column, so
	// neither wraps and there is no extra padding beyond the 1-space gap.
	const widthDots = 126
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Weight: 3, Elements: []receipt.Element{receipt.Text{Content: "ABCDEF"}}},
			{Weight: 1, Elements: []receipt.Element{receipt.Text{Content: "XY"}}},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ColumnsLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ColumnsLine", doc.Blocks[0].Element)
	}
	want := "ABCDEF XY"
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
}

func TestBuild_Columns_ThreeColumns_NoWidthConstraint_JoinedInOrder(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "Left"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "Middle"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "Right"}}},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ColumnsLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ColumnsLine", doc.Blocks[0].Element)
	}
	want := "Left Middle Right"
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
}

func TestBuild_Columns_EmptyColumn_SiblingStillRendersPadded(t *testing.T) {
	// A column with no Elements at all is valid (receipt.Columns.Validate
	// requires at least one column, not that every column be non-empty)
	// and must not prevent its non-empty sibling from rendering.
	const widthDots = 154 // same 5-char, 2-column budget as the equal-width test above
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "A"}}},
			{Elements: nil},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ColumnsLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ColumnsLine", doc.Blocks[0].Element)
	}
	want := "A" + strings.Repeat(" ", 5) // "A" padded to the 5-char column budget plus the 1-char gap; empty last column contributes nothing
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
}

func TestBuild_Columns_DifferingContentHeights_ShorterColumnPadsBlankLines(t *testing.T) {
	const widthDots = 154 // same 5-char, 2-column budget as the equal-width test above
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "A"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "X"}, receipt.Text{Content: "Y"}}},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (column 1 has 2 elements, so 2 composed rows)", len(doc.Blocks))
	}
	want := []string{
		"A" + strings.Repeat(" ", 5) + "X",
		strings.Repeat(" ", 6) + "Y",
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ColumnsLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.ColumnsLine", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
		if wantY := i * f.LineHeight(); doc.Blocks[i].Y != wantY {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, doc.Blocks[i].Y, wantY)
		}
	}
}

func TestBuild_Columns_UnsupportedElementType_ReturnsPermanentError(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Divider{}}},
		}},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_Columns_Heading_ReturnsPermanentErrorRatherThanSilentlyDowngrading(t *testing.T) {
	// A receipt.Heading nested in a column must never be silently rendered
	// as plain text: a composed ColumnsLine Block carries exactly one
	// Style for its whole line (Block.Style has no per-run/per-column
	// concept — see columnsLines), so a Heading's Bold/Size styling cannot
	// be preserved once its line is merged with a sibling column's own
	// (possibly plain-Text) content on the same row without a horizontal-
	// positioning or rich-text primitive, both out of scope here. Rather
	// than quietly dropping that styling (changing Heading's documented
	// meaning), Build rejects it the same way any other unsupported nested
	// element type already is.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Heading{Content: "Total"}}},
		}},
	}}
	_, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Build() error = %v, want apperr.KindPermanent", err)
	}
}

func TestBuild_ColumnsBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
		}},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3", len(doc.Blocks))
	}
	if got, ok := doc.Blocks[0].Element.(receipt.Text); !ok || got.Content != "Before" {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Before\"}", doc.Blocks[0].Element)
	}
	if _, ok := doc.Blocks[1].Element.(layout.ColumnsLine); !ok {
		t.Errorf("doc.Blocks[1].Element = %T, want layout.ColumnsLine", doc.Blocks[1].Element)
	}
	if wantY := f.LineHeight(); doc.Blocks[1].Y != wantY {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, wantY)
	}
	if wantY := 2 * f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
	if got, ok := doc.Blocks[2].Element.(receipt.Text); !ok || got.Content != "After" {
		t.Errorf("doc.Blocks[2].Element = %v, want Text{Content: \"After\"}", doc.Blocks[2].Element)
	}
}

func TestBuild_ColumnsThenDivider(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
		}},
		receipt.Divider{},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (1 columns line + divider)", len(doc.Blocks))
	}
	if wantY := f.LineHeight(); doc.Blocks[1].Y != wantY {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, wantY)
	}
	if _, ok := doc.Blocks[1].Element.(receipt.Divider); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.Divider", doc.Blocks[1].Element)
	}
}

func TestBuild_ColumnsDeterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Weight: 2, Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
			{Weight: 1, Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
		}},
	}}
	f := layout.EmbeddedFont{}
	profile := printer.Profile{WidthDots: 154}

	first, err := layout.Build(context.Background(), r, profile, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(context.Background(), r, profile, f, nil)
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
