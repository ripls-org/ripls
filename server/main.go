package main

// This is the main server binary that implements the Gear and Login service APIs.

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/ai/embedding"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/config"
	riplsfirebase "go.ripls.org/ripls/server/firebase"
	"go.ripls.org/ripls/server/logging"
	healthsvc "go.ripls.org/ripls/server/services/health"
	"go.ripls.org/ripls/server/storage"
)

// HTTP server timeouts.
//
// httpReadHeaderTimeout and httpReadTimeout guard against slow-loris on the
// request side and don't affect server-streaming RPCs.
//
// httpWriteTimeout is intentionally 0 (no cap). Connect server-streaming RPCs
// are written as a single ongoing HTTP response, so any non-zero WriteTimeout
// force-closes long-lived streams when the deadline hits — even though the
// streaming handlers in server/services/community already cap each stream at
// defaultStreamLifetime (~14 min, sized to sit under Cloud Run's 900 s
// frontend timeout). Stream handlers own their own deadline; the HTTP layer
// must not preempt them. See #1810.
//
// httpIdleTimeout is set above defaultStreamLifetime so persistent HTTP/2
// connections used for back-to-back streams aren't reaped mid-conversation.
const (
	httpReadHeaderTimeout = 10 * time.Second
	httpReadTimeout       = 30 * time.Second
	httpWriteTimeout      = 0
	httpIdleTimeout       = 15 * time.Minute
)

// initializeAIProvider initializes the AI provider based on configuration.
// Returns nil if no AI provider is configured.
func initializeAIProvider(ctx context.Context, cfg *config.Config, logger *logging.Logger) (ai.Provider, error) {
	// e2e/dev escape hatch: a deterministic in-process provider so tests can drive
	// the real unified-create UI without LLM credentials. Never set in production.
	if cfg.MockAIProvider {
		logger.Warn("using deterministic in-process AI provider (--mock-ai-provider); e2e/dev only, never production")
		return ai.NewE2EDeterministicProvider(), nil
	}

	provider, err := ai.NewAllProvidersWithFallback(ctx, ai.AllProvidersConfig{
		GeminiModelOverride:    cfg.GeminiModel,
		GeminiTemperature:      cfg.GeminiTemperature,
		VertexAIProject:        cfg.VertexAIProject,
		VertexAILocation:       cfg.VertexAILocation,
		OpenAIAPIKey:           cfg.OpenAIAPIKey,
		OpenAIModelOverride:    cfg.OpenAIModel,
		OpenAITemperature:      cfg.OpenAITemperature,
		AnthropicAPIKey:        cfg.AnthropicAPIKey,
		AnthropicModelOverride: cfg.AnthropicModel,
		AnthropicTemperature:   cfg.AnthropicTemperature,
	})
	if err != nil {
		return nil, err
	}
	if provider != nil {
		logger.Info("AI provider initialized")
	}
	return provider, nil
}

// initializeEmbedder creates the local embedding generator.
// Returns an error if the model files are not configured or initialization fails.
func initializeEmbedder(cfg *config.Config, logger *logging.Logger) (*embedding.Embedder, error) {
	if cfg.EmbeddingModelPath == "" {
		return nil, fmt.Errorf("embedding model path is required (-embedding-model-path)")
	}
	if cfg.EmbeddingVocabPath == "" {
		return nil, fmt.Errorf("embedding vocab path is required (-embedding-vocab-path)")
	}

	embedder, err := embedding.New(cfg.EmbeddingModelPath, cfg.EmbeddingVocabPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize embedder: %w", err)
	}

	info := embedder.Info()
	logger.Info("local embedder initialized",
		"model", info.Model,
		"dimensions", info.Dimensions)
	return embedder, nil
}

