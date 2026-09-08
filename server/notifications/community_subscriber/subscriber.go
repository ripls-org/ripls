package community_subscriber

import (
	"context"
	"sync/atomic"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/community"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// SubscriberName is the stable identifier used in pubsub log lines.
const SubscriberName = "community_notifications"

// Dispatch outcome values emitted in the per-recipient INFO log line. All
// outcomes share the same structured shape so Cloud Logging can group by
// jsonPayload.outcome. See README.md for the recommended filter.
const (
	outcomeSent                    = "sent"
	outcomeSuppressedActor         = "suppressed_actor"
	outcomeSuppressedActiveStream  = "suppressed_active_stream"
	outcomeSuppressedCategory      = "suppressed_category"
	outcomeSuppressedDuplicate     = "suppressed_duplicate"
	outcomeSuppressedNoDevices     = "suppressed_no_devices"
	outcomeSendFailed              = "send_failed"
	outcomeSkippedCommunityDeleted = "skipped_community_deleted"
)

// StreamChecker reports whether userID currently has an active event stream.
// When true, push delivery for that user is suppressed in favor of the stream
// broadcast. See server/services/community/stream_subscriber.go.
//
// It takes no community: a stream covers every community the user belongs to
// (#2867), so presence is a property of the user alone. The chat subscriber's
// same-named type is still keyed by conversation — those are separate hooks
// answering different questions, not a symmetry to restore.
type StreamChecker func(userID string) bool

// Subscriber is the push-notification consumer of community_event_bus.
// One instance is registered on the bus; it owns recipient resolution,
// preference gating, soft-delete gating, copy assembly, and FCM dispatch.
type Subscriber struct {
	storage             *storage.ProtoSQLStorage
	notificationService notifications.Service

	// streamChecker is late-bound via SetStreamChecker because the
	// communityService that owns the stream registry can't be constructed
	// until after the bus + this subscriber exist. Atomic load/store keeps
	// SetStreamChecker safe to call from main.go without explicit locking.
	streamChecker atomic.Pointer[StreamChecker]

	// dedup suppresses duplicate pushes when one user action fans out into
	// multiple per-community events sharing the originating RPC's request_id
	// (see #2088). It is consulted per recipient on the actual-send path.
	dedup *dedupSet
}

// New constructs a Subscriber. The streamChecker hook is left unset; call
// SetStreamChecker after the community service is constructed.
func New(s *storage.ProtoSQLStorage, notificationService notifications.Service) *Subscriber {
	return &Subscriber{
		storage:             s,
		notificationService: notificationService,
		dedup:               newDedupSet(dedupTTL),
	}
}

// SetStreamChecker registers the late-bound stream-presence hook used to
// suppress push delivery for users currently consuming events via
// StreamUserEvents. Pass nil to unset. Safe to call concurrently.
func (s *Subscriber) SetStreamChecker(fn StreamChecker) {
	if fn == nil {
		s.streamChecker.Store(nil)
		return
	}
	s.streamChecker.Store(&fn)
}

// Name implements pubsub.Subscriber.
func (s *Subscriber) Name() string { return SubscriberName }

// Handle implements pubsub.Subscriber. Decides whether to dispatch push for
// the given event, resolves recipients, gates by preferences and active
// streams, builds the notification copy, and calls notificationService.
//
// Returning an error here causes the underlying pubsub topic to log the
// dispatch as a failure (per docs/server/observability.md). Handler-internal
// failures (per-recipient send errors) do NOT bubble up — they are logged
// at WARN and the next recipient continues.
func (s *Subscriber) Handle(ctx context.Context, evt *cebus.PublishedEvent) error {
	if evt == nil || evt.Event == nil {
		return nil
	}
	event := evt.Event

	// Short-circuit: event types that never fire push (e.g. internal
	// retractions) are filtered here so we don't pay for recipient lookup.
	if !ShouldNotify(event.EventType) {
		return nil
	}

	// Thread community_event_id through the dispatch context so service.go
	// and fcm/provider.go log lines are correlatable back to this event.
	ctx = notifications.WithCommunityEventID(ctx, event.Id)

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "community_notifications.handle",
		"community_event_id", event.Id,
		"community_id", event.CommunityId,
		"event_type", event.EventType.String(),
	)

	// Soft-delete gate. The audit row was already recorded by the
	// publisher; this only suppresses push. See community.FiresForDeletedCommunity
	// for the explicit allow-list of events that intentionally fire AT or
	// AFTER soft-delete (currently COMMUNITY_DELETED).
	if !community.FiresForDeletedCommunity(event.EventType) && !community.IsActive(ctx, s.storage, event.CommunityId) {
		logger.InfoContext(ctx, "skipping notification dispatch — community is soft-deleted",
			"outcome", outcomeSkippedCommunityDeleted,
		)
		return nil
	}

	recipients := resolveRecipients(ctx, s.storage, evt)
	if len(recipients) == 0 {
		logger.DebugContext(ctx, "no recipients for notification")
		return nil
	}

	// Batch the preference rows for all recipients in one query. Fail open
	// on storage error: a transient hiccup must not silently mute pushes
	// for an entire community.
	category := community.CategoryFor(event.EventType)
	prefsByUser, prefsErr := community.FetchPreferencesForUsers(ctx, s.storage, event.CommunityId, recipients)
	if prefsErr != nil {
		logger.WarnContext(ctx, "failed to fetch notification preferences; failing open",
			"recipient_count", len(recipients),
			"error", prefsErr,
		)
		prefsByUser = nil
	}

	// Resolve recipient locales in one batch so the per-recipient
	// loop below does not run a query per recipient. Render the
	// notification copy once per distinct locale and reuse it across
	// every recipient sharing that locale.
	localesByUser := loadRecipientLocales(ctx, s.storage, recipients)
	notificationsByLocale := make(map[language.Tag]*models.Notification, 2)

	streamCheck := s.loadStreamChecker()

	for _, userID := range recipients {
		if userID == event.ActorId {
			// Safety check — never notify the actor.
			logger.InfoContext(ctx, "suppressing notification — recipient is actor",
				"user_id", userID,
				"outcome", outcomeSuppressedActor,
			)
			continue
		}

		if streamCheck != nil && streamCheck(userID) {
			logger.InfoContext(ctx, "suppressing notification — user has active event stream",
				"user_id", userID,
				"outcome", outcomeSuppressedActiveStream,
			)
			continue
		}

		if category != models.NotificationCategory_NOTIFICATION_CATEGORY_UNSPECIFIED {
			if !community.CategoryEnabled(prefsByUser[userID], category) {
				logger.InfoContext(ctx, "suppressing notification — category disabled by user preference",
					"user_id", userID,
					"category", category,
					"outcome", outcomeSuppressedCategory,
				)
				continue
			}
		}

		// Duplicate gate. When one user action fans out into multiple
		// per-community events (#2088), every sibling shares the
		// originating RPC's request_id, so reserve the key here — after
		// the actor/stream/category gates, so a suppressed sibling never
		// masks a real push — and skip if a sibling already claimed it.
		// A non-empty key is released below if delivery fails, so the
		// reservation only persists once a push actually goes out.
		dkey := dedupKey(ctx, event, userID)
		if dkey != "" && s.dedup.seenOrAdd(dkey) {
			logger.InfoContext(ctx, "suppressing notification — duplicate of same user action",
				"user_id", userID,
				"outcome", outcomeSuppressedDuplicate,
			)
			continue
		}

		tag := localesByUser[userID]
		notification := notificationsByLocale[tag]
		if notification == nil {
			loc, locErr := l10n.NewLocalizer(tag)
			if locErr != nil {
				// Recipient locale's bundle failed to load. Try
				// the default-tag bundle so the push renders
				// English copy instead of message-id literals
				// ("notif.community_event...") that the nil-safe
				// Localizer.T path would otherwise emit.
				logger.WarnContext(ctx, "failed to construct l10n Localizer; falling back to default tag",
					"error", locErr, "requested_locale", tag.String())
				loc, locErr = l10n.NewLocalizer(l10n.DefaultTag)
				if locErr != nil {
					// Default bundle also failed — Bundle() is
					// broken. Skip the user rather than push a
					// message-id literal to their lock screen.
					logger.ErrorContext(ctx, "default l10n bundle unavailable; skipping notification for user",
						"error", locErr, "user_id", userID)
					continue
				}
			}
			notification = buildNotification(ctx, s.storage, event, loc)
			notificationsByLocale[tag] = notification
		}

		if err := s.notificationService.NotifyUser(ctx, userID, notification); err != nil {
			// Release the reservation so a sibling event for the same
			// action can retry delivery instead of being suppressed.
			if dkey != "" {
				s.dedup.remove(dkey)
			}
			logger.WarnContext(ctx, "failed to send notification to user",
				"user_id", userID,
				"locale", tag.String(),
				"error", err,
				"outcome", outcomeSendFailed,
			)
		} else {
			logger.InfoContext(ctx, "notification dispatched to user",
				"user_id", userID,
				"locale", tag.String(),
				"title", notification.Title,
				"outcome", outcomeSent,
			)
		}
	}

	return nil
}

