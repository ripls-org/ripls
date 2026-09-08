package user

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/l10n"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/timezone"
)

// Service implements the UserService RPC interface.
type Service struct {
	userManager  *auth.UserManager
	storage      *storage.ProtoSQLStorage
	storyStorage *storage.StoryStorage
	// phoneAuth verifies Firebase phone OTP tokens for AddPhoneNumber.
	// Optional: nil (or a typed-nil verifier) disables the AddPhoneNumber RPC.
	phoneAuth auth.PhoneTokenVerifier
	// bus publishes the community-join event when AddPhoneNumber promotes a
	// phone-keyed provisional. May be nil in tests (event is then inserted
	// directly without notification fan-out).
	bus cebus.Publisher
}

// New creates a new user service.
func New(userManager *auth.UserManager, sqlStorage *storage.ProtoSQLStorage, phoneAuth auth.PhoneTokenVerifier, bus cebus.Publisher) *Service {
	return &Service{
		userManager:  userManager,
		storage:      sqlStorage,
		storyStorage: storage.NewStoryStorage(sqlStorage),
		phoneAuth:    phoneAuth,
		bus:          bus,
	}
}

// GetUser retrieves user information by user ID.
//
// The response is scoped to the caller's relationship with the target. A
// self-fetch (caller == target) returns the full profile, including the PII
// fields the account-settings UI needs: email and the residence / other
// location IDs. Any other authenticated caller receives only the display
// fields (name, avatar, bio, preferred timezone) — the same identity surface
// already exposed app-wide via the denormalized user loader — and never the
// email or location IDs. This prevents a caller who holds an enumerable
// user_id from harvesting (name, email, home location) profiles for arbitrary
// users (#2138).
func (s *Service) GetUser(
	ctx context.Context,
	req *connect.Request[api.GetUserRequest],
) (*connect.Response[api.GetUserResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("user_id is required"))
	}

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	user, err := s.userManager.GetUserByID(ctx, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("user not found: %w", err))
	}

	isSelf := authInfo.UserID == user.Id

	// target_user_id (not user_id, which LoggerWithContext injects as the
	// authenticated caller) identifies the requested user.
	logging.LoggerWithContext(ctx).With(
		"operation", "GetUser",
		"target_user_id", user.Id,
		"self_view", isSelf,
	).DebugContext(ctx, "get user")

	// Display fields are returned to any authenticated caller.
	response := &api.GetUserResponse{
		UserId:            user.Id,
		Name:              user.Name,
		MediaId:           services.PrimaryAvatarMediaID(user),
		Description:       user.Description,
		MediaIds:          user.MediaIds,
		PreferredTimezone: user.PreferredTimezone,
		PreferredLanguage: user.GetPreferredLanguage(),
	}

	// Email, phone, and location IDs are PII: returned only on self-fetch. The
	// account-settings UI reads phone_number to show whether a phone is attached
	// (#2596).
	if isSelf {
		response.Email = user.Email
		response.PhoneNumber = user.GetPhoneNumber()
		response.PrimaryResidenceLocationId = user.PrimaryResidenceLocationId
		response.OtherLocationIds = user.OtherLocationIds
	}

	return connect.NewResponse(response), nil
}

