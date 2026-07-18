package printer

// Connection describes how to reach a printer. It carries no information
// about what the printer can do — see Profile for that split, and
// docs/ARCHITECTURE.md §1 for why the two are never mixed in a single
// function signature. cmd/receiptd is the only code in the module that
// constructs a Connection.
type Connection struct {
	// Transport selects which of the fields below is meaningful:
	// "network" (v0.1); "usb" | "bluetooth" | "serial" later.
	Transport string
	// Address is the network transport's host:port.
	Address string
	// Device is the usb transport's device path (future).
	Device string
	// MAC is the bluetooth transport's MAC address (future).
	MAC string
}
