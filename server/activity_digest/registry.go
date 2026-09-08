package activity_digest

import (
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// This file is the digest-facing classification of every
// CommunityEventType: which reporting category an event belongs to and
// which email.ActivityTotals counter it increments, or why it is
// deliberately excluded. It is the single source of truth behind
// TestEveryDigestEventTypeClassified in registry_test.go — any new
// CommunityEventType added to the proto without an entry here fails CI,
// so an action can never silently vanish from the ops digest the way
// the pre-registry switch statement allowed (#2665). The undo-side
// sibling of this gate is server/undo/registry.go.

// Category is a digest reporting bucket for counted CommunityEvent rows.
// Per-community breakdown lines group actions by Category; the ACTIVITY
// section's curated rows are built from the individual ActivityTotals
// counters instead.
type Category int

const (
	// CategoryUnspecified marks an unclassified event type. Any counted
	// entry resolving to it fails TestEveryDigestEventTypeClassified.
	CategoryUnspecified Category = iota
	// CategoryGear covers sharing and unsharing gear items.
	CategoryGear
	// CategoryLoans covers loan-type transfer lifecycle and coordination.
	CategoryLoans
	// CategoryGiveaways covers giveaway-type transfer lifecycle and
	// coordination.
	CategoryGiveaways
	// CategoryRequests covers request lifecycle and offers.
	CategoryRequests
	// CategoryEvents covers Experience activity. The UI terminology rule
	// renders Experiences as "Events"; the digest follows it, and this is
	// the only sense in which the digest uses the word.
	CategoryEvents
	// CategoryPlanning covers collaborative needs & contributions.
	CategoryPlanning
	// CategoryMembership covers joins, leaves, and rejoins.
	CategoryMembership
	// CategoryCommunity covers community lifecycle (created, named,
	// deleted, restored, ownership transferred).
	CategoryCommunity
)

// Label returns the short human label for n actions in this category,
// used in per-community breakdown lines ("3 loans, 1 request").
func (c Category) Label(n int) string {
	plural := n != 1
	suffix := ""
	if plural {
		suffix = "s"
	}
	switch c {
	case CategoryGear:
		return "gear action" + suffix
	case CategoryLoans:
		return "loan" + suffix
	case CategoryGiveaways:
		return "giveaway" + suffix
	case CategoryRequests:
		return "request" + suffix
	case CategoryEvents:
		return "event" + suffix
	case CategoryPlanning:
		return "planning action" + suffix
	case CategoryMembership:
		return "membership change" + suffix
	case CategoryCommunity:
		return "community change" + suffix
	default:
		return "other"
	}
}

// Disposition says how the digest treats one CommunityEventType.
type Disposition int

const (
	// DispositionUnspecified is the zero value and indicates an event
	// type without a digest classification. Any event type mapping to it
	// fails TestEveryDigestEventTypeClassified.
	DispositionUnspecified Disposition = iota
	// DispositionCounted: the event is a member action — it increments an
	// ActivityTotals counter and counts toward the per-community
	// category breakdown and the "Member actions" total.
	DispositionCounted
	// DispositionExcludedRetraction: an UNDONE retraction row. Excluded
	// so the original action isn't double-counted; surfaced only as the
	// digest footer's "actions undone" count.
	DispositionExcludedRetraction
	// DispositionExcludedInternal: a server bookkeeping signal that is
	// not a user action (e.g. a roster-refresh ping). Excluded from all
	// action counts; surfaced only in the footer's excluded count.
	DispositionExcludedInternal
)

// Entry classifies one CommunityEventType for the digest.
type Entry struct {
	// Disposition selects counted vs. one of the excluded buckets.
	Disposition Disposition
	// CategoryOf returns the reporting bucket for one event row. Set only
	// for DispositionCounted. Transfer-scoped types split into Loans vs
	// Giveaways by the transfer_type stamped on the row.
	CategoryOf func(ev *models.CommunityEvent) Category
	// Apply increments the matching ActivityTotals counter for one event
	// row. Set only for DispositionCounted.
	Apply func(ev *models.CommunityEvent, t *email.ActivityTotals)
}

// counted builds a DispositionCounted entry.
func counted(categoryOf func(*models.CommunityEvent) Category, apply func(*models.CommunityEvent, *email.ActivityTotals)) Entry {
	return Entry{Disposition: DispositionCounted, CategoryOf: categoryOf, Apply: apply}
}

// fixedCategory returns a CategoryOf that ignores the row.
func fixedCategory(c Category) func(*models.CommunityEvent) Category {
	return func(*models.CommunityEvent) Category { return c }
}

// transferCategory splits transfer-scoped events into Loans vs Giveaways
// by the transfer_type stamped on the event row. Rows without a
// transfer_type default to Loans, matching the pre-registry behavior.
func transferCategory(ev *models.CommunityEvent) Category {
	if ev.GetTransferType() == models.TransferType_TRANSFER_TYPE_GIVEAWAY {
		return CategoryGiveaways
	}
	return CategoryLoans
}

// isGiveaway reports whether the event row is stamped as a giveaway.
func isGiveaway(ev *models.CommunityEvent) bool {
	return ev.GetTransferType() == models.TransferType_TRANSFER_TYPE_GIVEAWAY
}

// Registry maps every CommunityEventType to its digest classification.
// TestEveryDigestEventTypeClassified enforces completeness.
var Registry = map[models.CommunityEventType]Entry{
	// Gear.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED: counted(fixedCategory(CategoryGear),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.GearShared++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_UNSHARED: counted(fixedCategory(CategoryGear),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.GearUnshared++ }),

	// Transfer lifecycle — loans and giveaways split by transfer_type.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE: counted(transferCategory,
		func(ev *models.CommunityEvent, t *email.ActivityTotals) {
			if isGiveaway(ev) {
				t.GiveawaysStarted++
			} else {
				t.LoansStarted++
			}
		}),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED: counted(transferCategory,
		func(ev *models.CommunityEvent, t *email.ActivityTotals) {
			if isGiveaway(ev) {
				t.GiveawaysCompleted++
			} else {
				t.LoansCompleted++
			}
		}),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED: counted(transferCategory,
		func(ev *models.CommunityEvent, t *email.ActivityTotals) {
			if isGiveaway(ev) {
				t.GiveawaysCancelled++
			} else {
				t.LoansCancelled++
			}
		}),

	// Transfer coordination — the funnel motion between "shared" and
	// "started", not split by transfer type in the totals.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED: counted(transferCategory,
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.TransferInterestExpressed++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN: counted(transferCategory,
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.TransferInterestWithdrawn++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED: counted(transferCategory,
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.TransferRecipientsSelected++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED: counted(transferCategory,
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.TransferPickupsProposed++ }),

	// Requests.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestsPosted++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestOffersMade++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestOffersSelected++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestOffersWithdrawn++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestsFulfilled++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED: counted(fixedCategory(CategoryRequests),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RequestsCancelled++ }),

	// Experiences ("Events" in product copy).
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.ExperiencesPosted++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.ExperiencesStarted++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.ExperiencesUpdated++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.ExperiencesCompleted++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.ExperiencesCancelled++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RSVPYes++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RSVPMaybe++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_NO: counted(fixedCategory(CategoryEvents),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.RSVPNo++ }),

	// Planning — needs & contributions on experiences and requests.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningNeedsAdded++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_REMOVED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningNeedsRemoved++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningNeedsClaimed++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_UPDATED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningNeedsUpdated++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningContributionsAdded++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_REMOVED: counted(fixedCategory(CategoryPlanning),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.PlanningContributionsRemoved++ }),

	// Membership.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED: counted(fixedCategory(CategoryMembership),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.InvitationsRedeemed++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_LEFT: counted(fixedCategory(CategoryMembership),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.MembersLeft++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_MEMBER_REJOINED_WITHIN_WINDOW: counted(fixedCategory(CategoryMembership),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.MembersRejoined++ }),

	// Community lifecycle.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_CREATED: counted(fixedCategory(CategoryCommunity),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.CommunitiesCreated++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED: counted(fixedCategory(CategoryCommunity),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.CommunitiesNamed++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED: counted(fixedCategory(CategoryCommunity),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.CommunitiesDeleted++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED: counted(fixedCategory(CategoryCommunity),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.CommunitiesRestored++ }),
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED: counted(fixedCategory(CategoryCommunity),
		func(_ *models.CommunityEvent, t *email.ActivityTotals) { t.OwnershipTransferred++ }),

	// UNDONE retractions — excluded so the original action isn't
	// double-counted; reported via the footer's undone count.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED_UNDONE: {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE_UNDONE:             {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE:          {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED_UNDONE:          {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED_UNDONE:      {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED_UNDONE:           {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED_UNDONE:           {Disposition: DispositionExcludedRetraction},
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED_UNDONE:        {Disposition: DispositionExcludedRetraction},

	// Bookkeeping signals — not user actions.
	// EXPERIENCE_ROSTER_CHANGED exists purely to stream a "refresh the
	// roster" signal to clients (see its proto comment); counting it
	// would inflate every experience's apparent activity.
	models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_ROSTER_CHANGED: {Disposition: DispositionExcludedInternal},
}
