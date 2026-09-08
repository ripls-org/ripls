package community

import (
	"context"
	"sync"
	"testing"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/notifications/noop"
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

// setupTestService creates a service with a no-op notification provider for
// testing. It also wires the community-event bus + notification subscriber +
// stream subscriber against the new service so RecordCommunityEventAndNotify
// shim calls dispatch through the production code path. t is required so the
// publisher registration is cleaned up at test end.
func setupTestService(t *testing.T, sqlStorage *storage.ProtoSQLStorage) *Service {
	t.Helper()
	provider := noop.NewProvider(models.DevicePlatform_DEVICE_PLATFORM_ANDROID)
	notifService := notifications.NewService([]notifications.Provider{provider}, sqlStorage)

	// Mirror production wiring (#510 PR 3): build the bus first, pass it
	// into community.New, then subscribe both subscribers against the now-
	// existing service. Use nil for bucket and story creator (not needed
	// for most tests).
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	svc := New(sqlStorage, nil, notifService, bus, "test.example.com")

	notifSub := commsub.New(sqlStorage, notifService)
	notifSub.SetStreamChecker(svc.HasActiveUserStream)
	unsubNotif, err := bus.Subscribe(notifSub)
	if err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	t.Cleanup(unsubNotif)

	unsubStream, err := bus.Subscribe(svc.StreamSubscriber())
	if err != nil {
		t.Fatalf("subscribe stream subscriber: %v", err)
	}
	t.Cleanup(unsubStream)

	busBySvc.Store(svc, bus)
	t.Cleanup(func() { busBySvc.Delete(svc) })

	return svc
}

// newTestBus constructs a bus + notification subscriber for tests that
// don't build a community.Service but DO need to construct one of the
// other services (which all require a Publisher). Returns the bus so the
// test can pass it to community.New / NewWithNotificationSignal directly.
func newTestBus(t *testing.T, sqlStorage *storage.ProtoSQLStorage, notif notifications.Service) *cebus.InProcessBus {
	t.Helper()
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	notifSub := commsub.New(sqlStorage, notif)
	unsub, err := bus.Subscribe(notifSub)
	if err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	t.Cleanup(unsub)
	return bus
}

// newTestServiceWithSignal mirrors NewWithNotificationSignal + bus wiring
// the way production main.go does it. Returns the service, bus, and the
// done channel so tests that still rely on per-call done signaling can use
// it; new tests should prefer bus.Drain.
func newTestServiceWithSignal(t *testing.T, sqlStorage *storage.ProtoSQLStorage, notif notifications.Service) (*Service, *cebus.InProcessBus) {
	t.Helper()
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	svc, _ := NewWithNotificationSignal(sqlStorage, nil, notif, bus, "test.example.com")

	notifSub := commsub.New(sqlStorage, notif)
	notifSub.SetStreamChecker(svc.HasActiveUserStream)
	unsubNotif, err := bus.Subscribe(notifSub)
	if err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	t.Cleanup(unsubNotif)

	unsubStream, err := bus.Subscribe(svc.StreamSubscriber())
	if err != nil {
		t.Fatalf("subscribe stream subscriber: %v", err)
	}
	t.Cleanup(unsubStream)

	busBySvc.Store(svc, bus)
	t.Cleanup(func() { busBySvc.Delete(svc) })
	return svc, bus
}

// busBySvc lets tests recover the community-event bus that setupTestService
// registered for a given Service when they didn't capture the bus reference
// directly. Test-only.
var busBySvc sync.Map

// busFor returns the bus that setupTestService / newTestServiceWithSignal
// registered for svc. Test-only.
func busFor(t *testing.T, svc *Service) *cebus.InProcessBus {
	t.Helper()
	v, ok := busBySvc.Load(svc)
	if !ok {
		t.Fatal("busFor: no bus registered for service (did you call setupTestService?)")
	}
	return v.(*cebus.InProcessBus)
}

// drainBus waits for in-flight bus dispatch on svc to complete. Uses a 5s
// timeout to surface deadlocks rather than hang the test runner.
func drainBus(t *testing.T, svc *Service) {
	t.Helper()
	v, ok := busBySvc.Load(svc)
	if !ok {
		t.Fatal("drainBus: no bus registered for service (did you call setupTestService?)")
	}
	bus := v.(*cebus.InProcessBus)
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("drainBus: %v", err)
	}
}

