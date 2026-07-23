package printer

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// networkTimeout bounds both dialing a printer and, via conn's write
// deadline, delivering the encoded bytes to it. Without the latter, a
// printer that accepts the connection but never reads would block Send's
// Write forever; since the queue worker processes one Job at a time (see
// runWorker), that one printer would wedge the entire queue indefinitely.
// 30s is generous for a receipt's raster bytes to a LAN-local thermal
// printer while bounding the worst case to "one stuck job blocks the
// queue for 30s," not forever.
const networkTimeout = 30 * time.Second

// networkPrinter sends already-encoded bytes to a printer over TCP,
// dialing fresh for every Send and Status rather than holding a connection
// open: print jobs are infrequent and a long-idle socket to this class of
// device is unreliable enough that a per-call connection is simpler and
// more robust than managing a persistent one's staleness.
type networkPrinter struct {
	address string
	dial    func(ctx context.Context, network, address string) (net.Conn, error)
	timeout time.Duration
}

// NewNetworkPrinter returns a Printer that delivers bytes to conn.Address
// over TCP. It does not dial eagerly — construction cannot fail and the
// printer need not be reachable yet; only Send and Status touch the
// network. Routing Connection.Transport here is cmd/receiptd's decision,
// so conn.Transport is not consulted.
func NewNetworkPrinter(conn Connection) Printer {
	return &networkPrinter{
		address: conn.Address,
		dial:    (&net.Dialer{Timeout: networkTimeout}).DialContext,
		timeout: networkTimeout,
	}
}

// Send dials p.address and writes data in full before closing. A single
// Write may not consume the whole slice, so Send loops until every byte
// is sent or a call fails — the "keep writing" contract io.Copy
// implements, applied directly to net.Conn here. One p.timeout deadline
// covers the whole loop, not each Write — see networkTimeout for why an
// unbounded Write is unacceptable.
func (p *networkPrinter) Send(ctx context.Context, data []byte) error {
	conn, err := p.dial(ctx, "tcp", p.address)
	if err != nil {
		return apperr.Wrap(apperr.KindTransient, "printer.Send", err)
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetWriteDeadline(time.Now().Add(p.timeout)); err != nil {
		return apperr.Wrap(apperr.KindTransient, "printer.Send", err)
	}

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
// succeeds. Raw TCP — the JetDirect/AppSocket convention thermal printers
// speak — has no query command to report anything more specific.
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
