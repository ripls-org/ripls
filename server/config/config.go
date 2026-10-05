package config

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/branding"
	"go.ripls.org/ripls/server/secretsflag"
)

// Config holds all server configuration from flags and environment.
type Config struct {
	LogLevel                     string
	LogFormat                    string
	LogSourceLocation            bool
	DBURL                        string
	Port                         string
	DevMode                      bool
	MockAIProvider               bool
	MockWeatherProvider          bool
	CORSOrigins                  []string
	GoogleClientID               string
	GeminiModel                  string
	GeminiTemperature            float64
	VertexAIProject              string
	VertexAILocation             string
	OpenAIAPIKey                 string
	OpenAIModel                  string
	OpenAITemperature            float64
	AnthropicAPIKey              string
	AnthropicModel               string
	AnthropicTemperature         float64
	LocalMediaStorage            string
	GCSBucket                    string
	NotificationProvider         string
	FirebaseProject              string
	MailgunAPIKey                string
	MailgunWebhookSigningKey     string
	MailgunDomain                string
	MailgunFromAddress           string
	MailgunPostalAddress         string
	OffAppEmailEnabled           bool
	EmailOTPTestAccounts         string
	TwilioAccountSID             string
	TwilioAuthToken              string
	TwilioMessagingSID           string
	PlatformSMSEnabled           bool
	InviteLinkHostname           string
	UnsplashAccessKey            string
	PexelsAPIKey                 string
	PixabayAPIKey                string
	GitHubAppID                  int64
	GitHubInstallationID         int64
	GitHubAppPrivateKey          string
	FeedbackSignerKey            string
	FeedbackSignerEmail          string
	FeedbackSignerPrivateKey     string
	GitHubRepoOwner              string
	GitHubRepoName               string
	MapboxAccessToken            string
	GoogleMapsAPIKeyServer       string
	MapProvider                  string
	EmbeddingModelPath           string
	EmbeddingVocabPath           string
	WaitlistNotifyEmail          string
	JWTSigningSecret             string
	CommunityPurgeDisabled       bool
	CommunityPurgeDryRun         bool
	ActivityDigestEnabled        bool
	ActivityDigestNotifyEmail    string
	ActivityDigestTimezone       string
	ActivityDigestSendHour       int
	ActivityWeeklyDigestEnabled  bool
	ActivityWeeklyDigestWeekday  string
	ActivityWeeklyDigestSendHour int

	// Branding is this instance's user-facing identity — name, links, legal
	// details. See server/branding for why these are configuration rather than
	// literals in the templates.
	Branding branding.Config
}

// secretFile pairs a credential flag value with its optional -<name>-file
// counterpart so resolveSecrets can process them uniformly.
type secretFile struct {
	name  string
	value *string
	path  *string
}

