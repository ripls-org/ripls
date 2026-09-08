package chat_subscriber

import (
	"context"
	"sync/atomic"

	"golang.org/x/text/language"

	"go.ripls.org/ripls/server/chat_event_bus"
	"go.ripls.org/ripls/server/community"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// SubscriberName is the stable identifier used in pubsub log lines.
const SubscriberName = "chat_notifications"

// StreamChecker reports whether userID currently has an active stream for
// conversationID. When true and the user is in the foreground, push delivery
// is suppressed in favor of the real-time stream.
type StreamChecker func(conversationID, userID string) bool

// ForegroundChecker reports whether the user's app is currently in the
// foreground. Push delivery is suppressed when both StreamChecker and
// ForegroundChecker return true.
type ForegroundChecker func(userID string) bool

// Subscriber is the push-notification consumer of chat_event_bus.
// One instance is registered on the bus; it owns recipient resolution,
// preference gating, soft-delete gating, copy assembly, and FCM dispatch.
type Subscriber struct {
	storage             *storage.ProtoSQLStorage
	notificationService notifications.Service

	// streamChecker and foregroundChecker are late-bound via Set* because the
	// chat service that owns the registry can't be constructed until after the
	// bus + this subscriber exist. Atomic load/store keeps Set* safe to call
	// from main.go without explicit locking.
	streamChecker     atomic.Pointer[StreamChecker]
	foregroundChecker atomic.Pointer[ForegroundChecker]
}

// New constructs a Subscriber. The stream and foreground checker hooks are
// unset; call SetStreamChecker and SetForegroundChecker after the chat service
// is constructed.
func New(s *storage.ProtoSQLStorage, notificationService notifications.Service) *Subscriber {
	return &Subscriber{
		storage:             s,
		notificationService: notificationService,
	}
}

// SetStreamChecker registers the late-bound stream-presence hook. Pass nil to
// unset. Safe to call concurrently.
func (s *Subscriber) SetStreamChecker(fn StreamChecker) {
	if fn == nil {
		s.streamChecker.Store(nil)
		return
	}
	s.streamChecker.Store(&fn)
}

// SetForegroundChecker registers the late-bound foreground-presence hook. Pass
// nil to unset. Safe to call concurrently.
func (s *Subscriber) SetForegroundChecker(fn ForegroundChecker) {
	if fn == nil {
		s.foregroundChecker.Store(nil)
		return
	}
	s.foregroundChecker.Store(&fn)
}

// Name implements pubsub.Subscriber.
func (s *Subscriber) Name() string { return SubscriberName }

// Handle implements pubsub.Subscriber. Short-circuits non-user-message kinds
// and OnBehalfOf messages, then resolves recipients, gates by preferences and
// active stream + foreground state, assembles the notification copy, and
// dispatches via FCM.
//
// Returning an error causes the underlying pubsub topic to log the dispatch as
// a failure. Per-recipient send errors do NOT bubble up — they are logged at
// WARN and processing continues to the next recipient.
func (s *Subscriber) Handle(ctx context.Context, evt *chat_event_bus.PublishedEvent) error {
	if evt == nil || evt.Conversation == nil {
		return nil
	}

	// Only send push for user-authored messages.
	if evt.Kind != chat_event_bus.KindUserMessage {
		return nil
	}

	// Server-side on-behalf-of inserts have their own notification path.
	if evt.OnBehalfOf {
		return nil
	}

	if evt.Message == nil {
		return nil
	}

	userMsg := evt.Message.GetUserMessage()
	if userMsg == nil {
		return nil
	}

	senderID := userMsg.GetSenderId()

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "chat_notifications.handle",
		"conversation_id", evt.Conversation.Id,
		"message_id", evt.Message.Id,
	)

	// Soft-delete gate: skip when the parent community is in the deleted state.
	// Conversations with no community (private DM-style) bypass this gate.
	if evt.Conversation.CommunityId != "" && !community.IsActive(ctx, s.storage, evt.Conversation.CommunityId) {
		logger.InfoContext(ctx, "skipping chat notification dispatch — community is soft-deleted",
			"community_id", evt.Conversation.CommunityId,
			"reason", "community_deleted",
		)
		return nil
	}

	recipientIDs := resolveRecipients(ctx, s.storage, evt.Conversation, senderID, logger)
	if len(recipientIDs) == 0 {
		logger.DebugContext(ctx, "no recipients for chat notification")
		return nil
	}

	// Batch preference rows in one query. Fail open on error.
	prefsByUser, prefsErr := community.FetchPreferencesForUsers(ctx, s.storage, evt.Conversation.CommunityId, recipientIDs)
	if prefsErr != nil {
		logger.WarnContext(ctx, "failed to fetch chat notification preferences; failing open",
			"community_id", evt.Conversation.CommunityId,
			"recipient_count", len(recipientIDs),
			"error", prefsErr,
		)
		prefsByUser = nil
	}

	streamCheck := s.loadStreamChecker()
	fgCheck := s.loadForegroundChecker()

	// Resolve recipient locales in one batch so the per-recipient
	// loop renders frame copy (sender fallback, "mentioned you") in
	// the right language without a per-call user lookup.
	localesByUser := loadRecipientLocales(ctx, s.storage, recipientIDs)

	// Resolve the entity name attached to the conversation topic
	// (gear / experience / request / community) once per event so
	// a community-wide fan-out doesn't pay one lookup per recipient.
	topic := resolveTopic(ctx, s.storage, evt)

	for _, recipientID := range recipientIDs {
		recipientLogger := logger.With("participant_id", recipientID)

		// Apply per-community Chats preference gate. Unset = on.
		if !community.CategoryEnabled(prefsByUser[recipientID], chatsCategoryForPrefs()) {
			recipientLogger.DebugContext(ctx, "skipping chat notification — category disabled by user preference",
				"community_id", evt.Conversation.CommunityId,
			)
			continue
		}

		// Suppress when user is actively streaming AND app is in foreground.
		hasStream := streamCheck != nil && streamCheck(evt.Conversation.Id, recipientID)
		isInForeground := fgCheck != nil && fgCheck(recipientID)
		if hasStream && isInForeground {
			recipientLogger.DebugContext(ctx, "skipping chat notification — active stream and app in foreground")
			continue
		}

		if !s.notificationService.HasDevices(ctx, recipientID) {
			recipientLogger.DebugContext(ctx, "skipping chat notification — no registered devices")
			continue
		}

		loc, locErr := l10n.NewLocalizer(localesByUser[recipientID])
		if locErr != nil {
			// Recipient locale's bundle failed to load. Try the
			// default-tag bundle so the notification renders
			// English copy instead of message-id literals
			// ("notif.chat.sender_fallback") that the nil-safe
			// Localizer.T path would otherwise emit.
			recipientLogger.WarnContext(ctx, "failed to construct l10n Localizer; falling back to default tag",
				"error", locErr,
				"requested_locale", localesByUser[recipientID].String())
			loc, locErr = l10n.NewLocalizer(l10n.DefaultTag)
			if locErr != nil {
				// Default bundle also failed — Bundle() is broken.
				// Skip the send rather than push a message-id
				// literal to the recipient's lock screen.
				recipientLogger.ErrorContext(ctx, "default l10n bundle unavailable; skipping chat notification",
					"error", locErr)
				continue
			}
		}
		notification := buildNotification(ctx, evt, recipientID, loc, topic)

		if err := s.notificationService.NotifyUser(ctx, recipientID, notification); err != nil {
			recipientLogger.WarnContext(ctx, "failed to send chat notification", "error", err)
		} else {
			recipientLogger.InfoContext(ctx, "sent chat notification")
		}
	}

	return nil
}

// loadRecipientLocales returns userID → resolved locale tag for the
// given chat recipients. Reads User.preferred_language in one
// batched query; users without a stored preference default to
// l10n.DefaultTag. Mirrors the equivalent helper in
// community_subscriber.
func loadRecipientLocales(ctx context.Context, st *storage.ProtoSQLStorage, recipients []string) map[string]language.Tag {
	out := make(map[string]language.Tag, len(recipients))
	if len(recipients) == 0 {
		return out
	}
	for _, id := range recipients {
		out[id] = l10n.DefaultTag
	}
	users, err := st.GetByIDs(ctx, recipients, &models.User{})
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

func (s *Subscriber) loadStreamChecker() StreamChecker {
	p := s.streamChecker.Load()
	if p == nil {
		return nil
	}
	return *p
}

func (s *Subscriber) loadForegroundChecker() ForegroundChecker {
	p := s.foregroundChecker.Load()
	if p == nil {
		return nil
	}
	return *p
}
