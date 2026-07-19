package printer

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// This file is package printer (not printer_test), solely so it can set
// networkPrinter.dial directly. A real TCP socket's Write essentially never
// returns a partial count with a nil error — the standard library loops
// internally until all bytes are sent or an error occurs — so short writes
// can only be exercised deterministically by substituting a fake net.Conn,
// the same technique queue/retry_test.go uses on Queue.sleep to drive an
// otherwise-unreachable retry path.

// maxFakeConnWrites caps how many Write calls fakeConn will honor before
// forcing a call to error out. Its only job is to turn a genuine infinite
// loop in Send into a fast, deterministic test failure instead of a
// multi-minute hang against go test's default timeout.
const maxFakeConnWrites = 1000

// writeResult scripts one fakeConn.Write call's return values.
type writeResult struct {
	n   int
	err error
}

// fakeConn is a net.Conn test double that lets a test script a sequence of
// Write outcomes — one writeResult consumed per call, the last repeating
// for any call beyond len(writes). It records every byte actually
// consumed so a test can assert the complete stream arrived across
// multiple partial writes. Only Write and Close are ever exercised through
// networkPrinter, so every other method is left to the embedded nil
// net.Conn — a test that ends up calling one of those is testing the wrong
// thing.
type fakeConn struct {
	net.Conn
	writes  []writeResult
	calls   int
	written []byte
	closed  bool
}

func (c *fakeConn) Write(p []byte) (int, error) {
	c.calls++
	if c.calls > maxFakeConnWrites {
		return 0, errors.New("fakeConn: exceeded max Write calls, Send is likely looping forever")
	}

	i := c.calls - 1
	if i >= len(c.writes) {
		i = len(c.writes) - 1
	}
	wr := c.writes[i]

	n := wr.n
	if n > len(p) {
		n = len(p)
	}
	c.written = append(c.written, p[:n]...)
	return n, wr.err
}

func (c *fakeConn) Close() error {
	c.closed = true
	return nil
}

func newTestNetworkPrinter(fc *fakeConn) *networkPrinter {
	return &networkPrinter{address: "unused", dial: func(_ context.Context, _, _ string) (net.Conn, error) {
		return fc, nil
	}}
}

func TestNetworkPrinter_Send_MultiplePartialWrites_DeliversCompleteByteStream(t *testing.T) {
	data := []byte("hello world")
	fc := &fakeConn{writes: []writeResult{{n: 3}, {n: 4}, {n: 4}}} // 3+4+4 = len(data)
	p := newTestNetworkPrinter(fc)

	if err := p.Send(context.Background(), data); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if string(fc.written) != string(data) {
		t.Errorf("connection received %q, want %q", fc.written, data)
	}
}

func TestNetworkPrinter_Send_PartialWriteThenSuccess_CompletesSuccessfully(t *testing.T) {
	data := []byte("hello")
	fc := &fakeConn{writes: []writeResult{{n: 2}, {n: 3}}}
	p := newTestNetworkPrinter(fc)

	if err := p.Send(context.Background(), data); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if string(fc.written) != string(data) {
		t.Errorf("connection received %q, want %q", fc.written, data)
	}
}

func TestNetworkPrinter_Send_PartialWriteThenError_PropagatesError(t *testing.T) {
	fc := &fakeConn{writes: []writeResult{{n: 2}, {n: 0, err: errors.New("broken pipe")}}}
	p := newTestNetworkPrinter(fc)

	err := p.Send(context.Background(), []byte("hello"))
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Send() error = %v, want apperr.KindTransient", err)
	}
}

func TestNetworkPrinter_Send_ZeroByteWriteWithNoError_ReturnsErrorWithoutLooping(t *testing.T) {
	fc := &fakeConn{writes: []writeResult{{n: 0, err: nil}}}
	p := newTestNetworkPrinter(fc)

	err := p.Send(context.Background(), []byte("hello"))
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Send() error = %v, want apperr.KindTransient", err)
	}
	if fc.calls > maxFakeConnWrites {
		t.Errorf("Write called %d times, want it to stop after detecting a zero-byte, nil-error write", fc.calls)
	}
}

func TestNetworkPrinter_Send_WriteFailure_ReturnsTransientError(t *testing.T) {
	fc := &fakeConn{writes: []writeResult{{err: errors.New("broken pipe")}}}
	p := newTestNetworkPrinter(fc)

	err := p.Send(context.Background(), []byte("hello"))
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Send() error = %v, want apperr.KindTransient", err)
	}
}

func TestNetworkPrinter_Send_ClosesConnectionOnWriteFailure(t *testing.T) {
	fc := &fakeConn{writes: []writeResult{{err: errors.New("broken pipe")}}}
	p := newTestNetworkPrinter(fc)

	_ = p.Send(context.Background(), []byte("hello"))
	if !fc.closed {
		t.Error("Send() did not close the connection after a write failure")
	}
}

func TestNetworkPrinter_Send_ClosesConnectionAfterMultiplePartialWrites(t *testing.T) {
	fc := &fakeConn{writes: []writeResult{{n: 2}, {n: 3}}}
	p := newTestNetworkPrinter(fc)

	_ = p.Send(context.Background(), []byte("hello"))
	if !fc.closed {
		t.Error("Send() did not close the connection after completing multiple partial writes")
	}
}
