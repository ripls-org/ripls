package gear

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestGear_FieldRoundTrip verifies that user-provided fields on
// SaveGearRequest survive the save → fetch cycle through GetGear. See
// docs/proto_conventions.md for the round-trip convention and issue
// #1144 for the motivation.
func TestGear_FieldRoundTrip(t *testing.T) {
	testStorage := setupTestStorage(t)
	mockBucket := &services.MockBucketStorage{}
	svc := New(testStorage, mockBucket)

	// GetGear fetches owner info, so the auth user must exist in storage.
	owner := &models.User{Email: "rt@example.com", Name: "RT User"}
	ownerID, err := testStorage.Insert(t.Context(), owner)
	if err != nil {
		t.Fatalf("failed to seed owner: %v", err)
	}
	ctx := createAuthenticatedContext(ownerID, "rt@example.com", models.Role_ROLE_USER)

	input := &api.SaveGearRequest{
		Name:        proto.String("Cordless Drill"),
		Description: proto.String("18V Makita cordless drill with two batteries"),
		MediaIds:    []string{"media-a", "media-b"},
		LocationId:  proto.String("location-garage"),
	}

	var savedID string
	save := func() error {
		resp, err := svc.SaveGear(ctx, connect.NewRequest(input))
		if err != nil {
			return err
		}
		savedID = resp.Msg.Id
		return nil
	}
	if err := save(); err != nil {
		t.Fatalf("initial SaveGear failed: %v", err)
	}

	noopSave := func() error { return nil }
	get := func() (*api.GetGearResponse, error) {
		resp, err := svc.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: savedID}))
		if err != nil {
			return nil, err
		}
		return resp.Msg, nil
	}

	services.AssertFieldRoundTrip(t, "name", input.GetName(), noopSave,
		func() (string, error) { r, err := get(); return r.GetName(), err })
	services.AssertFieldRoundTrip(t, "description", input.GetDescription(), noopSave,
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldRoundTrip(t, "media_ids", input.MediaIds, noopSave,
		func() ([]string, error) { r, err := get(); return r.GetMediaIds(), err })
	services.AssertFieldRoundTrip(t, "location_id", input.GetLocationId(), noopSave,
		func() (string, error) { r, err := get(); return r.GetLocationId(), err })

	// Clear pass: sending proto.String("") for each optional scalar must zero the stored value.
	services.AssertFieldClear(t, "name",
		func() error {
			_, err := svc.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{Id: savedID, Name: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetName(), err })
	services.AssertFieldClear(t, "description",
		func() error {
			_, err := svc.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{Id: savedID, Description: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldClear(t, "location_id",
		func() error {
			_, err := svc.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{Id: savedID, LocationId: proto.String("")}))
			return err
		},
		func() (string, error) { r, err := get(); return r.GetLocationId(), err })
}
