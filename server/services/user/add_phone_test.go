package user

import (
	"context"
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// mockPhoneTokenVerifier implements auth.PhoneTokenVerifier for testing.
type mockPhoneTokenVerifier struct {
	phoneNumber string
	err         error
}

func (m *mockPhoneTokenVerifier) VerifyPhoneToken(_ context.Context, _ string) (string, error) {
	return m.phoneNumber, m.err
}

// setupPhoneUserService builds a user service wired with a mock phone verifier
// (and no bus — promotion join events are inserted directly without fan-out).
func setupPhoneUserService(t *testing.T, mock auth.PhoneTokenVerifier) (*Service, *auth.UserManager, *storage.ProtoSQLStorage) {
	t.Helper()
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	userManager := auth.NewUserManager(sqlStorage)
	service := New(userManager, sqlStorage, mock, cebus.Publisher(nil))
	return service, userManager, sqlStorage
}

// insertUserWithPhone inserts a real user account already carrying a phone.
func insertUserWithPhone(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, email, phone string, method models.AuthMethod) string {
	t.Helper()
	now := time.Now().Unix()
	u := &models.User{
		Id:          uuid.New().String(),
		Email:       email,
		Name:        "Has Phone",
		PhoneNumber: &phone,
		CreatedAt:   now,
		UpdatedAt:   now,
		Role:        models.Role_ROLE_USER,
		AuthMethod:  method,
	}
	if _, err := s.Insert(ctx, u); err != nil {
		t.Fatalf("insert user with phone: %v", err)
	}
	return u.Id
}

func TestService_AddPhoneNumber(t *testing.T) {
	const phone = "+15551234567"

	t.Run("attaches phone additively, leaving auth_method unchanged", func(t *testing.T) {
		service, userManager, _ := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()

		u, err := userManager.CreateUser(ctx, "emailer@example.com", "Emailer", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		// CreateUser leaves auth_method unspecified; set it to email/password to
		// model a real email-primary account.
		u.AuthMethod = models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD
		if err := userManager.UpdateUser(ctx, u); err != nil {
			t.Fatalf("seed auth method: %v", err)
		}

		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)
		resp, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if err != nil {
			t.Fatalf("AddPhoneNumber: %v", err)
		}
		if resp.Msg.PhoneNumber != phone {
			t.Errorf("response phone = %q, want %q", resp.Msg.PhoneNumber, phone)
		}

		got, err := userManager.GetUserByID(ctx, u.Id)
		if err != nil {
			t.Fatalf("reload user: %v", err)
		}
		if got.GetPhoneNumber() != phone {
			t.Errorf("stored phone = %q, want %q", got.GetPhoneNumber(), phone)
		}
		if got.AuthMethod != models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD {
			t.Errorf("auth_method = %v, want EMAIL_PASSWORD (additive must not switch it)", got.AuthMethod)
		}
	})

	t.Run("idempotent: re-attaching the same verified phone succeeds", func(t *testing.T) {
		service, userManager, _ := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()
		u, err := userManager.CreateUser(ctx, "idem@example.com", "Idem", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)
		req := &api.AddPhoneNumberRequest{FirebaseIdToken: "valid-token"}

		if _, err := service.AddPhoneNumber(authCtx, connect.NewRequest(req)); err != nil {
			t.Fatalf("first AddPhoneNumber: %v", err)
		}
		if _, err := service.AddPhoneNumber(authCtx, connect.NewRequest(req)); err != nil {
			t.Fatalf("second AddPhoneNumber (should be idempotent): %v", err)
		}

		got, _ := userManager.GetUserByID(ctx, u.Id)
		if got.GetPhoneNumber() != phone {
			t.Errorf("stored phone = %q, want %q", got.GetPhoneNumber(), phone)
		}
	})

	t.Run("rejects a phone already on a different account", func(t *testing.T) {
		service, userManager, sqlStorage := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()

		// Another account already owns this phone.
		insertUserWithPhone(t, ctx, sqlStorage, "owner@example.com", phone, models.AuthMethod_AUTH_METHOD_PHONE)

		u, err := userManager.CreateUser(ctx, "claimer@example.com", "Claimer", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create user: %v", err)
		}
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		_, err = service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if err == nil {
			t.Fatal("expected AlreadyExists for a phone on another account")
		}
		if connect.CodeOf(err) != connect.CodeAlreadyExists {
			t.Errorf("code = %v, want AlreadyExists", connect.CodeOf(err))
		}
		// The claimer's account must be untouched.
		got, _ := userManager.GetUserByID(ctx, u.Id)
		if got.GetPhoneNumber() != "" {
			t.Errorf("claimer phone = %q, want empty (rejected)", got.GetPhoneNumber())
		}
	})

	t.Run("replaces an existing different phone (change-phone)", func(t *testing.T) {
		const oldPhone = "+15550001111"
		service, userManager, sqlStorage := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()

		// Account already carries a different (old) phone.
		id := insertUserWithPhone(t, ctx, sqlStorage, "changer@example.com", oldPhone, models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD)
		authCtx := createAuthenticatedContext(id, "changer@example.com", models.Role_ROLE_USER)

		resp, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if err != nil {
			t.Fatalf("AddPhoneNumber (change): %v", err)
		}
		if resp.Msg.PhoneNumber != phone {
			t.Errorf("response phone = %q, want %q", resp.Msg.PhoneNumber, phone)
		}
		got, _ := userManager.GetUserByID(ctx, id)
		if got.GetPhoneNumber() != phone {
			t.Errorf("stored phone = %q, want %q (new replaces old)", got.GetPhoneNumber(), phone)
		}
	})

	t.Run("unconfigured phone auth returns Unimplemented", func(t *testing.T) {
		service, userManager, _ := setupTestService(t) // nil phoneAuth
		ctx := context.Background()
		u, _ := userManager.CreateUser(ctx, "noauth@example.com", "NoAuth", models.Role_ROLE_USER)
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		_, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Errorf("code = %v, want Unimplemented", connect.CodeOf(err))
		}
	})

	t.Run("typed-nil phone verifier returns Unimplemented", func(t *testing.T) {
		// A typed nil (*FirebaseAuth)(nil) wrapped in the interface is non-nil,
		// so the guard must reject it rather than panic on VerifyPhoneToken.
		service, userManager, _ := setupPhoneUserService(t, nil)
		var typedNil *auth.FirebaseAuth
		service.phoneAuth = typedNil
		ctx := context.Background()
		u, _ := userManager.CreateUser(ctx, "typednil@example.com", "TypedNil", models.Role_ROLE_USER)
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		_, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if connect.CodeOf(err) != connect.CodeUnimplemented {
			t.Errorf("code = %v, want Unimplemented", connect.CodeOf(err))
		}
	})

	t.Run("unauthenticated caller is rejected", func(t *testing.T) {
		service, _, _ := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		_, err := service.AddPhoneNumber(context.Background(), connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("empty firebase token is rejected", func(t *testing.T) {
		service, userManager, _ := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()
		u, _ := userManager.CreateUser(ctx, "blank@example.com", "Blank", models.Role_ROLE_USER)
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		_, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "",
		}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
		}
	})

	t.Run("invalid firebase token surfaces Unauthenticated", func(t *testing.T) {
		service, userManager, _ := setupPhoneUserService(t, &mockPhoneTokenVerifier{err: fmt.Errorf("bad token")})
		ctx := context.Background()
		u, _ := userManager.CreateUser(ctx, "badtok@example.com", "BadTok", models.Role_ROLE_USER)
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		_, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "bad-token",
		}))
		if connect.CodeOf(err) != connect.CodeUnauthenticated {
			t.Errorf("code = %v, want Unauthenticated", connect.CodeOf(err))
		}
	})

	t.Run("promotes a phone-keyed provisional across communities", func(t *testing.T) {
		service, userManager, sqlStorage := setupPhoneUserService(t, &mockPhoneTokenVerifier{phoneNumber: phone})
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}
		comm := insertTestCommunity(t, ctx, sqlStorage, inviter.Id)
		prov := insertPhoneProvisional(t, ctx, sqlStorage, comm.Id, inviter.Id, "Pat", phone)
		exp := insertTestExperience(t, ctx, sqlStorage)
		rsvp := insertProvisionalRSVP(t, ctx, sqlStorage, exp, comm.Id, prov)

		// The caller is an email account NOT yet in the community.
		u, err := userManager.CreateUser(ctx, "pat@example.com", "Pat", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create caller: %v", err)
		}
		authCtx := createAuthenticatedContext(u.Id, u.Email, u.Role)

		if _, err := service.AddPhoneNumber(authCtx, connect.NewRequest(&api.AddPhoneNumberRequest{
			FirebaseIdToken: "valid-token",
		})); err != nil {
			t.Fatalf("AddPhoneNumber: %v", err)
		}

		// Caller is now a member of the provisional's community.
		isMember, err := auth.IsMemberOfCommunity(ctx, sqlStorage, comm.Id, u.Id)
		if err != nil {
			t.Fatalf("IsMemberOfCommunity: %v", err)
		}
		if !isMember {
			t.Error("caller should be a member of the community after promotion")
		}
		// Provisional claimed by the caller.
		p := &models.ProvisionalUser{}
		if err := sqlStorage.GetByID(ctx, prov, p); err != nil {
			t.Fatalf("get provisional: %v", err)
		}
		if p.ClaimedByUserId == nil || *p.ClaimedByUserId != u.Id {
			t.Errorf("provisional claimed_by = %v, want %s", p.ClaimedByUserId, u.Id)
		}
		// RSVP migrated to the caller.
		r := &models.ExperienceRSVP{}
		if err := sqlStorage.GetByID(ctx, rsvp, r); err != nil {
			t.Fatalf("get rsvp: %v", err)
		}
		if r.UserId != u.Id || r.ProvisionalUserId != nil {
			t.Errorf("rsvp user_id = %q (prov=%v), want %q with no provisional ref", r.UserId, r.ProvisionalUserId, u.Id)
		}
	})
}