// Parse declares every server flag on the process-global flag set, parses the
// command line, resolves file-backed secrets, and validates startup
// invariants. It returns an error (rather than exiting) so main owns the exit
// path. Call it exactly once.
func Parse() (*Config, error) {
	logLevel := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	logFormat := flag.String("log-format", "json", "Log format: json, text")
	logSourceLocation := flag.Bool("log-source-location", false, "Include source file:line in log entries")
	dbURL := flag.String("db", "", "PostgreSQL connection URL (postgres://user:pass@host:port/db)")
	dbPasswordFile := flag.String("db-password-file", "", "Path to a file containing the postgres password. When set, -db must omit the password (e.g. postgres://user@host/db) and this file's contents are injected at startup.")
	port := flag.String("port", "8080", "Port to listen on")
	devMode := flag.Bool("dev-mode", false, "Enable development mode (dev authentication, simulation clock, database reset)")
	mockAIProvider := flag.Bool("mock-ai-provider", false, "Use a deterministic in-process AI provider instead of a live LLM (e2e/dev only; never production). Lets tests drive the real unified-create UI without LLM credentials.")
	mockWeatherProvider := flag.Bool("mock-weather-provider", false, "Use a deterministic in-process weather provider instead of a live forecast service (e2e/dev only; never production). Keeps runs hermetic and lets them render dates the live forecast horizon does not cover.")
	corsAllowedOrigins := flag.String("cors-allowed-origins", "", "Comma-separated list of allowed CORS origins (e.g., https://app.example.com,http://localhost:*). Required in production mode.")
	googleClientID := flag.String("google-client-id", "", "Google OAuth Client ID for OIDC authentication. Required to accept Google sign-in.")
	geminiModel := flag.String("gemini-model", "", "Gemini model name override")
	geminiTemperature := flag.Float64("gemini-temperature", ai.DefaultTemperature, "Gemini sampling temperature for Gen* calls")
	vertexAIProject := flag.String("vertex-ai-project", "", "GCP project ID for Vertex AI (required for AI features)")
	vertexAILocation := flag.String("vertex-ai-location", "global", "GCP location for Vertex AI; 'global' is the only endpoint that exposes newer Gemini families like 3.x")
	openAIAPIKey := flag.String("openai-api-key", "", "OpenAI API key")
	openAIAPIKeyFile := flag.String("openai-api-key-file", "", "Path to a file containing the OpenAI API key (mutually exclusive with -openai-api-key)")
	openAIModel := flag.String("openai-model", "", "OpenAI model name override")
	openAITemperature := flag.Float64("openai-temperature", ai.DefaultTemperature, "OpenAI sampling temperature for Gen* calls")
	anthropicAPIKey := flag.String("anthropic-api-key", "", "Anthropic API key")
	anthropicAPIKeyFile := flag.String("anthropic-api-key-file", "", "Path to a file containing the Anthropic API key (mutually exclusive with -anthropic-api-key)")
	anthropicModel := flag.String("anthropic-model", "", "Anthropic model name override")
	anthropicTemperature := flag.Float64("anthropic-temperature", ai.DefaultTemperature, "Anthropic sampling temperature for Gen* calls")
	localMediaStorage := flag.String("local-media-storage", "", "Local filesystem path for media bucket storage (development mode - mutually exclusive with --gcs-bucket)")
	gcsBucket := flag.String("gcs-bucket", "", "GCS bucket name for media storage (production mode, mutually exclusive with --local-media-storage)")
	notificationProvider := flag.String("notification-provider", "test", "Notification provider to use: 'fcm' or 'test' (default: test)")
	firebaseProject := flag.String("firebase-project", "", "Firebase / GCP project ID for FCM and phone auth (required locally under user ADC; when GOOGLE_APPLICATION_CREDENTIALS points at a service-account key, the SDK reads project_id from the key instead)")
	mailgunAPIKey := flag.String("mailgun-api-key", "", "Mailgun API key for sending emails (if empty, email service is disabled)")
	mailgunAPIKeyFile := flag.String("mailgun-api-key-file", "", "Path to a file containing the Mailgun API key (mutually exclusive with -mailgun-api-key)")
	mailgunWebhookSigningKey := flag.String("mailgun-webhook-signing-key", "", "Mailgun webhook signing key (distinct from the API key) used to verify delivery-event webhooks on POST /email/status. Empty rejects every webhook, so delivery metrics go dark rather than accepting unsigned payloads.")
	mailgunWebhookSigningKeyFile := flag.String("mailgun-webhook-signing-key-file", "", "Path to a file containing the Mailgun webhook signing key (mutually exclusive with -mailgun-webhook-signing-key)")
	mailgunDomain := flag.String("mailgun-domain", "", "Mailgun domain for sending emails")
	mailgunFromAddress := flag.String("mailgun-from", "", "From address for outgoing emails (e.g., \"Example <noreply@example.com>\")")
	mailgunPostalAddress := flag.String("mailgun-postal-address", "", "CAN-SPAM physical postal address rendered in the off-app notification email footer (non-secret; public in every email). Required before enabling the off-app email channel.")
	offAppEmailEnabled := flag.Bool("off-app-email-enabled", false, "Enable the off-app email notification channel: when a recipient has no active app device, send the notification as an email (EMAIL-1). Off by default — flip on once CAN-SPAM copy (postal address) and deliverability are signed off.")
	emailOTPTestAccounts := flag.String("email-otp-test-accounts", "", "Comma-separated address:code pairs whose email sign-in code is fixed instead of mailed, for app-store review and manual QA (#2571). Both the address and its code must match — a code alone never verifies an address. Empty (the default) means every address goes through the real mailed-code path.")
	emailOTPTestAccountsFile := flag.String("email-otp-test-accounts-file", "", "Path to a file containing the email-OTP test accounts (mutually exclusive with -email-otp-test-accounts)")
	twilioAccountSID := flag.String("twilio-account-sid", "", "Twilio Account SID for platform SMS (empty disables SMS)")
	twilioAccountSIDFile := flag.String("twilio-account-sid-file", "", "Path to a file containing the Twilio Account SID (mutually exclusive with -twilio-account-sid)")
	twilioAuthToken := flag.String("twilio-auth-token", "", "Twilio Auth Token for platform SMS and inbound webhook signature verification")
	twilioAuthTokenFile := flag.String("twilio-auth-token-file", "", "Path to a file containing the Twilio Auth Token (mutually exclusive with -twilio-auth-token)")
	twilioMessagingSID := flag.String("twilio-messaging-service-sid", "", "Twilio Messaging Service SID routing platform SMS through the registered sender pool and A2P campaign")
	twilioMessagingSIDFile := flag.String("twilio-messaging-service-sid-file", "", "Path to a file containing the Twilio Messaging Service SID (mutually exclusive with -twilio-messaging-service-sid)")
	platformSMSEnabled := flag.Bool("platform-sms-enabled", false, "Enable the platform-originated SMS notification channel (#2492): text a deviceless recipient who has a phone handle. Off by default — flip on only after A2P 10DLC registration and counsel sign-off. Email needs no flag.")
	inviteLinkHostname := flag.String("invite-link-hostname", "", "Hostname for user-facing links — invitations, password resets, and the web landings all derive their origin from this (e.g., app.example.com or dev.example.com). Required.")
	unsplashAccessKey := flag.String("unsplash-access-key", "", "Unsplash API access key for fetching stock images for requests (if empty disabled)")
	unsplashAccessKeyFile := flag.String("unsplash-access-key-file", "", "Path to a file containing the Unsplash access key (mutually exclusive with -unsplash-access-key)")
	pexelsAPIKey := flag.String("pexels-api-key", "", "Pexels API key for fetching stock images (if empty disabled)")
	pexelsAPIKeyFile := flag.String("pexels-api-key-file", "", "Path to a file containing the Pexels API key (mutually exclusive with -pexels-api-key)")
	pixabayAPIKey := flag.String("pixabay-api-key", "", "Pixabay API key for fetching stock videos as fallback (if empty disabled)")
	pixabayAPIKeyFile := flag.String("pixabay-api-key-file", "", "Path to a file containing the Pixabay API key (mutually exclusive with -pixabay-api-key)")
	githubAppID := flag.Int64("github-app-id", 0, "GitHub App ID for feedback bot")
	githubInstallationID := flag.Int64("github-installation-id", 0, "GitHub App Installation ID for ripls repository")
	githubAppPrivateKey := flag.String("github-app-private-key", "", "GitHub App private key (base64-encoded PEM format)")
	githubAppPrivateKeyFile := flag.String("github-app-private-key-file", "", "Path to a file containing the base64-encoded GitHub App private key PEM (mutually exclusive with -github-app-private-key)")
	feedbackSignerKey := flag.String("feedback-signer-key", "", "Service-account JSON key used to sign durable feedback screenshot URLs (mutually exclusive with -feedback-signer-key-file). When unset, feedback URLs fall back to short-lived signBlob signing. See #2549.")
	feedbackSignerKeyFile := flag.String("feedback-signer-key-file", "", "Path to a file containing the service-account JSON key for signing durable feedback screenshot URLs (mutually exclusive with -feedback-signer-key)")
	githubRepoOwner := flag.String("github-repo-owner", "", "GitHub repository owner that receives user-feedback issues")
	githubRepoName := flag.String("github-repo-name", "ripls", "GitHub repository name")
	mapboxAccessToken := flag.String("mapbox-access-token", "", "Mapbox API access token for geocoding (if empty disabled)")
	mapboxAccessTokenFile := flag.String("mapbox-access-token-file", "", "Path to a file containing the Mapbox access token (mutually exclusive with -mapbox-access-token)")
	googleMapsAPIKeyServer := flag.String("google-maps-api-key-server", "", "Google Maps Platform API key for the server-side location provider (#2188). Restricted to Places API (New) + Geocoding API; client SDK keys are distinct. Empty disables the Google client; Phase 3+ wires it through location.Provider.")
	googleMapsAPIKeyServerFile := flag.String("google-maps-api-key-server-file", "", "Path to a file containing the server-side Google Maps API key (mutually exclusive with -google-maps-api-key-server)")
	mapProvider := flag.String("map-provider", "mapbox", "Active location provider for geocoding + place search (#2188): 'mapbox' or 'google'. The selected provider's credential flag (--mapbox-access-token / --google-maps-api-key-server) must be non-empty.")
	embeddingModelPath := flag.String("embedding-model-path", "", "Path to local ONNX embedding model file")
	embeddingVocabPath := flag.String("embedding-vocab-path", "", "Path to embedding model vocabulary file (vocab.txt)")
	waitlistNotifyEmail := flag.String("waitlist-notify-email", "", "Email address to receive waitlist signup notifications. Empty disables the notification.")
	jwtSigningSecret := flag.String("jwt-signing-secret", "", "JWT signing secret (required, must be at least 32 bytes)")
	jwtSigningSecretFile := flag.String("jwt-signing-secret-file", "", "Path to a file containing the JWT signing secret (mutually exclusive with -jwt-signing-secret)")
	communityPurgeDisabled := flag.Bool("community-purge-disabled", false, "Disable the daily community purge job (#1620 kill switch). Use to stop ongoing hard-delete activity without a redeploy.")
	communityPurgeDryRun := flag.Bool("community-purge-dry-run", false, "Run the daily community purge in dry-run mode (#1620 Safeguard #2). Logs what would have been deleted but rolls back the cascade. Default false.")
	activityDigestEnabled := flag.Bool("activity-digest-enabled", false, "Enable the daily activity digest job (#1924). Off by default; flip to true once dev output looks right.")
	activityDigestNotifyEmail := flag.String("activity-digest-notify-email", "", "Recipient of the activity digest emails — daily and weekly share this address (#1924). Empty disables the digests.")
	activityDigestTimezone := flag.String("activity-digest-timezone", "UTC", "IANA timezone defining 'yesterday' for the daily activity digest and 'last week' for the weekly digest (#1924). Set this to the operator's local zone; UTC is a neutral default, not a good one.")
	activityDigestSendHour := flag.Int("activity-digest-send-hour", 8, "Local hour (0-23) the daily activity digest fires each day (#1924).")
	activityWeeklyDigestEnabled := flag.Bool("activity-digest-weekly-enabled", false, "Enable the weekly activity digest job (#1924). Off by default. Independent of --activity-digest-enabled.")
	activityWeeklyDigestWeekday := flag.String("activity-digest-weekly-weekday", "Monday", "Weekday the weekly activity digest fires (English day name). Defaults to Monday.")
	activityWeeklyDigestSendHour := flag.Int("activity-digest-weekly-send-hour", 8, "Local hour (0-23) the weekly activity digest fires.")
	branding := declareBrandingFlags()
	flag.Parse()

	cfg := &Config{
		LogLevel:                     *logLevel,
		LogFormat:                    *logFormat,
		LogSourceLocation:            *logSourceLocation,
		DBURL:                        *dbURL,
		Port:                         *port,
		DevMode:                      *devMode,
		MockAIProvider:               *mockAIProvider,
		MockWeatherProvider:          *mockWeatherProvider,
		CORSOrigins:                  parseCORSOrigins(*corsAllowedOrigins),
		GoogleClientID:               *googleClientID,
		GeminiModel:                  *geminiModel,
		GeminiTemperature:            *geminiTemperature,
		VertexAIProject:              *vertexAIProject,
		VertexAILocation:             *vertexAILocation,
		OpenAIAPIKey:                 *openAIAPIKey,
		OpenAIModel:                  *openAIModel,
		OpenAITemperature:            *openAITemperature,
		AnthropicAPIKey:              *anthropicAPIKey,
		AnthropicModel:               *anthropicModel,
		AnthropicTemperature:         *anthropicTemperature,
		LocalMediaStorage:            *localMediaStorage,
		GCSBucket:                    *gcsBucket,
		NotificationProvider:         *notificationProvider,
		FirebaseProject:              *firebaseProject,
		MailgunAPIKey:                *mailgunAPIKey,
		MailgunWebhookSigningKey:     *mailgunWebhookSigningKey,
		MailgunDomain:                *mailgunDomain,
		MailgunFromAddress:           *mailgunFromAddress,
		MailgunPostalAddress:         *mailgunPostalAddress,
		OffAppEmailEnabled:           *offAppEmailEnabled,
		EmailOTPTestAccounts:         *emailOTPTestAccounts,
		TwilioAccountSID:             *twilioAccountSID,
		TwilioAuthToken:              *twilioAuthToken,
		TwilioMessagingSID:           *twilioMessagingSID,
		PlatformSMSEnabled:           *platformSMSEnabled,
		InviteLinkHostname:           *inviteLinkHostname,
		UnsplashAccessKey:            *unsplashAccessKey,
		PexelsAPIKey:                 *pexelsAPIKey,
		PixabayAPIKey:                *pixabayAPIKey,
		GitHubAppID:                  *githubAppID,
		GitHubInstallationID:         *githubInstallationID,
		GitHubAppPrivateKey:          *githubAppPrivateKey,
		FeedbackSignerKey:            *feedbackSignerKey,
		GitHubRepoOwner:              *githubRepoOwner,
		GitHubRepoName:               *githubRepoName,
		MapboxAccessToken:            *mapboxAccessToken,
		GoogleMapsAPIKeyServer:       *googleMapsAPIKeyServer,
		MapProvider:                  *mapProvider,
		EmbeddingModelPath:           *embeddingModelPath,
		EmbeddingVocabPath:           *embeddingVocabPath,
		WaitlistNotifyEmail:          *waitlistNotifyEmail,
		JWTSigningSecret:             *jwtSigningSecret,
		CommunityPurgeDisabled:       *communityPurgeDisabled,
		CommunityPurgeDryRun:         *communityPurgeDryRun,
		ActivityDigestEnabled:        *activityDigestEnabled,
		ActivityDigestNotifyEmail:    *activityDigestNotifyEmail,
		ActivityDigestTimezone:       *activityDigestTimezone,
		ActivityDigestSendHour:       *activityDigestSendHour,
		ActivityWeeklyDigestEnabled:  *activityWeeklyDigestEnabled,
		ActivityWeeklyDigestWeekday:  *activityWeeklyDigestWeekday,
		ActivityWeeklyDigestSendHour: *activityWeeklyDigestSendHour,
		Branding:                     branding.resolve(*inviteLinkHostname, *mailgunPostalAddress),
	}

	// Resolve every credential that supports a paired -<name>-file flag.
	// Order tracks the flag declarations above. See server/secretsflag/README.md
	// for the precedence rules.
	secrets := []secretFile{
		{"openai-api-key", &cfg.OpenAIAPIKey, openAIAPIKeyFile},
		{"anthropic-api-key", &cfg.AnthropicAPIKey, anthropicAPIKeyFile},
		{"mailgun-api-key", &cfg.MailgunAPIKey, mailgunAPIKeyFile},
		{"mailgun-webhook-signing-key", &cfg.MailgunWebhookSigningKey, mailgunWebhookSigningKeyFile},
		{"email-otp-test-accounts", &cfg.EmailOTPTestAccounts, emailOTPTestAccountsFile},
		{"twilio-account-sid", &cfg.TwilioAccountSID, twilioAccountSIDFile},
		{"twilio-auth-token", &cfg.TwilioAuthToken, twilioAuthTokenFile},
		{"twilio-messaging-service-sid", &cfg.TwilioMessagingSID, twilioMessagingSIDFile},
		{"unsplash-access-key", &cfg.UnsplashAccessKey, unsplashAccessKeyFile},
		{"pexels-api-key", &cfg.PexelsAPIKey, pexelsAPIKeyFile},
		{"pixabay-api-key", &cfg.PixabayAPIKey, pixabayAPIKeyFile},
		{"github-app-private-key", &cfg.GitHubAppPrivateKey, githubAppPrivateKeyFile},
		{"feedback-signer-key", &cfg.FeedbackSignerKey, feedbackSignerKeyFile},
		{"mapbox-access-token", &cfg.MapboxAccessToken, mapboxAccessTokenFile},
		{"google-maps-api-key-server", &cfg.GoogleMapsAPIKeyServer, googleMapsAPIKeyServerFile},
		{"jwt-signing-secret", &cfg.JWTSigningSecret, jwtSigningSecretFile},
	}
	if err := cfg.finalize(secrets, *dbPasswordFile); err != nil {
		return nil, err
	}
	return cfg, nil
}

