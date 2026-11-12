package notification_content

import (
	"context"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
)

// SystemNotification builds the Notification for a system-generated
// community-event payload — a reminder, a close prompt, a nudge — rendering its
// push copy in the recipient's locale from the same catalog entry the off-app
// senders render.
//
// Producers call this instead of composing their own title/body. The push
// strings have to be materialized here because FCM and APNs display them
// verbatim, but the sentence itself is not written here: PushCopy reads
// notif.offapp.{kind}.{title,message}, and the off-app senders ignore
// Title/Body entirely and re-render the same .message from the payload. So the
// sentence exists once and cannot drift between surfaces.
//
// Before #2896 these producers wrote their copy inline with fmt.Sprintf, which
// meant the off-app renderers had no entry for the event type at all and fell
// through to the actorless default line — "Ripls:  posted an update in ".
//
// locale is the recipient's resolved locale. Callers resolve it in one batched
// read for the whole fan-out (LoadRecipientLocales, or SystemNotifications for
// a known recipient list) rather than per recipient (#3276). It is not taken
// from ctx: on the nudge paths ctx carries the acting organizer's locale, not
// the recipient's.
//
// The payload must carry every name its kind's copy interpolates; anything
// missing renders as a localized stand-in (see Content.params), never a blank.
func SystemNotification(ctx context.Context, locale language.Tag, payload *models.CommunityEventPayload) *models.Notification {
	title, body := FromCommunityEvent(ctx, payload).PushCopy(ctx, systemLocalizer(ctx, locale))
	return &models.Notification{
		Title: title,
		Body:  body,
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: payload,
		},
	}
}

// SystemNotifications renders payload for each recipient in their own locale:
// one batched preferred_language read for the whole list, and one render per
// distinct locale, shared by every recipient in it. The result holds an entry
// for every recipient; a failed locale read degrades to English (see
// LoadRecipientLocales).
func SystemNotifications(
	ctx context.Context, users UserBatchReader, recipients []string, payload *models.CommunityEventPayload,
) map[string]*models.Notification {
	locales := LoadRecipientLocales(ctx, users, recipients)
	byLocale := make(map[language.Tag]*models.Notification, 2)
	out := make(map[string]*models.Notification, len(recipients))
	for _, id := range recipients {
		tag := locales[id]
		n := byLocale[tag]
		if n == nil {
			n = SystemNotification(ctx, tag, payload)
			byLocale[tag] = n
		}
		out[id] = n
	}
	return out
}

// systemLocalizer returns a Localizer for locale, falling back to the default
// tag if locale's bundle cannot be built. NewLocalizer only fails when the
// embedded catalog is unloadable, which is a build-time problem, not a runtime
// one. If even the default fails it returns nil: Localizer.T is nil-safe and
// falls back to the message id, so the push is visible (if untranslated)
// rather than empty.
func systemLocalizer(ctx context.Context, locale language.Tag) *l10n.Localizer {
	loc, err := l10n.NewLocalizer(locale)
	if err == nil {
		return loc
	}
	logging.LoggerWithContext(ctx).WarnContext(ctx,
		"failed to construct l10n Localizer for system notification; falling back to default tag",
		"requested_locale", locale.String(), "error", err)
	loc, err = l10n.NewLocalizer(l10n.DefaultTag)
	if err != nil {
		return nil
	}
	return loc
}
