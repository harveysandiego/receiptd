package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/harveysandiego/receiptd/internal/auth"
	"github.com/harveysandiego/receiptd/internal/config"
	"github.com/harveysandiego/receiptd/internal/receipt"
)

// The types below are the CLI's own encoding of receiptd's REST wire
// format (docs/ARCHITECTURE.md §4's /api/v1/preview, /api/v1/print, and
// GET /api/v1/jobs/{id}). They deliberately duplicate internal/api's
// unexported request/response structs field-for-field rather than
// importing them: the CLI depends on the documented HTTP contract, not on
// the server package's Go types, so cmd/receipt has no import of
// internal/api at all.

// printRequest is the wire shape of a POST /api/v1/print request body.
type printRequest struct {
	Printer string          `json:"printer"`
	Receipt receipt.Receipt `json:"receipt"`
}

// printResponse is the wire shape of a successful POST /api/v1/print
// response body.
type printResponse struct {
	JobID string `json:"job_id"`
}

// jobStatusResponse is the wire shape of a successful GET
// /api/v1/jobs/{id} response body.
type jobStatusResponse struct {
	ID          string          `json:"id"`
	PrinterName string          `json:"printer_name"`
	Receipt     receipt.Receipt `json:"receipt"`
	State       string          `json:"state"`
	Attempts    int             `json:"attempts"`
	LastError   string          `json:"last_error"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// errorResponse is the wire shape of a non-2xx API response body.
type errorResponse struct {
	Error string `json:"error"`
}

// apiClient is a minimal HTTP client for receiptd's own REST API: it holds
// only what every request needs (base URL, bearer token, and an
// *http.Client) and turns a non-2xx response into an error. Receipt
// validation and rendering both stay server-side (docs/ARCHITECTURE.md
// §4) — apiClient only serializes requests and deserializes responses, it
// holds no receipt-processing logic of its own.
type apiClient struct {
	baseURL string
	token   string
	http    *http.Client
}

// newAPIClient builds an apiClient from cfg: baseURL derived from
// cfg.Server.Address, and token via the existing auth.ResolveToken. A bind
// address with no host (e.g. ":8080", the documented default) is treated
// as localhost, since the CLI's default use case is a receiptd running on
// the same host — the config schema has no separate client-facing
// address, and adding one is out of scope for this slice.
func newAPIClient(cfg *config.Config) (*apiClient, error) {
	token, err := auth.ResolveToken(cfg.Auth)
	if err != nil {
		return nil, err
	}
	return &apiClient{
		baseURL: apiBaseURL(cfg.Server.Address),
		token:   token,
		http:    http.DefaultClient,
	}, nil
}

func apiBaseURL(address string) string {
	if strings.HasPrefix(address, ":") {
		return "http://localhost" + address
	}
	return "http://" + address
}

// do sends a request built from method, path, and body (nil for none),
// attaching the bearer token when set, and returns the raw response body
// for any 2xx status. A transport failure (e.g. no daemon listening at
// baseURL) and a non-2xx response (decoded via errorResponse when
// possible) are both returned as plain errors ready for cobra to print.
func (c *apiClient) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("connecting to receiptd at %s: %w", c.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from receiptd: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr errorResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != "" {
			return nil, fmt.Errorf("receiptd: %s (%s)", apiErr.Error, resp.Status)
		}
		return nil, fmt.Errorf("receiptd: unexpected response: %s", resp.Status)
	}

	return data, nil
}

// preview calls POST /api/v1/preview with r's JSON encoding as the
// request body — no envelope, matching PreviewHandler's decode — and
// returns the PNG bytes it responds with.
func (c *apiClient) preview(ctx context.Context, r receipt.Receipt) ([]byte, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	return c.do(ctx, http.MethodPost, "/api/v1/preview", body)
}

// print calls POST /api/v1/print with r and printerName wrapped in a
// printRequest, and returns the queued Job's ID from the decoded
// printResponse.
func (c *apiClient) print(ctx context.Context, r receipt.Receipt, printerName string) (string, error) {
	body, err := json.Marshal(printRequest{Printer: printerName, Receipt: r})
	if err != nil {
		return "", err
	}

	data, err := c.do(ctx, http.MethodPost, "/api/v1/print", body)
	if err != nil {
		return "", err
	}

	var resp printResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}
	return resp.JobID, nil
}

// jobStatus calls GET /api/v1/jobs/{id} and returns the decoded
// jobStatusResponse.
func (c *apiClient) jobStatus(ctx context.Context, id string) (*jobStatusResponse, error) {
	data, err := c.do(ctx, http.MethodGet, "/api/v1/jobs/"+id, nil)
	if err != nil {
		return nil, err
	}

	var resp jobStatusResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