// finalize resolves file-backed secrets into cfg and enforces the startup
// invariants that used to abort main() directly. Split from Parse so the
// resolution and validation rules are unit-testable without re-declaring
// process-global flags.
func (cfg *Config) finalize(secrets []secretFile, dbPasswordFile string) error {
	if cfg.DBURL == "" {
		return fmt.Errorf("--db flag is required (e.g., postgres://user:pass@host:port/db)")
	}

	for _, s := range secrets {
		v, err := secretsflag.Resolve(s.name, *s.value, *s.path)
		if err != nil {
			return err
		}
		*s.value = v
	}

	// DB password is special — it lives inside the connection URL rather than
	// as a top-level flag, so the helper injects it into the URL when
	// -db-password-file is set.
	finalDBURL, err := secretsflag.InjectPostgresPassword(cfg.DBURL, dbPasswordFile)
	if err != nil {
		return err
	}
	cfg.DBURL = finalDBURL

	if cfg.GitHubAppPrivateKey != "" {
		pem, err := decodeBase64PEM(cfg.GitHubAppPrivateKey)
		if err != nil {
			return fmt.Errorf("--github-app-private-key is not valid base64: %w", err)
		}
		cfg.GitHubAppPrivateKey = pem
	}

	// Parse the feedback URL signer key (a GCP service-account JSON) into the
	// email + PEM private key the GCS storage layer needs. Empty ⇒ leave both
	// unset so feedback signing degrades to short-lived signBlob.
	if cfg.FeedbackSignerKey != "" {
		email, pem, err := parseServiceAccountSignerKey(cfg.FeedbackSignerKey)
		if err != nil {
			return fmt.Errorf("--feedback-signer-key is not a valid service-account key: %w", err)
		}
		cfg.FeedbackSignerEmail, cfg.FeedbackSignerPrivateKey = email, pem
	}

	// Vertex's project is inferable from the environment, so an unset flag is
	// not the same as "no AI". Cloud Run sets GOOGLE_CLOUD_PROJECT and
	// entrypoint.sh already forwards it as -vertex-ai-project; doing the same
	// here means local runs, CI, and production all resolve it one way instead
	// of a compiled-in default naming one particular deployment.
	if cfg.VertexAIProject == "" {
		for _, env := range []string{"VERTEX_AI_PROJECT", "GOOGLE_CLOUD_PROJECT"} {
			if v := os.Getenv(env); v != "" {
				cfg.VertexAIProject = v
				break
			}
		}
	}

	if cfg.InviteLinkHostname == "" {
		return fmt.Errorf("--invite-link-hostname is required (e.g., app.example.com or dev.example.com)")
	}

	if err := cfg.Branding.Validate(); err != nil {
		return err
	}

	if err := auth.ValidateSigningSecret([]byte(cfg.JWTSigningSecret)); err != nil {
		return fmt.Errorf("--jwt-signing-secret is required and must be at least 32 bytes: %w", err)
	}

	// --map-provider must be one of the known providers. Credential
	// availability is checked later, after Secret Manager fetches resolve.
	switch cfg.MapProvider {
	case "mapbox", "google":
	default:
		return fmt.Errorf("--map-provider must be 'mapbox' or 'google'; got %q", cfg.MapProvider)
	}

	// Resolve and validate CORS origins. In dev mode an empty list defaults to
	// wildcard; in production an explicit allowlist is required and wildcard is
	// rejected.
	if len(cfg.CORSOrigins) == 0 {
		if !cfg.DevMode {
			return fmt.Errorf("--cors-allowed-origins is required in production mode (e.g., https://app.example.com,https://dev.example.com)")
		}
		cfg.CORSOrigins = []string{"*"}
	}
	if !cfg.DevMode {
		for _, o := range cfg.CORSOrigins {
			if o == "*" {
				return fmt.Errorf("wildcard CORS origin (*) is not permitted in production mode; use an explicit --cors-allowed-origins allowlist")
			}
		}
	}

	return nil
}