// loadRecipientLocales returns a map of userID → resolved locale
// tag for the given recipients. Reads User.preferred_language in a
// single batched query; any user with an empty or unsupported tag
// falls through to l10n.DefaultTag.
func loadRecipientLocales(ctx context.Context, s *storage.ProtoSQLStorage, recipients []string) map[string]language.Tag {
	out := make(map[string]language.Tag, len(recipients))
	if len(recipients) == 0 {
		return out
	}

	// Default everyone to DefaultTag up front so a failed batch read
	// still produces a usable (English) notification for every
	// recipient instead of dropping the entire fan-out.
	for _, id := range recipients {
		out[id] = l10n.DefaultTag
	}

	users, err := s.GetByIDs(ctx, recipients, &models.User{})
	if err != nil {
		logging.LoggerWithContext(ctx).WarnContext(ctx,
			"failed to batch-load recipient users for locale resolution; defaulting to English",
			"recipient_count", len(recipients), "error", err,
		)
		return out
	}
	for _, msg := range users {
		u, ok := msg.(*models.User)
		if !ok || u == nil {
			continue
		}
		out[u.Id] = l10n.Normalize(u.GetPreferredLanguage())
	}
	return out
}

// loadStreamChecker returns the currently-registered stream-presence hook,
// or nil if SetStreamChecker has not been called.
func (s *Subscriber) loadStreamChecker() StreamChecker {
	p := s.streamChecker.Load()
	if p == nil {
		return nil
	}
	return *p
}
