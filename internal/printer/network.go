package printer

import (
	"context"
	"io"
	"net"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// networkPrinter sends already-encoded bytes to a printer reachable over
// TCP. It dials fresh for every Send and Status call rather than holding a
// connection open between them: print jobs are infrequent enough, and a
// long-idle socket to this class of device unreliable enough, that a
// short-lived per-call connection is simpler and more robust than managing
// a persistent one's staleness.
type networkPrinter struct {
	address string
	dial    func(ctx context.Context, network, address string) (net.Conn, error)
}

// NewNetworkPrinter returns a Printer that delivers bytes to conn.Address
// over TCP. It does not dial eagerly — construction cannot fail, and the
// printer need not be reachable yet for a Printer instance to exist for
// it — only Send and Status touch the network. Which Connection.Transport
// value routes here is cmd/receiptd's decision (the composition root), not
// this constructor's; conn.Transport is not consulted.
func NewNetworkPrinter(conn Connection) Printer {
	return &networkPrinter{
		address: conn.Address,
		dial:    (&net.Dialer{}).DialContext,
	}
}

// Send dials p.address and writes data in full before closing the
// connection. A single Write call is not guaranteed to consume the whole
// slice, so Send keeps calling Write with whatever remains until every
// byte is sent or a call fails — the same "keep writing" contract
// io.Copy/io.WriteString implement, applied here directly because net.Conn
// is used without going through one of those.
func (p *networkPrinter) Send(ctx context.Context, data []byte) error {
	conn, err := p.dial(ctx, "tcp", p.address)
	if err != nil {
		return apperr.Wrap(apperr.KindTransient, "printer.Send", err)
	}
	defer func() { _ = conn.Close() }()

	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return apperr.Wrap(apperr.KindTransient, "printer.Send", err)
		}
		if n == 0 {
			// A Write that reports neither progress nor an error would
			// otherwise spin this loop forever.
			return apperr.Wrap(apperr.KindTransient, "printer.Send", io.ErrShortWrite)
		}
		data = data[n:]
	}

	return nil
}

// Status reports the printer reachable if a TCP connection to p.address
// can be established. Raw TCP — the JetDirect/AppSocket convention thermal
// printers speak on this port — has no query command this transport could
// send to report anything more specific than that.
func (p *networkPrinter) Status(ctx context.Context) (Status, error) {
	conn, err := p.dial(ctx, "tcp", p.address)
	if err != nil {
		return Status{Online: false, Detail: err.Error()}, nil
	}
	_ = conn.Close()
	return Status{Online: true}, nil
}

// Close releases resources held by p. networkPrinter holds none between
// calls — see the type doc comment — so Close is a no-op.
func (p *networkPrinter) Close() error {
	return nil
}
