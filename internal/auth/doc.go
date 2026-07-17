// Package auth provides Receiptd's bearer-token (API/CLI) and basic-auth
// (browser) middleware, sharing one underlying token check. It is kept
// independent of both api and webui — both of which depend on it — so
// that neither of those packages needs to depend on the other. Wired in
// from Milestone 2 onward: the API never exists unsecured. See
// docs/ARCHITECTURE.md §1 and §10.
package auth
