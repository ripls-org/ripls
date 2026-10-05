package notification_content

import (
	"context"
	"sync"

	"go.ripls.org/ripls/server/logging"
)

// kindForEventType maps a CommunityEventPayload.event_type string to the short
// kind key that selects the copy in the l10n catalog
// (notif.offapp.{kind}.{message,cta}).
//
// event_type carries two vocabularies: CommunityEventType enum names for events
// published on the community bus, and synthetic strings minted by the producers
// of system-generated notifications (reminders, close prompts, nudges). Both are
// in offAppKinds; NotificationEventTypes is the canonical list and
// TestEveryNotificationEventTypeHasCopy enforces that every entry maps here.
//
// A type not in the table falls back to "default" and logs — a fallthrough means
// somebody added a notification without copy, which is how #2896 shipped a text
// message reading "Ripls:  posted an update in ". The default line itself
// degrades by which names the payload actually carries (see Content.messageKind)
// so the fallback is at worst vague, never ungrammatical.
func kindForEventType(ctx context.Context, eventType string) string {
	if k, ok := offAppKinds[eventType]; ok {
		return k
	}
	logUnmappedEventType(ctx, eventType)
	return KindDefault
}

// unmappedLog deduplicates the fallthrough warning per event type: once per
// process, matching the l10n missing-key gate (l10n.logMissing). A steady stream
// of identical warnings would bury the first one, and one line is enough for the
// log-based metric to fire on.
var unmappedLog sync.Map

func logUnmappedEventType(ctx context.Context, eventType string) {
	if _, loaded := unmappedLog.LoadOrStore(eventType, struct{}{}); loaded {
		return
	}
	logging.LoggerWithContext(ctx).WarnContext(ctx,
		"notification copy: event type has no kind mapping; falling back to default",
		"event_type", eventType,
		"outcome", "unmapped_event_type",
	)
}

// ResetUnmappedLogForTest clears the per-process warn-once gate so tests that
// exercise it stay independent.
func ResetUnmappedLogForTest() {
	unmappedLog = sync.Map{}
}

// KindDefault is the kind every unmapped event type falls back to. Its message
// degrades further by which names the payload carries — see Content.messageKind.
const KindDefault = "default"

// offAppKinds is the event-type → kind-key table. Every kind present here has
// `notif.offapp.{kind}.{message,cta}` entries in the catalog.
var offAppKinds = map[string]string{
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED":          "experience_created",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES":         "experience_rsvp_yes",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE":       "experience_rsvp_maybe",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO":          "experience_rsvp_no",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED":          "experience_updated",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED":        "experience_cancelled",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED":          "experience_started",
	"COMMUNITY_EVENT_TYPE_REQUEST_CREATED":             "request_created",
	"COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE":          "request_offer_made",
	"COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED":      "request_offer_selected",
	"COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED":           "request_fulfilled",
	"COMMUNITY_EVENT_TYPE_GEAR_SHARED":                 "gear_shared",
	"COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED": "transfer_interest_expressed",
	"COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN": "transfer_interest_withdrawn",
	"COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED": "transfer_recipient_selected",
	"COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE":             "transfer_active",
	"COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED":          "transfer_cancelled",
	"COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED":    "transfer_pickup_proposed",
	"COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED":           "request_cancelled",
	"COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN":     "request_offer_withdrawn",
	"COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED":        "experience_completed",
	"COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED":         "planning_need_added",
	"COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED":       "planning_need_claimed",
	"COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED": "planning_contribution_added",
	"COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED":        "invitation_link_used",
	"COMMUNITY_EVENT_TYPE_ITEM_SHARED_WITH_USER":       "item_shared_with_user",
	"COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED":           "community_deleted",
	"COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED":          "community_restored",
	"COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED":       "ownership_transferred",

	// System-generated notifications. These event types are not
	// CommunityEventType enum values — no bus event backs them, nothing is
	// persisted, and there is no actor. Their producers mint the string
	// directly (jobs/scheduled_notifications, the nudge RPCs).
	"EXPERIENCE_REMINDER_DAY_BEFORE": "experience_reminder_day_before",
	"EXPERIENCE_REMINDER_TWO_HOUR":   "experience_reminder_two_hour",
	"EXPERIENCE_REMINDER_STARTING":   "experience_reminder_starting",
	// Pre-split payloads: a push queued at FCM when the per-offset event types
	// rolled out still resolves to sane copy.
	"EXPERIENCE_REMINDER":           "experience_reminder",
	"EXPERIENCE_CLOSE_PROMPT":       "experience_close_prompt",
	"LOAN_RETURN_REMINDER_UPCOMING": "loan_return_reminder_upcoming",
	"LOAN_RETURN_REMINDER_DUE":      "loan_return_reminder_due",
	"LOAN_RETURN_REMINDER_OVERDUE":  "loan_return_reminder_overdue",
	"LOAN_RETURN_REMINDER":          "loan_return_reminder",
	// Never rename: app/lib/services/fcm_service.dart matches this string
	// verbatim to deep-link into the request's fulfillment UI, so an older
	// installed client would lose that routing.
	"REQUEST_FOLLOWUP_PROMPT": "request_followup_prompt",

	// Nudges, minted by the host-triggered nudge RPCs.
	"EXPERIENCE_NEEDS_NUDGE":    "experience_needs_nudge",
	"REQUEST_NEEDS_NUDGE":       "request_needs_nudge",
	"EXPERIENCE_TIME_NUDGE":     "experience_time_nudge",
	"EXPERIENCE_LOCATION_NUDGE": "experience_location_nudge",
}
