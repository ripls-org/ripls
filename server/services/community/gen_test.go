package community

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

func TestService_GenCommunity(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	srv := setupTestService(t, sqlStorage)

	// Configure mock AI provider for tests that need it
	mockAI := ai.NewMockProvider()
	srv.SetAIProvider(mockAI)

	// Create test user
	userID := setupTestUser(t, sqlStorage, "test@example.com", "Test User")
	ctx := createAuthenticatedContext(userID, "test@example.com", models.Role_ROLE_USER)

	t.Run("success - returns image suggestion without persisting", func(t *testing.T) {
		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A community for rock climbers in Boulder",
			Region: "Boulder, CO",
		})

		resp, err := srv.GenCommunity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// GenCommunity no longer generates any name/description text — it only
		// suggests a background image. With no stock imagery provider
		// configured, there is no media to return.
		assert.Empty(t, resp.Msg.MediaIds, "no media without a stock imagery provider")

		// Verify no community was saved to the database.
		communities, err := sqlStorage.QueryByField(ctx, "creator_id", userID, &models.Community{})
		require.NoError(t, err)
		assert.Empty(t, communities, "GenCommunity must not persist a community")
	})

	t.Run("error - empty prompt", func(t *testing.T) {
		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "",
		})

		resp, err := srv.GenCommunity(ctx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	})

	t.Run("error - no authentication", func(t *testing.T) {
		unauthCtx := context.Background()
		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A community for hikers",
		})

		resp, err := srv.GenCommunity(unauthCtx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("error - AI provider not configured", func(t *testing.T) {
		// Create a service without AI provider
		srvNoAI := setupTestService(t, sqlStorage)

		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A community for cyclists",
		})

		resp, err := srvNoAI.GenCommunity(ctx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
	})
}

func TestService_GenCommunity_WithStockImagery(t *testing.T) {
	sqlStorage := setupTestStorage(t)

	// Create bucket storage for stock imagery
	tmpDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	require.NoError(t, err)

	// Create service with bucket storage
	notifService := notifications.NewMockService()
	bus := newTestBus(t, sqlStorage, notifService)
	srv := New(sqlStorage, bucket, notifService, bus, "test.example.com")

	// Configure mock AI provider and stock imagery provider
	mockAI := ai.NewMockProvider()
	srv.SetAIProvider(mockAI)

	fakeProvider := media.NewFakeProvider(sqlStorage, bucket)
	srv.SetStockImageryProvider(fakeProvider)

	// Create test user
	userID := setupTestUser(t, sqlStorage, "test@example.com", "Test User")
	ctx := createAuthenticatedContext(userID, "test@example.com", models.Role_ROLE_USER)

	t.Run("success - includes media_id when stock imagery is available", func(t *testing.T) {
		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A photography community",
			Region: "San Francisco, CA",
		})

		resp, err := srv.GenCommunity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Stock imagery provider is configured and mock AI returns search keywords
		require.NotEmpty(t, resp.Msg.MediaIds, "media_ids should be set when stock imagery is available")

		// Verify the media copy has source_stock_image_id set for provenance tracking
		mediaRecord := &models.Media{}
		err = sqlStorage.GetByID(ctx, resp.Msg.MediaIds[0], mediaRecord)
		require.NoError(t, err)

		assert.NotEmpty(t, mediaRecord.SourceStockImageId, "media should have source_stock_image_id set")

		// Verify source_stock_image_id references a valid StockImage record
		stockImage := &models.StockImage{}
		err = sqlStorage.GetByID(ctx, mediaRecord.GetSourceStockImageId(), stockImage)
		require.NoError(t, err)
		assert.NotEmpty(t, stockImage.MediaId, "stock image should have media_id set")
	})

	t.Run("success - gracefully handles stock imagery errors", func(t *testing.T) {
		// Configure provider to simulate errors
		errorProvider := media.NewFakeProviderWithConfig(sqlStorage, bucket, media.FakeProviderConfig{
			SimulateNoResults: true,
		})
		srv.SetStockImageryProvider(errorProvider)

		// Even if stock imagery fails, the request should succeed
		req := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A community that will fail stock imagery search",
		})

		resp, err := srv.GenCommunity(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)

		// Generation should still succeed, just without media.
		assert.Empty(t, resp.Msg.MediaIds, "media_ids should be empty when stock imagery fails")
	})
}

