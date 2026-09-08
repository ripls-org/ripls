package main

// Service-graph construction for the server binary: wireServices builds every
// RPC service implementation and the shared libraries they depend on
// (notification channels, location/stock providers, event buses and their
// subscribers), in dependency order. Routes mount the results (routes.go);
// background jobs consume them (jobs.go).

import (
	"context"
	"fmt"
	"time"

	firebase "firebase.google.com/go/v4"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	chatlib "go.ripls.org/ripls/server/chat"
	"go.ripls.org/ripls/server/chat_event_bus"
	communitylib "go.ripls.org/ripls/server/community"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/config"
	"go.ripls.org/ripls/server/email"
	"go.ripls.org/ripls/server/emailsuppression"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/github"
	impactlib "go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	locationpkg "go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
	mediapkg "go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	chat_subscriber "go.ripls.org/ripls/server/notifications/chat_subscriber"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	"go.ripls.org/ripls/server/services/admin"
	"go.ripls.org/ripls/server/services/chat"
	"go.ripls.org/ripls/server/services/community"
	"go.ripls.org/ripls/server/services/device"
	esmsvc "go.ripls.org/ripls/server/services/esm"
	"go.ripls.org/ripls/server/services/experience"
	"go.ripls.org/ripls/server/services/feed"
	"go.ripls.org/ripls/server/services/feedback"
	"go.ripls.org/ripls/server/services/gear"
	healthsvc "go.ripls.org/ripls/server/services/health"
	"go.ripls.org/ripls/server/services/impact_metrics"
	"go.ripls.org/ripls/server/services/location"
	"go.ripls.org/ripls/server/services/login"
	"go.ripls.org/ripls/server/services/media"
	"go.ripls.org/ripls/server/services/portfolio"
	"go.ripls.org/ripls/server/services/profile"
	"go.ripls.org/ripls/server/services/request"
	"go.ripls.org/ripls/server/services/search"
	"go.ripls.org/ripls/server/services/transfer"
	"go.ripls.org/ripls/server/services/unified_create"
	"go.ripls.org/ripls/server/services/user"
	"go.ripls.org/ripls/server/services/waitlist"
	websvc "go.ripls.org/ripls/server/services/web"
	workshopsvc "go.ripls.org/ripls/server/services/workshop"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/story"
	story_subscriber "go.ripls.org/ripls/server/story/subscriber"
	"go.ripls.org/ripls/server/weather"
	"go.ripls.org/ripls/server/webfetch"
)

// refreshTokenExpiration is how long a login refresh token stays valid.
const refreshTokenExpiration = 90 * 24 * time.Hour

// wiringDeps carries the process-level resources main() acquires (with their
// defers and fatal exits) into service wiring.
type wiringDeps struct {
	health          *healthsvc.Service
	sqlStorage      *storage.ProtoSQLStorage
	bucketStorage   storage.BucketStorage
	aiProvider      ai.Provider // nil when no provider is configured
	firebaseApp     *firebase.App
	firebaseAuth    *auth.FirebaseAuth
	authTokenConfig *auth.TokenConfig
	oidcManager     *auth.OIDCProviderManager
}

// serverServices holds every constructed service the route table mounts and
// the background jobs consume.
type serverServices struct {
	health        *healthsvc.Service
	admin         *admin.Service
	chat          *chat.Service
	community     *community.Service
	gear          *gear.Service
	transfer      *transfer.Service
	request       *request.Service
	experience    *experience.Service
	unifiedCreate *unified_create.Service
	location      *location.Service
	media         *media.Service
	user          *user.Service
	device        *device.Service
	portfolio     *portfolio.Service
	feed          *feed.Service
	esm           *esmsvc.Service
	workshop      *workshopsvc.Service
	profile       *profile.Service
	search        *search.Service
	feedback      *feedback.Service
	impact        *impact_metrics.Service
	login         *login.Service
	waitlist      *waitlist.Service
	web           *websvc.Service

	// Shared libraries the background jobs need.
	notificationService notifications.Service
	emailService        email.Service
}

