package config

import (
	"errors"
	"fmt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Validate checks Config against the local invariants required by the
// frozen schema (docs/ARCHITECTURE.md §7): a supported queue.store value,
// positive queue.max_attempts and queue.retry_backoff, and, per printer, a
// non-empty name, valid dimensions/margins, and no duplicate names across
// printers[]. It does not invent defaults or policy beyond what the
// schema documents.
func (c Config) Validate() error {
	var errs []error

	switch c.Queue.Store {
	case "memory", "bbolt":
	default:
		errs = append(errs, fmt.Errorf("queue.store must be %q or %q, got %q", "memory", "bbolt", c.Queue.Store))
	}
	if c.Queue.MaxAttempts <= 0 {
		errs = append(errs, fmt.Errorf("queue.max_attempts must be positive, got %d", c.Queue.MaxAttempts))
	}
	if c.Queue.RetryBackoff <= 0 {
		errs = append(errs, fmt.Errorf("queue.retry_backoff must be positive, got %v", c.Queue.RetryBackoff))
	}

	seen := make(map[string]bool, len(c.Printers))
	for i, p := range c.Printers {
		if err := p.validate(); err != nil {
			errs = append(errs, fmt.Errorf("printers[%d] %q: %w", i, p.Name, err))
			continue
		}
		if seen[p.Name] {
			errs = append(errs, fmt.Errorf("printers[%d]: duplicate printer name %q", i, p.Name))
			continue
		}
		seen[p.Name] = true
	}

	if err := errors.Join(errs...); err != nil {
		return apperr.Wrap(apperr.KindValidation, "config.Validate", err)
	}
	return nil
}