// decodeBase64PEM decodes a base64-encoded PEM string and returns the raw PEM text.
func decodeBase64PEM(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// parseServiceAccountSignerKey extracts the client_email and PEM private_key
// from a GCP service-account JSON key, for use as an explicit GCS URL signer
// (#2549). Both fields are required; a malformed or partial key is an error
// rather than a silently disabled signer.
func parseServiceAccountSignerKey(jsonKey string) (email, privateKeyPEM string, err error) {
	var key struct {
		ClientEmail string `json:"client_email"`
		PrivateKey  string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(jsonKey), &key); err != nil {
		return "", "", fmt.Errorf("parse service-account JSON: %w", err)
	}
	if key.ClientEmail == "" || key.PrivateKey == "" {
		return "", "", fmt.Errorf("service-account key missing client_email or private_key")
	}
	return key.ClientEmail, key.PrivateKey, nil
}

// parseCORSOrigins splits a comma-separated origin string into a slice, trimming
// whitespace and dropping empty entries.
func parseCORSOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ParseWeekday converts an English weekday name ("Monday", "tue", etc.)
// into a time.Weekday. Case-insensitive; common 3-letter abbreviations
// are accepted.
func ParseWeekday(s string) (time.Weekday, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "sunday", "sun":
		return time.Sunday, nil
	case "monday", "mon":
		return time.Monday, nil
	case "tuesday", "tue", "tues":
		return time.Tuesday, nil
	case "wednesday", "wed":
		return time.Wednesday, nil
	case "thursday", "thu", "thur", "thurs":
		return time.Thursday, nil
	case "friday", "fri":
		return time.Friday, nil
	case "saturday", "sat":
		return time.Saturday, nil
	}
	return time.Sunday, fmt.Errorf("unrecognized weekday %q", s)
}
