package experience

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

// realTestJPEG returns a small, valid JPEG. The webpage-image upload path now
// sanitizes (decodes + re-encodes) fetched images, so test fixtures must be
// genuinely decodable, not hand-written byte stubs.
func realTestJPEG() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// createAuthenticatedContext creates a context with authentication info.
func createAuthenticatedContext(userID, email string, role models.Role) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   role,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

// setupTestStorage creates a PostgreSQL database for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

// setupTestBucket creates a local bucket storage for testing.
func setupTestBucket(t *testing.T) storage.BucketStorage {
	tempDir := t.TempDir()
	bucket, err := storage.NewLocalBucketStorage(tempDir, "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create test bucket: %v", err)
	}
	return bucket
}

// setupTestService creates a complete test service with storage, bucket, and mock notification service.
func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage, storage.BucketStorage) {
	sqlStorage := setupTestStorage(t)
	bucket := setupTestBucket(t)
	mockNotification := notifications.NewMockService()

	// Mirror production wiring (#510 PR 3): build the bus + notification
	// subscriber first, then construct the service with the bus.
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	notifSub := commsub.New(sqlStorage, mockNotification)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	service := New(sqlStorage, bucket, mockNotification, bus)
	// Wire estimator config so impact estimation is active in tests.
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("Failed to load estimator config: %v", err)
	}
	service.SetEstimatorConfig(cfg)
	return service, sqlStorage, bucket
}

// createTestUser creates a user in the database for testing.
func createTestUser(t *testing.T, storage *storage.ProtoSQLStorage, userID, email, name string) {
	user := &models.User{
		Id:    userID,
		Email: email,
		Name:  name,
	}
	_, err := storage.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
}

// createTestCommunity creates a community in the database for testing and returns its ID.
func createTestCommunity(t *testing.T, storage *storage.ProtoSQLStorage, name, creatorID string) string {
	community := &models.Community{
		Name:        name,
		CreatorId:   creatorID,
		OwnerUserId: creatorID,
	}

	communityID, err := storage.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	return communityID
}

// createTestCommunityMembership adds a user to a community.
func createTestCommunityMembership(t *testing.T, storage *storage.ProtoSQLStorage, communityID, userID string) {
	membership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
		InviterId:   userID, // For simplicity, user invites themselves
	}

	_, err := storage.Insert(context.Background(), membership)
	if err != nil {
		t.Fatalf("Failed to create test community membership: %v", err)
	}
}

// shareExperienceForTest shares an experience with a community as the user
// authenticated in ctx, and fails the test if that doesn't work. It calls the
// same ShareExperienceToCommunity core that CommunityService.ShareItem drives in
// production, so setup exercises the live sharing path.
//
// Auth is the caller's responsibility in the core, so this helper takes the
// actor from ctx exactly as the RPC layer does. Tests that need to assert a
// sharing *failure* should call the core directly rather than use this.
func shareExperienceForTest(t *testing.T, s *Service, ctx context.Context, experienceID, communityID string) {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("shareExperienceForTest: context is not authenticated: %v", err)
	}
	if err := s.ShareExperienceToCommunity(ctx, experienceID, communityID, authInfo.UserID); err != nil {
		t.Fatalf("Failed to share experience %s with community %s: %v", experienceID, communityID, err)
	}
}

// newMockAIProvider creates a MockProvider with preconfigured experience generation responses.
func newMockAIProvider(textResp, imageResp, webpageResp *ai.ExperienceGeneration) *ai.MockProvider {
	mock := ai.NewMockProvider()
	if textResp != nil {
		mock.GenerateExperienceFromTextFunc = func(_ context.Context, _, _, _ string) (*ai.ExperienceGeneration, error) {
			return textResp, nil
		}
	}
	if imageResp != nil {
		mock.GenerateExperienceFromImageFunc = func(_ context.Context, _ *ai.DetectionImage, _, _, _ string) (*ai.ExperienceGeneration, error) {
			return imageResp, nil
		}
	}
	if webpageResp != nil {
		mock.GenerateExperienceFromWebpageFunc = func(_ context.Context, _, _, _, _, _ string) (*ai.ExperienceGeneration, error) {
			return webpageResp, nil
		}
	}
	return mock
}
