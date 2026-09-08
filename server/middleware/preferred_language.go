package middleware

import (
	"context"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// PreferredLanguageInterceptor populates tier-3 of the
// [l10n.LocaleFromContext] priority chain — the authenticated user's
// stored `preferred_language` — for callers that don't send an
// Accept-Language header.
//
// The Flutter client always attaches Accept-Language on every RPC
// (see app/lib/core/utils/rpc_utils.dart), so this interceptor is a
// no-op for the app: tier 2 is set and tier 3 would lose the
// priority race anyway. The interceptor exists to cover non-Flutter
// callers — internal automation, future native clients, scripted
// access — so a user with `preferred_language="es"` and no header
// still sees Spanish output.
//
// The interceptor skips the storage lookup whenever a higher-priority
// signal is already in ctx: it only runs when the request reached
// the server without Accept-Language, and on every authenticated
// RPC. That keeps the cost at zero for the common path.
//
// Install after the authn middleware so [auth.GetAuthInfo] returns
// the resolved user; install via [connect.WithInterceptors] inside
// the common Connect handler options.
func PreferredLanguageInterceptor(store *storage.ProtoSQLStorage) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Accept-Language already wins the priority chain;
			// skip the storage lookup.
			if l10n.HasAcceptLanguage(ctx) {
				return next(ctx, req)
			}
			info, ok := auth.GetAuthInfo(ctx)
			if !ok || info.UserID == "" {
				return next(ctx, req)
			}
			user := &models.User{}
			if err := store.GetByID(ctx, info.UserID, user); err != nil {
				// Non-fatal: the user-record lookup is a best-
				// effort tier of the locale chain. The default
				// locale takes over if this fails.
				logging.LoggerWithContext(ctx).WarnContext(ctx,
					"preferred-language lookup failed; falling back to default locale",
					"operation", "PreferredLanguageInterceptor",
					"user_id", info.UserID,
					"error", err,
				)
				return next(ctx, req)
			}
			if pref := user.GetPreferredLanguage(); pref != "" {
				ctx = l10n.WithPreferredLanguage(ctx, pref)
			}
			return next(ctx, req)
		})
	}
}
