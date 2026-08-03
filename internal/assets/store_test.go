package assets_test

import (
	"context"
	"testing"
	"time"

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
			for _, info := range got {
				if !want[info.Name] {
					t.Errorf("List() contains unexpected name %q", info.Name)
				}
			}
		})
	}
}

func TestStore_List_ReportsSizeAndModTime(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			data := []byte("twelve bytes")
			if err := s.Put(ctx, "logo.png", data); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			got, err := s.List(ctx)
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}
			if len(got) != 1 {
				t.Fatalf("List() = %v, want 1 entry", got)
			}
			if got[0].Size != int64(len(data)) {
				t.Errorf("List()[0].Size = %d, want %d", got[0].Size, len(data))
			}
			if got[0].ModTime.IsZero() {
				t.Errorf("List()[0].ModTime is zero, want the time the asset was stored")
			}
		})
	}
}

// Both implementations must report a fresh ModTime after an overwrite,
// not the original Put's — the Web UI shows it, and a stale value would
// claim an asset hadn't changed when it had.
func TestStore_List_OverwriteAdvancesModTime(t *testing.T) {
	for name, newStore := range storeFactories(t) {
		t.Run(name, func(t *testing.T) {
			s := newStore()
			ctx := context.Background()
			if err := s.Put(ctx, "logo.png", []byte("first")); err != nil {
				t.Fatalf("Put() error = %v, want nil", err)
			}
			before, err := s.List(ctx)
			if err != nil {
				t.Fatalf("List() error = %v, want nil", err)
			}

			// Filesystem mod times have coarse resolution on some platforms;
			// retry the overwrite with backoff until ModTime actually
			// advances rather than trusting a single fixed sleep to clear
			// whatever tick granularity the underlying filesystem has.
			var after []assets.Info
			sleep := 10 * time.Millisecond
			for i := 0; i < 10; i++ {
				time.Sleep(sleep)
				if err := s.Put(ctx, "logo.png", []byte("second")); err != nil {
					t.Fatalf("Put() error = %v, want nil", err)
				}
				after, err = s.List(ctx)
				if err != nil {
					t.Fatalf("List() error = %v, want nil", err)
				}
				if after[0].ModTime.After(before[0].ModTime) {
					break
				}
				sleep *= 2
			}
			if !after[0].ModTime.After(before[0].ModTime) {
				t.Errorf("ModTime after overwrite = %v, want later than %v", after[0].ModTime, before[0].ModTime)
			}
			if after[0].Size != int64(len("second")) {
				t.Errorf("Size after overwrite = %d, want %d", after[0].Size, len("second"))
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
