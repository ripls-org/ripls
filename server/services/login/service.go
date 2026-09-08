package login

import (
	"time"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/branding"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the LoginService RPC interface.
type Service struct {
	userManager            *auth.UserManager
	authTokenConfig        *auth.TokenConfig
	oidcManager            *auth.OIDCProviderManager
	phoneAuth              auth.PhoneTokenVerifier
	storage                *storage.ProtoSQLStorage
	bucket                 storage.BucketStorage
	emailService           email.Service
	notificationService    notifications.Service
	bus                    cebus.Publisher // CommunityEvent publisher; required (#510 PR 3).
	devAuth                bool
	refreshTokenExpiration time.Duration

	// emailCodeTestAccounts maps a normalized address to the fixed sign-in
	// code it accepts instead of a mailed one, for app-store review and manual
	// QA (#2571). Nil in every ordinary deployment. An address must be a key
	// here for its code to be honored — the code alone never verifies an
	// address that is not listed.
	emailCodeTestAccounts map[string]string

	// branding supplies the app base URL for links this service mails out.
	branding branding.Config
}

// Config holds the configuration for creating a login service.
type Config struct {
	UserManager            *auth.UserManager
	AuthTokenConfig        *auth.TokenConfig
	OIDCManager            *auth.OIDCProviderManager
	PhoneAuth              auth.PhoneTokenVerifier // Optional: nil disables phone auth RPCs.
	Storage                *storage.ProtoSQLStorage
	BucketStorage          storage.BucketStorage
	EmailService           email.Service
	NotificationService    notifications.Service // Push notifications for join events; may be nil in tests.
	Bus                    cebus.Publisher       // CommunityEvent publisher; required (#510 PR 3).
	DevAuth                bool                  // Allows registration without an invitation code (dev/test only)
	RefreshTokenExpiration time.Duration         // Lifetime of issued refresh tokens

	// EmailCodeTestAccounts maps a normalized address to its fixed sign-in
	// code. Build it with ParseEmailOTPTestAccounts; nil disables the path.
	EmailCodeTestAccounts map[string]string

	// Branding supplies the app base URL for mailed links (password reset).
	Branding branding.Config
}

// New creates a new login service.
func New(cfg Config) *Service {
	return &Service{
		userManager:            cfg.UserManager,
		authTokenConfig:        cfg.AuthTokenConfig,
		oidcManager:            cfg.OIDCManager,
		phoneAuth:              cfg.PhoneAuth,
		storage:                cfg.Storage,
		bucket:                 cfg.BucketStorage,
		emailService:           cfg.EmailService,
		notificationService:    cfg.NotificationService,
		bus:                    cfg.Bus,
		devAuth:                cfg.DevAuth,
		refreshTokenExpiration: cfg.RefreshTokenExpiration,
		emailCodeTestAccounts:  cfg.EmailCodeTestAccounts,
		branding:               cfg.Branding,
	}
}

// Verify that Service implements the LoginServiceHandler interface.
var _ apiconnect.LoginServiceHandler = (*Service)(nil)
