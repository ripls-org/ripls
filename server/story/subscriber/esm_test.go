package story_subscriber

import (
	"context"
	"testing"
	"time"

	esmlib "go.ripls.org/ripls/server/esm"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestMaterializeESMPromptForStory_HappyPath(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})
	guestID, _ := s.Insert(context.Background(), &models.User{Name: "Guest", Email: "g@example.com"})

	exp := &models.Experience{Name: "Hike", OwnerId: hostID}
	expID, _ := s.Insert(context.Background(), exp)
	exp.Id = expID

	// Link the experience to the community so BuildPromptForExperience
	// can derive the community_id.
	if _, err := s.Insert(context.Background(), &models.CommunityExperience{
		ExperienceId: expID,
		CommunityId:  communityID,
	}); err != nil {
		t.Fatalf("link experience to community: %v", err)
	}
	if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: expID, UserId: hostID, CommunityId: communityID,
		Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_YES,
	}); err != nil {
		t.Fatalf("rsvp host: %v", err)
	}
	if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: expID, UserId: guestID, CommunityId: communityID,
		Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_YES,
	}); err != nil {
		t.Fatalf("rsvp guest: %v", err)
	}

	now := time.Unix(fixedTS, 0)
	if err := materializeESMPromptForStory(context.Background(), s, exp, communityID, "story-1", now); err != nil {
		t.Fatalf("materializeESMPromptForStory: %v", err)
	}

	prompts, err := esmlib.QueryPromptsByExperience(context.Background(), s, expID)
	if err != nil {
		t.Fatalf("query prompts: %v", err)
	}
	if len(prompts) != 1 {
		t.Fatalf("expected 1 prompt for the experience, got %d", len(prompts))
	}
	if prompts[0].CommunityId != communityID {
		t.Errorf("prompt.CommunityId = %q, want %q", prompts[0].CommunityId, communityID)
	}
	if prompts[0].Question != storyEmbeddedPromptQuestion {
		t.Errorf("prompt.Question = %q, want canonical embedded-prompt question", prompts[0].Question)
	}
}

func TestMaterializeESMPromptForStory_Idempotent(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})

	exp := &models.Experience{Name: "Solo", OwnerId: hostID}
	expID, _ := s.Insert(context.Background(), exp)
	exp.Id = expID
	if _, err := s.Insert(context.Background(), &models.CommunityExperience{
		ExperienceId: expID,
		CommunityId:  communityID,
	}); err != nil {
		t.Fatalf("link experience to community: %v", err)
	}
	if _, err := s.Insert(context.Background(), &models.ExperienceRSVP{
		ExperienceId: expID, UserId: hostID, CommunityId: communityID,
		Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_YES,
	}); err != nil {
		t.Fatalf("rsvp host: %v", err)
	}

	now := time.Unix(fixedTS, 0)
	if err := materializeESMPromptForStory(context.Background(), s, exp, communityID, "story-1", now); err != nil {
		t.Fatalf("first materialize: %v", err)
	}
	// Second call should be a no-op.
	if err := materializeESMPromptForStory(context.Background(), s, exp, communityID, "story-1", now); err != nil {
		t.Fatalf("second materialize (should be idempotent): %v", err)
	}
	prompts, _ := esmlib.QueryPromptsByExperience(context.Background(), s, expID)
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt after two materialize calls, got %d", len(prompts))
	}
}

func TestMaterializeESMPromptForStory_NoAttendeesIsNoOp(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	hostID, _ := s.Insert(context.Background(), &models.User{Name: "Host", Email: "h@example.com"})

	exp := &models.Experience{Name: "Empty", OwnerId: hostID}
	expID, _ := s.Insert(context.Background(), exp)
	exp.Id = expID
	// No RSVPs / no attendees.

	now := time.Unix(fixedTS, 0)
	if err := materializeESMPromptForStory(context.Background(), s, exp, communityID, "story-1", now); err != nil {
		t.Fatalf("materializeESMPromptForStory: %v", err)
	}
	prompts, _ := esmlib.QueryPromptsByExperience(context.Background(), s, expID)
	if len(prompts) != 0 {
		t.Errorf("expected 0 prompts when no attendees, got %d", len(prompts))
	}
}