// Parse flags and start a server backed by the specified database, listening on the specified port.
func main() {
	cfg, err := config.Parse()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	// Initialize structured logger
	logger := logging.NewLogger(logging.Options{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: cfg.LogSourceLocation,
	})
	logging.SetDefault(logger)

	// Initialize health service early so dependencies can register themselves
	healthService := healthsvc.New(logger)

	ctx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	sqlStorage, err := storage.InitializeDatabase(ctx, cfg.DBURL, logger)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		return
	}
	defer sqlStorage.Close()
	healthService.RegisterFunc("database", sqlStorage.CheckHealth)

	// Initialize bucket storage
	serverURL := fmt.Sprintf("http://localhost:%s", cfg.Port)
	bucketStorage, err := storage.InitializeBucketStorage(ctx, cfg.LocalMediaStorage, cfg.GCSBucket, serverURL, cfg.FeedbackSignerEmail, cfg.FeedbackSignerPrivateKey, logger)
	if err != nil {
		sqlStorage.Close() // Close storage before fatal exit
		logger.Error("failed to initialize bucket storage", "error", err)
		return
	}
	healthService.RegisterFunc("storage", bucketStorage.CheckHealth)

	// Set up token-based authentication
	authTokenConfig := &auth.TokenConfig{
		Secret:     []byte(cfg.JWTSigningSecret),
		Expiration: 1 * time.Hour,
	}
	authMiddleware := authn.NewMiddleware(authTokenConfig.NewAuthFunc())

	// Create OIDC manager for provider authentication
	oidcManager := auth.NewOIDCProviderManager()

	// Configure Google OIDC if client ID is provided
	if cfg.GoogleClientID != "" {
		logger.Info("configuring Google OIDC", "client_id_prefix", cfg.GoogleClientID[:min(len(cfg.GoogleClientID), 10)])
		if err := oidcManager.RegisterGoogleProvider(ctx, cfg.GoogleClientID); err != nil {
			logger.Warn("failed to register Google OIDC provider", "error", err)
		} else {
			logger.Info("Google OIDC provider registered")
		}
	} else {
		logger.Info("Google Client ID not set, Google OIDC disabled")
	}

	// Initialize AI provider
	aiProvider, err := initializeAIProvider(ctx, cfg, logger)
	if err != nil {
		logger.Warn("failed to initialize AI provider", "error", err)
		logger.Info("AI-powered features will be disabled")
	}
	if aiProvider != nil {
		healthService.RegisterFuncCached("ai", healthsvc.DefaultExternalTTL, aiProvider.CheckHealth)
	}

	// Initialize local embedder for semantic search (required)
	embedder, err := initializeEmbedder(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize embedder", "error", err)
		return
	}
	defer embedder.Close()
	if err := sqlStorage.SetEmbedder(ctx, embedder); err != nil {
		logger.Error("failed to configure embedder", "error", err)
		return
	}
	// Backfill missing embeddings in background. Uses the root ctx so
	// shutdown cancels the scan instead of racing the closing DB pool.
	logging.GoSafe(ctx, "backfill-embeddings", func() {
		sqlStorage.BackfillEmbeddings(ctx)
	})

	// One shared Firebase app backs both phone-verification auth and (in fcm
	// mode) the push-messaging client. Fatal if unavailable.
	firebaseApp, err := riplsfirebase.NewApp(ctx, cfg.FirebaseProject)
	if err != nil {
		logger.Error("failed to initialize Firebase app", "error", err)
		return
	}
	// Optional in dev mode, fatal otherwise. The auth client is the one piece
	// of startup that needs Google application default credentials, so
	// requiring it unconditionally meant the server could not boot at all
	// without them — and every test that starts a real server died behind
	// "server did not become ready", 39 of them in one run, from this single
	// cause (#2953). That is the situation in the open-source repo and in any
	// fork, neither of which has credentials.
	//
	// Nothing downstream assumes it exists: wireServices guards both uses of
	// firebaseAuth against nil (with an explicit note about the typed-nil
	// interface trap), so a nil here means phone verification is off, the same
	// state as a deployment that never configured it. That matches how the
	// rest of startup already behaves — Mailgun falls back to a mock when its
	// key is absent, and the notification provider is chosen by config.
	//
	// Gated on DevMode, which defaults to false, so production still refuses
	// to start half-configured.
	var firebaseAuth *auth.FirebaseAuth
	if fa, err := auth.NewFirebaseAuth(ctx, firebaseApp); err != nil {
		if !cfg.DevMode {
			logger.Error("failed to initialize Firebase auth", "error", err)
			return
		}
		logger.Warn("Firebase auth unavailable — phone verification disabled (dev mode)", "error", err)
	} else {
		firebaseAuth = fa
		logger.Info("Firebase auth initialized for phone verification")
	}

	// Wire the full service graph (wiring.go). Fatal wiring failures are
	// logged inside wireServices with the specific cause.
	svcs, err := wireServices(ctx, cfg, logger, wiringDeps{
		health:          healthService,
		sqlStorage:      sqlStorage,
		bucketStorage:   bucketStorage,
		aiProvider:      aiProvider,
		firebaseApp:     firebaseApp,
		firebaseAuth:    firebaseAuth,
		authTokenConfig: authTokenConfig,
		oidcManager:     oidcManager,
	})
	if err != nil {
		return
	}

	// Start background jobs (jobs.go) and assemble the HTTP surface (routes.go).
	startBackgroundJobs(ctx, cfg, logger, sqlStorage, svcs.notificationService, svcs.emailService)
	handler := buildHandler(cfg, logger, sqlStorage, bucketStorage, authMiddleware, svcs)

	addr := fmt.Sprintf("0.0.0.0:%s", cfg.Port)
	logger.Info("starting server", "address", addr)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Serve both HTTP/1.1 and unencrypted HTTP/2 (h2c). Connect clients
	// negotiate HTTP/2 over cleartext; the http.Server dispatches each
	// h2c stream through Handler, so the middleware chain runs per stream.
	// This replaces the deprecated golang.org/x/net/http2/h2c handler.
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	protocols.SetUnencryptedHTTP2(true)

	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		Protocols:         protocols,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
		IdleTimeout:       httpIdleTimeout,
	}

	// Bind the listener up front so a failure here aborts startup. Doing
	// this synchronously (instead of inside the goroutine that runs Serve)
	// guarantees the process exits non-zero when the port is already in
	// use rather than logging the error and hanging on the signal channel.
	var lc net.ListenConfig
	listener, err := lc.Listen(ctx, "tcp", server.Addr)
	if err != nil {
		logger.Error("server failed to bind listener", "address", server.Addr, "error", err)
		os.Exit(1)
	}

	// Start server in background goroutine
	logging.GoSafe(ctx, "http-serve", func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
		}
	})

	logger.Info("server started", "address", addr)

	// Block until shutdown signal received
	sig := <-sigCh
	fmt.Fprintf(os.Stderr, "\nserver shutting down (%s)...\n", sig)

	// Signal background goroutines (backfill jobs) to stop.
	rootCancel()

	// Give in-flight requests time to complete
	drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(drainCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
	fmt.Fprintln(os.Stderr, "server stopped.")
}
