package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"go.etcd.io/bbolt"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// jobsBucket holds one key/value entry per Job: the Job's ID as the key,
// its JSON encoding as the value.
var jobsBucket = []byte("jobs")

// boltStore is a Store backed by a bbolt file on disk, so Job state
// survives a process restart. See NewMemoryStore for the non-persistent
// alternative.
type boltStore struct {
	db *bbolt.DB
}

// NewBoltStore opens (creating if necessary) a bbolt database at path and
// returns a Store backed by it.
func NewBoltStore(path string) (Store, error) {
	db, err := bbolt.Open(path, 0o600, nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.KindPermanent, "queue.NewBoltStore", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(jobsBucket)
		return err
	})
	if err != nil {
		_ = db.Close()
		return nil, apperr.Wrap(apperr.KindPermanent, "queue.NewBoltStore", err)
	}

	return &boltStore{db: db}, nil
}

// Close releases the underlying bbolt file. It isn't part of the Store
// interface (see docs/ARCHITECTURE.md §2), since NewMemoryStore has
// nothing to close; a caller holding the concrete type NewBoltStore
// returns can still reach it with a type assertion.
func (s *boltStore) Close() error {
	return s.db.Close()
}

func (s *boltStore) Save(_ context.Context, j *Job) error {
	data, err := json.Marshal(j)
	if err != nil {
		return apperr.Wrap(apperr.KindPermanent, "queue.Store.Save", err)
	}

	err = s.db.Update(func(tx *bbolt.Tx) error {
		return tx.Bucket(jobsBucket).Put([]byte(j.ID), data)
	})
	if err != nil {
		return apperr.Wrap(apperr.KindPermanent, "queue.Store.Save", err)
	}
	return nil
}

func (s *boltStore) Get(_ context.Context, id string) (*Job, error) {
	j := new(Job)
	err := s.db.View(func(tx *bbolt.Tx) error {
		data := tx.Bucket(jobsBucket).Get([]byte(id))
		if data == nil {
			return apperr.Wrap(apperr.KindNotFound, "queue.Store.Get", fmt.Errorf("job %q not found", id))
		}
		return json.Unmarshal(data, j)
	})
	if err != nil {
		if apperr.Is(err, apperr.KindNotFound) {
			return nil, err
		}
		return nil, apperr.Wrap(apperr.KindPermanent, "queue.Store.Get", err)
	}
	return j, nil
}

// List returns every Job in the Store, ordered by ID. bbolt iterates a
// bucket's keys in ascending byte order, and Job IDs are hex strings, so
// this ordering falls out of the underlying B+tree for free — no
// additional sort is needed (contrast memoryStore.List, which sorts
// explicitly because a Go map has no iteration order).
func (s *boltStore) List(_ context.Context, _ Filter) ([]*Job, error) {
	var out []*Job
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(jobsBucket).ForEach(func(_, data []byte) error {
			j := new(Job)
			if err := json.Unmarshal(data, j); err != nil {
				return err
			}
			out = append(out, j)
			return nil
		})
	})
	if err != nil {
		return nil, apperr.Wrap(apperr.KindPermanent, "queue.Store.List", err)
	}
	return out, nil
}
