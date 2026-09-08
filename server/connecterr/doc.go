// Package connecterr constructs Connect RPC errors that log internal detail
// server-side without leaking it to clients.
//
// The Internal helper logs the underlying error via the context logger and
// returns a connect.CodeInternal error whose public message is a fixed,
// generic string. Callers should use it instead of
// connect.NewError(connect.CodeInternal, err) so that raw storage,
// third-party SDK, or decoder messages never reach the wire.
package connecterr
