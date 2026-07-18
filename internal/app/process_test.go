package app_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/harveysandiego/receiptd/internal/app"
	"github.com/harveysandiego/receiptd/internal/apperr"
	"github.com/harveysandiego/receiptd/internal/queue"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

func TestService_Process_Success_ReturnsNilError(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	j := &queue.Job{PrinterName: "front-desk", Receipt: validReceipt()}

	if err := s.Process(context.Background(), j); err != nil {
		t.Fatalf("Process() error = %v, want nil", err)
	}
}

func TestService_Process_RenderingError_Propagates(t *testing.T) {
	s := app.New(queue.New(queue.NewMemoryStore(), &noopProcessor{}))
	// receipt.Heading is a valid Element (passes Validate) but is not yet
	// supported by render/layout.Build, which only handles receipt.Text —
	// this is the current pipeline's real error path, not a contrived one.
	j := &queue.Job{
		PrinterName: "front-desk",
		Receipt:     receipt.Receipt{Elements: []receipt.Element{receipt.Heading{Content: "unsupported"}}},
	}

	err := s.Process(context.Background(), j)
	if !apperr.Is(err, apperr.KindPermanent) {
		t.Fatalf("Process() error = %v, want apperr.KindPermanent", err)
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
