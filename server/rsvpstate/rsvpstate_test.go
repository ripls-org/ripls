package rsvpstate

import (
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestIsGoing(t *testing.T) {
	tests := []struct {
		intention models.RSVPIntention
		want      bool
	}{
		{models.RSVPIntention_RSVP_INTENTION_YES, true},
		{models.RSVPIntention_RSVP_INTENTION_MAYBE, true},
		{models.RSVPIntention_RSVP_INTENTION_NO, false},
		{models.RSVPIntention_RSVP_INTENTION_UNSPECIFIED, false},
	}
	for _, tt := range tests {
		if got := IsGoing(tt.intention); got != tt.want {
			t.Errorf("IsGoing(%v) = %v, want %v", tt.intention, got, tt.want)
		}
	}
}
