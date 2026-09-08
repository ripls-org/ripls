// Package admin implements the AdminService RPC interface, which exposes
// dev-mode-only operations for simulation management and database cleanup.
// All RPCs in this package are gated behind a devMode flag and unavailable
// in production.
package admin
