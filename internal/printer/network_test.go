package printer_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/printer"
)

// These tests use real local net.Listen servers rather than mocks — see
// docs/ARCHITECTURE.md §9 ("printer (network): Local net.Listen fake
// server — no hardware in CI"). Scenarios that a real TCP socket can't be
// coaxed into deterministically (a partial write with a nil error, an
// arbitrary write failure) are covered instead in network_internal_test.go,
// which stubs networkPrinter's dial seam directly.

func TestNewNetworkPrinter_Send_DeliversCompleteByteStream(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()

	want := []byte{0x1B, 0x40, 0x1D, 0x76, 0x30, 0x00, 0xAA, 0x55}
	received := make(chan []byte, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			received <- nil
			return
		}
		defer func() { _ = conn.Close() }()
		got, _ := io.ReadAll(conn)
		received <- got
	}()

	p := printer.NewNetworkPrinter(printer.Connection{Transport: "network", Address: ln.Addr().String()})
	if err := p.Send(context.Background(), want); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}

	select {
	case got := <-received:
		if !bytes.Equal(got, want) {
			t.Errorf("server received % x, want % x", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received data")
	}
}

func TestNewNetworkPrinter_Send_ClosesConnectionOnSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		// ReadAll only returns once it observes EOF, which for a TCP
		// connection means the client closed its side.
		_, _ = io.ReadAll(conn)
	}()

	p := printer.NewNetworkPrinter(printer.Connection{Address: ln.Addr().String()})
	if err := p.Send(context.Background(), []byte("data")); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("server never observed EOF — Send did not close the connection")
	}
}

func TestNewNetworkPrinter_Send_ConnectionFailure_ReturnsTransientError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listening on addr now

	p := printer.NewNetworkPrinter(printer.Connection{Address: addr})
	err = p.Send(context.Background(), []byte("data"))
	if !apperr.Is(err, apperr.KindTransient) {
		t.Fatalf("Send() error = %v, want apperr.KindTransient", err)
	}
}

func TestNewNetworkPrinter_Status_Reachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	p := printer.NewNetworkPrinter(printer.Connection{Address: ln.Addr().String()})
	status, err := p.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if !status.Online {
		t.Errorf("status.Online = false, want true")
	}
}

func TestNewNetworkPrinter_Status_Unreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	p := printer.NewNetworkPrinter(printer.Connection{Address: addr})
	status, err := p.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v, want nil", err)
	}
	if status.Online {
		t.Error("status.Online = true, want false")
	}
	if status.Detail == "" {
		t.Error("status.Detail is empty, want a reason the printer is unreachable")
	}
}

func TestNewNetworkPrinter_Close_ReturnsNil(t *testing.T) {
	p := printer.NewNetworkPrinter(printer.Connection{Address: "127.0.0.1:0"})
	if err := p.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}