// setupTestUser creates a test user in the database and returns the user ID.
func setupTestUser(t *testing.T, storage *storage.ProtoSQLStorage, email, name string) string {
	user := &models.User{
		Email: email,
		Name:  name,
		Role:  models.Role_ROLE_USER,
	}
	userID, err := storage.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return userID
}

// mintCommunityInvite mints (or fetches) the community-invite share link for
// communityID via GetOrCreateShareLink and returns the response message. It is
// the test-side replacement for the removed GetOrCreateInviteLink shim; both
// mint/return the same community-invite share link (same short code, same
// member counts).
func mintCommunityInvite(t *testing.T, ctx context.Context, service *Service, communityID string) *api.GetOrCreateShareLinkResponse {
	t.Helper()
	resp, err := service.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("mintCommunityInvite: GetOrCreateShareLink failed: %v", err)
	}
	return resp.Msg
}

// addUserToCommunity adds a user to a community using an invitation link.
// This is a test helper that uses the invitation link flow.
func addUserToCommunity(t *testing.T, service *Service, communityID, inviterUserID, newMemberUserID, inviterEmail, newMemberEmail string) {
	inviterCtx := createAuthenticatedContext(inviterUserID, inviterEmail, models.Role_ROLE_USER)
	newMemberCtx := createAuthenticatedContext(newMemberUserID, newMemberEmail, models.Role_ROLE_USER)

	// Get invitation link
	inviteLink := mintCommunityInvite(t, inviterCtx, service, communityID)

	// Accept invitation link using short_code.
	acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLink.ShortCode,
	})
	if _, err := service.AcceptInvitationLink(newMemberCtx, acceptReq); err != nil {
		t.Fatalf("Failed to accept invitation link: %v", err)
	}
}

// shareGearForTest shares gear with a community as the user authenticated in
// ctx, and fails the test if that doesn't work. It calls the same
// ShareGearToCommunity core that ShareItem drives in production, so the gear
// inherits its item-wide Lend/Give availability (default FOR_LOAN) rather than
// carrying a per-community one.
//
// Auth is the caller's responsibility in the core, so this helper takes the
// actor from ctx exactly as the RPC layer does. Tests that need to assert a
// sharing *failure* should call the core (or VerifyGearOwner) directly, and
// tests that need a specific availability should use
// shareGearForTestWithAvailability.
func shareGearForTest(t *testing.T, s *Service, ctx context.Context, gearID, communityID string) {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("shareGearForTest: context is not authenticated: %v", err)
	}
	if err := s.ShareGearToCommunity(ctx, gearID, communityID, authInfo.UserID); err != nil {
		t.Fatalf("Failed to share gear %s with community %s: %v", gearID, communityID, err)
	}
}

// shareGearForTestWithAvailability is shareGearForTest with an explicit
// Lend/Give choice, mirroring how the per-item community provisioner carries the
// creation-time availability (#2687).
func shareGearForTestWithAvailability(
	t *testing.T, s *Service, ctx context.Context, gearID, communityID string, availability models.Availability,
) {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("shareGearForTestWithAvailability: context is not authenticated: %v", err)
	}
	if err := s.ShareGearToCommunityWithAvailability(ctx, gearID, communityID, authInfo.UserID, availability); err != nil {
		t.Fatalf("Failed to share gear %s with community %s: %v", gearID, communityID, err)
	}
}

// unshareGearFrom removes gear from a community as the user authenticated in ctx
// and returns the error, so callers can assert both the success and the
// rejection paths. Drives the UnshareGearFromCommunity core behind
// CommunityService.UnshareItem — including the GEAR_UNSHARED event and the
// idempotent no-op when the gear isn't shared there.
func unshareGearFrom(t *testing.T, s *Service, ctx context.Context, gearID, communityID string) error {
	t.Helper()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		t.Fatalf("unshareGearFrom: context is not authenticated: %v", err)
	}
	return s.UnshareGearFromCommunity(ctx, gearID, communityID, authInfo.UserID)
}

// TestNew verifies the service constructor works correctly.
func TestNew(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(t, sqlStorage)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.storage != sqlStorage {
		t.Error("Expected storage to be set correctly")
	}
}
