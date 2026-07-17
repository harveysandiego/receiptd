package apperr

import (
	"errors"
	"fmt"
	"strings"
)

// Kind classifies why an operation failed, independent of any specific
// error message — it's what callers branch on instead of message text.
// See docs/adr/0005-error-handling.md.
type Kind int

// The closed set of failure kinds. A new failure mode should almost
// always map onto one of these by asking what the API, queue, or logs
// should *do* about it, not by adding a kind that describes it more
// precisely.
const (
	// KindUnknown is the zero value: a Kind was never assigned.
	KindUnknown Kind = iota
	// KindValidation is a bad Receipt or bad request. Maps to HTTP 400;
	// never retried.
	KindValidation
	// KindNotFound is a missing asset, template, or job. Maps to HTTP
	// 404; never retried.
	KindNotFound
	// KindTransient is a temporary failure (printer offline, transport
	// timeout). The only Kind the job queue spends retry budget on.
	KindTransient
	// KindPermanent is an unrecoverable failure (e.g. a renderer bug).
	// Maps to HTTP 500; never retried.
	KindPermanent
	// KindUnauthorized is a missing or invalid credential. Maps to HTTP
	// 401; never retried.
	KindUnauthorized
)

// String returns a human-readable name for k, suitable for direct
// display. Code that needs to branch on failure category should use Is,
// not this string.
func (k Kind) String() string {
	switch k {
	case KindUnknown:
		return "unknown"
	case KindValidation:
		return "validation"
	case KindNotFound:
		return "not found"
	case KindTransient:
		return "transient"
	case KindPermanent:
		return "permanent"
	case KindUnauthorized:
		return "unauthorized"
	default:
		return fmt.Sprintf("apperr.Kind(%d)", int(k))
	}
}

// Error is a typed, wrapped error: the Kind it should be treated as, the
// operation that produced it (e.g. "layout.Build", "assets.Get"), and,
// optionally, the underlying cause.
//
// Error deliberately has no Msg field: Op plus the wrapped Err already
// carry the human-readable detail, and Kind is meant to be read
// programmatically via Is, not reconstructed from message text.
type Error struct {
	Kind Kind
	Op   string
	Err  error
}

// Error implements the error interface. A nil *Error returns a
// placeholder instead of panicking, since Go permits calling methods on
// a nil pointer.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	var b strings.Builder
	if e.Op != "" {
		b.WriteString(e.Op)
	}
	if e.Kind != KindUnknown {
		if b.Len() > 0 {
			b.WriteString(": ")
		}
		b.WriteString(e.Kind.String())
	}
	if e.Err != nil {
		if b.Len() > 0 {
			b.WriteString(": ")
		}
		b.WriteString(e.Err.Error())
	}

	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// Unwrap returns the wrapped cause, if any, so that errors.Is and
// errors.As can see through an *Error to whatever it wraps.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Wrap creates an *Error tagging err with kind and the operation, op,
// that produced it. Wrap always returns a non-nil *Error, even when err
// is nil, so it can also be used to construct a fresh leaf error (e.g.
// apperr.Wrap(apperr.KindNotFound, "assets.Get", nil)) without a second
// constructor — callers never need to guard against a nil result.
func Wrap(kind Kind, op string, err error) *Error {
	return &Error{Kind: kind, Op: op, Err: err}
}

// Is reports whether err, or any error it wraps, is an *Error of the
// given Kind. Consumers that need to branch on failure category (HTTP
// status, retry-or-not, log severity) should use Is, never compare
// err.Error() strings.
func Is(err error, kind Kind) bool {
	for err != nil {
		var e *Error
		if !errors.As(err, &e) || e == nil {
			return false
		}
		if e.Kind == kind {
			return true
		}
		err = errors.Unwrap(e)
	}
	return false
}
