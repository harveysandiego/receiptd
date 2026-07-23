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

func (s *retryStore) NextPending(_ context.Context) (*Job, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.job == nil || s.job.State != JobPending {
		return nil, nil
	}
	cp := *s.job
	return &cp, nil
}

// ClaimNextPending mirrors what a real Store's atomic claim does — filter
// by printerName, transition to JobRunning, persist via Save (so the
// existing saveCalls/saves bookkeeping stays accurate for any test that
// exercises it) — scoped to retryStore's single-job model. No existing
// test in this file calls ProcessNextForPrinter; this exists so retryStore
// keeps satisfying the Store interface.
func (s *retryStore) ClaimNextPending(ctx context.Context, printerName string) (*Job, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.job == nil || s.job.State != JobPending || s.job.PrinterName != printerName {
		return nil, nil
	}
	claimed := *s.job
	claimed.State = JobRunning
	claimed.UpdatedAt = time.Now()
	if err := s.Save(ctx, &claimed); err != nil {
		return nil, err
	}
	return &claimed, nil
}

func (s *retryStore) Get(_ context.Context, id string) (*Job, error) {
	if s.job == nil || s.job.ID != id {
		return nil, apperr.Wrap(apperr.KindNotFound, "retryStore.Get", errors.New("not found"))
	}
	cp := *s.job
	return &cp, nil
}

func (s *retryStore) EnqueueIdempotent(_ context.Context, _ *Job, _ time.Time) (*Job, bool, error) {
	return nil, false, nil
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

// recordingSleep returns a Queue.sleep stub that records every requested
// duration instead of actually waiting, ignoring ctx entirely — the tests
// that use it are asserting on retry counts and backoff durations, not on
// cancellation behavior (see TestQueue_ProcessNext_RetryBackoff_Interrupted...
// below for that).
func recordingSleep(durations *[]time.Duration) func(context.Context, time.Duration) {
	return func(_ context.Context, d time.Duration) { *durations = append(*durations, d) }
}

// noopSleep is a Queue.sleep stub for tests that need retries to happen
// without delay but don't care what durations were requested.
func noopSleep(context.Context, time.Duration) {}

func TestQueue_ProcessNext_TransientError_RetriesUntilSuccess(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, nil}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

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
	wantSlept := []time.Duration{defaultBaseBackoff}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v", slept, wantSlept)
	}
}

func TestQueue_ProcessNext_SuccessAfterMultipleRetries(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, nil}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

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
	wantSlept := []time.Duration{defaultBaseBackoff, defaultBaseBackoff * 2}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v", slept, wantSlept)
	}
}

func TestQueue_ProcessNext_RetriesExhausted_ResultsInJobFailed(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, transient}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != defaultMaxAttempts {
		t.Errorf("proc.calls = %d, want %d", proc.calls, defaultMaxAttempts)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	if store.job.Attempts != defaultMaxAttempts {
		t.Errorf("job.Attempts = %d, want %d", store.job.Attempts, defaultMaxAttempts)
	}
	if store.job.LastError != transient.Error() {
		t.Errorf("job.LastError = %q, want %q", store.job.LastError, transient.Error())
	}
	if len(slept) != defaultMaxAttempts-1 {
		t.Errorf("len(slept) = %d, want %d (one fewer than maxAttempts: no sleep after the final attempt)", len(slept), defaultMaxAttempts-1)
	}
}

// --- ProcessNextForPrinter shares runClaimedJob with ProcessNext (see
// process.go), so its retry/backoff behavior for an already-claimed Job
// must be identical — these two tests exercise that shared loop through
// the new entry point instead of duplicating every ProcessNext retry case
// above. ---

func TestQueue_ProcessNextForPrinter_TransientError_RetriesUntilSuccess(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, nil}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

	if err := q.ProcessNextForPrinter(context.Background(), "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
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
	wantSlept := []time.Duration{defaultBaseBackoff}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v", slept, wantSlept)
	}
}

func TestQueue_ProcessNextForPrinter_RetriesExhausted_ResultsInJobFailed(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", PrinterName: "front-desk", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, transient}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

	if err := q.ProcessNextForPrinter(context.Background(), "front-desk"); err != nil {
		t.Fatalf("ProcessNextForPrinter() error = %v, want nil", err)
	}
	if proc.calls != defaultMaxAttempts {
		t.Errorf("proc.calls = %d, want %d", proc.calls, defaultMaxAttempts)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	if store.job.Attempts != defaultMaxAttempts {
		t.Errorf("job.Attempts = %d, want %d", store.job.Attempts, defaultMaxAttempts)
	}
	if len(slept) != defaultMaxAttempts-1 {
		t.Errorf("len(slept) = %d, want %d (one fewer than maxAttempts: no sleep after the final attempt)", len(slept), defaultMaxAttempts-1)
	}
}

func TestQueue_ProcessNext_PermanentError_NotRetried(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	permErr := apperr.Wrap(apperr.KindPermanent, "test", errors.New("renderer bug"))
	proc := &scriptedProcessor{errs: []error{permErr}}
	var slept []time.Duration
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

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
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: recordingSleep(&slept)}

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
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: noopSleep}

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
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: noopSleep}

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
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: noopSleep}

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
	q := &Queue{store: store, processor: proc, maxAttempts: defaultMaxAttempts, baseBackoff: defaultBaseBackoff, sleep: noopSleep}

	err := q.ProcessNext(context.Background())
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("ProcessNext() error = %v, want apperr.KindPermanent", err)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1", proc.calls)
	}
}

