package request

import (
	"context"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/storage"
)

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

// setupTestServiceWithNotifications creates a request service with notification signal for testing.
// Returns the service, storage, mock notification service, notification done channel, and stock imagery done channel.
// Tests that call SubmitRequest without media_ids should drain the stock imagery channel before completing.
func setupTestServiceWithNotifications(t *testing.T) (*Service, *storage.ProtoSQLStorage, *notifications.MockService, chan struct{}, chan struct{}) {
	testStorage := setupTestStorage(t)

	// Create local bucket storage for testing
	tmpDir := t.TempDir()
	mockBucket, err := storage.NewLocalBucketStorage(tmpDir+"/bucket", "http://localhost:8080")
	if err != nil {
		t.Fatalf("Failed to create bucket storage: %v", err)
	}

	mockNotif := notifications.NewMockService()
	fakeProvider := media.NewFakeProvider(testStorage, mockBucket)
	mockAIProvider := ai.NewMockProvider() // Use mock AI provider for tests
	systemMessageWriter := chat.NewSystemMessageWriter(testStorage, nil)

	// Mirror production wiring (#510 PR 3): build the bus + notification
	// subscriber first, then construct the service with the bus.
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	notifSub := commsub.New(testStorage, mockNotif)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	service, notifDone, stockImageryDone := NewForTesting(testStorage, mockBucket, mockNotif, bus, fakeProvider, mockAIProvider, systemMessageWriter)
	// Subscribe the done-tap AFTER the production subscribers so the
	// legacy <-notifDone pattern continues to resolve.
	tap := &doneTapSubscriber{ch: notifDone}
	if _, err := bus.Subscribe(tap); err != nil {
		t.Fatalf("subscribe done-tap subscriber: %v", err)
	}

	// Wire estimator config so impact estimation is active in tests.
	cfg, err2 := estimator.LoadConfigFromEmbed()
	if err2 != nil {
		t.Fatalf("Failed to load estimator config: %v", err2)
	}
	service.SetEstimatorConfig(cfg)
	return service, testStorage, mockNotif, notifDone, stockImageryDone
}

// doneTapSubscriber is a no-op subscriber that signals a chan struct{} on
// every dispatched event. Used to bridge legacy <-notifDone test patterns
// to the bus dispatch model.
type doneTapSubscriber struct {
	ch chan struct{}
}

func (d *doneTapSubscriber) Name() string { return "request_test_done_tap" }

func (d *doneTapSubscriber) Handle(_ context.Context, _ *cebus.PublishedEvent) error {
	select {
	case d.ch <- struct{}{}:
	default:
		// Channel full — tests that don't drain explicitly accept that
		// later signals are dropped. Matches the legacy semantics where
		// the buffered done channel could overflow under bursty dispatch.
	}
	return nil
}

// setupTestUser creates a user and returns the ID.
func setupTestUser(t *testing.T, storage *storage.ProtoSQLStorage, name, email string) string {
	ctx := context.Background()
	user := &models.User{
		Name:  name,
		Email: email,
	}

	userID, err := storage.Insert(ctx, user)
	if err != nil {
		t.Fatalf("Failed to insert test user: %v", err)
	}

	return userID
}

// setupCommunityWithMembers creates a community and adds members to it.
func setupCommunityWithMembers(t *testing.T, storage *storage.ProtoSQLStorage, creatorID string, memberIDs ...string) string {
	ctx := context.Background()

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   creatorID,
		OwnerUserId: creatorID,
	}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Add creator as member
	creatorMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      creatorID,
		InviterId:   creatorID,
	}
	_, err = storage.Insert(ctx, creatorMembership)
	if err != nil {
		t.Fatalf("Failed to add creator membership: %v", err)
	}

	// Add other members
	for _, memberID := range memberIDs {
		if memberID == creatorID {
			continue // Already added
		}
		membership := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      memberID,
			InviterId:   creatorID,
		}
		_, err = storage.Insert(ctx, membership)
		if err != nil {
			t.Fatalf("Failed to add member %s: %v", memberID, err)
		}
	}

	return communityID
}

// shareRequestInto shares a request into a community as the user authenticated
// in ctx, and fails the test if that doesn't work. It calls the same
// ShareRequestToCommunity core that CommunityService.ShareItem drives in
// production, so setup exercises the live sharing path.
//
// Auth is the caller's responsibility in the core, so this helper takes the
// actor from ctx exactly as the RPC layer does. Tests that need to assert a
// sharing *failure* should call the core (or VerifyRequestOwner) directly.
func shareRequestInto(t *testing.T, ctx context.Context, service *Service, requestID, communityID string) {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("shareRequestInto: context is not authenticated: %v", err)
	}
	if err := service.ShareRequestToCommunity(ctx, requestID, communityID, authInfo.UserID); err != nil {
		t.Fatalf("shareRequestInto(%s): %v", communityID, err)
	}
}

// unshareRequestFrom removes a request from a community as the user
// authenticated in ctx and returns the error, so callers can assert both the
// success and the rejection paths (owner-only, last-community, never-shared).
// Drives the UnshareRequestFromCommunity core behind
// CommunityService.UnshareItem.
func unshareRequestFrom(t *testing.T, ctx context.Context, service *Service, requestID, communityID string) error {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("unshareRequestFrom: context is not authenticated: %v", err)
	}
	return service.UnshareRequestFromCommunity(ctx, requestID, communityID, authInfo.UserID)
}

// TestNew tests the New constructor.
func TestNew(t *testing.T) {
	service, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != testStorage {
		t.Error("Expected storage to be set correctly")
	}
}
