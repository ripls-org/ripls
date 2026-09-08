package transfer

import (
	"context"
	"testing"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/chat"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
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

// setupTestServiceWithNotifications creates a transfer service with notification signal for testing.
func setupTestServiceWithNotifications(t *testing.T) (*Service, *storage.ProtoSQLStorage, *notifications.MockService, chan struct{}) {
	testStorage := setupTestStorage(t)
	mockNotif := notifications.NewMockService()
	systemMessageWriter := chat.NewSystemMessageWriter(testStorage, nil)

	// Mirror production wiring (#510 PR 3): build the bus + notification
	// subscriber first, then construct the service with the bus.
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	notifSub := commsub.New(testStorage, mockNotif)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	service, done := NewWithNotificationSignal(testStorage, mockNotif, bus, systemMessageWriter)
	// Subscribe the done-tap AFTER the production subscribers so the
	// legacy <-done pattern continues to resolve. New tests should prefer
	// bus.Drain.
	tap := &doneTapSubscriber{ch: done}
	if _, err := bus.Subscribe(tap); err != nil {
		t.Fatalf("subscribe done-tap subscriber: %v", err)
	}

	// Wire estimator config so impact estimation is active in tests.
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("Failed to load estimator config: %v", err)
	}
	service.SetEstimatorConfig(cfg)
	return service, testStorage, mockNotif, done
}

// doneTapSubscriber is a no-op subscriber that signals a chan struct{} on
// every dispatched event. Bridges legacy <-done test patterns to the bus
// dispatch model.
type doneTapSubscriber struct {
	ch chan struct{}
}

func (d *doneTapSubscriber) Name() string { return "transfer_test_done_tap" }

func (d *doneTapSubscriber) Handle(_ context.Context, _ *cebus.PublishedEvent) error {
	select {
	case d.ch <- struct{}{}:
	default:
		// Channel full — accept dropping later signals. Matches the
		// legacy semantics where the buffered done channel could overflow.
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

// setupTestGear creates test gear and returns its ID.
func setupTestGear(t *testing.T, storage *storage.ProtoSQLStorage, ownerID string, state models.GearState) string {
	ctx := context.Background()
	gear := &models.Gear{
		Name:        "Test Drill",
		Description: "A test drill",
		OwnerId:     ownerID,
		State:       state,
	}

	gearID, err := storage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("Failed to insert test gear: %v", err)
	}

	return gearID
}

// setupCommunityAndShareGear creates a community, adds owner and borrower as members, and shares the gear.
// Returns the community ID.
func setupCommunityAndShareGear(t *testing.T, storage *storage.ProtoSQLStorage, ownerID, borrowerID, gearID string, availability models.Availability) string {
	ctx := context.Background()

	// Create community
	community := &models.Community{
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	}
	communityID, err := storage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}

	// Add owner as member (creator)
	ownerMembership := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      ownerID,
		InviterId:   ownerID,
	}
	_, err = storage.Insert(ctx, ownerMembership)
	if err != nil {
		t.Fatalf("Failed to add owner membership: %v", err)
	}

	// Add borrower as member (skip if borrower == owner, already added above).
	if borrowerID != ownerID {
		borrowerMembership := &models.CommunityUser{
			CommunityId: communityID,
			UserId:      borrowerID,
			InviterId:   ownerID,
		}
		_, err = storage.Insert(ctx, borrowerMembership)
		if err != nil {
			t.Fatalf("Failed to add borrower membership: %v", err)
		}
	}

	// Share gear with community
	communityGear := &models.CommunityGear{
		CommunityId:  communityID,
		GearId:       gearID,
		Availability: availability,
	}
	_, err = storage.Insert(ctx, communityGear)
	if err != nil {
		t.Fatalf("Failed to share gear: %v", err)
	}

	return communityID
}

func TestNew(t *testing.T) {
	service, testStorage, _, _ := setupTestServiceWithNotifications(t)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != testStorage {
		t.Error("Expected storage to be set correctly")
	}
}
