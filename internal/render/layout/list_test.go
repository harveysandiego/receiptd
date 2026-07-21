package layout_test

import (
	"context"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// A List becomes one layout.ListLine Block per output line — see Build's
// doc comment and ListLine's own. Like TableLine and ColumnsLine, a
// ListLine keeps its own identity: render/canvas.Paint still paints it
// via the exact same glyph-painting path Text uses (see
// TestPaint_ListLineAndTextWithSameContent_RenderIdentically in
// render/canvas), but a Block derived from a List is never mistaken for
// one a caller wrote directly as receipt.Text.

func TestBuild_List_SingleBulletItem(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{{Content: "Milk"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ListLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ListLine", doc.Blocks[0].Element)
	}
	if want := "- Milk"; got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
	if doc.Blocks[0].Style != (layout.Style{Size: 1}) {
		t.Errorf("doc.Blocks[0].Style = %+v, want normalized Style{Size: 1}", doc.Blocks[0].Style)
	}
}

func TestBuild_List_MultipleBulletItems(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{{Content: "Milk"}, {Content: "Eggs"}, {Content: "Bread"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"- Milk", "- Eggs", "- Bread"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ListLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want ListLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
		if wantY := i * f.LineHeight(); doc.Blocks[i].Y != wantY {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, doc.Blocks[i].Y, wantY)
		}
	}
}

func TestBuild_List_ExplicitBulletKind_SameAsDefault(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "bullet", Items: []receipt.ListItem{{Content: "Milk"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	got, ok := doc.Blocks[0].Element.(layout.ListLine)
	if !ok || got.Content != "- Milk" {
		t.Errorf("doc.Blocks[0].Element = %v, want ListLine{Content: %q}", doc.Blocks[0].Element, "- Milk")
	}
}

// Numbering is 1-based, sequential across the whole Items slice,
// independent of Indent — a deliberate limitation, not a bug: see
// docs/adr/0014-list-elements.md "Bullet and number generation".
func TestBuild_List_NumberedItems_SequentialRegardlessOfIndent(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "number", Items: []receipt.ListItem{
			{Content: "Step A"},
			{Content: "Step B", Indent: 1},
			{Content: "Step C"},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"1. Step A", "  2. Step B", "3. Step C"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ListLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want ListLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

func TestBuild_List_CheckboxItems_CheckedAndUnchecked(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "checkbox", Items: []receipt.ListItem{
			{Content: "Done", Checked: true},
			{Content: "Not done", Checked: false},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"[x] Done", "[ ] Not done"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ListLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want ListLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

// Indent is a semantic nesting level: each level shifts the marker (and
// hence the content) by the same fixed leading-space amount, composed as
// content — never as a coordinate on Block.
func TestBuild_List_IndentedItems_LeadingSpacesPerLevel(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{
			{Content: "Parent"},
			{Content: "Child", Indent: 1},
			{Content: "Grandchild", Indent: 2},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"- Parent", "  - Child", "    - Grandchild"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ListLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want ListLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

// TestBuild_List_WrappedItem_HangIndentsUnderContent constrains WidthDots
// so "Whole Milk" wraps onto two lines: the marker/indent prefix appears
// only on the first line, and the continuation line is left-padded with
// blank space measuring the same width as that prefix (listHangIndent),
// so it lines up under the item's content, not under its marker.
func TestBuild_List_WrappedItem_HangIndentsUnderContent(t *testing.T) {
	const widthDots = 98 // "- " (2 chars * 14 dots) + a 5-char (70 dot) content budget
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{{Content: "Whole Milk"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"- Whole", "  Milk"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d: %+v", len(doc.Blocks), len(want), doc.Blocks)
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.ListLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.ListLine", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
	}
}

// A numbered list's markers are not all the same width ("1." vs. "10.");
// the hanging indent must still line up under each item's own content.
func TestBuild_List_WrappedNumberedItem_HangIndentMatchesMarkerWidth(t *testing.T) {
	const widthDots = 112 // "10. " (4 chars * 14 dots) + a 4-char (56 dot) content budget
	f := layout.EmbeddedFont{}
	// Pad the slice so the wrapped item is the 10th (marker "10. ").
	items := make([]receipt.ListItem, 10)
	for i := range items {
		items[i] = receipt.ListItem{Content: "x"}
	}
	items[9] = receipt.ListItem{Content: "Whole Milk"}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "number", Items: items},
	}}

	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	// 9 single-"x" lines, then "Whole Milk" wrapped onto 2 lines under
	// marker "10. ".
	if len(doc.Blocks) != 11 {
		t.Fatalf("len(doc.Blocks) = %d, want 11", len(doc.Blocks))
	}
	want := []string{"10. Whole", "    Milk"}
	for i, w := range want {
		got, ok := doc.Blocks[9+i].Element.(layout.ListLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.ListLine", 9+i, doc.Blocks[9+i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", 9+i, got.Content, w)
		}
	}
}

// TestBuild_List_MaximumIndentation_DegradesRatherThanFails constrains
// WidthDots narrower than a maximally indented item's own marker/indent
// prefix — the item's content budget floors to 1 dot (the same "still
// constrains, wraps aggressively" fallback tableColumnWidths and
// columnWidths already use) rather than Build failing.
func TestBuild_List_MaximumIndentation_DegradesRatherThanFails(t *testing.T) {
	const maxListIndent = 8 // mirrors receipt.maxListIndent
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{{Content: "Deep", Indent: maxListIndent}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 100}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ListLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ListLine", doc.Blocks[0].Element)
	}
	want := "                - Deep" // 16 leading spaces (8 levels * 2) + "- " + content
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
}

func TestBuild_List_ZeroProfileWidthDots_NoWrapping(t *testing.T) {
	// printer.Profile{} is Build's documented "no printer configured"
	// sentinel — mirrors wrapText's own widthDots <= 0 behaviour: an
	// item's content is composed with its marker but never wrapped.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{
			{Content: "Whole milk, two litres, semi-skimmed alternative available"},
		}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("len(doc.Blocks) = %d, want 1", len(doc.Blocks))
	}
	got, ok := doc.Blocks[0].Element.(layout.ListLine)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want layout.ListLine", doc.Blocks[0].Element)
	}
	want := "- Whole milk, two litres, semi-skimmed alternative available"
	if got.Content != want {
		t.Errorf("doc.Blocks[0].Element.Content = %q, want %q", got.Content, want)
	}
}

func TestBuild_List_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "number", Items: []receipt.ListItem{{Content: "Whole Milk"}, {Content: "Eggs", Indent: 1}}},
	}}
	f := layout.EmbeddedFont{}
	profile := printer.Profile{WidthDots: 98}

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

