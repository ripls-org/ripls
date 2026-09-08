package login

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	"go.ripls.org/ripls/server/contact"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/provisional"
)

// PhoneRegister creates a new user account with a verified phone number (passwordless).
// The client verifies phone ownership via Firebase Phone Auth OTP, then sends the
// resulting Firebase ID token here for server-side validation.
func (s *Service) PhoneRegister(
	ctx context.Context,
	req *connect.Request[api.PhoneRegisterRequest],
) (*connect.Response[api.PhoneRegisterResponse], error) {
	if !auth.PhoneAuthConfigured(s.phoneAuth) {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("phone authentication is not configured"))
	}

	if req.Msg.FirebaseIdToken == "" || req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("firebase_id_token and name are required"))
	}

	// Validate Firebase ID token and extract verified phone number.
	phoneNumber, err := s.phoneAuth.VerifyPhoneToken(ctx, req.Msg.FirebaseIdToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("phone verification failed: %w", err))
	}

	// Check if phone number is already registered.
	existingUser, err := s.userManager.GetUserByPhone(ctx, phoneNumber)
	if err != nil {
		return nil, connecterr.Internal(ctx, "PhoneRegister", err, "detail", "failed to check existing phone")
	}
	if existingUser != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("an account already exists with this phone number"))
	}

	// Validate invitation short code (same pattern as EmailRegister).
	var invitation *models.ShareLink
	if req.Msg.ShortCode == "" {
		if s.devAuth {
			invitation = nil
		} else {
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("short code is required"))
		}
	} else {
		hasUsers, err := s.storage.HasAnyUsers(ctx)
		if err != nil {
			return nil, connecterr.Internal(ctx, "PhoneRegister", err, "detail", "failed to check existing users")
		}

		if hasUsers {
			invitation, err = s.validateShortCode(ctx, req.Msg.ShortCode)
			if err != nil {
				return nil, err
			}
		}
	}

	// Create user with PHONE auth method — no password.
	now := clock.UnixSec(ctx)
	user := &models.User{
		Id:          uuid.New().String(),
		Name:        req.Msg.Name,
		PhoneNumber: &phoneNumber,
		CreatedAt:   now,
		UpdatedAt:   now,
		Role:        models.Role_ROLE_USER,
		AuthMethod:  models.AuthMethod_AUTH_METHOD_PHONE,
	}
	seedPreferredLanguage(ctx, user)

	if invitation != nil {
		user.InvitedById = invitation.InviterId
	}

	if req.Msg.SimulationId != nil {
		user.SimulationId = req.Msg.SimulationId
	}

	// Complete registration: insert user, join community, generate tokens.
	var accessToken, rawRefreshToken string
	if invitation != nil {
		accessToken, rawRefreshToken, err = s.completeRegistration(ctx, user, invitation, req.Msg.GearId)
		if err != nil {
			return nil, err
		}
	} else {
		// Bootstrap or dev-mode user — just insert and generate tokens.
		if _, err := s.storage.Insert(ctx, user); err != nil {
			return nil, connecterr.Internal(ctx, "PhoneRegister", err, "detail", "failed to create user")
		}

		accessToken, err = s.authTokenConfig.GenerateToken(user.Id, "", user.Role, phoneNumber)
		if err != nil {
			return nil, connecterr.Internal(ctx, "PhoneRegister", err, "detail", "failed to generate token")
		}

		rawRefreshToken, err = s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
		if err != nil {
			return nil, connecterr.Internal(ctx, "PhoneRegister", err, "detail", "failed to generate refresh token")
		}
	}

	// Promote any provisional placeholders seeded for this verified phone number
	// — in this community or any other — into the new real account, joining the
	// communities and merging their activity history. Best-effort: a failure
	// here is logged inside the helper but never blocks a successful
	// registration (the link-tied merge in completeRegistration is independent).
	// Shared with user.Service.AddPhoneNumber so both phone paths reconcile
	// identically.
	provisional.PromoteByPhone(ctx, s.storage, s.bus, user.Id, phoneNumber)

	// Send the one-time opt-in confirmation ("welcome") text to the freshly
	// verified phone. Best-effort: a failure must never block a successful
	// registration. Nil in some tests. Beyond confirming the new subscription
	// to the user, the live send lets a carrier reviewer verify the opt-in flow
	// end-to-end during RCS/A2P registration.
	if s.notificationService != nil {
		_ = s.notificationService.SendPhoneOptInWelcome(ctx, user)
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user registered",
		"user_id", user.Id,
		"auth_method", "phone",
	)

	return connect.NewResponse(&api.PhoneRegisterResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}), nil
}