// weatherProviderFor returns the forecast source, cached per place either way.
//
// The mock exists for the same reason the AI one does: a run that reaches a
// live service is neither hermetic nor repeatable. It also decouples what can
// be rendered from what the real forecast horizon happens to cover — a run
// filming a date months away, or in the past, still gets weather.
func weatherProviderFor(cfg *config.Config, logger *logging.Logger) weather.Provider {
	if cfg.MockWeatherProvider {
		logger.Warn("using deterministic in-process weather provider (--mock-weather-provider); e2e/dev only, never production")
		return weather.NewCachedProvider(weather.NewFakeProvider())
	}
	return weather.NewCachedProvider(weather.NewOpenMeteoProvider())
}

// wireServices constructs the full service graph in dependency order. Fatal
// misconfigurations are logged here (with the same messages main() used to
// emit) and returned as errors; the caller just exits.
func wireServices(ctx context.Context, cfg *config.Config, logger *logging.Logger, deps wiringDeps) (*serverServices, error) {
	sqlStorage := deps.sqlStorage
	bucketStorage := deps.bucketStorage
	aiProvider := deps.aiProvider
	healthService := deps.health

	// Email + push + off-app notification channels (channels.go). The
	// share-link resolver is late-bound: assigned below once the community
	// service exists.
	var resolveShareLink notifications.ShareLinkResolver
	notificationService, emailService, err := buildNotificationChannels(ctx, cfg, logger, deps.firebaseApp, sqlStorage, bucketStorage, &resolveShareLink)
	if err != nil {
		return nil, err
	}
	healthService.RegisterFuncCached("email", healthsvc.DefaultExternalTTL, emailService.CheckHealth)

	// Create service instances
	userManager := auth.NewUserManager(sqlStorage)

	// Location providers (#2188): construct any client whose credential
	// is present so its CheckHealth registers during a cutover window;
	// the active provider used by services is picked from --map-provider.
	// Both location probes are cached: the Google Maps probe issues a
	// billable Geocoding API request, so running it per uptime probe turned
	// monitoring frequency into 45% of the GCP bill (#2809).
	mapboxClient := locationpkg.NewMapboxClient(cfg.MapboxAccessToken)
	if cfg.MapboxAccessToken != "" {
		healthService.RegisterFuncCached("mapbox", healthsvc.DefaultExternalTTL, mapboxClient.CheckHealth)
	}
	var googleMapsClient *locationpkg.GoogleMapsClient
	if cfg.GoogleMapsAPIKeyServer != "" {
		googleMapsClient = locationpkg.NewGoogleMapsClient(cfg.GoogleMapsAPIKeyServer)
		healthService.RegisterFuncCached("google_maps", healthsvc.DefaultExternalTTL, googleMapsClient.CheckHealth)
	}

	// Resolve the active provider per --map-provider. The selected
	// provider's credential must be present in production — otherwise the
	// server would silently degrade every geocoding-dependent flow. In
	// dev-mode (e2e + integration tests), missing credentials are allowed
	// and the services that consume locationProvider already nil-guard.
	var locationProvider locationpkg.Provider
	switch cfg.MapProvider {
	case "mapbox":
		if cfg.MapboxAccessToken == "" {
			if !cfg.DevMode {
				logger.Error("--map-provider=mapbox requires --mapbox-access-token (or --mapbox-access-token-file) to be non-empty")
				return nil, fmt.Errorf("map provider misconfigured")
			}
			logger.Warn("--map-provider=mapbox but no token supplied; running without a location provider (dev-mode)")
		} else {
			locationProvider = mapboxClient
		}
	case "google":
		if googleMapsClient == nil {
			if !cfg.DevMode {
				logger.Error("--map-provider=google requires --google-maps-api-key-server (or --google-maps-api-key-server-file) to be non-empty")
				return nil, fmt.Errorf("map provider misconfigured")
			}
			logger.Warn("--map-provider=google but no key supplied; running without a location provider (dev-mode)")
		} else {
			locationProvider = googleMapsClient
		}
	}
	logger.Info("location provider selected",
		"map_provider", cfg.MapProvider,
		"has_provider", locationProvider != nil)

	// Stock imagery (Unsplash→Pexels) and video (Pexels→Pixabay) provider
	// chains, semantic-cache wrapped; nil when no keys are configured.
	stockImageryProvider, stockVideoProvider, err := mediapkg.NewStockProviders(mediapkg.StockProviderKeys{
		UnsplashAccessKey: cfg.UnsplashAccessKey,
		PexelsAPIKey:      cfg.PexelsAPIKey,
		PixabayAPIKey:     cfg.PixabayAPIKey,
	}, sqlStorage, bucketStorage, logger)
	if err != nil {
		logger.Error("failed to initialize stock media providers", "error", err)
		return nil, err
	}
	// Note: stock_imagery health check removed — it calls the Pexels API on
	// every invocation which consumes the 200 req/hour rate limit (#929).

	// Initialize story generation
	storyStorage := storage.NewStoryStorage(sqlStorage)
	storyCreator := story.NewGenerator(storyStorage)

	adminService := admin.NewService(sqlStorage, cfg.DevMode)

	// Construct the chat event bus before the chat service so the bus is wired
	// at construction time. Subscribers register after chatService exists so the
	// stream-presence hooks (HasActiveStream, IsUserInForeground) can be bound.
	chatEventTopic := pubsub.NewMemTopic[*chat_event_bus.PublishedEvent](chat_event_bus.TopicName)
	chatEventBus := chat_event_bus.NewInProcessBus(sqlStorage, chatEventTopic)

	chatService := chat.New(sqlStorage, chatEventBus)
	if aiProvider != nil {
		chatService.SetAIProvider(aiProvider)
	}

	// Subscribe the stream broadcaster and push-notification subscriber.
	// Stream-presence hooks are bound here because chatService's registry
	// only exists after chat.New returns.
	chatNotifSub := chat_subscriber.New(sqlStorage, notificationService)
	chatNotifSub.SetStreamChecker(chatService.HasActiveStream)
	chatNotifSub.SetForegroundChecker(chatService.IsUserInForeground)
	if _, err := chatEventBus.Subscribe(chatNotifSub); err != nil {
		logger.Error("failed to subscribe chat notification subscriber", "error", err)
		return nil, err
	}
	if _, err := chatEventBus.Subscribe(chatService.StreamSubscriber()); err != nil {
		logger.Error("failed to subscribe chat stream subscriber", "error", err)
		return nil, err
	}
	estimatorCfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		logger.Error("failed to load estimator config", "error", err)
		return nil, err
	}
	impactService := impact_metrics.NewService(sqlStorage, estimatorCfg)

	// Construct the community-event bus before any emitter service, so each
	// constructor can take the bus directly (#510 PR 3). Subscribers register
	// after their respective services exist.
	communityEventTopic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	communityEventBus := cebus.NewInProcessBus(sqlStorage, communityEventTopic)

	communityService := community.New(sqlStorage, bucketStorage, notificationService, communityEventBus, cfg.InviteLinkHostname)
	// Late-bind the off-app notification share-link resolver (declared above the
	// notification service it can't be passed to at construction).
	resolveShareLink = communityService.ShortLinkCodeForEntity
	if aiProvider != nil {
		communityService.SetAIProvider(aiProvider)
	}
	if stockImageryProvider != nil {
		communityService.SetStockImageryProvider(stockImageryProvider)
	}

	// Subscribe the notification + stream subscribers now that
	// communityService exists. The notification subscriber's stream-presence
	// hook is bound after construction since the stream registry only exists
	// once communityService is built.
	communityNotifSub := commsub.New(sqlStorage, notificationService)
	communityNotifSub.SetStreamChecker(communityService.HasActiveUserStream)
	if _, err := communityEventBus.Subscribe(communityNotifSub); err != nil {
		logger.Error("failed to subscribe community notification subscriber", "error", err)
		return nil, err
	}
	if _, err := communityEventBus.Subscribe(communityService.StreamSubscriber()); err != nil {
		logger.Error("failed to subscribe community stream subscriber", "error", err)
		return nil, err
	}
	portfolioMetricDetailCalc := impactlib.NewMetricDetailCalculator(sqlStorage, estimatorCfg)
	portfolioService := portfolio.New(sqlStorage, portfolioMetricDetailCalc)
	impactCalculator := impactlib.NewCalculator(sqlStorage, estimatorCfg)
	deviceService := device.New(sqlStorage, notificationService)
	feedService := feed.New(sqlStorage)
	// The Home view surfaces a contextual nudge by reusing the feed nudge system.
	portfolioService.SetNudgeProvider(feedService)
	// The Home calendar paints per-day weather via the keyless Open-Meteo
	// provider, cached per place. One shared instance so the Home calendar and
	// the event view hit the same per-place cache. See docs/weather.md.
	weatherProvider := weatherProviderFor(cfg, logger)
	portfolioService.SetWeatherProvider(weatherProvider)
	esmService := esmsvc.New(sqlStorage)
	workshopService := workshopsvc.New(sqlStorage, impactCalculator)
	profileService := profile.New(sqlStorage, impactCalculator, aiProvider)
	if aiProvider != nil {
		feedService.SetAIProvider(aiProvider)
	}
	if stockImageryProvider != nil {
		feedService.SetStockImageryProvider(stockImageryProvider)
	}
	feedService.SetBucketStorage(bucketStorage)
	gearService := gear.New(sqlStorage, bucketStorage)
	if aiProvider != nil {
		gearService.SetAIProvider(aiProvider)
	}
	if stockImageryProvider != nil {
		gearService.SetStockImageryProvider(stockImageryProvider)
	}
	gearService.SetWebFetcher(webfetch.NewHTTPFetcher())
	gearService.SetEstimatorConfig(estimatorCfg)
	systemMessageWriter := chatlib.NewSystemMessageWriter(sqlStorage, chatEventBus)
	communityService.SetSystemMessageWriter(systemMessageWriter)
	transferService := transfer.New(sqlStorage, notificationService, communityEventBus, systemMessageWriter)
	transferService.SetEstimatorConfig(estimatorCfg)
	socialResolver := impactlib.NewConnectionContextResolver(sqlStorage)
	transferService.SetSocialResolver(socialResolver)
	requestService := request.New(sqlStorage, bucketStorage, notificationService, communityEventBus, stockImageryProvider, aiProvider, systemMessageWriter)
	requestService.SetEstimatorConfig(estimatorCfg)
	requestService.SetSocialResolver(socialResolver)
	requestService.SetLocationProvider(locationProvider)
	experienceService := experience.New(sqlStorage, bucketStorage, notificationService, communityEventBus)
	experienceService.SetEstimatorConfig(estimatorCfg)
	experienceService.SetSocialResolver(socialResolver)
	experienceService.SetSystemMessageWriter(systemMessageWriter)
	experienceService.SetBranding(cfg.Branding)

	// Subscribe story creation to the community-event bus (#510 PR 4).
	// The subscriber listens for the same TRANSFER_COMPLETED /
	// REQUEST_FULFILLED / EXPERIENCE_COMPLETED / INVITATION_LINK_USED
	// events that the four emitter services above publish, and produces
	// the same Story.CreateStoryRequest the inline calls used to.
	storySub := story_subscriber.New(sqlStorage, storyCreator)
	if _, err := communityEventBus.Subscribe(storySub); err != nil {
		logger.Error("failed to subscribe story subscriber", "error", err)
		return nil, err
	}
	if aiProvider != nil {
		experienceService.SetAIProvider(aiProvider)
	}
	if stockImageryProvider != nil {
		experienceService.SetStockImageryProvider(stockImageryProvider)
	}
	if stockVideoProvider != nil {
		experienceService.SetStockVideoProvider(stockVideoProvider)
		// Note: stock_video health check removed — it calls the Pexels API on
		// every invocation which consumes the 200 req/hour rate limit (#929).
	}
	experienceService.SetLocationProvider(locationProvider)
	experienceService.SetWebFetcher(webfetch.NewHTTPFetcher())
	// The event view shows the forecast for the event's day at its location.
	experienceService.SetWeatherProvider(weatherProvider)

	// Register the per-item-type sharers so CommunityService.ShareItem (and the
	// per-item-community provisioning the create RPCs do) can share each item type
	// into a community. Experience + request sharers live on their own services;
	// gear sharing lives on the community service (#2492).
	communityService.SetItemSharer(community.ItemKindExperience, community.ItemSharer{
		VerifyOwner:          experienceService.VerifyExperienceOwner,
		VerifyViewer:         experienceService.VerifyExperienceViewer,
		ShareToCommunity:     experienceService.ShareExperienceToCommunity,
		UnshareFromCommunity: experienceService.UnshareExperienceFromCommunity,
	})
	communityService.SetItemSharer(community.ItemKindGear, community.ItemSharer{
		VerifyOwner:          communityService.VerifyGearOwner,
		VerifyViewer:         communityService.VerifyGearViewer,
		ShareToCommunity:     communityService.ShareGearToCommunity,
		UnshareFromCommunity: communityService.UnshareGearFromCommunity,
	})
	communityService.SetItemSharer(community.ItemKindRequest, community.ItemSharer{
		VerifyOwner:          requestService.VerifyRequestOwner,
		VerifyViewer:         requestService.VerifyRequestViewer,
		ShareToCommunity:     requestService.ShareRequestToCommunity,
		UnshareFromCommunity: requestService.UnshareRequestFromCommunity,
	})

	// Per-item communities (#2492): the experience + request create RPCs provision
	// their item's per-item community at insert via the shared `community` library.
	// Gear sharing lives on the community service, so the gear create RPC can't
	// call the library directly without a gear→community-service dependency; wire
	// a composed provisioner instead (library provision + community-service gear
	// share carrying the creation-time Lend/Give availability, #2687) so a new
	// gear is likewise born with its community.
	gearService.SetItemCommunityProvisioner(func(ctx context.Context, hostUserID, gearID string, availability models.Availability) (string, error) {
		itemCommunityID, err := communitylib.ProvisionPerItemCommunity(ctx, sqlStorage, communityEventBus, hostUserID, communitylib.Origin{GearID: gearID})
		if err != nil {
			return "", err
		}
		if err := communityService.ShareGearToCommunityWithAvailability(ctx, gearID, itemCommunityID, hostUserID, availability); err != nil {
			return "", err
		}
		return itemCommunityID, nil
	})

	// OfferTransfer (#2702) shares the offered gear into the request's community;
	// gear sharing lives on the community service, so inject it as a hook to keep
	// the transfer service free of a service-to-service dependency.
	transferService.SetGearSharer(communityService.ShareGearToCommunityWithAvailability)

	// Gear-backed request offers (#2702) couple the two lifecycles over the
	// bus: a handoff auto-fulfills the request, a fulfilled request stands down its
	// open offers, a cancelled offer unwinds its claim, and undoing a
	// handoff-driven fulfillment cancels the transfer that drove it.
	if _, err := communityEventBus.Subscribe(requestService.TransferEventSubscriber()); err != nil {
		logger.Error("failed to subscribe request transfer-coupling subscriber", "error", err)
		return nil, err
	}
	if _, err := communityEventBus.Subscribe(transferService.RequestEventSubscriber()); err != nil {
		logger.Error("failed to subscribe transfer request-coupling subscriber", "error", err)
		return nil, err
	}

	// Event child transfers (#2708): when an event completes, the gear brought
	// to it completes its loan/giveaway; when the event is cancelled, those
	// pre-handoff transfers stand down. One-directional (experience → transfer),
	// plus the reverse unwind — a cancelled offer reopens its need slot.
	if _, err := communityEventBus.Subscribe(transferService.ExperienceEventSubscriber()); err != nil {
		logger.Error("failed to subscribe transfer experience-coupling subscriber", "error", err)
		return nil, err
	}
	if _, err := communityEventBus.Subscribe(experienceService.TransferEventSubscriber()); err != nil {
		logger.Error("failed to subscribe experience transfer-coupling subscriber", "error", err)
		return nil, err
	}

	// Wire the off-app invite email sender, gated by the same channel flag as the
	// off-app notification email. When disabled, email invitees still join the
	// ad-hoc community and are reachable via the host-shared open link.
	if cfg.OffAppEmailEnabled {
		communityService.SetInviteEmailSender(email.NewInviteSender(
			emailService,
			"https://"+cfg.InviteLinkHostname,
			[]byte(cfg.JWTSigningSecret),
			func(ctx context.Context, addr string) (bool, error) {
				return emailsuppression.IsSuppressed(ctx, sqlStorage, addr)
			},
		))
		logger.Info("off-app invite email sender enabled")
	}

	locationService := location.New(sqlStorage, bucketStorage, locationProvider)
	mediaService := media.New(sqlStorage, bucketStorage)
	mediaService.SetBranding(cfg.Branding)
	// Inject stock providers so AddMediaFromURL can route candidate
	// imports through the trusted by-id path — preserves attribution
	// and bypasses the body cap that protects the generic-URL surface.
	if stockImageryProvider != nil {
		mediaService.SetStockImageryProvider(stockImageryProvider)
	}
	if stockVideoProvider != nil {
		mediaService.SetStockVideoProvider(stockVideoProvider)
	}
	searchService := search.New(sqlStorage)
	waitlistService := waitlist.New(sqlStorage, emailService, cfg.WaitlistNotifyEmail)
	// Build login service config. Guard the PhoneAuth assignment to avoid the
	// Go nil-interface trap: a typed nil (*FirebaseAuth)(nil) stored in an
	// interface is non-nil, bypassing the nil check in PhoneRegister/PhoneLogin.
	// A malformed test-account list is a configuration mistake that would
	// otherwise surface as a failed app-store review, so it is fatal here
	// rather than silently disabling the path.
	emailCodeTestAccounts, err := login.ParseEmailOTPTestAccounts(cfg.EmailOTPTestAccounts)
	if err != nil {
		return nil, fmt.Errorf("invalid -email-otp-test-accounts: %w", err)
	}
	if len(emailCodeTestAccounts) > 0 {
		// Count only — the codes themselves are credentials.
		logging.Default().Warn("email OTP test accounts configured",
			"count", len(emailCodeTestAccounts),
		)
	}

	loginCfg := login.Config{
		UserManager:            userManager,
		AuthTokenConfig:        deps.authTokenConfig,
		OIDCManager:            deps.oidcManager,
		Storage:                sqlStorage,
		BucketStorage:          bucketStorage,
		EmailService:           emailService,
		NotificationService:    notificationService,
		Bus:                    communityEventBus,
		DevAuth:                cfg.DevMode,
		RefreshTokenExpiration: refreshTokenExpiration,
		EmailCodeTestAccounts:  emailCodeTestAccounts,
		Branding:               cfg.Branding,
	}
	if deps.firebaseAuth != nil {
		loginCfg.PhoneAuth = deps.firebaseAuth
	}
	loginService := login.New(loginCfg)
	// Pass the same typed-nil-guarded phone verifier the login service uses so
	// AddPhoneNumber is enabled exactly when phone auth is configured. The bus
	// lets phone-attach promotion announce community joins like an invite-link join.
	var userPhoneAuth auth.PhoneTokenVerifier
	if deps.firebaseAuth != nil {
		userPhoneAuth = deps.firebaseAuth
	}
	userService := user.New(userManager, sqlStorage, userPhoneAuth, communityEventBus)
	feedbackService := feedback.NewFromGitHubApp(ctx, github.AppConfig{
		AppID:          cfg.GitHubAppID,
		InstallationID: cfg.GitHubInstallationID,
		PrivateKey:     []byte(cfg.GitHubAppPrivateKey),
		Owner:          cfg.GitHubRepoOwner,
		Repo:           cfg.GitHubRepoName,
	}, bucketStorage)
	if cfg.GitHubAppID != 0 && cfg.GitHubInstallationID != 0 && cfg.GitHubAppPrivateKey != "" {
		healthService.RegisterFuncCached("github", healthsvc.DefaultExternalTTL, feedbackService.CheckHealth)
	}
	webService, err := websvc.New(sqlStorage, bucketStorage, cfg.InviteLinkHostname, []byte(cfg.JWTSigningSecret))
	if err != nil {
		logger.Error("failed to initialize web service", "error", err)
		return nil, err
	}
	// Product name, app host, custom scheme, store links, and legal footer for
	// the /go/ landings. Unset values omit their affordance rather than
	// pointing at another operator's listing.
	webService.SetBranding(cfg.Branding)
	// Verify inbound SMS webhook + delivery-status-callback signatures with the
	// Twilio auth token when present; without it (dev/local) both accept unsigned
	// requests.
	if cfg.TwilioAuthToken != "" {
		webService.SetSMSAuthToken(cfg.TwilioAuthToken)
	}
	// Verify Mailgun delivery-event webhooks. Unlike the SMS callbacks above,
	// an absent key does NOT mean "accept unsigned" — /email/status rejects
	// everything without it, because fabricated delivery events would corrupt
	// the signal that tells us whether sign-in codes are arriving at all.
	if cfg.MailgunWebhookSigningKey != "" {
		webService.SetEmailWebhookSigningKey(cfg.MailgunWebhookSigningKey)
	} else {
		logging.Default().Warn("Mailgun webhook signing key not set; email delivery events will be rejected and delivery metrics will be empty")
	}

	// The unified-create classifier rides the same Provider chain as every
	// other AI surface (Gemini primary, Anthropic fallback, ...), so it
	// inherits per-provider routing, fallback, and health-check rollup
	// automatically. When no Provider is configured, fall back to the stub
	// (dev / no-API-key environments).
	unifiedClassifier := unified_create.NewStubClassifier()
	if aiProvider != nil {
		unifiedClassifier = unified_create.NewProviderClassifier(aiProvider)
	}
	unifiedCreateService := unified_create.New(unifiedClassifier, aiProvider, sqlStorage, bucketStorage, webfetch.NewHTTPFetcher(), experienceService, gearService, requestService)

	return &serverServices{
		health:        healthService,
		admin:         adminService,
		chat:          chatService,
		community:     communityService,
		gear:          gearService,
		transfer:      transferService,
		request:       requestService,
		experience:    experienceService,
		unifiedCreate: unifiedCreateService,
		location:      locationService,
		media:         mediaService,
		user:          userService,
		device:        deviceService,
		portfolio:     portfolioService,
		feed:          feedService,
		esm:           esmService,
		workshop:      workshopService,
		profile:       profileService,
		search:        searchService,
		feedback:      feedbackService,
		impact:        impactService,
		login:         loginService,
		waitlist:      waitlistService,
		web:           webService,

		notificationService: notificationService,
		emailService:        emailService,
	}, nil
}
