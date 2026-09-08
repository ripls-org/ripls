package user

import (
	"testing"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestUser_FieldRoundTrip verifies that user-settable fields on
// SaveUserRequest survive the save → fetch cycle through GetUser. See
// docs/proto_conventions.md for the round-trip convention and issue
// #1144 for the motivation.
func TestUser_FieldRoundTrip(t *testing.T) {
	svc, userManager, _ := setupTestService(t)
	ctx := t.Context()

	created, err := userManager.CreateUser(ctx, "rt@example.com", "Round Trip", models.Role_ROLE_USER)
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}

	// SaveUser requires auth; attach it to the context.
	authCtx := authn.SetInfo(ctx, &auth.Info{
		UserID: created.Id,
		Email:  created.Email,
		Role:   models.Role_ROLE_USER,
	})

	input := &api.SaveUserRequest{
		UserId:                     created.Id,
		Name:                       proto.String("New Display Name"),
		PrimaryResidenceLocationId: proto.String("loc-primary"),
		OtherLocationIds:           []string{"loc-a", "loc-b"},
		MediaId:                    proto.String("media-avatar"),
		Description:                proto.String("Borrowing drills since 2026"),
		MediaIds:                   []string{"media-avatar", "media-extra"},
		PreferredTimezone:          proto.String("America/Denver"),
		PreferredLanguage:          proto.String("es"),
	}

	save := func() error {
		_, err := svc.SaveUser(authCtx, connect.NewRequest(input))
		return err
	}
	if err := save(); err != nil {
		t.Fatalf("SaveUser failed: %v", err)
	}

	noopSave := func() error { return nil }
	get := func() (*api.GetUserResponse, error) {
		resp, err := svc.GetUser(authCtx, connect.NewRequest(&api.GetUserRequest{UserId: created.Id}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	services.AssertFieldRoundTrip(t, "name", input.GetName(), noopSave,
		func() (string, error) { r, err := get(); return r.GetName(), err })
	services.AssertFieldRoundTrip(t, "primary_residence_location_id", input.GetPrimaryResidenceLocationId(), noopSave,
		func() (string, error) { r, err := get(); return r.GetPrimaryResidenceLocationId(), err })
	services.AssertFieldRoundTrip(t, "other_location_ids", input.OtherLocationIds, noopSave,
		func() ([]string, error) { r, err := get(); return r.GetOtherLocationIds(), err })
	services.AssertFieldRoundTrip(t, "description", input.GetDescription(), noopSave,
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldRoundTrip(t, "media_ids", input.MediaIds, noopSave,
		func() ([]string, error) { r, err := get(); return r.GetMediaIds(), err })
	services.AssertFieldRoundTrip(t, "media_id", input.GetMediaId(), noopSave,
		func() (string, error) { r, err := get(); return r.GetMediaId(), err })
	services.AssertFieldRoundTrip(t, "preferred_timezone", input.GetPreferredTimezone(), noopSave,
		func() (string, error) { r, err := get(); return r.GetPreferredTimezone(), err })
	services.AssertFieldRoundTrip(t, "preferred_language", input.GetPreferredLanguage(), noopSave,
		func() (string, error) { r, err := get(); return r.GetPreferredLanguage(), err })

	// Clear pass: sending proto.String("") for each optional scalar must zero the stored value.
	services.AssertFieldClear(t, "name",
		func() error {
			_, err := svc.SaveUser(authCtx, connect.NewRequest(&api.SaveUserRequest{UserId: created.Id, Name: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetName(), err })
	services.AssertFieldClear(t, "description",
		func() error {
			_, err := svc.SaveUser(authCtx, connect.NewRequest(&api.SaveUserRequest{UserId: created.Id, Description: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldClear(t, "preferred_timezone",
		func() error {
			_, err := svc.SaveUser(authCtx, connect.NewRequest(&api.SaveUserRequest{UserId: created.Id, PreferredTimezone: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetPreferredTimezone(), err })
	services.AssertFieldClear(t, "preferred_language",
		func() error {
			_, err := svc.SaveUser(authCtx, connect.NewRequest(&api.SaveUserRequest{UserId: created.Id, PreferredLanguage: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetPreferredLanguage(), err })
}
