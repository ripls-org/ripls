package user

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/provisional"
)

// AddPhoneNumber attaches a verified phone number to the calling account as an
// *additional* login method (#2596). The client verifies phone ownership via
// Firebase Phone Auth OTP, then sends the resulting Firebase ID token here for
// server-side validation.
//
// Auth methods are additive: this sets User.phone_number but leaves
// User.auth_method untouched, so the account's original method (email/OIDC)
// keeps working and phone OTP becomes an additional way in. PhoneLogin accepts
// any account holding a verified phone (it no longer gates on auth_method), and
// the only writers of User.phone_number — PhoneRegister and this RPC — are both
// OTP-gated, so a stored phone is always verified.
//
// It also reconciles the inverse of phone-first promote-on-verify: a phone-keyed
// provisional placeholder seeded when this person was invited by phone (in any
// community) is claimed and merged into the account.
func (s *Service) AddPhoneNumber(
	ctx context.Context,
	req *connect.Request[api.AddPhoneNumberRequest],
) (*connect.Response[api.AddPhoneNumberResponse], error) {
	if !auth.PhoneAuthConfigured(s.phoneAuth) {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("phone authentication is not configured"))
	}

	// Caller-only: the phone always attaches to the authenticated caller. The
	// request carries no user_id, so there is no way to add a phone to another
	// account — no IDOR surface.
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if req.Msg.FirebaseIdToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("firebase_id_token is required"))
	}

	// Validate the Firebase ID token and extract the verified phone number.
	phoneNumber, err := s.phoneAuth.VerifyPhoneToken(ctx, req.Msg.FirebaseIdToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("phone verification failed: %w", err))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "AddPhoneNumber",
		"user_id", authInfo.UserID,
		"phone", logging.MaskPhone(phoneNumber),
	)

	// Conflict check. ProvisionalUser rows are not in the User table, so this
	// only ever finds a *real* account — provisional collisions are handled by
	// the promotion merge below, not here.
	existing, err := s.userManager.GetUserByPhone(ctx, phoneNumber)
	if err != nil {
		return nil, connecterr.Internal(ctx, "AddPhoneNumber", err, "detail", "failed to check existing phone")
	}
	if existing != nil && existing.Id != authInfo.UserID {
		// Already on a different full account. A full account<->account merge is
		// out of scope (tracked in #2602). The caller proved ownership of this
		// number via OTP, so "already on an account" reveals nothing new.
		logger.WarnContext(ctx, "add_phone_rejected_other_account", "existing_user_id", existing.Id)
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("this phone number is already linked to another account"))
	}

	// Load the caller's account.
	currentUser, err := s.userManager.GetUserByID(ctx, authInfo.UserID)
	if err != nil {
		return nil, connecterr.Internal(ctx, "AddPhoneNumber", err, "detail", "failed to load user")
	}

	// Attach the phone (additive — leave auth_method as the primary method).
	// Idempotent: re-running with the same verified number is a no-op write.
	if currentUser.GetPhoneNumber() != phoneNumber {
		currentUser.PhoneNumber = &phoneNumber
		if err := s.userManager.UpdateUser(ctx, currentUser); err != nil {
			return nil, connecterr.Internal(ctx, "AddPhoneNumber", err, "detail", "failed to attach phone")
		}
		logger.InfoContext(ctx, "phone added")
	}

	// Promote any phone-keyed provisional placeholders for this verified number
	// — in any community — into the account, joining the communities and merging
	// their activity history. Best-effort; shared with PhoneRegister.
	provisional.PromoteByPhone(ctx, s.storage, s.bus, currentUser.Id, phoneNumber)

	return connect.NewResponse(&api.AddPhoneNumberResponse{PhoneNumber: phoneNumber}), nil
}