// TestService_GetUser_SelfPhone covers the #1142-class read-path: phone_number
// must appear on a self-fetch and never on a non-self fetch.
func TestService_GetUser_SelfPhone(t *testing.T) {
	const phone = "+15559990000"
	service, userManager, sqlStorage := setupTestService(t)
	ctx := context.Background()

	ownerID := insertUserWithPhone(t, ctx, sqlStorage, "self@example.com", phone, models.AuthMethod_AUTH_METHOD_PHONE)
	other, err := userManager.CreateUser(ctx, "other@example.com", "Other", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	// Self-fetch exposes the phone.
	selfCtx := createAuthenticatedContext(ownerID, "self@example.com", models.Role_ROLE_USER)
	selfResp, err := service.GetUser(selfCtx, connect.NewRequest(&api.GetUserRequest{UserId: ownerID}))
	if err != nil {
		t.Fatalf("self GetUser: %v", err)
	}
	if selfResp.Msg.PhoneNumber != phone {
		t.Errorf("self phone = %q, want %q", selfResp.Msg.PhoneNumber, phone)
	}

	// Non-self fetch never exposes it.
	otherCtx := createAuthenticatedContext(other.Id, other.Email, other.Role)
	otherResp, err := service.GetUser(otherCtx, connect.NewRequest(&api.GetUserRequest{UserId: ownerID}))
	if err != nil {
		t.Fatalf("non-self GetUser: %v", err)
	}
	if otherResp.Msg.PhoneNumber != "" {
		t.Errorf("non-self phone = %q, want empty (PII must not leak)", otherResp.Msg.PhoneNumber)
	}
}

// TestService_SaveUser_CannotSetPhone guards the security invariant behind the
// relaxed PhoneLogin gate: SaveUser must never write User.phone_number, so the
// only writers stay OTP-gated (PhoneRegister, AddPhoneNumber). SaveUserRequest
// has no phone field, so this asserts an existing phone survives a SaveUser and
// no SaveUser path can introduce one.
func TestService_SaveUser_CannotSetPhone(t *testing.T) {
	const phone = "+15557778888"
	service, _, sqlStorage := setupTestService(t)
	ctx := context.Background()

	id := insertUserWithPhone(t, ctx, sqlStorage, "saver@example.com", phone, models.AuthMethod_AUTH_METHOD_PHONE)
	authCtx := createAuthenticatedContext(id, "saver@example.com", models.Role_ROLE_USER)

	if _, err := service.SaveUser(authCtx, connect.NewRequest(&api.SaveUserRequest{
		UserId: id,
		Name:   proto.String("Renamed"),
	})); err != nil {
		t.Fatalf("SaveUser: %v", err)
	}

	got, err := sqlStorage.QueryByField(ctx, "phone_number", phone, &models.User{})
	if err != nil {
		t.Fatalf("query by phone: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("phone holders = %d, want exactly 1 (SaveUser must neither clear nor duplicate the phone)", len(got))
	}
	if u := got[0].(*models.User); u.Name != "Renamed" {
		t.Errorf("name = %q, want Renamed (the SaveUser actually applied)", u.Name)
	}
}

// insertTestCommunity creates a minimal community owned by ownerID.
func insertTestCommunity(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, ownerID string) *models.Community {
	t.Helper()
	now := time.Now().Unix()
	c := &models.Community{
		Id:               uuid.New().String(),
		Name:             "Test Community",
		CreatorId:        ownerID,
		OwnerUserId:      ownerID,
		CreatedAtUnixSec: now,
		UpdatedAtUnixSec: now,
	}
	if _, err := s.Insert(ctx, c); err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return c
}

// insertPhoneProvisional creates a phone-keyed provisional placeholder.
func insertPhoneProvisional(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, communityID, creatorID, name, phone string) string {
	t.Helper()
	prov := &models.ProvisionalUser{
		Id:              uuid.New().String(),
		Name:            name,
		CommunityId:     communityID,
		CreatedByUserId: creatorID,
		Contact:         &models.ProvisionalUser_PhoneNumber{PhoneNumber: phone},
	}
	if _, err := s.Insert(ctx, prov); err != nil {
		t.Fatalf("insert phone provisional: %v", err)
	}
	return prov.Id
}

// insertTestExperience creates a minimal active experience.
func insertTestExperience(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage) string {
	t.Helper()
	exp := &models.Experience{
		Id:    uuid.New().String(),
		Name:  "Test Experience",
		State: models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
	}
	if _, err := s.Insert(ctx, exp); err != nil {
		t.Fatalf("insert experience: %v", err)
	}
	return exp.Id
}

// insertProvisionalRSVP attaches a provisional RSVP to an experience.
func insertProvisionalRSVP(t *testing.T, ctx context.Context, s *storage.ProtoSQLStorage, experienceID, communityID, provisionalUserID string) string {
	t.Helper()
	provID := provisionalUserID
	rsvp := &models.ExperienceRSVP{
		Id:                uuid.New().String(),
		ExperienceId:      experienceID,
		CommunityId:       communityID,
		ProvisionalUserId: &provID,
		Intention:         models.RSVPIntention_RSVP_INTENTION_YES,
	}
	if _, err := s.Insert(ctx, rsvp); err != nil {
		t.Fatalf("insert provisional RSVP: %v", err)
	}
	return rsvp.Id
}