func TestBuild_ListBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.List{Items: []receipt.ListItem{{Content: "Milk"}, {Content: "Eggs"}}},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	// Before, then 2 list lines, then After.
	if len(doc.Blocks) != 4 {
		t.Fatalf("len(doc.Blocks) = %d, want 4", len(doc.Blocks))
	}
	if got, ok := doc.Blocks[0].Element.(receipt.Text); !ok || got.Content != "Before" {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Before\"}", doc.Blocks[0].Element)
	}
	for i := 1; i <= 2; i++ {
		if _, ok := doc.Blocks[i].Element.(layout.ListLine); !ok {
			t.Errorf("doc.Blocks[%d].Element = %T, want layout.ListLine", i, doc.Blocks[i].Element)
		}
		if _, ok := doc.Blocks[i].Element.(receipt.Text); ok {
			t.Errorf("doc.Blocks[%d].Element is receipt.Text, want it to remain distinct from a list-derived Block", i)
		}
	}
	if wantY := 3 * f.LineHeight(); doc.Blocks[3].Y != wantY {
		t.Errorf("doc.Blocks[3].Y = %d, want %d", doc.Blocks[3].Y, wantY)
	}
	if got, ok := doc.Blocks[3].Element.(receipt.Text); !ok || got.Content != "After" {
		t.Errorf("doc.Blocks[3].Element = %v, want Text{Content: \"After\"}", doc.Blocks[3].Element)
	}
}

func TestBuild_ListThenDivider(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Items: []receipt.ListItem{{Content: "Milk"}}},
		receipt.Divider{},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (1 list line + divider)", len(doc.Blocks))
	}
	if wantY := f.LineHeight(); doc.Blocks[1].Y != wantY {
		t.Errorf("doc.Blocks[1].Y = %d, want %d", doc.Blocks[1].Y, wantY)
	}
	if _, ok := doc.Blocks[1].Element.(receipt.Divider); !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want receipt.Divider", doc.Blocks[1].Element)
	}
}