// CheckPhoneRegistered reports whether a phone number already has an account,
// so a sign-in surface can decline before starting verification.
//
// This exists because the one-time code is sent by the identity provider from
// the client, not by this server. Without a pre-check, signing in with an
// unregistered number sends a real text message — at real cost, and burning
// provider quota — that cannot lead anywhere, and only after entering it does
// the person learn there is no account.
//
// It reports account existence to an unauthenticated caller. That is the same
// trade RequestEmailCode's require_existing_account makes, accepted for the
// same reason: there is no better answer to "I tried to sign in and no such
// account exists" than saying so. Rate-limited per IP.
func (s *Service) CheckPhoneRegistered(
	ctx context.Context,
	req *connect.Request[api.CheckPhoneRegisteredRequest],
) (*connect.Response[api.CheckPhoneRegisteredResponse], error) {
	if req.Msg.PhoneNumber == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("phone_number is required"))
	}

	// Match the normalized form registration stores, so a differently-formatted
	// number does not read as unregistered and send a pointless text.
	normalized, err := contact.NormalizePhoneE164(req.Msg.PhoneNumber)
	if err != nil {
		// Unparseable is not registered — and saying so beats sending a code to
		// a number the provider will reject anyway.
		return connect.NewResponse(&api.CheckPhoneRegisteredResponse{IsRegistered: false}), nil
	}

	existing, err := s.userManager.GetUserByPhone(ctx, normalized)
	if err != nil {
		return nil, connecterr.Internal(ctx, "CheckPhoneRegistered", err, "detail", "failed to check phone")
	}

	return connect.NewResponse(&api.CheckPhoneRegisteredResponse{
		IsRegistered: existing != nil,
	}), nil
}

// PhoneLogin logs in an existing user with a verified phone number (passwordless).
// The client verifies phone ownership via Firebase Phone Auth OTP on every login,
// then sends the resulting Firebase ID token here for server-side validation.
func (s *Service) PhoneLogin(
	ctx context.Context,
	req *connect.Request[api.PhoneLoginRequest],
) (*connect.Response[api.PhoneLoginResponse], error) {
	if !auth.PhoneAuthConfigured(s.phoneAuth) {
		return nil, connect.NewError(connect.CodeUnimplemented,
			fmt.Errorf("phone authentication is not configured"))
	}

	if req.Msg.FirebaseIdToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("firebase_id_token is required"))
	}

	// Validate Firebase ID token and extract verified phone number.
	phoneNumber, err := s.phoneAuth.VerifyPhoneToken(ctx, req.Msg.FirebaseIdToken)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"reason", "phone_token_invalid",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("phone verification failed: %w", err))
	}

	// Look up user by phone number.
	user, err := s.userManager.GetUserByPhone(ctx, phoneNumber)
	if err != nil {
		return nil, connecterr.Internal(ctx, "PhoneLogin", fmt.Errorf("authentication failed"))
	}
	if user == nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"reason", "unknown_phone",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeNotFound,
			fmt.Errorf("no account found with this phone number — please register first"))
	}

	// No auth-method gate: auth methods are additive (#2596). Finding the
	// account by its verified phone number (GetUserByPhone above) is itself
	// proof the account holds the phone credential — whether phone is the
	// primary auth method (PhoneRegister) or was attached later to an
	// email/OIDC account (AddPhoneNumber). The only writers of
	// User.phone_number are OTP-gated, so a row's phone is always verified.

	// Generate tokens.
	accessToken, err := s.authTokenConfig.GenerateToken(user.Id, user.Email, user.Role, phoneNumber)
	if err != nil {
		return nil, connecterr.Internal(ctx, "PhoneLogin", err, "detail", "failed to generate token")
	}

	rawRefreshToken, err := s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
	if err != nil {
		return nil, connecterr.Internal(ctx, "PhoneLogin", err, "detail", "failed to generate refresh token")
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user logged in",
		"user_id", user.Id,
		"login_method", "phone",
	)

	return connect.NewResponse(&api.PhoneLoginResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}), nil
}
