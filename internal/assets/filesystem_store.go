package assets

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// filesystemStore is a Store backed by one file per asset, all directly
// under root — the persistent implementation docs/ARCHITECTURE.md §1
// describes ("assets/ Store interface + filesystem implementation"),
// configured via config.AssetsConfig.Path. validateName (shared with
// memoryStore) already rejects any name containing a path separator or a
// bare "."/"..", so name is always a single path segment by the time it
// reaches filepath.Join — root is never escaped.
type filesystemStore struct {
	root string
}

// NewFilesystemStore returns a Store that persists each asset as a single
// file named name directly under root. root is created on first Put if it
// doesn't already exist; Get/Delete/List against a not-yet-created root
// behave exactly as they would against an existing, empty directory
// (missing asset, or an empty List) rather than surfacing root's own
// absence as an error — a fresh Receiptd install has no assets yet, which
// isn't itself a failure.
func NewFilesystemStore(root string) Store {
	return &filesystemStore{root: root}
}

func (s *filesystemStore) path(name string) (string, error) {
	if err := validateName("assets.Store", name); err != nil {
		return "", err
	}
	return filepath.Join(s.root, name), nil
}

func (s *filesystemStore) Get(_ context.Context, name string) ([]byte, error) {
	p, err := s.path(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, apperr.Wrap(apperr.KindNotFound, "assets.Store.Get", err)
		}
		return nil, apperr.Wrap(apperr.KindPermanent, "assets.Store.Get", err)
	}
	return data, nil
}

func (s *filesystemStore) Put(_ context.Context, name string, data []byte) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return apperr.Wrap(apperr.KindPermanent, "assets.Store.Put", err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return apperr.Wrap(apperr.KindPermanent, "assets.Store.Put", err)
	}
	return nil
}

func (s *filesystemStore) Delete(_ context.Context, name string) error {
	p, err := s.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return apperr.Wrap(apperr.KindNotFound, "assets.Store.Delete", err)
		}
		return apperr.Wrap(apperr.KindPermanent, "assets.Store.Delete", err)
	}
	return nil
}

func (s *filesystemStore) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, apperr.Wrap(apperr.KindPermanent, "assets.Store.List", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}
