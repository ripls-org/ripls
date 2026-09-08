package notification_content

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
)

// SystemNotification builds the Notification for a system-generated
// community-event payload — a reminder, a close prompt, a nudge — rendering its
// push copy from the same catalog entry the off-app senders render.
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
// The payload must carry every name its kind's copy interpolates; anything
// missing renders as a localized stand-in (see Content.params), never a blank.
//
// TODO(#2897): push renders with the default (English) localizer. The
// dispatcher holds only the recipient's user id, and resolving their
// preferred_language here would be one storage read per due row inside the
// per-row dispatch loop. Off-app copy IS localized (those senders load the
// recipient anyway), and push copy for these notifications was hardcoded
// English before this change, so this is a carried-forward gap rather than a
// regression. #2897 tracks resolving the locale once per tick or in the
// notification service, where the recipient is already loaded.
func SystemNotification(ctx context.Context, payload *models.CommunityEventPayload) *models.Notification {
	loc, err := l10n.NewLocalizer(l10n.DefaultTag)
	if err != nil {
		// NewLocalizer only fails when the embedded catalog is unloadable, which
		// is a build-time problem, not a runtime one. Localizer.T is nil-safe
		// and falls back to the message id, so a nil loc still produces a
		// visible (if untranslated) push rather than an empty one.
		loc = nil
	}
	title, body := FromCommunityEvent(ctx, payload).PushCopy(ctx, loc)
	return &models.Notification{
		Title: title,
		Body:  body,
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: payload,
		},
	}
}
