package assets

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// Store is named asset storage: Get resolves a previously-stored name back
// to its bytes (the only method render/layout.Build itself calls, to
// resolve a receipt.Asset — docs/ARCHITECTURE.md §3 "Image vs. Asset");
// Put, Delete, and List round out the CRUD surface a future asset
// management endpoint (Milestone 4's webui) needs, so that surface is
// defined once here rather than grown field-by-field later. See
// docs/ARCHITECTURE.md §2 for the full interface as designed.
type Store interface {
	Get(ctx context.Context, name string) ([]byte, error)
	Put(ctx context.Context, name string, data []byte) error
	Delete(ctx context.Context, name string) error
	List(ctx context.Context) ([]string, error)
}

// validateName rejects any name that isn't a safe, single-segment asset
// key, before either Store implementation does anything with it — the
// same rule receipt.Asset.Validate() already applies to a Name arriving
// via a Receipt, reused here so a name arriving directly at a Store
// method (e.g. a future asset-management API handler, which never goes
// through receipt.Asset.Validate() at all) gets the same protection
// against escaping FilesystemStore's root via ".." or a path separator.
// MemoryStore has no filesystem to escape, but applies the same rule so
// both implementations reject exactly the same set of names — a caller
// can't depend on behaviour that happens to differ between them.
func validateName(op, name string) error {
	if name == "" {
		return apperr.Wrap(apperr.KindValidation, op, errors.New("asset name is required"))
	}
	if strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return apperr.Wrap(apperr.KindValidation, op, fmt.Errorf("invalid asset name %q", name))
	}
	return nil
}
