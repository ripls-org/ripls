package connecterr

import (
	"context"
	"errors"
	"runtime/debug"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// publicInternalMessage is the fixed public Error() string returned for every
// CodeInternal error built by Internal. Per-call detail belongs in the log
// line, not the wire response.
const publicInternalMessage = "internal server error"

// internalErrorCode is the LocalizedErrorDetail code attached to every
// connect.CodeInternal error built by Internal. The client maps it to
// rpcErrorInternal in the recipient's locale. The wire-level public
// message stays the fixed English `publicInternalMessage` so older
// clients without LocalizedErrorDetail handling still display readable
// text.
const internalErrorCode = "internal_error_generic"

// Internal logs err at Error level using the context logger and returns a
// connect.CodeInternal error whose public message is a fixed generic string.
// The original err is never serialized to the wire. A LocalizedErrorDetail
// with code `internal_error_generic` is attached so locale-aware clients
// render the message in the recipient's language.
//
// op is the short operation name recorded under the "operation" field
// (matches the field-name convention in docs/server/observability.md).
// Trailing kv pairs are forwarded to the structured log as additional fields.
//
// Calling Internal with a nil err is a programming bug: it logs a Warn entry
// with a stack trace so the offending site is findable, then still returns a
// valid CodeInternal error so the caller's contract holds.
func Internal(ctx context.Context, op string, err error, kv ...any) error {
	logger := logging.LoggerWithContext(ctx)
	if err == nil {
		logger.WarnContext(ctx, "connecterr.Internal called with nil error",
			"operation", op,
			"stack", string(debug.Stack()),
		)
		return newInternalError()
	}
	args := make([]any, 0, 2+len(kv))
	args = append(args, "error", err)
	args = append(args, kv...)
	logger.ErrorContext(ctx, op, args...)
	return newInternalError()
}

// newInternalError builds the fixed-shape CodeInternal error with a
// LocalizedErrorDetail attached. Detail-attachment failures fall back
// to a bare CodeInternal error so the caller's contract still holds.
func newInternalError() error {
	cerr := connect.NewError(connect.CodeInternal, errors.New(publicInternalMessage))
	detail, derr := connect.NewErrorDetail(&api.LocalizedErrorDetail{
		Code: internalErrorCode,
	})
	if derr == nil {
		cerr.AddDetail(detail)
	}
	return cerr
}
