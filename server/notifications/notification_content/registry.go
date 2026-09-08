package notification_content

import "go.ripls.org/ripls/server/gen/ripls/models"

// SystemEventTypes is every CommunityEventPayload.event_type value minted by a
// producer of system-generated notifications — reminders, close prompts, nudges.
// These are not CommunityEventType enum members: no bus event backs them,
// nothing is persisted under them, and there is no actor.
//
// Adding one without a kind in offAppKinds is what shipped #2896: a text message
// reading "Ripls:  posted an update in ". TestEveryNotificationEventTypeHasCopy
// makes that a build failure instead.
//
// Producers, in the same order:
//   - jobs/scheduled_notifications/experience_reminders.go
//   - jobs/scheduled_notifications/loan_return_reminders.go
//   - jobs/scheduled_notifications/request_followup_prompts.go
//   - services/experience/nudge_needs.go, poll_manage.go (via pollSpec)
//   - services/request/nudge_needs.go
var SystemEventTypes = []string{
	"EXPERIENCE_REMINDER_DAY_BEFORE",
	"EXPERIENCE_REMINDER_TWO_HOUR",
	"EXPERIENCE_REMINDER_STARTING",
	"EXPERIENCE_REMINDER",
	"EXPERIENCE_CLOSE_PROMPT",
	"LOAN_RETURN_REMINDER_UPCOMING",
	"LOAN_RETURN_REMINDER_DUE",
	"LOAN_RETURN_REMINDER_OVERDUE",
	"LOAN_RETURN_REMINDER",
	"REQUEST_FOLLOWUP_PROMPT",
	"EXPERIENCE_NEEDS_NUDGE",
	"REQUEST_NEEDS_NUDGE",
	"EXPERIENCE_TIME_NUDGE",
	"EXPERIENCE_LOCATION_NUDGE",
}

// BusEventTypesWithoutCopy lists the CommunityEventType values that deliberately
// have no copy, with the reason each one is exempt. Every other enum value must
// map to a kind.
//
// An entry here is a claim that the event never reaches a recipient. If one
// starts notifying, delete its entry and give it copy — leaving it listed would
// send the generic default line to a real person.
var BusEventTypesWithoutCopy = map[models.CommunityEventType]string{
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED:                   "not a real event",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED:                 "bookkeeping: an item leaving a community notifies nobody",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT:                   "no notification; the owner sees membership in the community itself",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW: "bookkeeping for the leave/rejoin grace window",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED:            "the completion prompt is a scheduled notification, not this event",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED:             "the creator is the only member; telling them is noise",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED:               "an ad-hoc community gaining a name is not news to its members",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED:     "internal roster bookkeeping behind RSVP events",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_REMOVED:         "removals are quiet; only additions and claims notify",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED:         "edits are quiet; only additions and claims notify",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_REMOVED: "removals are quiet; only additions and claims notify",

	// Retraction events. An undo is reflected in the entity's own state; a
	// second push saying "never mind" would be worse than silence.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE: "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE:             "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE:          "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE:          "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED_UNDONE:      "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE:           "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE:           "retraction",
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE:        "retraction",
}

// NotificationEventTypes returns every event_type string that can reach a
// recipient: the CommunityEventType names that are not exempt, plus the
// synthetic system types. It is the canonical list the copy gate walks and the
// review page renders.
func NotificationEventTypes() []string {
	out := make([]string, 0, len(models.CommunityEventType_name)+len(SystemEventTypes))
	for value, name := range models.CommunityEventType_name {
		if _, exempt := BusEventTypesWithoutCopy[models.CommunityEventType(value)]; exempt {
			continue
		}
		out = append(out, name)
	}
	return append(out, SystemEventTypes...)
}
