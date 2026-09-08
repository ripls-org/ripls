package integration_tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/notifications/noop"
	"go.ripls.org/ripls/server/notifications/sms"
	"go.ripls.org/ripls/server/smsoptout"
	"go.ripls.org/ripls/server/storage"
)

// off_app_fallback_test.go is the integration proof for the off-app
// notification channel (#2492): when a recipient has no active app device,
// NotifyUser must reach them off-app — SMS for a phone handle, email otherwise —
// and the message must carry a *resolvable* `/go/{code}` short link pointing at
// the notification's entity. It exercises the real path end to end:
// NotifyUser → device lookup (real DB, no devices) → dispatchOffApp → the real
// smsoptout + quiethours stores → the real community ShortLinkCodeForEntity
// resolver (real community service + DB) → the channel senders.
//
// It drives NotifyUser directly rather than through the community event bus
// because the bus detaches dispatch onto a fresh context (deriveDispatchCtx
// keeps only request_id/user_id, not the simulation clock), so a bus-driven
// test could not deterministically control the quiet-hours window. Who gets
// notified for a given event (the subscriber's recipient resolution) is covered
// by the community_subscriber tests; this test owns the off-app *delivery*.

const (
	offAppInviteePhone = "+14155557654"
	offAppInviteeEmail = "invitee-offapp@example.com"
	offAppDenver       = "America/Denver"
)

// offAppHarness is a fully-wired off-app notification stack over a fresh test
// DB: the real notification service (with mock SMS + mock email senders and the
// real community share-link resolver), plus a seeded host, a deviceless invitee
// (phone + email on file), a community the invitee belongs to, and an
// experience shared into it. The notification under test references that
// experience so the deep link has a real target to resolve.
type offAppHarness struct {
	db           *storage.ProtoSQLStorage
	notif        notifications.Service
	mockSMS      *sms.MockSMSSender
	mockEmail    *email.MockEmailService
	inviteeID    string
	communityID  string
	experienceID string
	notification *models.Notification
}

func newOffAppHarness(t *testing.T) *offAppHarness {
	t.Helper()
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	ctx := context.Background()

	bucket, err := storage.NewLocalBucketStorage(t.TempDir(), "http://localhost:8080")
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}

	mockSMS := &sms.MockSMSSender{}
	mockEmail := &email.MockEmailService{}

	// The community service (which owns the resolver) needs the notification
	// service, and the notification service needs the resolver — the same
	// late-binding main.go uses. Resolve through a closure assigned once the
	// community service exists.
	var resolve notifications.ShareLinkResolver
	notif := notifications.NewService(
		[]notifications.Provider{noop.NewProvider(models.DevicePlatform_DEVICE_PLATFORM_IOS)},
		db,
		notifications.WithSMS(notifications.SMSChannel{
			Enabled:    true,
			Sender:     mockSMS,
			AppBaseURL: "https://test.example.com",
		}),
		notifications.WithOffAppEmail(notifications.OffAppEmailConfig{
			Enabled:     true,
			Sender:      mockEmail,
			AppBaseURL:  "https://test.example.com",
			UnsubSecret: []byte("test-unsub-secret"),
		}),
		notifications.WithShareLinkResolver(func(ctx context.Context, c, e, g, r string) (string, error) {
			return resolve(ctx, c, e, g, r)
		}),
	)
	communitySvc, _ := newTestCommunityService(t, db, bucket, notif)
	resolve = communitySvc.ShortLinkCodeForEntity

	hostID := uuid.New().String()
	inviteeID := uuid.New().String()
	insertUser(t, db, hostID, "host-offapp@example.com", "Host")

	// The invitee is deviceless (no UserDevice row) with both a phone and an
	// email on file, so the channel is chosen by handle, not availability.
	phone := offAppInviteePhone
	if _, err := db.Insert(ctx, &models.User{
		Id:                inviteeID,
		Name:              "Deviceless Invitee",
		Email:             offAppInviteeEmail,
		PhoneNumber:       &phone,
		PreferredTimezone: offAppDenver,
	}); err != nil {
		t.Fatalf("seed invitee: %v", err)
	}

	communityID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Community{
		Id: communityID, Name: "Block Party Crew", CreatorId: hostID, OwnerUserId: hostID,
	}); err != nil {
		t.Fatalf("seed community: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityUser{
		Id: uuid.New().String(), CommunityId: communityID, UserId: inviteeID, InviterId: hostID,
	}); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	experienceID := uuid.New().String()
	if _, err := db.Insert(ctx, &models.Experience{
		Id: experienceID, Name: "Block Party", OwnerId: hostID,
		State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}); err != nil {
		t.Fatalf("seed experience: %v", err)
	}
	if _, err := db.Insert(ctx, &models.CommunityExperience{
		Id: uuid.New().String(), CommunityId: communityID, ExperienceId: experienceID,
	}); err != nil {
		t.Fatalf("seed community-experience: %v", err)
	}

	notification := &models.Notification{
		Title: "New event: Block Party",
		Body:  "Block Party was just added to Block Party Crew.",
		Payload: &models.Notification_CommunityEvent{
			CommunityEvent: &models.CommunityEventPayload{
				CommunityId:  communityID,
				ExperienceId: &experienceID,
				EventType:    "EXPERIENCE_CREATED",
			},
		},
	}

	return &offAppHarness{
		db: db, notif: notif, mockSMS: mockSMS, mockEmail: mockEmail,
		inviteeID: inviteeID, communityID: communityID,
		experienceID: experienceID, notification: notification,
	}
}

