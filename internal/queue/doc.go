// Package queue implements Receiptd's asynchronous, persistent print job
// queue: Job, JobState, the Store interface (with bbolt-backed and
// in-memory implementations), and Queue itself, which retries only
// apperr.KindTransient failures with bounded exponential backoff.
//
// queue must never import app: app.Service implements the Processor
// interface defined here structurally, so the dependency only ever
// points one way. See docs/ARCHITECTURE.md §6 and §11, and
// docs/adr/0003-print-queue.md.
package queue