// --- Configured retry settings (docs/ARCHITECTURE.md §7's
// queue.max_attempts/queue.retry_backoff) must actually drive ProcessNext,
// not the package's own defaultMaxAttempts/defaultBaseBackoff. ---

func TestQueue_ProcessNext_HonoursConfiguredMaxAttempts(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	proc := &scriptedProcessor{errs: []error{transient, transient, transient, transient, transient}}
	var slept []time.Duration
	const configuredMaxAttempts = 5
	const configuredBaseBackoff = 10 * time.Millisecond
	q := NewWithRetry(store, proc, configuredMaxAttempts, configuredBaseBackoff)
	q.sleep = recordingSleep(&slept)

	if err := q.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	if proc.calls != configuredMaxAttempts {
		t.Errorf("proc.calls = %d, want %d (the configured max_attempts, not defaultMaxAttempts=%d)", proc.calls, configuredMaxAttempts, defaultMaxAttempts)
	}
	if store.job.State != JobFailed {
		t.Errorf("job.State = %v, want %v", store.job.State, JobFailed)
	}
	wantSlept := []time.Duration{
		configuredBaseBackoff,
		configuredBaseBackoff * 2,
		configuredBaseBackoff * 4,
		configuredBaseBackoff * 8,
	}
	if !reflect.DeepEqual(slept, wantSlept) {
		t.Errorf("slept = %v, want %v (the configured retry_backoff, not defaultBaseBackoff=%v)", slept, wantSlept, defaultBaseBackoff)
	}
}

func TestNew_UsesDefaultRetrySettings(t *testing.T) {
	q := New(NewMemoryStore(), &scriptedProcessor{})
	if q.maxAttempts != defaultMaxAttempts {
		t.Errorf("maxAttempts = %d, want %d", q.maxAttempts, defaultMaxAttempts)
	}
	if q.baseBackoff != defaultBaseBackoff {
		t.Errorf("baseBackoff = %v, want %v", q.baseBackoff, defaultBaseBackoff)
	}
}

func TestNewWithRetry_UsesGivenRetrySettings(t *testing.T) {
	q := NewWithRetry(NewMemoryStore(), &scriptedProcessor{}, 7, 3*time.Second)
	if q.maxAttempts != 7 {
		t.Errorf("maxAttempts = %d, want 7", q.maxAttempts)
	}
	if q.baseBackoff != 3*time.Second {
		t.Errorf("baseBackoff = %v, want %v", q.baseBackoff, 3*time.Second)
	}
}

// --- A retry backoff wait must be interruptible by context cancellation
// (docs/ARCHITECTURE.md's shutdown story assumes ProcessNext never blocks
// past a cancelled ctx). These tests use the real sleepCtx, not a stubbed
// sleep, since that's exactly the behavior under test. ---

func TestSleepCtx_WaitsTheFullDurationWhenNotCancelled(t *testing.T) {
	const d = 30 * time.Millisecond
	start := time.Now()
	sleepCtx(context.Background(), d)
	if elapsed := time.Since(start); elapsed < d {
		t.Errorf("sleepCtx returned after %v, want at least %v", elapsed, d)
	}
}

func TestSleepCtx_ReturnsEarlyWhenContextAlreadyCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	const generous = 5 * time.Second
	start := time.Now()
	sleepCtx(ctx, generous)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("sleepCtx took %v for an already-cancelled context, want it to return almost immediately (well under the %v duration)", elapsed, generous)
	}
}

func TestSleepCtx_ReturnsEarlyWhenContextCancelledMidWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	const generous = 5 * time.Second
	start := time.Now()
	sleepCtx(ctx, generous)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Errorf("sleepCtx took %v after mid-wait cancellation, want it to return shortly after cancel (well under the %v duration)", elapsed, generous)
	}
}

// TestQueue_ProcessNext_RetryBackoff_InterruptedByContextCancellation pins
// docs/adr/0018-graceful-shutdown.md's Decision: a backoff wait cut short
// by ctx cancellation must leave the Job JobRunning, not JobFailed —
// marking it Failed would misrepresent a shutdown as the Job's own retry
// policy having been exhausted. Before ADR-0018 this asserted JobFailed;
// that was the behavior being changed, not a regression.
func TestQueue_ProcessNext_RetryBackoff_InterruptedByContextCancellation(t *testing.T) {
	store := &retryStore{job: &Job{ID: "job-1", State: JobPending}}
	transient := apperr.Wrap(apperr.KindTransient, "test", errors.New("printer offline"))
	// Every attempt fails transiently, so without cancellation ProcessNext
	// would keep retrying up to configuredMaxAttempts.
	proc := &scriptedProcessor{errs: []error{transient, transient, transient, transient, transient}}
	const configuredBaseBackoff = 300 * time.Millisecond
	q := NewWithRetry(store, proc, 5, configuredBaseBackoff)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if err := q.ProcessNext(ctx); err != nil {
		t.Fatalf("ProcessNext() error = %v, want nil", err)
	}
	elapsed := time.Since(start)

	if elapsed >= configuredBaseBackoff {
		t.Errorf("ProcessNext() took %v, want well under the %v backoff — cancellation should have cut the wait short", elapsed, configuredBaseBackoff)
	}
	if proc.calls != 1 {
		t.Errorf("proc.calls = %d, want 1 (retrying must stop once ctx is cancelled during the backoff wait)", proc.calls)
	}
	if store.job.State != JobRunning {
		t.Errorf("job.State = %v, want %v (ADR-0018: a mid-backoff cancellation leaves the Job non-terminal, it is not marked Failed)", store.job.State, JobRunning)
	}
}
