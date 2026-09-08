package email

import (
	"context"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/l10n"
)

// localeForRecipient resolves the language tag to render an email
// in. Email locale resolution differs from the general RPC priority
// in [l10n.LocaleFromContext]: a recipient's stored
// preferred_language wins over the Accept-Language header of the
// request that triggered the send. Rationale: the email is read by
// the recipient, who may not be the requester (password reset
// triggered from a different device) and who chose their stored
// language deliberately.
//
// Priority:
//  1. recipientPreferred when non-empty (normalized; an unsupported
//     stored tag folds to l10n.DefaultTag).
//  2. l10n.LocaleFromContext(ctx) — folds any RPC-supplied locale
//     and the request's Accept-Language header. For flows with no
//     recipient User row (waitlist welcome) this is the only signal.
//  3. l10n.DefaultTag (covered by LocaleFromContext's own fallback).
//
// The returned tag is always a member of l10n.Supported().
func localeForRecipient(ctx context.Context, recipientPreferred string) language.Tag {
	if recipientPreferred != "" {
		return l10n.Normalize(recipientPreferred)
	}
	return l10n.LocaleFromContext(ctx)
}
