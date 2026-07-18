package app_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// countingWriter is an io.Writer test double that records how many times
// Write was called and everything written to it, or returns writeErr
// (without recording anything) if set.
type countingWriter struct {
	bytes.Buffer
	calls    int
	writeErr error
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return w.Buffer.Write(p)
}

func TestService_Process_Success_ReturnsNilError(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
}

func TestService_Process_UnconfiguredLogSink_DoesNotPanic(t *testing.T) {
	// A freshly constructed Service (app.New, with LogSink never assigned)
	// must behave exactly as before LogSink existed: Process succeeds
	// without a caller having to know to configure a sink first.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
}

func TestService_Process_RenderingError_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sink := &countingWriter{}
	s.LogSink = sink
	// receipt.Divider is a valid Element (passes Validate) but is not yet
	// supported by render/layout.Build, which only handles receipt.Text and
	// receipt.Heading — this is the current pipeline's real error path, not
	// a contrived one.
	j := &queue.Job{
		PrinterName: "front-desk",
		Receipt:     receipt.Receipt{Elements: []receipt.Element{receipt.Divider{Style: "solid"}}},
	}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Process() error = %v, want apperr.KindPermanent", err)
	}
	if sink.calls != 0 {
		t.Errorf("LogSink.Write was called %d times, want 0 (a rendering failure must never reach the sink)", sink.calls)
	}
}

func TestService_Process_UnknownPrinterName_StillSucceeds(t *testing.T) {
	// Process performs no printer resolution or communication in this
	// slice: a PrinterName with no configured printer behind it must
	// still render successfully, since nothing looks it up yet.
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	j := &queue.Job{PrinterName: "does-not-exist", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil (no printer should ever be contacted)", err)
	}
}

func TestService_Process_DoesNotMutateJob(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	created := time.Now().Add(-time.Hour)
	j := &queue.Job{
		ID:          "fixed-id",
		PrinterName: "front-desk",
		Receipt:     validReceipt(),
		State:       queue.JobRunning,
		Attempts:    1,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	before := *j

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if !reflect.DeepEqual(*j, before) {
		t.Errorf("Process() mutated the Job: got %+v, want unchanged %+v", *j, before)
	}
}

func TestService_Process_WritesRenderedOutputToLogSink(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sink := &countingWriter{}
	s.LogSink = sink
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if got := sink.Bytes(); !bytes.HasPrefix(got, pngSignature) {
		t.Errorf("LogSink received %d bytes not starting with the PNG signature: %x", len(got), got)
	}
}

func TestService_Process_WritesDistinctOutputPerReceipt(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))

	shortSink := &countingWriter{}
	s.LogSink = shortSink
	short := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hi"}}}}
	if err := s.Process(context.Background(), short); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	longSink := &countingWriter{}
	s.LogSink = longSink
	long := &queue.Job{PrinterName: "front-desk", Receipt: receipt.Receipt{Elements: []receipt.Element{receipt.Text{Content: "hello world"}}}}
	if err := s.Process(context.Background(), long); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if bytes.Equal(shortSink.Bytes(), longSink.Bytes()) {
		t.Error("Process() wrote identical output for different Receipts, want each Job's own rendered content to reach the sink")
	}
}

func TestService_Process_InvokesLogSinkExactlyOnce(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	sink := &countingWriter{}
	s.LogSink = sink
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}

	if sink.calls != 1 {
		t.Errorf("LogSink.Write was called %d times, want exactly 1", sink.calls)
	}
}

func TestService_Process_LogSinkWriteError_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	writeErr := errors.New("disk full")
	s.LogSink = &countingWriter{writeErr: writeErr}
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Process() error = %v, want apperr.KindPermanent", err)
	}
	if !errors.Is(err, writeErr) {
		t.Errorf("Process() error = %v, want it to wrap %v", err, writeErr)
	}
}

func TestService_Process_LogSinkWriteError_DoesNotMutateJob(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	s.LogSink = &countingWriter{writeErr: errors.New("disk full")}
	created := time.Now().Add(-time.Hour)
	j := &queue.Job{
		ID:          "fixed-id",
		PrinterName: "front-desk",
		Receipt:     validReceipt(),
		State:       queue.JobRunning,
		Attempts:    1,
		CreatedAt:   created,
		UpdatedAt:   created,
	}
	before := *j

	_ = s.Process(context.Background(), j)

	if !reflect.DeepEqual(*j, before) {
		t.Errorf("Process() mutated the Job on a sink error: got %+v, want unchanged %+v", *j, before)
	}
}
