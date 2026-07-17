// Package app is Receiptd's service/use-case layer. Service wires
// layout, canvas, escpos, printer, queue, templates, providers, and
// assets together, and is the only package api and webui call into for
// business logic. Service also implements queue.Processor structurally —
// queue must never import app back; see docs/ARCHITECTURE.md §11's
// dependency-graph note for why this is worth a deliberate watch during
// review.
package app
