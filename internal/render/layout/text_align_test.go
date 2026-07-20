package layout_test

import (
	"context"
	"strings"
	"testing"

	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

// leadingSpaces counts s's leading space runes.
func leadingSpaces(s string) int {
	return len(s) - len(strings.TrimLeft(s, " "))
}

func buildOneText(t *testing.T, text receipt.Text, widthDots int) string {
	t.Helper()
	r := receipt.Receipt{Elements: []receipt.Element{text}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: widthDots}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	got, ok := doc.Blocks[0].Element.(receipt.Text)
	if !ok {
		t.Fatalf("doc.Blocks[0].Element = %T, want receipt.Text", doc.Blocks[0].Element)
	}
	return got.Content
}

func TestBuild_TextAlignOmitted_NoPadding(t *testing.T) {
	got := buildOneText(t, receipt.Text{Content: "Milk"}, 1000)
	if got != "Milk" {
		t.Errorf("Content = %q, want %q (omitted Align behaves like left, unchanged)", got, "Milk")
	}
}

func TestBuild_TextAlignLeft_NoPadding(t *testing.T) {
	got := buildOneText(t, receipt.Text{Content: "Milk", Align: "left"}, 1000)
	if got != "Milk" {
		t.Errorf("Content = %q, want %q (explicit left, unchanged)", got, "Milk")
	}
}

func TestBuild_TextAlignCenter_PadsLeadingSpaces(t *testing.T) {
	got := buildOneText(t, receipt.Text{Content: "Milk", Align: "center"}, 1000)
	if !strings.HasPrefix(got, " ") {
		t.Errorf("Content = %q, want leading space padding to center it within a much wider printable width", got)
	}
	if strings.TrimLeft(got, " ") != "Milk" {
		t.Errorf("Content = %q, want padding only, original content unchanged", got)
	}
}

func TestBuild_TextAlignRight_PadsMoreThanCenter(t *testing.T) {
	center := buildOneText(t, receipt.Text{Content: "Milk", Align: "center"}, 1000)
	right := buildOneText(t, receipt.Text{Content: "Milk", Align: "right"}, 1000)
	if leadingSpaces(right) <= leadingSpaces(center) {
		t.Errorf("leadingSpaces(right) = %d, leadingSpaces(center) = %d, want right to pad more than center", leadingSpaces(right), leadingSpaces(center))
	}
}

func TestBuild_TextAlignRight_ReachesRightEdge(t *testing.T) {
	f := layout.EmbeddedFont{}
	const width = 1000
	got := buildOneText(t, receipt.Text{Content: "Milk", Align: "right"}, width)
	measured := f.Measure(got)
	spaceWidth := f.Measure(" ")
	if measured > width {
		t.Errorf("measured content width = %d, want <= %d (printable width)", measured, width)
	}
	if measured <= width-spaceWidth {
		t.Errorf("measured content width = %d, want > %d (within one space glyph of the right edge)", measured, width-spaceWidth)
	}
}

func TestBuild_TextAlign_WrappedLinesEachAlignedIndependently(t *testing.T) {
	// "A" and "BB" are different natural widths; Align: "right" must pad
	// each wrapped line's own leading space count independently so both
	// reach the same right edge, rather than aligning the whole element as
	// one block against the longer line.
	f := layout.EmbeddedFont{}
	const width = 1000
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "A\nBB", Align: "right"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: width}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("len(doc.Blocks) = %d, want 2", len(doc.Blocks))
	}
	line0 := doc.Blocks[0].Element.(receipt.Text).Content
	line1 := doc.Blocks[1].Element.(receipt.Text).Content
	if strings.TrimLeft(line0, " ") != "A" {
		t.Errorf("line0 = %q, want trimmed content %q", line0, "A")
	}
	if strings.TrimLeft(line1, " ") != "BB" {
		t.Errorf("line1 = %q, want trimmed content %q", line1, "BB")
	}
	if leadingSpaces(line0) <= leadingSpaces(line1) {
		t.Errorf("leadingSpaces(line0=%q) = %d, leadingSpaces(line1=%q) = %d, want line0 (shorter content) padded more to reach the same right edge", line0, leadingSpaces(line0), line1, leadingSpaces(line1))
	}
	spaceWidth := f.Measure(" ")
	for i, line := range []string{line0, line1} {
		measured := f.Measure(line)
		if measured > width || measured <= width-spaceWidth {
			t.Errorf("line %d measured width = %d, want within one space glyph of %d", i, measured, width)
		}
	}
}

func TestBuild_TextAlignRight_ContentWiderThanPrintableWidth_NoPaddingPanic(t *testing.T) {
	f := layout.EmbeddedFont{}
	const content = "Milk"
	width := f.Measure(content) // exactly as wide as the content itself
	got := buildOneText(t, receipt.Text{Content: content, Align: "right"}, width)
	if got != content {
		t.Errorf("Content = %q, want %q unchanged (no room to pad)", got, content)
	}
}

func TestBuild_TextAlignRight_ZeroWidthDots_NoPadding(t *testing.T) {
	// printer.Profile{WidthDots: 0} is Build's documented "no printer
	// configured" sentinel — the same fallback wrapText and alignPad both
	// already apply.
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk", Align: "right"},
	}}
	doc, err := layout.Build(context.Background(), r, printer.Profile{}, layout.EmbeddedFont{}, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	got := doc.Blocks[0].Element.(receipt.Text).Content
	if got != "Milk" {
		t.Errorf("Content = %q, want %q unchanged (no printable width to align against)", got, "Milk")
	}
}

func TestBuild_TextAlignCenter_Deterministic(t *testing.T) {
	r := receipt.Receipt{Elements: []receipt.Element{
		receipt.Text{Content: "Milk", Align: "center"},
	}}
	f := layout.EmbeddedFont{}
	first, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 500}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	second, err := layout.Build(context.Background(), r, printer.Profile{WidthDots: 500}, f, nil)
	if err != nil {
		t.Fatalf("Build() error = %v, want nil", err)
	}
	if first.Blocks[0].Element != second.Blocks[0].Element {
		t.Errorf("Blocks[0].Element = %v, then %v, want equal", first.Blocks[0].Element, second.Blocks[0].Element)
	}
}
