package rsvpstate

import "go.ripls.org/ripls/server/gen/ripls/models"

// IsGoing reports whether an intention counts as planning to attend. YES and
// MAYBE both do — the distinction matters for headcount, not for whether the
// person is engaged with the experience, so notification recipients, needs
// eligibility, and attendee lists all treat them alike.
func IsGoing(intention models.RSVPIntention) bool {
	return intention == models.RSVPIntention_RSVP_INTENTION_YES ||
		intention == models.RSVPIntention_RSVP_INTENTION_MAYBE
}
