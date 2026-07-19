package app

// This file is a deliberate, disclosed exception to this codebase's
// black-box-only test convention (every other _test.go file in the repo
// is package x_test). render is unexported and has no observable effect
// through Process's error-only return in this slice, so "processing
// returns a rendered result" can only be asserted by testing render
// directly. See docs/ARCHITECTURE.md §2's Font exception for the
// precedent of naming a deliberate one-off rather than treating it as
// the default.

import (
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
	"github.com/harveysandiego/receiptd/internal/receipt"
	"github.com/harveysandiego/receiptd/internal/render/layout"
)

func TestService_render_Success_ReturnsRenderedCanvas(t *testing.T) {
	s := &Service{}
	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hello"}}}

	c, err := s.render(r, printer.Profile{})
	if err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	if c == nil {
		t.Fatal("render() canvas = nil, want a rendered Canvas")
	}

	f := layout.EmbeddedFont{}
	if c.Width != f.Measure("hello") {
		t.Errorf("canvas.Width = %d, want %d", c.Width, f.Measure("hello"))
	}
	if c.Height != f.LineHeight() {
		t.Errorf("canvas.Height = %d, want %d", c.Height, f.LineHeight())
	}
}

func TestService_render_ReceiptContentReachesRendererUnchanged(t *testing.T) {
	s := &Service{}
	f := layout.EmbeddedFont{}

	short, err := s.render(receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hi"}}}, printer.Profile{})
	if err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	long, err := s.render(receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hello world"}}}, printer.Profile{})
	if err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}

	if short.Width != f.Measure("hi") {
		t.Errorf("short canvas.Width = %d, want %d", short.Width, f.Measure("hi"))
	}
	if long.Width != f.Measure("hello world") {
		t.Errorf("long canvas.Width = %d, want %d", long.Width, f.Measure("hello world"))
	}
	if short.Width >= long.Width {
		t.Errorf("short.Width = %d, long.Width = %d, want short < long (each Receipt's own content must reach the renderer, not a shared/placeholder value)", short.Width, long.Width)
	}
}

// unsupportedElement is a receipt.Element with no render/layout.Build or
// render/canvas.Paint support, used to exercise render's error
// propagation. receipt.Divider filled this role until it became a
// supported element in its own right — every other element type
// documented in docs/ARCHITECTURE.md §3 (image, asset, qrcode, etc.)
// doesn't exist as a receipt.Go type yet, so a small local fake is now
// the only way to construct a "valid but unrenderable" Receipt.
type unsupportedElement struct{}

func (unsupportedElement) Validate() error { return nil }

func TestService_render_UnsupportedElement_ReturnsPermanentError(t *testing.T) {
	s := &Service{}
	r := receipt.Receipt{Elements: []receipt.Element{unsupportedElement{}}}

	c, err := s.render(r, printer.Profile{})
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("render() error = %v, want apperr.KindPermanent", err)
	}
	if c != nil {
		t.Errorf("render() canvas = %+v, want nil on error", c)
	}
}

func TestService_render_ProfileWidthDots_SetsCanvasWidth(t *testing.T) {
	s := &Service{}
	r := receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hi"}}}

	c, err := s.render(r, printer.Profile{WidthDots: 200})
	if err != nil {
		t.Fatalf("render() error = %v, want nil", err)
	}
	if c.Width != 200 {
		t.Errorf("canvas.Width = %d, want 200 (profile.WidthDots, not content-fit)", c.Width)
	}
}
