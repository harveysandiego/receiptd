package canvas_test

import (
	"context"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_ColumnsLineAndTextWithSameContent_RenderIdentically(t *testing.T) {
	// layout.ColumnsLine exists so a Columns-derived Block keeps its own
	// identity through layout (see layout.ColumnsLine's doc comment), not
	// so canvas.Paint gains a second way to draw text: given the same
	// Content and Style, a ColumnsLine Block must paint pixel-for-pixel
	// identically to a receipt.Text Block — the same guarantee
	// TestPaint_TableLineAndTextWithSameContent_RenderIdentically already
	// proves for layout.TableLine.
	f := layout.EmbeddedFont{}
	style := layout.Style{Bold: true, Size: 2}
	columnsDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: layout.ColumnsLine{Content: "Item Qty"}, Style: style},
	}}
	textDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "Item Qty"}, Style: style},
	}}

	ccolumns, err := canvas.Paint(columnsDoc)
	if err != nil {
		t.Fatalf("Paint(columnsDoc) error = %v, want nil", err)
	}
	ctext, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint(textDoc) error = %v, want nil", err)
	}

	if ccolumns.Width != ctext.Width || ccolumns.Height != ctext.Height {
		t.Fatalf("ColumnsLine dimensions = %dx%d, Text = %dx%d, want equal", ccolumns.Width, ccolumns.Height, ctext.Width, ctext.Height)
	}
	if string(ccolumns.Bits) != string(ctext.Bits) {
		t.Errorf("ColumnsLine and Text Bits differ given the same Content and Style, want identical")
	}
}

func TestPaint_ColumnsFromBuild_ProducesSameBitmapAsEquivalentHandWrittenText(t *testing.T) {
	// End-to-end version of the test above: a real receipt.Columns run
	// through layout.Build and canvas.Paint must produce exactly the same
	// bitmap a caller would get by writing the same composed lines as
	// plain receipt.Text elements directly — the same proof
	// TestPaint_TableFromBuild_ProducesSameBitmapAsEquivalentHandWrittenText
	// already gives for receipt.Table.
	f := layout.EmbeddedFont{}
	widthDots := 154 // see layout.TestBuild_Columns_TwoEqualWidthColumns_ColumnsStayAlignedAcrossRows
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Columns{Columns: []receipt.Column{
			{Elements: []receipt.Element{receipt.Text{Content: "Item"}}},
			{Elements: []receipt.Element{receipt.Text{Content: "Qty"}}},
		}},
	}}
	columnsDoc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	ccolumns, err := canvas.Paint(columnsDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	var textBlocks []layout.Block
	for i, b := range columnsDoc.Blocks {
		line, ok := b.Element.(layout.ColumnsLine)
		if !ok {
			t.Fatalf("columnsDoc.Blocks[%d].Element = %T, want layout.ColumnsLine", i, b.Element)
		}
		textBlocks = append(textBlocks, layout.Block{Y: b.Y, Element: receipt.Text{Content: line.Content}, Style: b.Style})
	}
	textDoc := layout.Document{WidthDots: widthDots, Font: f, Blocks: textBlocks}
	ctext, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if ccolumns.Width != ctext.Width || ccolumns.Height != ctext.Height {
		t.Fatalf("Columns dimensions = %dx%d, equivalent Text = %dx%d, want equal", ccolumns.Width, ccolumns.Height, ctext.Width, ctext.Height)
	}
	if string(ccolumns.Bits) != string(ctext.Bits) {
		t.Errorf("Columns and equivalent hand-written Text Bits differ, want identical")
	}
}