// SaveUser updates user information (name and locations).
func (s *Service) SaveUser(
	ctx context.Context,
	req *connect.Request[api.SaveUserRequest],
) (*connect.Response[api.SaveUserResponse], error) {
	if req.Msg.UserId == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("user_id is required"))
	}

	// Check authentication
	authInfo, ok := auth.GetAuthInfo(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("authentication required"))
	}

	// Authorization: user can only update their own information
	if authInfo.UserID != req.Msg.UserId {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("not authorized to update this user"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "SaveUser",
	)

	logger.InfoContext(ctx, "updating user profile")

	// Get existing user
	user, err := s.userManager.GetUserByID(ctx, req.Msg.UserId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("user not found: %w", err))
	}

	// Update only the provided fields (nil = omitted = leave unchanged; non-nil = set, including "").
	if req.Msg.Name != nil {
		user.Name = *req.Msg.Name
	}

	// Store the old primary location before updating.
	oldPrimaryLocationID := user.PrimaryResidenceLocationId

	if req.Msg.PrimaryResidenceLocationId != nil {
		user.PrimaryResidenceLocationId = *req.Msg.PrimaryResidenceLocationId
	}

	// Handle description
	if req.Msg.Description != nil {
		user.Description = *req.Msg.Description
	}

	// Storage uses User.media_ids[] as the sole avatar source after
	// #2083 Phase 4a. The API still accepts the legacy singular
	// media_id field on the request — promote it to media_ids[0]
	// when the array isn't provided.
	if len(req.Msg.MediaIds) > 0 {
		user.MediaIds = req.Msg.MediaIds
	} else if req.Msg.MediaId != nil {
		user.MediaIds = []string{*req.Msg.MediaId}
	}

	// For repeated fields, we replace the entire list (even if empty).
	// Deduplicate while preserving the caller's ordering — iterating a map
	// would randomize the output and flake FieldRoundTrip tests.
	newPrimaryID := req.Msg.GetPrimaryResidenceLocationId()
	seen := make(map[string]bool, len(req.Msg.OtherLocationIds)+1)
	ordered := make([]string, 0, len(req.Msg.OtherLocationIds)+1)
	for _, locID := range req.Msg.OtherLocationIds {
		if locID == "" || locID == newPrimaryID || seen[locID] {
			continue
		}
		seen[locID] = true
		ordered = append(ordered, locID)
	}

	// If a non-empty new primary location was set and there was a different old primary,
	// append the old primary as an other location (dedup-guarded).
	// Skip when clearing the primary (newPrimaryID == "") — no demotion needed.
	if req.Msg.PrimaryResidenceLocationId != nil &&
		newPrimaryID != "" &&
		oldPrimaryLocationID != "" &&
		oldPrimaryLocationID != newPrimaryID &&
		!seen[oldPrimaryLocationID] {
		seen[oldPrimaryLocationID] = true
		ordered = append(ordered, oldPrimaryLocationID)
	}

	user.OtherLocationIds = ordered

	// Handle preferred timezone: validate only when non-empty so callers can clear with "".
	if req.Msg.PreferredTimezone != nil {
		if *req.Msg.PreferredTimezone != "" {
			if err := timezone.ValidateIANA(*req.Msg.PreferredTimezone); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument,
					fmt.Errorf("invalid timezone: %w", err))
			}
		}
		user.PreferredTimezone = *req.Msg.PreferredTimezone
	}

	// Handle preferred language: empty string clears the field;
	// non-empty values are normalized against the supported-locale
	// list (unsupported tags fold to the default). Storing the
	// normalized form keeps the server catalog consistent with what
	// the user asked for.
	if req.Msg.PreferredLanguage != nil {
		if raw := *req.Msg.PreferredLanguage; raw == "" {
			user.PreferredLanguage = nil
		} else {
			normalized := l10n.Normalize(raw).String()
			user.PreferredLanguage = &normalized
		}
	}

	// Save the updated user
	err = s.userManager.UpdateUser(ctx, user)
	if err != nil {
		logger.ErrorContext(ctx, "failed to update user", "error", err)
		return nil, connecterr.Internal(ctx, "SaveUser", err)
	}

	response := &api.SaveUserResponse{
		UserId: user.Id,
	}

	return connect.NewResponse(response), nil
}

// DeleteUser soft-deletes the authenticated user's account.
// Users can only delete their own account.
func (s *Service) DeleteUser(
	ctx context.Context,
	req *connect.Request[api.DeleteUserRequest],
) (*connect.Response[api.DeleteUserResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DeleteUser",
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
	)

	logger.InfoContext(ctx, "deleting user account")

	// Delete the user (soft delete)
	err = s.userManager.DeleteUser(ctx, authInfo.UserID)
	if err != nil {
		logger.ErrorContext(ctx, "failed to delete user", "error", err)
		return nil, connecterr.Internal(ctx, "DeleteUser", err)
	}

	logger.InfoContext(ctx, "user account deleted successfully")

	return connect.NewResponse(&api.DeleteUserResponse{}), nil
}

// Verify that Service implements the UserServiceHandler interface.
var _ apiconnect.UserServiceHandler = (*Service)(nil)
