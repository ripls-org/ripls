package community

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/storage"
)

// GenCommunity finds a relevant background image for a community from a user
// prompt. It does NOT save the community to the database and does NOT generate
// any name/description text — the client supplies those and calls
// CreateCommunity to persist.
//
// New clients should prefer StreamGenCommunity to surface the media_ready
// event as it resolves, instead of blocking for the full round-trip. The
// Provider-level GenerateCommunityContent is a drain over the streaming
// variant, so the AI schema is owned by schemas.go either way.
func (s *Service) GenCommunity(
	ctx context.Context,
	req *connect.Request[api.GenCommunityRequest],
) (*connect.Response[api.GenCommunityResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Prompt == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("prompt cannot be empty"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "generating community from prompt")

	// 1. Generate community content using AI
	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	generation, err := s.aiProvider.GenerateCommunityContent(ctx, req.Msg.Prompt, req.Msg.Region)
	if err != nil {
		logger.ErrorContext(ctx, "failed to generate community image keywords", "error", err)
		return nil, connecterr.Internal(ctx, "GenCommunity", err)
	}

	// 2. Search and download image from stock imagery provider (for preview)
	var mediaID string
	if s.stockImageryProvider != nil && len(generation.SearchKeywords) > 0 {
		mediaID, err = s.findAndStoreStockImage(ctx, generation.SearchKeywords)
		if err != nil {
			// Log but don't fail - image is optional
			logger.WarnContext(ctx, "failed to get stock image", "error", err)
		}
	}

	var mediaIDs []string
	if mediaID != "" {
		mediaIDs = []string{mediaID}
	}

	logger.InfoContext(ctx, "generated community background image",
		"keyword_count", len(generation.SearchKeywords),
		"media_count", len(mediaIDs))

	// Return the suggested background image WITHOUT saving to database.
	// Client supplies name/description and calls CreateCommunity to persist.
	return connect.NewResponse(&api.GenCommunityResponse{
		MediaIds: mediaIDs,
	}), nil
}

// findAndStoreStockImage searches for a stock image using keywords and creates a copy for the community.
// Returns the media ID of the stored image, or empty string if no image found.
// Uses the copy-on-use pattern to support cascade deletion.
func (s *Service) findAndStoreStockImage(ctx context.Context, keywords []string) (string, error) {
	if s.stockImageryProvider == nil {
		return "", fmt.Errorf("stock imagery provider not configured")
	}

	if len(keywords) == 0 {
		return "", fmt.Errorf("no keywords provided")
	}

	logger := logging.LoggerWithContext(ctx)

	// Join keywords into a single query
	query := strings.Join(keywords, " ")
	logger.DebugContext(ctx, "searching for stock image", "query", query)

	// Get stock image from provider
	stockImage, err := s.stockImageryProvider.GetStockImage(ctx, query, nil)
	if err != nil {
		return "", fmt.Errorf("failed to get stock image: %w", err)
	}

	// Create a copy for this community (enables cascade deletion and attribution tracking)
	mediaID, err := storage.CopyStockImageForUser(
		ctx,
		s.storage,
		s.bucket,
		stockImage,
		"system",
		"community-stock.jpg",
		fmt.Sprintf("Stock image for community: %s", query),
	)
	if err != nil {
		return "", fmt.Errorf("failed to copy stock image: %w", err)
	}

	logger.InfoContext(ctx, "stored stock image copy for community",
		"media_id", mediaID,
		"stock_image_id", stockImage.Id)

	return mediaID, nil
}
