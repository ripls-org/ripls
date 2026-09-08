package login

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// TestService_EmailRegister_PromotesProvisionalUsers covers EMAIL-1b: registering
// an email promotes provisional placeholders seeded for that email across all
// communities (the email analog of TestService_PhoneRegister_PromotesProvisionalUsers).
//
// Since #2571 promotion requires *proven* ownership of the address, so these
// register through the code flow. The password path no longer promotes — that
// is asserted separately below.
func TestService_EmailRegister_PromotesProvisionalUsers(t *testing.T) {
	const inviteeEmail = "invitee-promote@example.com"

	t.Run("promotes provisional placeholders across communities", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter-em@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}

		// The user registers through commA's link; commB is a separate community
		// they were also invited to as an email-keyed provisional placeholder.
		commA := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		commB := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, commA.Id, inviter.Id)

		provA := insertEmailProvisional(t, ctx, sqlStorage, commA.Id, inviter.Id, "Pat", inviteeEmail)
		provB := insertEmailProvisional(t, ctx, sqlStorage, commB.Id, inviter.Id, "Pat", inviteeEmail)

		expA := insertTestExperience(t, ctx, sqlStorage)
		expB := insertTestExperience(t, ctx, sqlStorage)
		rsvpA := insertProvisionalRSVP(t, ctx, sqlStorage, expA, commA.Id, provA)
		rsvpB := insertProvisionalRSVP(t, ctx, sqlStorage, expB, commB.Id, provB)

		resp, err := service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:           inviteeEmail,
			Name:            "Pat",
			EmailProofToken: proveEmail(t, ctx, service, inviteeEmail),
			ShortCode:       invitation.ShortCode,
		}))
		if err != nil {
			t.Fatalf("EmailRegister: %v", err)
		}
		userID := resp.Msg.User.Id

		// Member of BOTH communities: commA via the link, commB via promotion.
		for _, c := range []string{commA.Id, commB.Id} {
			isMember, err := auth.IsMemberOfCommunity(ctx, sqlStorage, c, userID)
			if err != nil {
				t.Fatalf("IsMemberOfCommunity(%s): %v", c, err)
			}
			if !isMember {
				t.Errorf("user should be a member of community %s after promotion", c)
			}
		}

		// Both provisional placeholders claimed by the new account.
		for _, id := range []string{provA, provB} {
			p := &models.ProvisionalUser{}
			if err := sqlStorage.GetByID(ctx, id, p); err != nil {
				t.Fatalf("get provisional %s: %v", id, err)
			}
			if p.ClaimedByUserId == nil || *p.ClaimedByUserId != userID {
				t.Errorf("provisional %s claimed_by = %v, want %s", id, p.ClaimedByUserId, userID)
			}
		}

		// Both RSVPs migrated to the real user.
		for _, id := range []string{rsvpA, rsvpB} {
			r := &models.ExperienceRSVP{}
			if err := sqlStorage.GetByID(ctx, id, r); err != nil {
				t.Fatalf("get rsvp %s: %v", id, err)
			}
			if r.UserId != userID {
				t.Errorf("rsvp %s user_id = %q, want %q", id, r.UserId, userID)
			}
			if r.ProvisionalUserId != nil {
				t.Errorf("rsvp %s still references provisional user", id)
			}
		}
	})

	t.Run("idempotent when the invite link is tied to the provisional user", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter-em2@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}
		comm := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		prov := insertEmailProvisional(t, ctx, sqlStorage, comm.Id, inviter.Id, "Pat", inviteeEmail)

		// Link tied to the provisional user: completeRegistration merges + joins,
		// then promotion finds the same email match — it must not double-join.
		invitation := createTestInvitation(t, ctx, sqlStorage, comm.Id, inviter.Id)
		invitation.ProvisionalUserId = &prov
		if err := sqlStorage.Update(ctx, invitation); err != nil {
			t.Fatalf("update invitation: %v", err)
		}

		resp, err := service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:           inviteeEmail,
			Name:            "Pat",
			EmailProofToken: proveEmail(t, ctx, service, inviteeEmail),
			ShortCode:       invitation.ShortCode,
		}))
		if err != nil {
			t.Fatalf("EmailRegister: %v", err)
		}
		userID := resp.Msg.User.Id

		// Exactly one membership row — the link join and the promotion join must
		// not produce a duplicate.
		members, err := sqlStorage.QueryByFields(ctx, map[string]any{
			"community_id": comm.Id,
			"user_id":      userID,
		}, &models.CommunityUser{})
		if err != nil {
			t.Fatalf("query memberships: %v", err)
		}
		if len(members) != 1 {
			t.Errorf("got %d membership rows, want exactly 1 (idempotent join)", len(members))
		}
	})

	// The #2571 fix, now enforced a step earlier. A password proves nothing
	// about the address, so registering one must not absorb the placeholders
	// keyed to it — otherwise anyone who knows an invitee's address can
	// register it first and inherit their identity across every community they
	// were invited to.
	//
	// #2864 closed the branch outright: registration will not accept a password
	// at all, so the unproven registrant never gets an account to promote. The
	// assertion is correspondingly stronger — not "registers but claims
	// nothing", but "does not register".
	t.Run("does not register at all when ownership was not proven", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter-em3@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}

		// The shadowing shape: the attacker registers through commA's open
		// link, while the victim's placeholder sits in commB.
		commA := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		commB := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, commA.Id, inviter.Id)
		prov := insertEmailProvisional(t, ctx, sqlStorage, commB.Id, inviter.Id, "Pat", inviteeEmail)

		_, err = service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:     inviteeEmail,
			Name:      "Not Pat",
			Password:  "password123",
			ShortCode: invitation.ShortCode,
		}))
		if err == nil {
			t.Fatal("a registration that proved nothing about the address was accepted")
		}
		if got := connect.CodeOf(err); got != connect.CodeInvalidArgument {
			t.Errorf("expected InvalidArgument, got %v: %v", got, err)
		}

		// No account exists, so nothing could have been promoted into it.
		if u, err := userManager.GetUserByEmail(ctx, inviteeEmail); err != nil {
			t.Fatalf("GetUserByEmail: %v", err)
		} else if u != nil {
			t.Errorf("an unproven registration created an account for %s", inviteeEmail)
		}

		p := &models.ProvisionalUser{}
		if err := sqlStorage.GetByID(ctx, prov, p); err != nil {
			t.Fatalf("get provisional: %v", err)
		}
		if p.ClaimedByUserId != nil {
			t.Errorf("an unverified registration claimed a provisional placeholder (claimed_by = %v)", p.ClaimedByUserId)
		}
	})
}

// insertEmailProvisional inserts an email-keyed provisional user and returns its ID.
func insertEmailProvisional(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, communityID, creatorID, name, email string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		Id:              uuid.New().String(),
		Name:            name,
		CommunityId:     communityID,
		CreatedByUserId: creatorID,
		Contact:         &models.ProvisionalUser_Email{Email: email},
	}
	if _, err := sqlStorage.Insert(ctx, prov); err != nil {
		t.Fatalf("insert email provisional: %v", err)
	}
	return prov.Id
}
