package story

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// Creator is the interface injected into services that trigger story generation.
// This avoids service-to-service dependencies by using dependency injection.
// Services that need to create stories accept this interface in their constructors.
type Creator interface {
	// CreateStory generates and stores a story from the provided request.
	// Returns the created story or an error if generation/storage fails.
	CreateStory(ctx context.Context, req CreateStoryRequest) (*models.Story, error)
}

// CreateStoryRequest contains the information needed to generate a story.
type CreateStoryRequest struct {
	// CommunityID is the community this story belongs to.
	CommunityID string

	// StoryType specifies what kind of story to generate.
	StoryType string

	// ParticipantIDs are the users featured in this story.
	ParticipantIDs []string

	// ParticipantNames are the display names for participants (parallel to ParticipantIDs).
	// Used in story content generation.
	ParticipantNames []string

	// MediaIDs are the media items to include with the story.
	MediaIDs []string

	// RelatedEntityID is the ID of the entity this story is about (loan, gear, etc.).
	// Used for deduplication.
	RelatedEntityID string

	// RelatedEntityName is the name of the related entity (for story content).
	RelatedEntityName string

	// CommunityName is the name of the community (for story content).
	CommunityName string

	// Related entity IDs for navigation (set the appropriate one based on StoryType)
	RelatedGearID       string
	RelatedLoanID       string
	RelatedExperienceID string
	RelatedRequestID    string
	RelatedCommunityID  string

	// Impact metrics for transaction stories (optional).
	CostSavedUSD     float32
	TimeSavedMinutes float32
	Co2SavedGrams    float32

	// CommunityEventID is the id of the CommunityEvent that recorded
	// the action this story commemorates — set for stories tied to a
	// server-authoritative undoable action (loan/giveaway completion,
	// request fulfillment, experience completion). Persisted on
	// models.Story so the story-screen UndoableAction surface is a
	// direct read.
	CommunityEventID string

	// Kicker is the pre-formatted editorial line rendered above the
	// title on the story screen (e.g. "TUE · APR 28 · CLEAR CREEK, CO").
	// Caller is responsible for formatting; an empty string means the
	// kicker is omitted on render.
	Kicker string
}
