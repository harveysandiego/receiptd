package queue

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/apperr"
)

// This file is package queue (not queue_test, unlike the rest of this
// package's tests) solely so it can set Queue.sleep directly: ProcessNext's
// retries use real exponential backoff in production, and driving that
// through a stubbed, recording sleep function is the only way to assert on
// retry/backoff behavior without these tests actually blocking for several
// seconds per case.

// retryStore is a Store test double for ProcessNext's retry tests. It
// tracks every persisted Save so a test can assert on Attempts/State/
// LastError/UpdatedAt at each step, and can be configured to fail List or a
// specific Save call.
type retryStore struct {
	job          *Job
	listErr      error
	saveErr      error
	failSaveCall int // 1-indexed Save() call to fail; 0 means every call fails if saveErr != nil
	saveCalls    int
	saves        []Job
}

func (s *retryStore) List(_ context.Context, _ Filter) ([]*Job, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.job == nil {
		return nil, nil
	}
	return []*Job{s.job}, nil
}

func (s *retryStore) Save(_ context.Context, j *Job) error {
	s.saveCalls++
	if s.saveErr != nil && (s.failSaveCall == 0 || s.saveCalls == s.failSaveCall) {
		return s.saveErr
	}
	cp := *j
	s.saves = append(s.saves, cp)
	s.job = &cp
	return nil
}

func (s *retryStore) Get(_ context.Context, id string) (*Job, error) {
	if s.job == nil || s.job.ID != id {
		return nil, apperr.Wrap(apperr.KindNotFound, "retryStore.Get", errors.New("not found"))
	}
	cp := *s.job
	return &cp, nil
}

// scriptedProcessor returns each error in errs in sequence, one per call,
// repeating the last entry for any call beyond len(errs). A nil entry means
// success.
type scriptedProcessor struct {
	errs  []error
	calls int
}

func (p *scriptedProcessor) Process(_ context.Context, _ *Job) error {
	i := p.calls
	if i >= len(p.errs) {
		i = len(p.errs) - 1
	}
	p.calls++
	return p.errs[i]
}

func recordingSleep(durations *[]time.Duration) func(time.Duration) {
	return func(d time.Duration) { *durations = append(*durations, d) }
}

func TestQueue_ProcessNext_TransientError_RetriesUntilSuccess(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, nil}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 2 {
		t.Errorf("proc.calls = %d, want 2", proc.calls)
	}
	if store.job.State != JobDone {
		t.Errorf("job.State = %v, want %v", store.job.State, JobDone)
	}
	if store.job.Attempts != 2 {
		t.Errorf("job.Attempts = %d, want 2", store.job.Attempts)
	}
	wantSlept := []time.Duration{baseBackoff}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v", slept, wantSlept)
	}
}

func TestQueue_ProcessNext_SuccessAfterMultipleRetries(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, nil}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 3 {
		t.Errorf("proc.calls = %d, want 3", proc.calls)
	}
	if store.job.State != JobDone {
		t.Errorf("job.State = %v, want %v", store.job.State, JobDone)
	}
	if store.job.Attempts != 3 {
		t.Errorf("job.Attempts = %d, want 3", store.job.Attempts)
	}
	wantSlept := []time.Duration{baseBackoff, baseBackoff * 2}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v", slept, wantSlept)
	}
}

func TestQueue_ProcessNext_RetriesExhausted_ResultsInJobFailed(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, transient}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != maxAttempts {
		t.Errorf("proc.calls = %d, want %d", proc.calls, maxAttempts)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	if store.job.Attempts != maxAttempts {
		t.Errorf("job.Attempts = %d, want %d", store.job.Attempts, maxAttempts)
	}
	if store.job.LastError != transient.Error() {
		t.Errorf("job.LastError = %q, want %q", store.job.LastError, transient.Error())
	}
	if len(slept) != maxAttempts-1 {
		t.Errorf("len(slept) = %d, want %d (one fewer than maxAttempts: no sleep after the final attempt)", len(slept), maxAttempts-1)
	}
}

func TestQueue_ProcessNext_PermanentError_NotRetried(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	permErr := apperr.Wrap(apperr.KindPermanent, "test", errors.New("renderer bug"))
	proc := &scriptedProcessor{errs: []error{permErr}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1 (a permanent error must not be retried)", proc.calls)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	if len(slept) != 0 {
		t.Errorf("slept = %v, want no sleeps", slept)
	}
}

func TestQueue_ProcessNext_ValidationError_NotRetried(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	valErr := apperr.Wrap(apperr.KindValidation, "test", errors.New("bad receipt"))
	proc := &scriptedProcessor{errs: []error{valErr}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1 (a validation error must not be retried)", proc.calls)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	if len(slept) != 0 {
		t.Errorf("slept = %v, want no sleeps", slept)
	}
}

func TestQueue_ProcessNext_UpdatesTimestampsAcrossRetries(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending, UpdatedAt: time.Now().Add(-time.Hour)}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, nil}}
	q := &Queue{store: store, processor: proc, sleep: func(time.Duration) {}}

	before := time.Now()
	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}

	if len(store.saves) != 2 {
		t.Fatalf("len(store.saves) = %d, want 2 (Running persist, final persist)", len(store.saves))
	}
	if store.saves[0].UpdatedAt.Before(before) {
		t.Errorf("saves[0] (Running).UpdatedAt = %v, want at or after %v", store.saves[0].UpdatedAt, before)
	}
	if store.saves[1].UpdatedAt.Before(store.saves[0].UpdatedAt) {
		t.Errorf("saves[1] (final).UpdatedAt = %v, want at or after saves[0].UpdatedAt = %v", store.saves[1].UpdatedAt, store.saves[0].UpdatedAt)
	}
}

func TestQueue_ProcessNext_ListErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "retryStore.List", errors.New("disk error"))
	store := &retryStore{listErr: wantErr}
	proc := &scriptedProcessor{}
	q := &Queue{store: store, processor: proc, sleep: func(time.Duration) {}}

	err := q.ProcessNext(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ProcessNext() error = %v, want apperr.KindPermanent", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0", proc.calls)
	}
}

func TestQueue_ProcessNext_SaveRunningStateErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "retryStore.Save", errors.New("disk full"))
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}, saveErr: wantErr}
	proc := &scriptedProcessor{}
	q := &Queue{store: store, processor: proc, sleep: func(time.Duration) {}}

	err := q.ProcessNext(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ProcessNext() error = %v, want apperr.KindPermanent", err)
	}
	if proc.calls != 0 {
		t.Errorf("proc.calls = %d, want 0 (the Processor must not run if the Running transition can't be persisted)", proc.calls)
	}
}

func TestQueue_ProcessNext_SaveFinalStateErrorPropagates(t *testing.T) {
	wantErr := apperr.Wrap(apperr.KindPermanent, "retryStore.Save", errors.New("disk full"))
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}, saveErr: wantErr, failSaveCall: 2}
	proc := &scriptedProcessor{errs: []error{nil}}
	q := &Queue{store: store, processor: proc, sleep: func(time.Duration) {}}

	err := q.ProcessNext(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ProcessNext() error = %v, want apperr.KindPermanent", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1", proc.calls)
	}
}
