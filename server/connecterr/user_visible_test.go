package connecterr

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// extractLocalizedDetail walks the Connect error's details and
// returns the first attached LocalizedErrorDetail (and the wire
// code). Mirrors what the client does at receive time.
func extractLocalizedDetail(t *testing.T, err error) (connect.Code, *api.LocalizedErrorDetail) {
	t.Helper()
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	for _, d := range cerr.Details() {
		msg, mErr := d.Value()
		if mErr != nil {
			continue
		}
		if got, ok := msg.(*api.LocalizedErrorDetail); ok {
			return cerr.Code(), got
		}
	}
	return cerr.Code(), nil
}

func TestUserVisible_NoParams(t *testing.T) {
	err := UserVisible(
		context.Background(),
		connect.CodePermissionDenied,
		"experience_owner_required_for_delete",
		"only the owner can delete this event",
		nil,
	)
	if err == nil {
		t.Fatal("UserVisible returned nil")
	}

	code, detail := extractLocalizedDetail(t, err)
	if code != connect.CodePermissionDenied {
		t.Errorf("connect code = %v; want %v", code, connect.CodePermissionDenied)
	}
	if detail == nil {
		t.Fatal("no LocalizedErrorDetail attached")
	}
	if detail.GetCode() != "experience_owner_required_for_delete" {
		t.Errorf("detail.Code = %q; want %q", detail.GetCode(), "experience_owner_required_for_delete")
	}
	if len(detail.GetParams()) != 0 {
		t.Errorf("detail.Params should be empty, got %v", detail.GetParams())
	}

	// The fallback message must remain visible on the wire so
	// older clients (which do not yet read LocalizedErrorDetail)
	// still display readable text.
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *connect.Error")
	}
	if cerr.Message() != "only the owner can delete this event" {
		t.Errorf("public message = %q; want fallback verbatim", cerr.Message())
	}
}

func TestUserVisible_WithParams(t *testing.T) {
	params := map[string]string{
		"eventName": "Saturday potluck",
		"actorName": "Alice",
	}
	err := UserVisible(
		context.Background(),
		connect.CodeAlreadyExists,
		"experience_already_shared",
		"event already shared to this community",
		params,
	)

	_, detail := extractLocalizedDetail(t, err)
	if detail == nil {
		t.Fatal("no LocalizedErrorDetail attached")
	}
	got := detail.GetParams()
	if got["eventName"] != "Saturday potluck" {
		t.Errorf("params[eventName] = %q; want %q", got["eventName"], "Saturday potluck")
	}
	if got["actorName"] != "Alice" {
		t.Errorf("params[actorName] = %q; want %q", got["actorName"], "Alice")
	}
}

func TestInternal_AttachesLocalizedDetail(t *testing.T) {
	// connecterr.Internal always carries a LocalizedErrorDetail with
	// the internal_error_generic code so locale-aware clients render
	// rpcErrorInternal in the user's language. Older clients keep
	// seeing the fixed public string on the wire.
	err := Internal(context.Background(), "TestOp", errors.New("storage timeout"))
	code, detail := extractLocalizedDetail(t, err)
	if code != connect.CodeInternal {
		t.Errorf("connect code = %v; want CodeInternal", code)
	}
	if detail == nil {
		t.Fatal("no LocalizedErrorDetail attached")
	}
	if detail.GetCode() != "internal_error_generic" {
		t.Errorf("detail.Code = %q; want %q", detail.GetCode(), "internal_error_generic")
	}

	// Wire-compatible: the public message stays the fixed English
	// string so pre-Phase-4 clients render readable text.
	var cerr *connect.Error
	if !errors.As(err, &cerr) {
		t.Fatalf("expected *connect.Error")
	}
	if cerr.Message() != "internal server error" {
		t.Errorf("public message = %q; want fixed internal string", cerr.Message())
	}
}

func TestInternal_NilErrorStillAttachesDetail(t *testing.T) {
	// Calling Internal with nil err is a programming bug, but the
	// returned error still satisfies the locale-rendering contract.
	err := Internal(context.Background(), "TestOp", nil)
	_, detail := extractLocalizedDetail(t, err)
	if detail == nil || detail.GetCode() != "internal_error_generic" {
		t.Errorf("nil-err path must still attach the LocalizedErrorDetail; got %v", detail)
	}
}

func TestUserVisible_PreservesConnectCode(t *testing.T) {
	cases := []connect.Code{
		connect.CodePermissionDenied,
		connect.CodeFailedPrecondition,
		connect.CodeNotFound,
		connect.CodeAlreadyExists,
		connect.CodeInvalidArgument,
	}
	for _, code := range cases {
		t.Run(code.String(), func(t *testing.T) {
			err := UserVisible(context.Background(), code, "test_code", "fallback", nil)
			if got := connect.CodeOf(err); got != code {
				t.Errorf("CodeOf = %v; want %v", got, code)
			}
		})
	}
}
