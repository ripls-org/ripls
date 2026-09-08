package location

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// SuggestGearLocation is deprecated and no longer implemented (#2209).
//
// It previously extracted GPS from a gear photo's EXIF and reverse-geocoded
// it to suggest a location, but no client ever called it — location is always
// set explicitly via the LocationPicker plus permissioned device GPS. The
// implementation and its EXIF helpers (server/location/exif.go) were removed.
// The RPC stays declared (marked deprecated in the proto) for wire
// compatibility; this stub satisfies the generated handler interface and
// returns Unimplemented to any caller.
func (s *Service) SuggestGearLocation(
	_ context.Context,
	_ *connect.Request[api.SuggestGearLocationRequest],
) (*connect.Response[api.SuggestGearLocationResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented,
		fmt.Errorf("SuggestGearLocation is deprecated and unimplemented (#2209)"))
}
