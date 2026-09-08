package login

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

// EmailLogin logs in an existing user with email/password authentication.
func (s *Service) EmailLogin(
	ctx context.Context,
	req *connect.Request[api.EmailLoginRequest],
) (*connect.Response[api.EmailLoginResponse], error) {
	if req.Msg.Email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email is required"))
	}

	email := auth.NormalizeEmail(req.Msg.Email)

	// Sign-in still honours a password: the accounts that hold one predate the
	// code flow and it is the only credential they have. Registration no longer
	// does (#2864), so this branch serves a set that can only shrink.
	//nolint:staticcheck // SA1019: Password is deprecated and read here on purpose — accepting it is what keeps builds released before the code flow working through the migration window. Leaves with the field (#2571).
	emailVerified, err := s.resolveEmailCredential(ctx, email, req.Msg.EmailProofToken, req.Msg.Password, true)
	if err != nil {
		return nil, err
	}

	// Get user by email
	user, err := s.userManager.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "EmailLogin", fmt.Errorf("authentication failed"))
	}

	if user == nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"user_email", logging.MaskEmail(email),
			"reason", "unknown_email",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("authentication failed"))
	}

	if emailVerified {
		// No auth_method gate on the verified path — the same move #2596 made
		// for PhoneLogin. The caller proved they receive mail at this address,
		// and the account's email column is only ever written by an
		// ownership-proving path, so finding the row by that address is itself
		// proof the account holds the credential. This deliberately lets an
		// OIDC-registered account sign in by code: their address is
		// provider-verified, their provider's own recovery runs through the
		// same inbox, and the alternative is a dead end with no way back in.
		s.stampVerifiedEmailIfMissing(ctx, user, true)
	} else {
		// Legacy password path keeps its original gate. It is removed once a
		// release carrying the code flow has soaked (#2571).
		if user.AuthMethod != models.AuthMethod_AUTH_METHOD_EMAIL_PASSWORD {
			authMethodName := getAuthMethodName(user.AuthMethod)
			return nil, connect.NewError(connect.CodeInvalidArgument,
				fmt.Errorf("please login with %s", authMethodName))
		}

		//nolint:staticcheck // SA1019: see the note on resolveEmailCredential above — the legacy password path is deliberately still live (#2571).
		if err := auth.VerifyPassword(req.Msg.Password, user.PasswordHash); err != nil {
			logger := logging.LoggerWithContext(ctx)
			logger.WarnContext(ctx, "authentication_failed",
				"user_email", logging.MaskEmail(email),
				"reason", "wrong_password",
				"remote_addr", logging.RemoteAddrFromContext(ctx),
			)
			return nil, connect.NewError(connect.CodeUnauthenticated,
				fmt.Errorf("authentication failed"))
		}
	}

	// Generate access token and refresh token.
	accessToken, err := s.authTokenConfig.GenerateToken(user.Id, user.Email, user.Role)
	if err != nil {
		return nil, connecterr.Internal(ctx, "EmailLogin", err, "detail", "failed to generate token")
	}

	rawRefreshToken, err := s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
	if err != nil {
		return nil, connecterr.Internal(ctx, "EmailLogin", err, "detail", "failed to generate refresh token")
	}

	// login_method distinguishes the two credentials rather than labelling
	// both "email_password". Watching password sign-ins fall to zero is the
	// signal that gates removing the password path (#2571), so the log has to
	// be able to tell them apart.
	loginMethod := "email_password"
	if emailVerified {
		loginMethod = "email_code"
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user logged in",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
		"login_method", loginMethod,
	)

	response := &api.EmailLoginResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}

	return connect.NewResponse(response), nil
}

// OIDCLogin authenticates an existing user with an OIDC provider and returns a token.
func (s *Service) OIDCLogin(
	ctx context.Context,
	req *connect.Request[api.OIDCLoginRequest],
) (*connect.Response[api.OIDCLoginResponse], error) {
	if req.Msg.Provider == api.OIDCProvider_OIDC_PROVIDER_UNSPECIFIED {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("provider must be specified"))
	}

	if req.Msg.IdToken == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("id_token is required"))
	}

	// Validate the OIDC token with the provider
	idToken, err := s.oidcManager.ValidateToken(ctx, req.Msg.Provider, req.Msg.IdToken)
	if err != nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"reason", "token_validation_failed",
			"provider", req.Msg.Provider.String(),
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("token validation failed: %w", err))
	}

	// Extract user information from the validated token
	userInfo, err := auth.ExtractUserInfo(idToken)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCLogin", err, "detail",

			// Get existing user by email
			"failed to extract user info")
	}

	user, err := s.userManager.GetUserByEmail(ctx, userInfo.Email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCLogin", fmt.Errorf("authentication failed"))
	}

	if user == nil {
		logger := logging.LoggerWithContext(ctx)
		logger.WarnContext(ctx, "authentication_failed",
			"user_email", logging.MaskEmail(userInfo.Email),
			"reason", "unknown_email",
			"provider", req.Msg.Provider.String(),
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, connect.NewError(connect.CodeUnauthenticated,
			fmt.Errorf("no account found - please register first"))
	}

	// Verify user registered with correct OIDC provider
	expectedAuthMethod := mapProviderToAuthMethod(req.Msg.Provider)
	if user.AuthMethod != expectedAuthMethod {
		authMethodName := getAuthMethodName(user.AuthMethod)
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("please login with %s", authMethodName))
	}

	// Stamp the verified-email signal on accounts that predate it (#2571).
	// The provider re-asserts email_verified on every sign-in, so accounts
	// created before the field existed acquire it the next time they log in —
	// no backfill job needed. Best-effort: a write failure costs nothing this
	// request and is retried on the next sign-in.
	s.stampVerifiedEmailIfMissing(ctx, user, userInfo.EmailVerified)

	// Generate access token and refresh token.
	accessToken, err := s.authTokenConfig.GenerateToken(user.Id, user.Email, user.Role)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCLogin", err, "detail", "failed to generate token")
	}

	rawRefreshToken, err := s.createRefreshToken(ctx, user.Id, models.RefreshTokenOrigin_REFRESH_TOKEN_ORIGIN_INTERACTIVE)
	if err != nil {
		return nil, connecterr.Internal(ctx, "OIDCLogin", err, "detail", "failed to generate refresh token")
	}

	logger := logging.LoggerWithContext(ctx)
	logger.InfoContext(ctx, "user logged in",
		"user_id", user.Id,
		"user_email", logging.MaskEmail(user.Email),
		"login_method", req.Msg.Provider.String(),
	)

	response := &api.OIDCLoginResponse{
		User:   toAPIUser(user),
		Tokens: &api.Tokens{AccessToken: accessToken, RefreshToken: rawRefreshToken},
	}

	return connect.NewResponse(response), nil
}
