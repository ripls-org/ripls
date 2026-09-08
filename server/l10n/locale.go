package l10n

import (
	"context"
	"slices"
	"strings"

	"golang.org/x/text/language"
)

// localeCtxKey is the unexported context-key type for an explicit
// locale set by an RPC handler (e.g. when a request carries an
// explicit locale parameter). Avoids string-key collisions per Go
// convention.
type localeCtxKey struct{}

// acceptLanguageCtxKey carries the raw Accept-Language header value
// captured by the HTTP middleware. Kept distinct from localeCtxKey
// so the resolver can apply the documented priority order without
// guessing which key has higher precedence.
type acceptLanguageCtxKey struct{}

// preferredLanguageCtxKey carries the authenticated user's stored
// preferred_language. Populated from the user record after auth
// (#1904); when unset, the resolver falls through to the default.
type preferredLanguageCtxKey struct{}

// WithLocale returns a child context with an explicit locale tag.
// Used by handlers that accept a locale RPC argument. The tag wins
// over Accept-Language and the stored user preference.
func WithLocale(ctx context.Context, tag string) context.Context {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ctx
	}
	return context.WithValue(ctx, localeCtxKey{}, tag)
}

// WithAcceptLanguage returns a child context carrying the
// Accept-Language header value of the inbound request. The HTTP
// middleware sets this for every authenticated RPC; the resolver
// uses it second-priority, after an explicit WithLocale value.
func WithAcceptLanguage(ctx context.Context, header string) context.Context {
	header = strings.TrimSpace(header)
	if header == "" {
		return ctx
	}
	return context.WithValue(ctx, acceptLanguageCtxKey{}, header)
}

// WithPreferredLanguage returns a child context carrying the
// authenticated user's stored preferred_language. Used by the auth
// layer once the user record has been read; falls through to the
// default if the user has no recorded preference.
func WithPreferredLanguage(ctx context.Context, tag string) context.Context {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ctx
	}
	return context.WithValue(ctx, preferredLanguageCtxKey{}, tag)
}

// HasAcceptLanguage reports whether the AcceptLanguage middleware
// captured an Accept-Language header value into ctx. Used by the
// PreferredLanguage interceptor to skip a redundant user-record
// lookup when the request already carries a higher-priority signal.
func HasAcceptLanguage(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, ok := ctx.Value(acceptLanguageCtxKey{}).(string)
	return ok && v != ""
}

// LocaleFromContext resolves the locale for the current request. The
// returned tag is normalized: it always equals one of Supported() or
// DefaultTag. Resolution priority:
//
//  1. Explicit WithLocale value (RPC handler set it directly).
//  2. Accept-Language header value (set by middleware).
//  3. Stored User.preferred_language (set by the PreferredLanguage
//     Connect interceptor after auth).
//  4. DefaultTag (English).
func LocaleFromContext(ctx context.Context) language.Tag {
	if ctx != nil {
		if v, ok := ctx.Value(localeCtxKey{}).(string); ok && v != "" {
			return Normalize(v)
		}
		if v, ok := ctx.Value(acceptLanguageCtxKey{}).(string); ok && v != "" {
			return Normalize(firstAcceptLanguageTag(v))
		}
		if v, ok := ctx.Value(preferredLanguageCtxKey{}).(string); ok && v != "" {
			return Normalize(v)
		}
	}
	return DefaultTag
}

// Normalize folds the input to the canonical supported tag.
//
// Rules:
//   - Empty input → DefaultTag.
//   - Region subtags are stripped (`en-US` → `en`).
//   - Case is folded (`ES`, `Es-mx` → `es`).
//   - Unsupported tags fall back to DefaultTag.
//
// The returned tag is guaranteed to be a member of Supported().
func Normalize(tag string) language.Tag {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return DefaultTag
	}

	parsed, err := language.Parse(tag)
	if err != nil {
		return DefaultTag
	}
	base, _ := parsed.Base()
	baseTag, err := language.Parse(base.String())
	if err != nil {
		return DefaultTag
	}
	if slices.Contains(supportedTags, baseTag) {
		return baseTag
	}
	return DefaultTag
}

// DisplayName returns the English-language name of a supported tag
// ("English", "Spanish"). Used in AI prompts where the caller asks
// the model to respond in a specific language and wants a
// deterministic, unambiguous name.
//
// Returns "English" for any tag not in [Supported] — this is a
// last-resort guard; in practice callers should pre-normalize via
// [Normalize] before calling.
func DisplayName(tag language.Tag) string {
	switch tag {
	case language.English:
		return "English"
	case language.Spanish:
		return "Spanish"
	default:
		return "English"
	}
}

// firstAcceptLanguageTag returns the first language tag of an
// Accept-Language header value, stripping any `q=` weight. Uses a
// simple "first wins" parser; full RFC 7231 quality-value ordering
// is a deferred follow-up (see docs/server/l10n.md).
func firstAcceptLanguageTag(header string) string {
	for raw := range strings.SplitSeq(header, ",") {
		// Drop any q= suffix and surrounding whitespace.
		if idx := strings.IndexByte(raw, ';'); idx >= 0 {
			raw = raw[:idx]
		}
		raw = strings.TrimSpace(raw)
		if raw != "" {
			return raw
		}
	}
	return ""
}