// daytime returns a context whose simulation clock sits at noon in the
// invitee's timezone — safely outside the 22:00–07:00 quiet-hours window.
func daytime() context.Context {
	denver, _ := time.LoadLocation(offAppDenver)
	return clock.WithSimulationTime(context.Background(), time.Date(2026, 6, 26, 12, 0, 0, 0, denver))
}

// shortLinkCodeIn extracts the `/go/{code}` short code from a message body, or
// fails the test if none is present.
func shortLinkCodeIn(t *testing.T, body string) string {
	t.Helper()
	const marker = "/go/"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("expected a /go short link in the message, got: %q", body)
	}
	rest := body[i+len(marker):]
	if j := strings.IndexAny(rest, " \n\t"); j >= 0 {
		rest = rest[:j]
	}
	if rest == "" {
		t.Fatalf("empty short code after /go/ in: %q", body)
	}
	return rest
}

// assertResolvesToExperience looks the short code up in ShareLink storage and
// asserts it targets the expected experience — proving the link in the message
// is real and points at the right entity, not just well-formed.
func (h *offAppHarness) assertResolvesToExperience(t *testing.T, code string) {
	t.Helper()
	rows, err := h.db.QueryByField(context.Background(), "short_code", code, &models.ShareLink{})
	if err != nil {
		t.Fatalf("look up share link %q: %v", code, err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 share link for code %q, got %d", code, len(rows))
	}
	link := rows[0].(*models.ShareLink)
	if got := link.GetExperienceId(); got != h.experienceID {
		t.Errorf("share link targets experience %q, want %q", got, h.experienceID)
	}
	if link.CommunityId != h.communityID {
		t.Errorf("share link community %q, want %q", link.CommunityId, h.communityID)
	}
}

// TestOffAppFallback_PhoneRecipientGetsSMSWithResolvableLink is the headline
// case: a deviceless, opted-in, phone-bearing recipient is texted, and the SMS
// carries a /go link that resolves to the notification's experience.
func TestOffAppFallback_PhoneRecipientGetsSMSWithResolvableLink(t *testing.T) {
	h := newOffAppHarness(t)

	if err := h.notif.NotifyUser(daytime(), h.inviteeID, h.notification); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}

	msgs := h.mockSMS.Messages()
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 SMS, got %d", len(msgs))
	}
	if msgs[0].To != offAppInviteePhone {
		t.Errorf("SMS to %q, want %q", msgs[0].To, offAppInviteePhone)
	}
	if !strings.Contains(msgs[0].Body, "Reply STOP") {
		t.Errorf("SMS missing STOP/HELP footer: %q", msgs[0].Body)
	}
	h.assertResolvesToExperience(t, shortLinkCodeIn(t, msgs[0].Body))

	if got := len(h.mockEmail.Notifications); got != 0 {
		t.Errorf("expected 0 emails when SMS sends, got %d", got)
	}
}

