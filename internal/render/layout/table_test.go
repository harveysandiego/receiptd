package layout_test

import (
	"context"
	"image/color"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// A Table becomes one layout.TableLine Block per output line — see
// Build's doc comment and TableLine's own. Unlike a synthetic receipt.Text
// would, a TableLine keeps its own identity: render/canvas.Paint still
// paints it via the exact same glyph-painting path Text uses (see
// TestPaint_TableLineAndTextWithSameContent_RenderIdentically in
// render/canvas), but a Block derived from a Table is never mistaken for
// one a caller wrote directly as receipt.Text.

func TestBuild_Table_NoWidthConstraint_RowsJoinCellsWithSingleSpace(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows: [][]string{
				{"Milk", "1"},
				{"Eggs", "12"},
			},
		},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (1 header + 2 rows)", len(doc.Blocks))
	}
	want := []string{"Item Qty", "Milk 1", "Eggs 12"}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.TableLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.TableLine", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
		if wantY := i * f.LineHeight(); doc.Blocks[i].Y != wantY {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, doc.Blocks[i].Y, wantY)
		}
		if doc.Blocks[i].Style != (layout.Style{Size: 1}) {
			t.Errorf("doc.Blocks[%d].Style = %+v, want normalized Style{Size: 1}", i, doc.Blocks[i].Style)
		}
	}
}

func TestBuild_Table_SingleColumnSingleRow(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	want := []string{"Item", "Milk"}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.TableLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want TableLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

func TestBuild_Table_MultipleRows_PreserveOrder(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item"},
			Rows:    [][]string{{"Milk"}, {"Eggs"}, {"Bread"}},
		},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	want := []string{"Item", "Milk", "Eggs", "Bread"}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d", len(doc.Blocks), len(want))
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.TableLine)
		if !ok || got.Content != w {
			t.Errorf("doc.Blocks[%d].Element = %v, want TableLine{Content: %q}", i, doc.Blocks[i].Element, w)
		}
	}
}

// TestBuild_Table_ColumnsStayAlignedAcrossRows constrains WidthDots so that
// each of 2 columns gets exactly 5 characters (70 dots) of content budget
// plus a 1-character (14 dot) gap between them — EmbeddedFont's Face7x13
// glyphs each advance a fixed 7 dots, doubled to 14 by its baked-in 2x
// legibility upscale (docs/adr/0008-embedded-font-legibility.md), so a
// budget in exact multiples of 14 lands on exact character boundaries.
// Neither "A" nor "Bread" needs wrapping, so this isolates column-width
// derivation and padding from wrapping.
func TestBuild_Table_ColumnsStayAlignedAcrossRows(t *testing.T) {
	const widthDots = 154 // 2*(5 content chars * 14) + 1 gap char * 14
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows: [][]string{
				{"A", "1"},
				{"Bread", "144"},
			},
		},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (1 header + 2 rows, none wrap)", len(doc.Blocks))
	}

	// Column 0's content budget is 5 characters; column 1 is last, so it is
	// never padded. Each short cell is right-padded with spaces to fill
	// that budget, then one more space is the inter-column gap.
	want := []string{
		"Item " + " Qty", // "Item" (4 chars) + 1 pad space to reach 5 + 1 gap space
		"A    " + " 1",   // "A" (1 char) + 4 pad spaces to reach 5 + 1 gap space
		"Bread" + " 144", // "Bread" (5 chars, exactly full) + 1 gap space, no pad
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.TableLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.TableLine", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
	}

	// The structural property that actually matters: every line fits within
	// the printable width, and (from want's construction above) column 1
	// always starts at the same 6-character dot offset.
	f := layout.EmbeddedFont{}
	for i, w := range want {
		if got := f.Measure(w); got > widthDots {
			t.Errorf("doc.Blocks[%d]: f.Measure(%q) = %d, want <= %d (widthDots)", i, w, got, widthDots)
		}
	}
}

func TestBuild_Table_WrappedCells(t *testing.T) {
	const widthDots = 98 // 2*(3 content chars * 14) + 1 gap char * 14
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows:    [][]string{{"Whole Milk", "1"}},
		},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}

	// "Item" and "Qty" are single words, each individually wider than the
	// 3-character column budget, but wrapText never splits a single word
	// (see wrapText's doc comment) — so the header stays one line. "Whole
	// Milk" has two words, so it wraps onto 2 lines; "1" only needs one, so
	// column 1 is blank on the row's second line.
	want := []string{"Item Qty", "Whole 1", "Milk "}
	if len(doc.Blocks) != len(want) {
		t.Fatalf("len(doc.Blocks) = %d, want %d: %+v", len(doc.Blocks), len(want), doc.Blocks)
	}
	for i, w := range want {
		got, ok := doc.Blocks[i].Element.(layout.TableLine)
		if !ok {
			t.Fatalf("doc.Blocks[%d].Element = %T, want layout.TableLine", i, doc.Blocks[i].Element)
		}
		if got.Content != w {
			t.Errorf("doc.Blocks[%d].Element.Content = %q, want %q", i, got.Content, w)
		}
		if wantY := i * f.LineHeight(); doc.Blocks[i].Y != wantY {
			t.Errorf("doc.Blocks[%d].Y = %d, want %d", i, doc.Blocks[i].Y, wantY)
		}
	}
}

