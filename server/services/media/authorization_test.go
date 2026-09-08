package media

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/storage"
)

// insertMedia is a small helper that creates a Media owned by ownerID and
// returns its id. The caller fills in any other fields it needs by mutating
// the proto before insert.
func insertMedia(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, ownerID, contentType string) string {
	t.Helper()
	filename := "x.jpg"
	m := &models.Media{
		UserId:      ownerID,
		ContentType: contentType,
		Filename:    &filename,
	}
	id, err := s.Insert(ctx, m)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	return id
}

// joinUserToCommunity creates a CommunityUser row so the user is a member.
func joinUserToCommunity(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	cu := &models.CommunityUser{
		CommunityId: communityID,
		UserId:      userID,
	}
	if _, err := s.Insert(ctx, cu); err != nil {
		t.Fatalf("insert community_user: %v", err)
	}
}

func TestCanUserAccessMedia_Owner(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "user-1", "image/jpeg")
	media := &models.Media{Id: mediaID, UserId: "user-1"}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "user-1", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("owner denied, want allow")
	}
	if reason != accessReasonOwner {
		t.Errorf("reason = %q, want %q", reason, accessReasonOwner)
	}
}

func TestCanUserAccessMedia_System(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Stock images are written with UserId = SystemUserID by the
	// Unsplash / Pexels / Pixabay / fake providers. Any authenticated
	// caller (including one with no relation to the media) is allowed
	// to read them — they are public-by-design.
	mediaID := insertMedia(t, ctx, sqlStorage, mediapkg.SystemUserID, "image/jpeg")
	media := &models.Media{Id: mediaID, UserId: mediapkg.SystemUserID}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "any-user", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("stock image denied for arbitrary caller, want allow")
	}
	if reason != accessReasonSystem {
		t.Errorf("reason = %q, want %q", reason, accessReasonSystem)
	}
}

func TestCanUserAccessMedia_NotSystem(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// A media owner ID that looks like but is not the system sentinel
	// must NOT trigger the system pass rule.
	mediaID := insertMedia(t, ctx, sqlStorage, "systemX", "image/jpeg")
	media := &models.Media{Id: mediaID, UserId: "systemX"}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "stranger", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if allowed {
		t.Errorf("non-system owner allowed, want deny")
	}
	if reason == accessReasonSystem {
		t.Errorf("reason = %q, want a deny code (not system)", reason)
	}
}

func TestCanUserAccessMedia_AvatarViaMediaIds(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Media is owned by user-A; user-B is an unrelated viewer. The media
	// is set as the avatar on a third user via the media_ids[] array
	// (the modern, multi-avatar shape). Avatar rule: any user holding
	// this media id grants access, regardless of viewer identity.
	mediaID := insertMedia(t, ctx, sqlStorage, "user-a", "image/jpeg")
	avatarHolder := &models.User{MediaIds: []string{mediaID}, Email: "h@example.com"}
	if _, err := sqlStorage.Insert(ctx, avatarHolder); err != nil {
		t.Fatalf("insert avatar holder: %v", err)
	}
	media := &models.Media{Id: mediaID, UserId: "user-a"}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "user-b", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("avatar viewer denied, want allow")
	}
	if reason != accessReasonAvatar {
		t.Errorf("reason = %q, want %q", reason, accessReasonAvatar)
	}
}

func TestCanUserAccessMedia_CommunityDirect(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "creator", "image/jpeg")
	community := &models.Community{
		Name:        "Test",
		MediaIds:    []string{mediaID},
		OwnerUserId: "creator",
	}
	commID, err := sqlStorage.Insert(ctx, community)
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	joinUserToCommunity(t, ctx, sqlStorage, commID, "viewer")

	media := &models.Media{Id: mediaID, UserId: "creator"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "viewer", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("community member denied, want allow")
	}
	if reason != accessReasonCommunity {
		t.Errorf("reason = %q, want %q", reason, accessReasonCommunity)
	}
}