// TestOffAppFallback_OptedOutNumberFallsBackToEmail proves an opted-out phone
// degrades to email — and the email carries the same resolvable /go link.
func TestOffAppFallback_OptedOutNumberFallsBackToEmail(t *testing.T) {
	h := newOffAppHarness(t)
	if err := smsoptout.RecordOptOut(context.Background(), h.db, offAppInviteePhone); err != nil {
		t.Fatalf("RecordOptOut: %v", err)
	}

	if err := h.notif.NotifyUser(daytime(), h.inviteeID, h.notification); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}

	if got := len(h.mockSMS.Messages()); got != 0 {
		t.Errorf("expected 0 SMS to an opted-out number, got %d", got)
	}
	if len(h.mockEmail.Notifications) != 1 {
		t.Fatalf("expected exactly 1 email fallback, got %d", len(h.mockEmail.Notifications))
	}
	in := h.mockEmail.Notifications[0]
	if in.ToEmail != offAppInviteeEmail {
		t.Errorf("email to %q, want %q", in.ToEmail, offAppInviteeEmail)
	}
	h.assertResolvesToExperience(t, shortLinkCodeIn(t, in.ActionURL))
}

// TestOffAppFallback_QuietHoursFallsBackToEmail proves a text that would land
// during the recipient's quiet hours is sent as email instead.
func TestOffAppFallback_QuietHoursFallsBackToEmail(t *testing.T) {
	h := newOffAppHarness(t)
	denver, _ := time.LoadLocation(offAppDenver)
	nightCtx := clock.WithSimulationTime(context.Background(), time.Date(2026, 6, 26, 3, 0, 0, 0, denver))

	if err := h.notif.NotifyUser(nightCtx, h.inviteeID, h.notification); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}

	if got := len(h.mockSMS.Messages()); got != 0 {
		t.Errorf("expected 0 SMS during quiet hours, got %d", got)
	}
	if got := len(h.mockEmail.Notifications); got != 1 {
		t.Errorf("expected exactly 1 email fallback during quiet hours, got %d", got)
	}
}

// TestOffAppFallback_DeviceUserGetsPushNotOffApp proves a recipient WITH an
// active device is pushed (handled by the provider), and no off-app message is
// sent — the off-app channel is strictly the no-device fallback.
func TestOffAppFallback_DeviceUserGetsPushNotOffApp(t *testing.T) {
	h := newOffAppHarness(t)
	if _, err := h.db.Insert(context.Background(), &models.UserDevice{
		Id: uuid.New().String(), UserId: h.inviteeID,
		DeviceToken: "tok-" + uuid.New().String(),
		Platform:    models.DevicePlatform_DEVICE_PLATFORM_IOS,
	}); err != nil {
		t.Fatalf("register device: %v", err)
	}

	if err := h.notif.NotifyUser(daytime(), h.inviteeID, h.notification); err != nil {
		t.Fatalf("NotifyUser: %v", err)
	}

	if got := len(h.mockSMS.Messages()); got != 0 {
		t.Errorf("device user must not be texted, got %d SMS", got)
	}
	if got := len(h.mockEmail.Notifications); got != 0 {
		t.Errorf("device user must not be emailed off-app, got %d emails", got)
	}
}
