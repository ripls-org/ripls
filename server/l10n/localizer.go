package l10n

import (
	"context"
	"maps"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/logging"
)

// Localizer renders messages for a single locale. It wraps
// go-i18n's *i18n.Localizer and adds the project-specific
// behaviors documented in docs/server/l10n.md:
//
//   - Missing keys fall back to the DefaultTag rendering, never
//     panic, never bubble up.
//   - The first miss for any (locale, key) pair emits a single
//     rate-limited Warn log line via
//     logging.LoggerWithContext(ctx); subsequent misses for the
//     same pair are silent for the lifetime of the process.
//
// Construct via NewLocalizer or NewLocalizerForContext. The zero
// value is not usable.
type Localizer struct {
	tag   language.Tag
	inner *i18n.Localizer
}

// NewLocalizer returns a Localizer bound to tag. tag is normalized
// before use, so callers may pass a raw user-supplied string. The
// returned Localizer always renders something: an unsupported tag
// falls through to DefaultTag.
func NewLocalizer(tag language.Tag) (*Localizer, error) {
	b, err := Bundle()
	if err != nil {
		return nil, err
	}
	t := Normalize(tag.String())
	return &Localizer{
		tag:   t,
		inner: i18n.NewLocalizer(b, t.String(), DefaultTag.String()),
	}, nil
}

// NewLocalizerForContext returns a Localizer for the locale resolved
// by LocaleFromContext(ctx). This is the common entry point for RPC
// handlers and pubsub subscribers.
func NewLocalizerForContext(ctx context.Context) (*Localizer, error) {
	return NewLocalizer(LocaleFromContext(ctx))
}

// Tag reports the normalized language tag this Localizer renders
// for.
func (l *Localizer) Tag() language.Tag {
	return l.tag
}

// T renders the message keyed by id with the given template data.
// On any rendering failure — missing key, malformed template,
// missing required placeholder — the call falls back to the
// English string and logs a rate-limited warning. T never returns
// an error: a server l10n bug must not break the user-visible
// surface that triggered the call.
//
// data is the template substitution map; nil is acceptable for
// keys with no placeholders.
//
// ctx is used only for logging; the resolved locale is fixed at
// construction time.
func (l *Localizer) T(ctx context.Context, id string, data map[string]any) string {
	if l == nil || l.inner == nil {
		return id
	}
	cfg := &i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
	}
	out, err := l.inner.Localize(cfg)
	if err == nil {
		return out
	}
	logMissing(ctx, l.tag, id, err)
	// Fall back to the message id itself when even the default
	// locale has no entry. Renders something visible (the key) so
	// operators notice in screenshots / push logs, rather than an
	// empty string in a notification body.
	return id
}

// TCount renders a plural-bearing message keyed by id, selecting the CLDR
// plural form for count in the localizer's locale (go-i18n applies the
// per-locale one/other/… rules). count is also exposed to the template as
// {{.Count}} unless data already supplies it, so the common case
// ("{{.Count}} items") needs no explicit param. Like T, TCount never
// returns an error: on any failure it falls back to English and then to
// the key, logging a rate-limited warning.
//
// data is copied before Count is injected, so the caller's map is never
// mutated; nil is acceptable.
func (l *Localizer) TCount(ctx context.Context, id string, count int, data map[string]any) string {
	if l == nil || l.inner == nil {
		return id
	}
	merged := make(map[string]any, len(data)+1)
	maps.Copy(merged, data)
	if _, ok := merged["Count"]; !ok {
		merged["Count"] = count
	}
	cfg := &i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: merged,
		PluralCount:  count,
	}
	out, err := l.inner.Localize(cfg)
	if err == nil {
		return out
	}
	logMissing(ctx, l.tag, id, err)
	return id
}

// missLog deduplicates missing-key warnings per (locale, key).
// Each pair logs at most once per process lifetime — missing
// translations are expected steady-state, not alert-worthy.
var missLog sync.Map

type missKey struct {
	locale string
	id     string
}

func logMissing(ctx context.Context, tag language.Tag, id string, err error) {
	k := missKey{locale: tag.String(), id: id}
	if _, loaded := missLog.LoadOrStore(k, struct{}{}); loaded {
		return
	}
	logging.LoggerWithContext(ctx).WarnContext(ctx,
		"l10n: missing or malformed translation",
		"locale", k.locale,
		"key", id,
		"error", err,
	)
}

// ResetMissLogForTest clears the per-process warn-once gate. Tests
// that exercise the gate use this between cases to keep them
// independent.
func ResetMissLogForTest() {
	missLog = sync.Map{}
}
