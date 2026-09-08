package story

import (
	"context"
	"fmt"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// Generator creates stories from community events.
// This is a standalone library, not a service - it has no RPC handlers.
// It's injected into services that need to create stories.
type Generator struct {
	storyStorage *storage.StoryStorage
	logger       *logging.Logger
}

// NewGenerator creates a new story generator.
func NewGenerator(storyStorage *storage.StoryStorage) *Generator {
	return &Generator{
		storyStorage: storyStorage,
		logger:       logging.Default(),
	}
}

// CreateStory generates and stores a story from the provided request.
// Returns the created story or an error if generation/storage fails.
// Implements the Creator interface.
func (g *Generator) CreateStory(ctx context.Context, req CreateStoryRequest) (*models.Story, error) {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "CreateStory",
		"story_type", req.StoryType,
		"community_id", req.CommunityID,
		"related_entity_id", req.RelatedEntityID,
	)

	startTime := time.Now()

	// Check for duplicate story (deduplication)
	if req.RelatedEntityID != "" {
		exists, err := g.storyStorage.Exists(ctx, req.CommunityID, req.StoryType, req.RelatedEntityID)
		if err != nil {
			logger.ErrorContext(ctx, "failed to check story existence", "error", err)
			return nil, fmt.Errorf("failed to check story existence: %w", err)
		}

		if exists {
			logger.InfoContext(ctx, "story already exists, skipping creation")
			return nil, fmt.Errorf("story already exists for this event")
		}
	}

	// Generate story content. AI path returns an empty templateKey
	// (free-text prose); template path returns a key + params so
	// new clients can render in the viewer's locale via ARB.
	content := g.generateContent(ctx, req)

	// Create story model
	story := &models.Story{
		CommunityId:      req.CommunityID,
		StoryType:        req.StoryType,
		Title:            content.Title,
		Description:      content.Description,
		MediaIds:         req.MediaIDs,
		ParticipantIds:   req.ParticipantIDs,
		GearId:           req.RelatedGearID,
		LoanId:           req.RelatedLoanID,
		ExperienceId:     req.RelatedExperienceID,
		RequestId:        req.RelatedRequestID,
		CostSavedUsd:     req.CostSavedUSD,
		TimeSavedMinutes: req.TimeSavedMinutes,
		Co2SavedGrams:    req.Co2SavedGrams,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if req.CommunityEventID != "" {
		story.CommunityEventId = &req.CommunityEventID
	}
	if req.Kicker != "" {
		story.Kicker = &req.Kicker
	}
	if content.TemplateKey != "" {
		story.TemplateKey = &content.TemplateKey
		story.TemplateParams = content.Params
	}

	// Insert into storage
	storyID, err := g.storyStorage.Insert(ctx, story)
	if err != nil {
		logger.ErrorContext(ctx, "failed to insert story", "error", err)
		return nil, fmt.Errorf("failed to insert story: %w", err)
	}

	// Update story with generated ID
	story.Id = storyID

	duration := time.Since(startTime)
	logger.InfoContext(ctx, "story created successfully",
		"story_id", storyID,
		"duration_ms", duration.Milliseconds(),
	)

	return story, nil
}

// generateContent generates the title and description for a story.
//
// Every story renders from a structured template: the result carries a
// template_key plus params the client resolves in the viewer's locale,
// alongside literal English strings kept for older clients
// (TODO(#2159) retires those once historical rows age out).
func (g *Generator) generateContent(ctx context.Context, req CreateStoryRequest) templateResult {
	logger := logging.LoggerWithContext(ctx).With(
		"operation", "generateContent",
		"story_type", req.StoryType,
	)

	out := generateFromTemplate(req)

	logger.DebugContext(ctx, "story content generated from template",
		"title_length", len(out.Title),
		"description_length", len(out.Description),
		"template_key", out.TemplateKey,
	)

	return out
}
