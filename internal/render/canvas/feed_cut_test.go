package canvas_test

import (
	"context"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/canvas"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestPaint_FeedBetweenTextBlocks_NoGapInPixels(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Feed{Lines: 10},
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: f.Measure("A") + 200}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpB) // immediately after A's line, Feed added no vertical space
	if c.Height != 2*lh {
		t.Errorf("c.Height = %d, want %d (A's line plus B's line, nothing extra for Feed)", c.Height, 2*lh)
	}
}

func TestPaint_CutBetweenTextBlocks_NoGapInPixels(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Cut{},
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: f.Measure("A") + 200}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}

	bmpA, _ := f.Glyph('A')
	bmpB, _ := f.Glyph('B')
	assertGlyphPainted(t, c, 0, bmpA)
	assertGlyphPainted(t, c, lh, bmpB)
	if c.Height != 2*lh {
		t.Errorf("c.Height = %d, want %d (A's line plus B's line, nothing extra for Cut)", c.Height, 2*lh)
	}
}

func TestPaint_FeedRecordedAsControl(t *testing.T) {
	f := layout.EmbeddedFont{}
	lh := f.LineHeight()
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Feed{Lines: 6},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if len(c.Controls) != 1 {
		t.Fatalf("len(c.Controls) = %d, want 1", len(c.Controls))
	}
	got := c.Controls[0]
	if got.Y != lh {
		t.Errorf("Controls[0].Y = %d, want %d", got.Y, lh)
	}
	if got.Element != (receipt.Feed{Lines: 6}) {
		t.Errorf("Controls[0].Element = %v, want Feed{Lines: 6}", got.Element)
	}
	if !got.Terminal {
		t.Errorf("Controls[0].Terminal = false, want true (last Block in the Document)")
	}
}

func TestPaint_CutRecordedAsControl_NotTerminalWhenFollowedByMoreContent(t *testing.T) {
	f := layout.EmbeddedFont{}
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A"},
		receipt.Cut{Mode: "partial"},
		receipt.Text{Content: "B"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, f, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	if len(c.Controls) != 1 {
		t.Fatalf("len(c.Controls) = %d, want 1", len(c.Controls))
	}
	if c.Controls[0].Terminal {
		t.Errorf("Controls[0].Terminal = true, want false (Text \"B\" follows the Cut)")
	}
	if c.Controls[0].Element != (receipt.Cut{Mode: "partial"}) {
		t.Errorf("Controls[0].Element = %v, want Cut{Mode: \"partial\"}", c.Controls[0].Element)
	}
}

func TestPaint_MultipleFeedAndCutControls_PreserveOrder(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Feed{Lines: 1},
		receipt.Cut{Mode: "partial"},
		receipt.Feed{Lines: 2},
		receipt.Cut{Mode: "full"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	want := []receipt.Element{
		receipt.Feed{Lines: 1},
		receipt.Cut{Mode: "partial"},
		receipt.Feed{Lines: 2},
		receipt.Cut{Mode: "full"},
	}
	if len(c.Controls) != len(want) {
		t.Fatalf("len(c.Controls) = %d, want %d", len(c.Controls), len(want))
	}
	for i, el := range want {
		if c.Controls[i].Element != el {
			t.Errorf("Controls[%d].Element = %v, want %v", i, c.Controls[i].Element, el)
		}
	}
	if !c.Controls[len(want)-1].Terminal {
		t.Errorf("Controls[%d].Terminal = false, want true (last Block in the Document)", len(want)-1)
	}
	for i := 0; i < len(want)-1; i++ {
		if c.Controls[i].Terminal {
			t.Errorf("Controls[%d].Terminal = true, want false", i)
		}
	}
}

func TestPaint_FeedAndCutDoNotAffectDocumentWidth(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Hi"},
		receipt.Feed{Lines: 200},
		receipt.Cut{},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("layout.Build() error = %v, want nil", err)
	}
	c, err := canvas.Paint(doc)
	if err != nil {
		t.Fatalf("Paint() error = %v, want nil", err)
	}
	f := layout.EmbeddedFont{}
	if want := f.Measure("Hi"); c.Width != want {
		t.Errorf("c.Width = %d, want %d (Feed/Cut contribute no width)", c.Width, want)
	}
}
