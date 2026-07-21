package canvas_test

import (
	"context"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_ListLineAndTextWithSameContent_RenderIdentically(t *testing.T) {
	// layout.ListLine exists so a List-derived Block keeps its own
	// identity through layout (see layout.ListLine's doc comment), not so
	// canvas.Paint gains a second way to draw text: given the same Content
	// and Style, a ListLine Block must paint pixel-for-pixel identically to
	// a receipt.Text Block — the same guarantee
	// TestPaint_TableLineAndTextWithSameContent_RenderIdentically and
	// TestPaint_ColumnsLineAndTextWithSameContent_RenderIdentically already
	// prove for layout.TableLine and layout.ColumnsLine.
	f := layout.EmbeddedFont{}
	style := layout.Style{Bold: true, Size: 2}
	listDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: layout.ListLine{Content: "- Milk"}, Style: style},
	}}
	textDoc := layout.Document{Font: f, Blocks: []layout.Block{
		{Y: 0, Element: receipt.Text{Content: "- Milk"}, Style: style},
	}}

	clist, err := canvas.Paint(listDoc)
	if err != nil {
		t.Fatalf("Paint(listDoc) error = %v, want nil", err)
	}
	ctext, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint(textDoc) error = %v, want nil", err)
	}

	if clist.Width != ctext.Width || clist.Height != ctext.Height {
		t.Fatalf("ListLine dimensions = %dx%d, Text = %dx%d, want equal", clist.Width, clist.Height, ctext.Width, ctext.Height)
	}
	if string(clist.Bits) != string(ctext.Bits) {
		t.Errorf("ListLine and Text Bits differ given the same Content and Style, want identical")
	}
}

func TestPaint_ListFromBuild_ProducesSameBitmapAsEquivalentHandWrittenText(t *testing.T) {
	// End-to-end version of the test above: a real receipt.List run
	// through layout.Build and canvas.Paint must produce exactly the same
	// bitmap a caller would get by writing the same composed lines as
	// plain receipt.Text elements directly — the same proof
	// TestPaint_ColumnsFromBuild_ProducesSameBitmapAsEquivalentHandWrittenText
	// already gives for receipt.Columns.
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.List{Kind: "checkbox", Items: []receipt.ListItem{
			{Content: "Done", Checked: true},
			{Content: "Not done", Indent: 1},
		}},
	}}
	listDoc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	clist, err := canvas.Paint(listDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	var textBlocks []layout.Block
	for i, b := range listDoc.Blocks {
		line, ok := b.Element.(layout.ListLine)
		if !ok {
			t.Fatalf("listDoc.Blocks[%d].Element = %T, want layout.ListLine", i, b.Element)
		}
		textBlocks = append(textBlocks, layout.Block{Y: b.Y, Element: receipt.Text{Content: line.Content}, Style: b.Style})
	}
	textDoc := layout.Document{Font: f, Blocks: textBlocks}
	ctext, err := canvas.Paint(textDoc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	if clist.Width != ctext.Width || clist.Height != ctext.Height {
		t.Fatalf("List dimensions = %dx%d, equivalent Text = %dx%d, want equal", clist.Width, clist.Height, ctext.Width, ctext.Height)
	}
	if string(clist.Bits) != string(ctext.Bits) {
		t.Errorf("List and equivalent hand-written Text Bits differ, want identical")
	}
}
