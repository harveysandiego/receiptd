package assets_test

import (
	"context"
	"testing"

	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/assets"
)

// storeFactories is every assets.Store implementation this file's
// behavioural tests run against, so both implementations are proven to
// satisfy the exact same contract rather than testing one and assuming
// the other matches (docs/ARCHITECTURE.md §1: "assets/ Store interface +
// filesystem implementation").
func storeFactories(t *testing.T) map[string]func() assets.Store {
	return map[string]func() assets.Store{
		"MemoryStore": assets.NewMemoryStore,
		"FilesystemStore": func() assets.Store {
			return assets.NewFilesystemStore(t.TempDir())
		},
	}
}

func TestStore_GetMissing_ReturnsNotFound(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			_, err := s.Get(context.Background(), "logo.png")
			if !apperr.Is(err, apperr.KindNotFound) {
				t.Fatalf("Get() error = %v, want apperr.KindNotFound", err)
			}
		})
	}
}

func TestStore_PutThenGet_ReturnsSameBytes(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			want := []byte("some image bytes")
			if err := s.Put(ctx, "logo.png", want); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			got, err := s.Get(ctx, "logo.png")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if string(got) != string(want) {
				t.Errorf("Get() = %q, want %q", got, want)
			}
		})
	}
}

func TestStore_PutOverwritesExisting(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			if err := s.Put(ctx, "logo.png", []byte("first")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			if err := s.Put(ctx, "logo.png", []byte("second")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			got, err := s.Get(ctx, "logo.png")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if string(got) != "second" {
				t.Errorf("Get() = %q, want %q", got, "second")
			}
		})
	}
}

func TestStore_Delete_RemovesAsset(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			if err := s.Put(ctx, "logo.png", []byte("data")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			if err := s.Delete(ctx, "logo.png"); err != nil {
				t.Fatalf("Delete() error = %v, want nil", err)
			}
			if _, err := s.Get(ctx, "logo.png"); !apperr.Is(err, apperr.KindNotFound) {
				t.Fatalf("Get() after Delete() error = %v, want apperr.KindNotFound", err)
			}
		})
	}
}

func TestStore_DeleteMissing_ReturnsNotFound(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			if err := s.Delete(context.Background(), "logo.png"); !apperr.Is(err, apperr.KindNotFound) {
				t.Fatalf("Delete() error = %v, want apperr.KindNotFound", err)
			}
		})
	}
}

func TestStore_List_EmptyStoreReturnsNoNames(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			got, err := s.List(context.Background())
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			if len(got) != 0 {
				t.Errorf("List() = %v, want empty", got)
			}
		})
	}
}

func TestStore_List_ReturnsEveryPutName(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			if err := s.Put(ctx, "a.png", []byte("a")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			if err := s.Put(ctx, "b.png", []byte("b")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			got, err := s.List(ctx)
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			want := map[string]bool{"a.png": true, "b.png": true}
			if len(got) != len(want) {
				t.Fatalf("List() = %v, want 2 names", got)
			}
			for _, n := range got {
				if !want[n] {
					t.Errorf("List() contains unexpected name %q", n)
				}
			}
		})
	}
}

func TestStore_InvalidName_ReturnsValidationError(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			for _, badName := range []string{"", "..", "../escape", "sub/dir.png", `sub\dir.png`} {
				if _, err := s.Get(ctx, badName); !apperr.Is(err, apperr.KindValidation) {
					t.Errorf("Get(%q) error = %v, want apperr.KindValidation", badName, err)
				}
				if err := s.Put(ctx, badName, []byte("x")); !apperr.Is(err, apperr.KindValidation) {
					t.Errorf("Put(%q) error = %v, want apperr.KindValidation", badName, err)
				}
			}
		})
	}
}

func TestStore_Deterministic(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			data := []byte("data")
			if err := s.Put(ctx, "logo.png", data); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			first, err := s.Get(ctx, "logo.png")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			second, err := s.Get(ctx, "logo.png")
			if err != nil {
				t.Fatalf("Get() error = %v, want nil", err)
			}
			if string(first) != string(second) {
				t.Errorf("Get() = %q, then %q, want equal", first, second)
			}
		})
	}
}