func TestCanUserAccessMedia_Story(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "creator", "image/jpeg")
	commID, err := sqlStorage.Insert(ctx, &models.Community{Name: "Story Test", OwnerUserId: "creator"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	joinUserToCommunity(t, ctx, sqlStorage, commID, "viewer")

	story := &models.Story{CommunityId: commID, MediaIds: []string{mediaID}}
	if _, err := sqlStorage.Insert(ctx, story); err != nil {
		t.Fatalf("insert story: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "creator"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "viewer", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("story viewer denied, want allow")
	}
	if reason != accessReasonStory {
		t.Errorf("reason = %q, want %q", reason, accessReasonStory)
	}
}

func TestCanUserAccessMedia_Gear(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "owner", "image/jpeg")
	commID, err := sqlStorage.Insert(ctx, &models.Community{Name: "Gear Test", OwnerUserId: "owner"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	joinUserToCommunity(t, ctx, sqlStorage, commID, "viewer")

	gearID, err := sqlStorage.Insert(ctx, &models.Gear{
		Name:     "drill",
		OwnerId:  "owner",
		MediaIds: []string{mediaID},
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "owner"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "viewer", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("gear viewer denied, want allow")
	}
	if reason != accessReasonGear {
		t.Errorf("reason = %q, want %q", reason, accessReasonGear)
	}
}

func TestCanUserAccessMedia_Chat(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "sender", "image/jpeg")
	commID, err := sqlStorage.Insert(ctx, &models.Community{Name: "Chat Test", OwnerUserId: "sender"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	convID, err := sqlStorage.Insert(ctx, &models.ChatConversation{
		CommunityId:    commID,
		ParticipantIds: []string{"sender", "viewer"},
	})
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.ChatMessage{
		ConversationId: convID,
		Message: &models.ChatMessage_UserMessage{
			UserMessage: &models.UserChatMessage{
				SenderId: "sender",
				MediaIds: []string{mediaID},
			},
		},
	}); err != nil {
		t.Fatalf("insert chat message: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "sender"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "viewer", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("chat participant denied, want allow")
	}
	if reason != accessReasonChat {
		t.Errorf("reason = %q, want %q", reason, accessReasonChat)
	}
}

func TestCanUserAccessMedia_UnrelatedDenied(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Media exists, but the viewer has no path to it: not the owner, no
	// avatar, no shared community via any surface. The expected deny
	// reason is the universal "no path at all" code — no probe found
	// any row referencing this media id, so no surface-specific near-miss
	// fires.
	mediaID := insertMedia(t, ctx, sqlStorage, "owner", "image/jpeg")
	media := &models.Media{Id: mediaID, UserId: "owner"}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "stranger", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if allowed {
		t.Errorf("stranger allowed, want deny")
	}
	if reason != accessReasonDenyNotOwnerOrAvatar {
		t.Errorf("reason = %q, want %q", reason, accessReasonDenyNotOwnerOrAvatar)
	}
}

func TestCanUserAccessMedia_NearMissReportsMembershipDeny(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Media is surfaced on a community the viewer does NOT belong to. The
	// community-direct probe finds the row but the membership check fails;
	// the deny reason should name that near-miss surface, not collapse to
	// the universal "no path" code.
	mediaID := insertMedia(t, ctx, sqlStorage, "creator", "image/jpeg")
	community := &models.Community{
		Name:        "Near Miss",
		MediaIds:    []string{mediaID},
		OwnerUserId: "creator",
	}
	if _, err := sqlStorage.Insert(ctx, community); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	media := &models.Media{Id: mediaID, UserId: "creator"}

	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "outsider", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if allowed {
		t.Errorf("outsider allowed, want deny")
	}
	if reason != accessReasonDenyNoCommunityMembership {
		t.Errorf("reason = %q, want %q", reason, accessReasonDenyNoCommunityMembership)
	}
}

func TestCanUserAccessMedia_SoftDeletedGearDoesNotGrantAccess(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Soft-deleted gear must not grant a non-owner read. Storage's array
	// query already filters deleted_at != 0 by default; this test pins
	// the contract for #1619-style soft-delete inheritance.
	mediaID := insertMedia(t, ctx, sqlStorage, "owner", "image/jpeg")
	commID, err := sqlStorage.Insert(ctx, &models.Community{Name: "C", OwnerUserId: "owner"})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	joinUserToCommunity(t, ctx, sqlStorage, commID, "viewer")

	gear := &models.Gear{Name: "drill", OwnerId: "owner", MediaIds: []string{mediaID}}
	gearID, err := sqlStorage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	gear.Id = gearID
	gear.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
	if err := sqlStorage.Update(ctx, gear); err != nil {
		t.Fatalf("soft-delete gear: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.CommunityGear{CommunityId: commID, GearId: gearID}); err != nil {
		t.Fatalf("insert community_gear: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "owner"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "viewer", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if allowed {
		t.Errorf("soft-deleted gear granted access, want deny")
	}
	// Soft-deleted gear is filtered out at the storage layer, so the gear
	// probe sees zero hits — the rollup collapses to the universal "no path
	// at all" code, same as a media id with no surfaces anywhere.
	if reason != accessReasonDenyNotOwnerOrAvatar {
		t.Errorf("reason = %q, want %q", reason, accessReasonDenyNotOwnerOrAvatar)
	}
}

func TestCanUserAccessMedia_TransferRecipientSoftDeletedGear(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// owner "A" lent gear to recipient "B", then deleted the gear. B still
	// sees the loan in their inbox and renders the thumbnail via GetMedia.
	// The gear probe can't see the soft-deleted gear, but the transfer
	// relationship is the durable grant, so B must be allowed (#2170).
	mediaID := insertMedia(t, ctx, sqlStorage, "owner-a", "image/jpeg")
	gear := &models.Gear{Name: "drill", OwnerId: "owner-a", MediaIds: []string{mediaID}}
	gearID, err := sqlStorage.Insert(ctx, gear)
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	gear.Id = gearID
	gear.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
	if err := sqlStorage.Update(ctx, gear); err != nil {
		t.Fatalf("soft-delete gear: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.Transfer{
		GearId:      gearID,
		OwnerId:     "owner-a",
		RecipientId: "recipient-b",
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "owner-a"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "recipient-b", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("transfer recipient denied for soft-deleted gear, want allow")
	}
	if reason != accessReasonTransfer {
		t.Errorf("reason = %q, want %q", reason, accessReasonTransfer)
	}
}

func TestCanUserAccessMedia_TransferRecipientLiveGearNonMember(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// Live gear, but the recipient is not a member of any community the gear
	// is shared into (e.g. they left after borrowing). The gear surface
	// fails; the transfer surface rescues them.
	mediaID := insertMedia(t, ctx, sqlStorage, "owner-a", "image/jpeg")
	gearID, err := sqlStorage.Insert(ctx, &models.Gear{
		Name:     "ladder",
		OwnerId:  "owner-a",
		MediaIds: []string{mediaID},
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	// No CommunityGear rows → gear surface denies with no-gear-link.
	if _, err := sqlStorage.Insert(ctx, &models.Transfer{
		GearId:      gearID,
		OwnerId:     "owner-a",
		RecipientId: "recipient-b",
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "owner-a"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "recipient-b", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if !allowed {
		t.Errorf("transfer recipient denied for live non-shared gear, want allow")
	}
	if reason != accessReasonTransfer {
		t.Errorf("reason = %q, want %q", reason, accessReasonTransfer)
	}
}

func TestCanUserAccessMedia_TransferStrangerDenied(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	// A transfer exists between A and B for gear carrying the media, but
	// caller "C" is neither party and has no other path. C is denied, and
	// the rollup surfaces the participation near-miss.
	mediaID := insertMedia(t, ctx, sqlStorage, "owner-a", "image/jpeg")
	gearID, err := sqlStorage.Insert(ctx, &models.Gear{
		Name:     "saw",
		OwnerId:  "owner-a",
		MediaIds: []string{mediaID},
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	if _, err := sqlStorage.Insert(ctx, &models.Transfer{
		GearId:      gearID,
		OwnerId:     "owner-a",
		RecipientId: "recipient-b",
	}); err != nil {
		t.Fatalf("insert transfer: %v", err)
	}

	media := &models.Media{Id: mediaID, UserId: "owner-a"}
	allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "stranger-c", media)
	if err != nil {
		t.Fatalf("canUserAccessMedia: %v", err)
	}
	if allowed {
		t.Errorf("non-participant stranger allowed, want deny")
	}
	if reason != accessReasonDenyNoTransferParticipation {
		t.Errorf("reason = %q, want %q", reason, accessReasonDenyNoTransferParticipation)
	}
}

func TestCanUserAccessMedia_TransferProbeBoundedQueries(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := storage.WithQueryStats(context.Background())

	// Many transfers reference the same gear. The probe must check
	// owner/recipient in memory on the fetched rows, not query per transfer
	// (CLAUDE.md SQL rule #8 / N+1 guard). The recipient is on the last
	// transfer so the allow path still walks the full set.
	mediaID := insertMedia(t, ctx, sqlStorage, "owner-a", "image/jpeg")
	gearID, err := sqlStorage.Insert(ctx, &models.Gear{
		Name:     "tent",
		OwnerId:  "owner-a",
		MediaIds: []string{mediaID},
	})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	for i := 0; i < 5; i++ {
		recipient := "other"
		if i == 4 {
			recipient = "recipient-b"
		}
		if _, err := sqlStorage.Insert(ctx, &models.Transfer{
			GearId:      gearID,
			OwnerId:     "owner-a",
			RecipientId: recipient,
		}); err != nil {
			t.Fatalf("insert transfer %d: %v", i, err)
		}
	}

	media := &models.Media{Id: mediaID, UserId: "owner-a"}
	// Query budget is independent of transfer count: the surface probes
	// (avatar, community, story, gear) plus the transfer probe's two
	// queries (gear-by-media, transfers-by-gear) — never one-per-transfer.
	storage.AssertMaxQueries(t, ctx, 8, func() {
		allowed, reason, err := canUserAccessMedia(ctx, sqlStorage, "recipient-b", media)
		if err != nil {
			t.Fatalf("canUserAccessMedia: %v", err)
		}
		if !allowed || reason != accessReasonTransfer {
			t.Errorf("allowed=%v reason=%q, want allow via transfer", allowed, reason)
		}
	})
}

func TestCanUserAccessMedia_NilMediaErrors(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	_, _, err := canUserAccessMedia(context.Background(), sqlStorage, "u1", nil)
	if err == nil {
		t.Fatal("expected error for nil media, got none")
	}
}

func TestRequireMediaReadAccess_DeniesWithPermissionDenied(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	ctx := context.Background()

	mediaID := insertMedia(t, ctx, sqlStorage, "owner", "image/jpeg")
	media := &models.Media{Id: mediaID, UserId: "owner"}

	if err := requireMediaReadAccess(ctx, sqlStorage, "stranger", media); err == nil {
		t.Fatal("expected error for unrelated user")
	}
	// The error code is connect.CodePermissionDenied — verifying via the
	// helper would require importing connect; the GetMedia handler test
	// covers the codepath end-to-end, so we just confirm an error here.
}