func TestService_GenCommunity_Integration(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	srv := setupTestService(t, sqlStorage)

	// Configure mock AI provider
	mockAI := ai.NewMockProvider()
	srv.SetAIProvider(mockAI)

	// Create test user
	userID := setupTestUser(t, sqlStorage, "test@example.com", "Test User")
	ctx := createAuthenticatedContext(userID, "test@example.com", models.Role_ROLE_USER)

	t.Run("full flow - generate image then create with user text", func(t *testing.T) {
		// Step 1: Generate a background-image suggestion.
		genReq := connect.NewRequest(&api.GenCommunityRequest{
			Prompt: "A book club for science fiction fans",
			Region: "Seattle, WA",
		})

		genResp, err := srv.GenCommunity(ctx, genReq)
		require.NoError(t, err)
		require.NotNil(t, genResp)

		// Step 2: The client supplies name + description; only the media (if any)
		// comes from generation.
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name:        "Sci-Fi Book Club",
			Description: "For science fiction fans",
			MediaIds:    genResp.Msg.MediaIds,
		})

		createResp, err := srv.CreateCommunity(ctx, createReq)
		require.NoError(t, err)
		require.NotNil(t, createResp)
		assert.NotEmpty(t, createResp.Msg.Id)

		// Verify community was saved with the user-supplied text.
		community := &models.Community{}
		err = sqlStorage.GetByID(ctx, createResp.Msg.Id, community)
		require.NoError(t, err)
		assert.Equal(t, "Sci-Fi Book Club", community.Name)
		assert.Equal(t, "For science fiction fans", community.Description)
		assert.Equal(t, genResp.Msg.MediaIds, community.MediaIds)
	})

	t.Run("create with empty description round-trips", func(t *testing.T) {
		// Description is optional now; an empty description must round-trip
		// through CreateCommunity → GetCommunity unchanged.
		createReq := connect.NewRequest(&api.CreateCommunityRequest{
			Name:        "Trail Runners",
			Description: "",
		})

		createResp, err := srv.CreateCommunity(ctx, createReq)
		require.NoError(t, err)
		require.NotEmpty(t, createResp.Msg.Id)

		getResp, err := srv.GetCommunity(ctx, connect.NewRequest(&api.GetCommunityRequest{
			Id: createResp.Msg.Id,
		}))
		require.NoError(t, err)
		assert.Equal(t, "Trail Runners", getResp.Msg.Name)
		assert.Empty(t, getResp.Msg.Description, "empty description must round-trip")
	})

	t.Run("multiple generations without creating", func(t *testing.T) {
		// Count communities before generating
		communitiesBefore, err := sqlStorage.QueryByField(ctx, "creator_id", userID, &models.Community{})
		require.NoError(t, err)
		countBefore := len(communitiesBefore)

		// User can call GenCommunity multiple times before creating
		for i := 0; i < 3; i++ {
			req := connect.NewRequest(&api.GenCommunityRequest{
				Prompt: "A running club",
			})

			resp, err := srv.GenCommunity(ctx, req)
			require.NoError(t, err)
			require.NotNil(t, resp)
		}

		// Verify no NEW communities were created (count should be the same)
		communitiesAfter, err := sqlStorage.QueryByField(ctx, "creator_id", userID, &models.Community{})
		require.NoError(t, err)
		countAfter := len(communitiesAfter)
		assert.Equal(t, countBefore, countAfter, "GenCommunity should not create any communities")
	})
}
