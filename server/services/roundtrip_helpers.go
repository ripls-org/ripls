package services

import (
	"reflect"
	"testing"
)

// AssertFieldRoundTrip asserts that a field value survives a save-then-fetch
// round-trip through the service boundary. It calls save, then fetch, and
// fails the test if fetch returns an error or if the returned value is not
// deeply equal to want.
//
// The helper asserts an API contract: any field the client can set on a
// save-style RPC and expects to read back on a read-style RPC should round
// trip equal. It doesn't know or care how (or whether) the server stores
// the value — it exercises the API surface only. A round-trip test also
// catches the class of bug where a new API field is wired into the save
// path but forgotten on one of the read paths, because it forces both
// paths to be exercised. See docs/proto_conventions.md and issue #1142
// for the motivating incident.
//
// Typical usage:
//
//	services.AssertFieldRoundTrip(t, "external_place_id", "mapbox:poi.123",
//	    func() error {
//	        _, err := svc.SaveLocation(ctx, connect.NewRequest(&api.SaveLocationRequest{
//	            Name:            "Central Park",
//	            ExternalPlaceId: "mapbox:poi.123",
//	            // ...
//	        }))
//	        return err
//	    },
//	    func() (string, error) {
//	        resp, err := svc.GetLocation(ctx, connect.NewRequest(&api.GetLocationRequest{
//	            Id: savedID,
//	        }))
//	        if err != nil {
//	            return "", err
//	        }
//	        return resp.Msg.Location.ExternalPlaceId, nil
//	    },
//	)
//
// The comparison uses reflect.DeepEqual, so scalars, slices, maps, and
// pointer-to-scalar (e.g. optional proto fields) all work. If you need
// custom comparison (e.g. ignoring a sub-field), assert directly instead
// of using this helper.
func AssertFieldRoundTrip[T any](
	t testing.TB,
	fieldName string,
	want T,
	save func() error,
	fetch func() (T, error),
) {
	t.Helper()
	if err := save(); err != nil {
		t.Fatalf("round-trip save for field %q failed: %v", fieldName, err)
	}
	got, err := fetch()
	if err != nil {
		t.Fatalf("round-trip fetch for field %q failed: %v", fieldName, err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("field %q did not survive round-trip\n  got:  %+v\n  want: %+v", fieldName, got, want)
	}
}

// AssertFieldClear asserts that setting an optional scalar field to its zero
// value ("" for strings, 0 for numbers) via save is reflected by fetch
// returning the zero value. This verifies that "send empty string → clears
// the field" works correctly for optional fields whose presence is tracked by
// the proto `optional` keyword.
//
// Typical usage:
//
//	services.AssertFieldClear(t, "description",
//	    func() error {
//	        _, err := svc.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
//	            Id:          savedID,
//	            Description: proto.String(""),
//	        }))
//	        return err
//	    },
//	    func() (string, error) {
//	        resp, err := svc.GetGear(ctx, connect.NewRequest(&api.GetGearRequest{Id: savedID}))
//	        if err != nil {
//	            return "", err
//	        }
//	        return resp.Msg.GetDescription(), nil
//	    },
//	)
func AssertFieldClear[T any](
	t testing.TB,
	fieldName string,
	save func() error,
	fetch func() (T, error),
) {
	t.Helper()
	if err := save(); err != nil {
		t.Fatalf("clear-field save for field %q failed: %v", fieldName, err)
	}
	got, err := fetch()
	if err != nil {
		t.Fatalf("clear-field fetch for field %q failed: %v", fieldName, err)
	}
	var zero T
	if !reflect.DeepEqual(got, zero) {
		t.Errorf("field %q was not cleared: got %+v, want zero value %+v", fieldName, got, zero)
	}
}
