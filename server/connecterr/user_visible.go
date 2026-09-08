package connecterr

import (
	"context"
	"errors"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// UserVisible constructs a Connect error carrying a
// LocalizedErrorDetail so the client can render the message in the
// recipient's locale.
//
// code is a stable, machine-readable identifier (snake_case) that
// the client maps to an ARB key. params provides ICU substitutions
// for the resolved string; pass nil when the message has no
// placeholders. fallbackMessage is the server-side English text
// used as the Connect error's public message — it is what older
// clients (or anything that ignores the detail) display. New
// clients prefer the detail's code+params.
//
// connectCode is the wire-level Connect code; pick the one that
// best categorizes the error (FailedPrecondition, PermissionDenied,
// NotFound, etc.). InvalidArgument and Internal generally have
// their own dedicated helpers, but UserVisible accepts any code.
//
// Callers SHOULD provide a fallbackMessage in English so that
// today's clients (which do not yet read LocalizedErrorDetail)
// still show something readable.
func UserVisible(
	ctx context.Context,
	connectCode connect.Code,
	code, fallbackMessage string,
	params map[string]string,
) error {
	cerr := connect.NewError(connectCode, errors.New(fallbackMessage))
	detail, derr := connect.NewErrorDetail(&api.LocalizedErrorDetail{
		Code:   code,
		Params: params,
	})
	if derr != nil {
		// Attaching a detail should never fail for a well-formed
		// proto message; if it does, log and fall back to the bare
		// Connect error so the caller's contract still holds.
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to build LocalizedErrorDetail",
			"error", derr,
			"code", code,
		)
		return cerr
	}
	cerr.AddDetail(detail)
	return cerr
}