func TestBuild_Table_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item", "Qty"},
			Rows:    [][]string{{"Whole Milk", "1"}, {"Eggs", "12"}},
		},
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

func TestBuild_TableBetweenTextBlocks_PreservesOrderAndPosition(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Before"},
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}, {"Eggs"}}},
		receipt.Text{Content: "After"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	// Before, then 3 table lines (header + 2 rows), then After.
	if len(doc.Blocks) != 5 {
		t.Fatalf("len(doc.Blocks) = %d, want 5", len(doc.Blocks))
	}
	if got, ok := doc.Blocks[0].Element.(receipt.Text); !ok || got.Content != "Before" {
		t.Errorf("doc.Blocks[0].Element = %v, want Text{Content: \"Before\"}", doc.Blocks[0].Element)
	}
	// The table-derived Blocks in between are layout.TableLine, never
	// receipt.Text — a real receipt.Text{Content: "Item"} a caller wrote by
	// hand must not be indistinguishable from this Table's header line.
	for i := 1; i <= 3; i++ {
		if _, ok := doc.Blocks[i].Element.(layout.TableLine); !ok {
			t.Errorf("doc.Blocks[%d].Element = %T, want layout.TableLine", i, doc.Blocks[i].Element)
		}
		if _, ok := doc.Blocks[i].Element.(receipt.Text); ok {
			t.Errorf("doc.Blocks[%d].Element is receipt.Text, want it to remain distinct from a table-derived Block", i)
		}
	}
	if wantY := 4 * f.LineHeight(); doc.Blocks[4].Y != wantY {
		t.Errorf("doc.Blocks[4].Y = %d, want %d", doc.Blocks[4].Y, wantY)
	}
	if got, ok := doc.Blocks[4].Element.(receipt.Text); !ok || got.Content != "After" {
		t.Errorf("doc.Blocks[4].Element = %v, want Text{Content: \"After\"}", doc.Blocks[4].Element)
	}
}

func TestBuild_TableThenDivider(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
		receipt.Divider{},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (2 table lines + divider)", len(doc.Blocks))
	}
	if wantY := 2 * f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
	if _, ok := doc.Blocks[2].Element.(receipt.Divider); !ok {
		t.Fatalf("doc.Blocks[2].Element = %T, want receipt.Divider", doc.Blocks[2].Element)
	}
}

func TestBuild_DividerThenTable(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Divider{},
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (divider + 2 table lines)", len(doc.Blocks))
	}
	if doc.Blocks[1].Y != layout.DividerThickness {
		t.Errorf("doc.Blocks[1].Y = %d, want %d (DividerThickness)", doc.Blocks[1].Y, layout.DividerThickness)
	}
	if wantY := layout.DividerThickness + f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
}

func TestBuild_TableThenImage(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
		receipt.Image{Data: solidPNG(t, 4, 6, color.Black)},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (2 table lines + image)", len(doc.Blocks))
	}
	if wantY := 2 * f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
	if _, ok := doc.Blocks[2].Element.(receipt.Image); !ok {
		t.Fatalf("doc.Blocks[2].Element = %T, want receipt.Image", doc.Blocks[2].Element)
	}
}

func TestBuild_TableThenBarcode(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
		receipt.Barcode{Content: "HELLO-128", Symbology: "code128"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (2 table lines + barcode)", len(doc.Blocks))
	}
	if wantY := 2 * f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
	if _, ok := doc.Blocks[2].Element.(receipt.Barcode); !ok {
		t.Fatalf("doc.Blocks[2].Element = %T, want receipt.Barcode", doc.Blocks[2].Element)
	}
}

func TestBuild_TableThenQRCode(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{Headers: []string{"Item"}, Rows: [][]string{{"Milk"}}},
		receipt.QRCode{Content: "https://example.com", Size: 60},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("len(doc.Blocks) = %d, want 3 (2 table lines + qrcode)", len(doc.Blocks))
	}
	if wantY := 2 * f.LineHeight(); doc.Blocks[2].Y != wantY {
		t.Errorf("doc.Blocks[2].Y = %d, want %d", doc.Blocks[2].Y, wantY)
	}
	if _, ok := doc.Blocks[2].Element.(receipt.QRCode); !ok {
		t.Fatalf("doc.Blocks[2].Element = %T, want receipt.QRCode", doc.Blocks[2].Element)
	}
}

func TestBuild_Table_ZeroProfileWidthDots_NoWrapping(t *testing.T) {
	// printer.Profile{} is Build's documented "no printer configured"
	// sentinel (see Document.WidthDots) — mirrors wrapText's own
	// widthDots <= 0 behaviour: rows are joined, not wrapped or padded.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Table{
			Headers: []string{"Item", "Description"},
			Rows:    [][]string{{"Milk", "Whole milk, two litres, semi-skimmed alternative available"}},
		},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2 (1 header + 1 row, no wrapping)", len(doc.Blocks))
	}
	got, ok := doc.Blocks[1].Element.(layout.TableLine)
	if !ok {
		t.Fatalf("doc.Blocks[1].Element = %T, want layout.TableLine", doc.Blocks[1].Element)
	}
	want := "Milk Whole milk, two litres, semi-skimmed alternative available"
	if got.Content != want {
		t.Errorf("doc.Blocks[1].Element.Content = %q, want %q", got.Content, want)
	}
}
