package apperr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

func TestKindString(t *testing.T) {
	tests := []struct {
		name string
		kind apperr.Kind
		want string
	}{
		{"unknown", apperr.KindUnknown, "unknown"},
		{"validation", apperr.KindValidation, "validation"},
		{"not found", apperr.KindNotFound, "not found"},
		{"transient", apperr.KindTransient, "transient"},
		{"permanent", apperr.KindPermanent, "permanent"},
		{"unauthorized", apperr.KindUnauthorized, "unauthorized"},
		{"out of range", apperr.Kind(99), "apperr.Kind(99)"},
		{"negative", apperr.Kind(-1), "apperr.Kind(-1)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.String(); got != tt.want {
				t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	err := apperr.Wrap(apperr.KindTransient, "printer.Send", cause)

	if err == nil {
		t.Fatal("Wrap returned nil")
	}
	if err.Kind != apperr.KindTransient {
		t.Errorf("Kind = %v, want %v", err.Kind, apperr.KindTransient)
	}
	if err.Op != "printer.Send" {
		t.Errorf("Op = %q, want %q", err.Op, "printer.Send")
	}
	if !errors.Is(err.Err, cause) {
		t.Errorf("Err = %v, want %v", err.Err, cause)
	}
}

func TestWrapNilErr(t *testing.T) {
	err := apperr.Wrap(apperr.KindNotFound, "assets.Get", nil)

	if err == nil {
		t.Fatal("Wrap(_, _, nil) returned nil, want a non-nil *Error")
	}
	if err.Err != nil {
		t.Errorf("Err = %v, want nil", err.Err)
	}
	if err.Kind != apperr.KindNotFound {
		t.Errorf("Kind = %v, want %v", err.Kind, apperr.KindNotFound)
	}
}

func TestErrorError(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name string
		err  *apperr.Error
		want string
	}{
		{"nil receiver", nil, "<nil>"},
		{"all zero", &apperr.Error{}, "unknown"},
		{"op only", &apperr.Error{Op: "assets.Get"}, "assets.Get"},
		{"kind only", &apperr.Error{Kind: apperr.KindNotFound}, "not found"},
		{"err only", &apperr.Error{Err: boom}, "boom"},
		{"op and kind", &apperr.Error{Op: "assets.Get", Kind: apperr.KindNotFound}, "assets.Get: not found"},
		{"op and err", &apperr.Error{Op: "assets.Get", Err: boom}, "assets.Get: boom"},
		{"kind and err", &apperr.Error{Kind: apperr.KindNotFound, Err: boom}, "not found: boom"},
		{
			"op, kind, and err",
			&apperr.Error{Op: "assets.Get", Kind: apperr.KindNotFound, Err: boom},
			"assets.Get: not found: boom",
		},
		{
			"unknown kind is not printed",
			&apperr.Error{Op: "assets.Get", Kind: apperr.KindUnknown, Err: boom},
			"assets.Get: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	err := apperr.Wrap(apperr.KindValidation, "receipt.Validate", errors.New("missing field"))
	want := "receipt.Validate: validation: missing field"

	if got := err.Error(); got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
	if got := fmt.Sprintf("%v", err); got != want {
		t.Errorf("%%v = %q, want %q", got, want)
	}
	if got := fmt.Sprintf("%s", err); got != want {
		t.Errorf("%%s = %q, want %q", got, want)
	}
}

func TestErrorUnwrap(t *testing.T) {
	cause := errors.New("boom")
	err := apperr.Wrap(apperr.KindPermanent, "render.Paint", cause)

	if got := err.Unwrap(); !errors.Is(got, cause) {
		t.Errorf("Unwrap() = %v, want %v", got, cause)
	}
}

func TestErrorUnwrapNilReceiver(t *testing.T) {
	var err *apperr.Error
	if got := err.Unwrap(); got != nil {
		t.Errorf("Unwrap() on nil *Error = %v, want nil", got)
	}
}

func TestErrorUnwrapNoCause(t *testing.T) {
	err := apperr.Wrap(apperr.KindValidation, "receipt.Validate", nil)
	if got := err.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestErrorsAs(t *testing.T) {
	cause := errors.New("dial tcp: timeout")
	wrapped := fmt.Errorf("send: %w", apperr.Wrap(apperr.KindTransient, "printer.Send", cause))

	var target *apperr.Error
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As failed to find *apperr.Error in the chain")
	}
	if target.Kind != apperr.KindTransient {
		t.Errorf("Kind = %v, want %v", target.Kind, apperr.KindTransient)
	}
	if target.Op != "printer.Send" {
		t.Errorf("Op = %q, want %q", target.Op, "printer.Send")
	}
}

func TestErrorsIs(t *testing.T) {
	sentinel := errors.New("not found on disk")
	wrapped := apperr.Wrap(apperr.KindNotFound, "assets.Get", sentinel)

	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is failed to see through *apperr.Error to the wrapped sentinel")
	}
}

// TestErrorsIsThroughMultipleLayers guards against a regression where
// Unwrap only unwraps one level: a sentinel must stay discoverable via
// errors.Is even under a fmt.Errorf wrapping two nested apperr.Wrap
// calls.
func TestErrorsIsThroughMultipleLayers(t *testing.T) {
	sentinel := errors.New("printer offline")
	wrapped := fmt.Errorf("send: %w", apperr.Wrap(apperr.KindPermanent, "app.Print",
		apperr.Wrap(apperr.KindTransient, "printer.Send", sentinel)))

	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is failed to find the sentinel through nested apperr.Wrap layers and an outer fmt.Errorf")
	}
}

func TestIs(t *testing.T) {
	sentinel := errors.New("boom")

	tests := []struct {
		name string
		err  error
		kind apperr.Kind
		want bool
	}{
		{"nil error", nil, apperr.KindNotFound, false},
		{"plain error, no Kind", sentinel, apperr.KindNotFound, false},
		{
			"matching kind",
			apperr.Wrap(apperr.KindNotFound, "assets.Get", sentinel),
			apperr.KindNotFound,
			true,
		},
		{
			"non-matching kind",
			apperr.Wrap(apperr.KindNotFound, "assets.Get", sentinel),
			apperr.KindTransient,
			false,
		},
		{
			"zero-value kind matches KindUnknown",
			apperr.Wrap(apperr.KindUnknown, "x", sentinel),
			apperr.KindUnknown,
			true,
		},
		{
			"wrapped via fmt.Errorf",
			fmt.Errorf("op: %w", apperr.Wrap(apperr.KindUnauthorized, "auth.Check", sentinel)),
			apperr.KindUnauthorized,
			true,
		},
		{
			"nested apperr wrapping, outer kind",
			apperr.Wrap(apperr.KindPermanent, "app.Print", apperr.Wrap(apperr.KindTransient, "printer.Send", sentinel)),
			apperr.KindPermanent,
			true,
		},
		{
			"nested apperr wrapping, inner kind still reachable",
			apperr.Wrap(apperr.KindPermanent, "app.Print", apperr.Wrap(apperr.KindTransient, "printer.Send", sentinel)),
			apperr.KindTransient,
			true,
		},
		{
			"nested apperr wrapping, kind absent from chain",
			apperr.Wrap(apperr.KindPermanent, "app.Print", apperr.Wrap(apperr.KindTransient, "printer.Send", sentinel)),
			apperr.KindValidation,
			false,
		},
		{
			"typed nil *Error in a non-nil interface",
			error((*apperr.Error)(nil)),
			apperr.KindNotFound,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := apperr.Is(tt.err, tt.kind); got != tt.want {
				t.Errorf("Is() = %v, want %v", got, tt.want)
			}
		})
	}
}
