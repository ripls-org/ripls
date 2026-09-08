package integration_tests

// Shared test infrastructure for integration tests.
// This file contains common helpers for server startup, user registration,
// and authenticated client creation.

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/middleware"
	"go.ripls.org/ripls/server/notifications"
	commsub "go.ripls.org/ripls/server/notifications/community_subscriber"
	"go.ripls.org/ripls/server/pubsub"
	community_svc "go.ripls.org/ripls/server/services/community"
	"go.ripls.org/ripls/server/storage"
)

// serverBinaryOnce ensures the server binary is built only once per test run.
var (
	serverBinaryOnce sync.Once
	serverBinaryPath string
	errServerBuild   error
)

// buildServerBinary builds the server binary once and returns its path.
func buildServerBinary(t *testing.T) string {
	serverBinaryOnce.Do(func() {
		tmpDir := os.TempDir()
		serverBinaryPath = filepath.Join(tmpDir, "ripls-test-server")
		cmd := exec.Command("go", "build", "-o", serverBinaryPath, ".")
		cmd.Dir = ".."
		if output, err := cmd.CombinedOutput(); err != nil {
			errServerBuild = fmt.Errorf("failed to build server: %w\n%s", err, output)
		}
	})
	if errServerBuild != nil {
		t.Fatal(errServerBuild)
	}
	return serverBinaryPath
}

// stopServer sends SIGTERM for graceful shutdown, then waits.
func stopServer(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// Send SIGTERM for graceful shutdown.
	_ = cmd.Process.Signal(syscall.SIGTERM)
	// Wait briefly for clean exit; force kill if it hangs.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

// authTransport adds Authorization header to requests.
type authTransport struct {
	base  http.RoundTripper
	token string
}

// RoundTrip implements http.RoundTripper.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token != "" {
		req.Header.Set("Authorization", "Bearer "+t.token)
	}
	return t.base.RoundTrip(req)
}

// findFreePort finds an available port on localhost.
func findFreePort() (string, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return fmt.Sprintf("%d", addr.Port), nil
}

// waitForServerReady waits for the server to respond to health checks.
func waitForServerReady(serverURL string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{Timeout: 1 * time.Second}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("server did not become ready within %v", timeout)
		case <-ticker.C:
			hreq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, serverURL+"/", nil)
			if reqErr != nil {
				return fmt.Errorf("build readiness request: %w", reqErr)
			}
			resp, err := client.Do(hreq)
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// startTestServer starts the actual server binary using go run with dev auth enabled.
func startTestServer(t *testing.T, dbURL string) (*exec.Cmd, string) {
	mediaStoragePath := t.TempDir()
	return startTestServerWithMedia(t, dbURL, mediaStoragePath)
}

// startTestServerWithMedia starts the server with local media storage path.
func startTestServerWithMedia(t *testing.T, dbURL, mediaStoragePath string) (*exec.Cmd, string) {
	binary := buildServerBinary(t)

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	serverURL := fmt.Sprintf("http://localhost:%s", port)

	args := []string{
		"-db", dbURL, "-port", port, "-dev-mode", "-log-format", "json",
		"-embedding-model-path", "../model_tuning/ripls_embedding.onnx",
		"-embedding-vocab-path", "../model_tuning/ripls_embedding_tokenizer/vocab.txt",
		"-invite-link-hostname", "test.example.app",
		"-jwt-signing-secret", "test-jwt-signing-secret-not-real-32-or-more-bytes-required",
	}
	if mediaStoragePath != "" {
		args = append(args, "-local-media-storage", mediaStoragePath)
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = ".."

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	t.Cleanup(func() { stopServer(cmd) })

	if err := waitForServerReady(serverURL, 30*time.Second); err != nil {
		t.Fatalf("Server failed to start: %v", err)
	}

	return cmd, serverURL
}

// TestMain tears down the shared PostgreSQL instance after all integration tests.
func TestMain(m *testing.M) {
	code := m.Run()
	storage.CleanupSharedPostgreSQL()
	os.Exit(code)
}

// registerFirstUser registers the first user (bootstrap user) without an invitation token.
// This only works when the database has zero users. Returns the auth token and user ID.
func registerFirstUser(t *testing.T, serverURL, email, name string) (string, string) {
	loginClient := apiconnect.NewLoginServiceClient(
		http.DefaultClient,
		serverURL,
	)

	ctx := context.Background()

	regReq := &api.EmailRegisterRequest{
		Email:           email,
		Name:            name,
		EmailProofToken: proveEmailOwnership(t, loginClient, email),
	}

	resp, err := loginClient.EmailRegister(ctx, connect.NewRequest(regReq))
	if err != nil {
		t.Fatalf("Failed to register first user %s: %v", email, err)
	}

	return resp.Msg.Tokens.AccessToken, resp.Msg.User.Id
}

// proveEmailOwnership runs the mailed-code loop and returns the proof token
// that EmailRegister/EmailLogin accept in place of a password.
//
// It reads the code from RequestEmailCodeResponse.dev_code, which the server
// only populates in dev mode — the seam that exists precisely so tests can
// complete the flow without a mailbox. The integration server is started with
// -dev-mode (see startTestServer), so this is available here and unreachable
// in production.
func proveEmailOwnership(t *testing.T, loginClient apiconnect.LoginServiceClient, email string) string {
	t.Helper()
	ctx := context.Background()

	codeResp, err := loginClient.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: email,
	}))
	if err != nil {
		t.Fatalf("Failed to request an email code for %s: %v", email, err)
	}
	if codeResp.Msg.GetDevCode() == "" {
		t.Fatalf("No dev_code returned for %s — is the test server running with -dev-mode?", email)
	}

	verifyResp, err := loginClient.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: email,
		Code:  codeResp.Msg.GetDevCode(),
	}))
	if err != nil {
		t.Fatalf("Failed to verify the email code for %s: %v", email, err)
	}

	return verifyResp.Msg.EmailProofToken
}

// registerUserByInvite gets an invitation link for a community and registers a new user.
// Takes an inviter's token and community ID, gets/creates an invitation link, and completes registration.
// Returns the new user's auth token and user ID.
func registerUserByInvite(t *testing.T, serverURL, inviterToken, communityID, email, name string) (string, string) {
	ctx := context.Background()

	communityClient := apiconnect.NewCommunityServiceClient(
		&http.Client{
			Transport: &authTransport{
				token: inviterToken,
				base:  http.DefaultTransport,
			},
		},
		serverURL,
	)

	inviteResp, err := communityClient.GetOrCreateShareLink(ctx, connect.NewRequest(&api.GetOrCreateShareLinkRequest{
		CommunityId: communityID,
		Target:      &api.GetOrCreateShareLinkRequest_CommunityInvite{CommunityInvite: communityID},
	}))
	if err != nil {
		t.Fatalf("Failed to get/create share link for community %s: %v", communityID, err)
	}

	shortCode := inviteResp.Msg.ShortCode
	if shortCode == "" {
		t.Fatalf("Expected short code from GetOrCreateShareLink")
	}

	loginClient := apiconnect.NewLoginServiceClient(http.DefaultClient, serverURL)
	regResp, err := loginClient.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
		Email:           email,
		Name:            name,
		EmailProofToken: proveEmailOwnership(t, loginClient, email),
		ShortCode:       shortCode,
	}))
	if err != nil {
		t.Fatalf("Failed to register user %s with invitation: %v", email, err)
	}

	return regResp.Msg.Tokens.AccessToken, regResp.Msg.User.Id
}

// Helper functions to create authenticated clients.

func createAuthGearClient(token, serverURL string) apiconnect.GearServiceClient {
	return apiconnect.NewGearServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthCommunityClient(token, serverURL string) apiconnect.CommunityServiceClient {
	return apiconnect.NewCommunityServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthTransferClient(token, serverURL string) apiconnect.TransferServiceClient {
	return apiconnect.NewTransferServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

// createAuthLoanClient is an alias for createAuthTransferClient (historical naming).
func createAuthLoanClient(token, serverURL string) apiconnect.TransferServiceClient {
	return createAuthTransferClient(token, serverURL)
}

func createAuthLocationClient(token, serverURL string) apiconnect.LocationServiceClient {
	return apiconnect.NewLocationServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthChatClient(token, serverURL string) apiconnect.ChatServiceClient {
	return apiconnect.NewChatServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthRequestClient(token, serverURL string) apiconnect.RequestServiceClient {
	return apiconnect.NewRequestServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthExperienceClient(token, serverURL string) apiconnect.ExperienceServiceClient {
	return apiconnect.NewExperienceServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthSearchClient(token, serverURL string) apiconnect.SearchServiceClient {
	return apiconnect.NewSearchServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthImpactClient(token, serverURL string) apiconnect.ImpactServiceClient {
	return apiconnect.NewImpactServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

func createAuthUserClient(token, serverURL string) apiconnect.UserServiceClient {
	return apiconnect.NewUserServiceClient(
		&http.Client{Transport: &authTransport{token: token, base: http.DefaultTransport}},
		serverURL,
	)
}

// setupTestCommunity creates a community and returns its ID.
func setupTestCommunity(t *testing.T, ctx context.Context, communityClient apiconnect.CommunityServiceClient, name, description string) string {
	communityResp, err := communityClient.CreateCommunity(ctx, connect.NewRequest(&api.CreateCommunityRequest{
		Name:        name,
		Description: description,
	}))
	if err != nil {
		t.Fatalf("CreateCommunity failed: %v", err)
	}
	return communityResp.Msg.Id
}

// setupTestLocation creates a location and returns its ID.
func setupTestLocation(t *testing.T, ctx context.Context, locationClient apiconnect.LocationServiceClient, locality string) string {
	locationResp, err := locationClient.SaveLocation(ctx, connect.NewRequest(&api.SaveLocationRequest{
		Locality:   locality,
		RegionCode: "US",
	}))
	if err != nil {
		t.Fatalf("SaveLocation failed: %v", err)
	}
	return locationResp.Msg.Id
}

// setupTestGear creates gear with a location and AI metadata, then returns the gear ID.
// Includes value estimate, material category, and weight so that impact estimation
// produces meaningful results (embodied carbon is computed at save time).
func setupTestGear(t *testing.T, ctx context.Context, gearClient apiconnect.GearServiceClient, name, description, locationID string) string {
	addResp, err := gearClient.SaveGear(ctx, connect.NewRequest(&api.SaveGearRequest{
		Name:        &name,
		Description: &description,
		LocationId:  &locationID,
		Metadata: &api.GearMetadata{
			MaterialCategory: &api.TrackedMaterialCategory{
				Value: api.MaterialCategory_MATERIAL_CATEGORY_MIXED_PLASTIC_METAL,
			},
			WeightGrams: &api.TrackedEstimate{
				Value: &api.Estimate{
					Mean:   2000,
					Stddev: 200,
				},
			},
			ValueEstimate: &api.ValueEstimate{
				EstimatedValueUsd: 50.0,
				Provenance: &api.Provenance{
					Source:    api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:      "genai_from_text",
					Reasoning: proto.String("Test value estimate"),
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("SaveGear failed: %v", err)
	}
	return addResp.Msg.Id
}

// --- Impact assertion helpers ---.

// assertImpactPopulated verifies that an ImpactEstimate is non-nil with at least time_saved populated.
func assertImpactPopulated(t *testing.T, impact *api.ImpactEstimate, label string) {
	t.Helper()
	if impact == nil {
		t.Errorf("%s: ImpactEstimate is nil", label)
		return
	}
	if impact.TimeSaved == nil || impact.TimeSaved.Minutes == nil {
		t.Errorf("%s: time_saved is not populated", label)
	}
}

// assertEstimatePositive verifies that an Estimate has mean > 0 and stddev > 0.
func assertEstimatePositive(t *testing.T, est *api.Estimate, label string) {
	t.Helper()
	if est == nil {
		t.Errorf("%s: Estimate is nil", label)
		return
	}
	if est.Mean <= 0 {
		t.Errorf("%s: expected mean > 0, got %f", label, est.Mean)
	}
	if est.Stddev <= 0 {
		t.Errorf("%s: expected stddev > 0, got %f", label, est.Stddev)
	}
}

// assertEstimateApproxEqual verifies two Estimates have means within 1% relative tolerance.
func assertEstimateApproxEqual(t *testing.T, a, b *api.Estimate, label string) {
	t.Helper()
	if a == nil && b == nil {
		return
	}
	if a == nil || b == nil {
		t.Errorf("%s: one estimate is nil (a=%v, b=%v)", label, a, b)
		return
	}
	if a.Mean == 0 && b.Mean == 0 {
		return
	}
	relDiff := math.Abs(float64(a.Mean-b.Mean)) / math.Max(math.Abs(float64(a.Mean)), math.Abs(float64(b.Mean)))
	if relDiff > 0.01 {
		t.Errorf("%s: means differ by %.1f%% (a=%f, b=%f)", label, relDiff*100, a.Mean, b.Mean)
	}
}

// estimateMean safely extracts the mean from an Estimate, returning 0 if nil.
func estimateMean(est *api.Estimate) float32 {
	if est == nil {
		return 0
	}
	return est.Mean
}

// totalCarbonGrams extracts total carbon grams (manufacture_avoided + waste_reduced) from PreventedEmissions.
func totalCarbonGrams(pe *api.PreventedEmissions) float32 {
	if pe == nil {
		return 0
	}
	var total float32
	if pe.ManufactureAvoidedCarbon != nil {
		total += estimateMean(pe.ManufactureAvoidedCarbon.Co2EGrams)
	}
	if pe.WasteReducedCarbon != nil {
		total += estimateMean(pe.WasteReducedCarbon.Co2EGrams)
	}
	return total
}

// assertImpactZero verifies an ImpactEstimate is nil or has all zero/nil dimensions.
func assertImpactZero(t *testing.T, impact *api.ImpactEstimate, label string) {
	t.Helper()
	if impact == nil {
		return
	}
	if impact.MoneySaved != nil && estimateMean(impact.MoneySaved.ValueUsd) > 0 {
		t.Errorf("%s: expected zero money_saved, got mean=%f", label, impact.MoneySaved.ValueUsd.Mean)
	}
	if totalCarbonGrams(impact.EmissionsPrevented) > 0 {
		t.Errorf("%s: expected zero carbon, got %f grams", label, totalCarbonGrams(impact.EmissionsPrevented))
	}
	if impact.TimeSaved != nil && estimateMean(impact.TimeSaved.Minutes) > 0 {
		t.Errorf("%s: expected zero time_saved, got mean=%f", label, impact.TimeSaved.Minutes.Mean)
	}
}

// assertCommunityImpactZero verifies that community impact metrics savings are zero.
func assertCommunityImpactZero(t *testing.T, metrics *api.CommunityImpactMetrics) {
	t.Helper()
	if metrics == nil {
		t.Error("CommunityImpactMetrics is nil")
		return
	}
	if estimateMean(metrics.CostSavingsUsd) > 0 {
		t.Errorf("expected zero cost_savings_usd, got mean=%f", metrics.CostSavingsUsd.Mean)
	}
	if estimateMean(metrics.CarbonSavingsGrams) > 0 {
		t.Errorf("expected zero carbon_savings_grams, got mean=%f", metrics.CarbonSavingsGrams.Mean)
	}
	if estimateMean(metrics.TimeBankedMinutes) > 0 {
		t.Errorf("expected zero time_banked_minutes, got mean=%f", metrics.TimeBankedMinutes.Mean)
	}
}

// getCommunityImpactMetrics fetches community impact metrics for assertions.
func getCommunityImpactMetrics(t *testing.T, ctx context.Context, impactClient apiconnect.ImpactServiceClient, communityID string) *api.CommunityImpactMetrics {
	t.Helper()
	resp, err := impactClient.GetCommunityImpactMetrics(ctx, connect.NewRequest(&api.GetCommunityImpactMetricsRequest{
		CommunityId: communityID,
	}))
	if err != nil {
		t.Fatalf("GetCommunityImpactMetrics failed: %v", err)
	}
	return resp.Msg.Metrics
}

// requestIDTransport wraps another transport and adds X-Request-ID header.
type requestIDTransport struct {
	base      http.RoundTripper
	requestID string
}

// RoundTrip implements http.RoundTripper.
func (t *requestIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.requestID != "" {
		req.Header.Set(middleware.RequestIDHeader, t.requestID)
	}
	return t.base.RoundTrip(req)
}

// createGearClientWithRequestID creates a gear client that adds auth and request ID headers.
func createGearClientWithRequestID(token, serverURL, requestID string) apiconnect.GearServiceClient {
	return apiconnect.NewGearServiceClient(
		&http.Client{
			Transport: &requestIDTransport{
				requestID: requestID,
				base:      &authTransport{token: token, base: http.DefaultTransport},
			},
		},
		serverURL,
	)
}

// LogCapture collects and parses JSON log entries from server output.
// It is safe for concurrent use.
type LogCapture struct {
	entries []map[string]any
	mu      sync.RWMutex
}

// NewLogCapture creates a new LogCapture instance.
func NewLogCapture() *LogCapture {
	return &LogCapture{
		entries: make([]map[string]any, 0),
	}
}

// Writer returns an io.Writer that captures log lines.
// Each line is parsed as JSON and stored.
func (lc *LogCapture) Writer() io.Writer {
	return &logCaptureWriter{lc: lc}
}

type logCaptureWriter struct {
	lc     *LogCapture
	buffer []byte
}

func (w *logCaptureWriter) Write(p []byte) (n int, err error) {
	w.buffer = append(w.buffer, p...)

	// Process complete lines
	for {
		idx := strings.Index(string(w.buffer), "\n")
		if idx == -1 {
			break
		}

		line := string(w.buffer[:idx])
		w.buffer = w.buffer[idx+1:]

		// Try to parse as JSON
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			w.lc.mu.Lock()
			w.lc.entries = append(w.lc.entries, entry)
			w.lc.mu.Unlock()
		}
	}

	return len(p), nil
}

// ParseFrom reads log lines from a reader and parses them.
// This is useful for parsing logs after server shutdown.
func (lc *LogCapture) ParseFrom(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			lc.mu.Lock()
			lc.entries = append(lc.entries, entry)
			lc.mu.Unlock()
		}
	}
	return scanner.Err()
}

// Entries returns a copy of all captured log entries.
func (lc *LogCapture) Entries() []map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	result := make([]map[string]any, len(lc.entries))
	copy(result, lc.entries)
	return result
}

// Clear removes all captured entries.
func (lc *LogCapture) Clear() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.entries = make([]map[string]any, 0)
}

// FindByField returns all entries where the given field equals the given value.
func (lc *LogCapture) FindByField(field string, value any) []map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var result []map[string]any
	for _, entry := range lc.entries {
		if entry[field] == value {
			result = append(result, entry)
		}
	}
	return result
}

// FindByRequestID returns all entries with the given request_id.
func (lc *LogCapture) FindByRequestID(requestID string) []map[string]any {
	return lc.FindByField("request_id", requestID)
}

// FindByPath returns all entries with the given path.
func (lc *LogCapture) FindByPath(path string) []map[string]any {
	return lc.FindByField("path", path)
}

// FindByLevel returns all entries with the given log level.
func (lc *LogCapture) FindByLevel(level string) []map[string]any {
	return lc.FindByField("level", level)
}

// FindByMessage returns all entries where message contains the given substring.
// Note: The logging package uses "message" as the key (not "msg").
func (lc *LogCapture) FindByMessage(substring string) []map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var result []map[string]any
	for _, entry := range lc.entries {
		if msg, ok := entry["message"].(string); ok {
			if strings.Contains(msg, substring) {
				result = append(result, entry)
			}
		}
	}
	return result
}

// HasEntry returns true if any entry matches the given field/value.
func (lc *LogCapture) HasEntry(field string, value any) bool {
	return len(lc.FindByField(field, value)) > 0
}

// Count returns the number of captured entries.
func (lc *LogCapture) Count() int {
	lc.mu.RLock()
	defer lc.mu.RUnlock()
	return len(lc.entries)
}

// HTTPRequestEntries returns all entries that appear to be HTTP request logs.
func (lc *LogCapture) HTTPRequestEntries() []map[string]any {
	return lc.FindByMessage("http request")
}

// startTestServerWithLogCapture starts the server and captures JSON logs for verification.
// Returns the LogCapture which can be used to query log entries after requests.
func startTestServerWithLogCapture(t *testing.T, dbURL string) (*exec.Cmd, string, *LogCapture) {
	mediaStoragePath := t.TempDir()
	return startTestServerWithOptions(t, dbURL, mediaStoragePath, true)
}

// startTestServerWithOptions is the internal function that handles all server startup options.
func startTestServerWithOptions(t *testing.T, dbURL, mediaStoragePath string, captureLog bool) (*exec.Cmd, string, *LogCapture) {
	binary := buildServerBinary(t)

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	serverURL := fmt.Sprintf("http://localhost:%s", port)

	args := []string{
		"-db", dbURL, "-port", port, "-dev-mode", "-log-format", "json", "-log-level", "debug",
		"-embedding-model-path", "../model_tuning/ripls_embedding.onnx",
		"-embedding-vocab-path", "../model_tuning/ripls_embedding_tokenizer/vocab.txt",
		"-invite-link-hostname", "test.example.app",
		"-jwt-signing-secret", "test-jwt-signing-secret-not-real-32-or-more-bytes-required",
	}
	if mediaStoragePath != "" {
		args = append(args, "-local-media-storage", mediaStoragePath)
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = ".."

	var logCapture *LogCapture
	if captureLog {
		logCapture = NewLogCapture()

		// Use pipes for non-blocking capture
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatalf("Failed to create stdout pipe: %v", err)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			t.Fatalf("Failed to create stderr pipe: %v", err)
		}

		// Start goroutines to read from pipes
		go func() {
			_ = logCapture.ParseFrom(stdout)
		}()
		go func() {
			_ = logCapture.ParseFrom(stderr)
		}()
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	t.Cleanup(func() { stopServer(cmd) })

	if err := waitForServerReady(serverURL, 30*time.Second); err != nil {
		t.Fatalf("Server failed to start: %v", err)
	}

	return cmd, serverURL, logCapture
}

// waitForLogEntry polls the LogCapture until an entry matching the predicate is found.
// Returns the matching entry or nil if not found within the timeout.
func waitForLogEntry(lc *LogCapture, predicate func(map[string]any) bool, timeout time.Duration) map[string]any {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, entry := range lc.Entries() {
			if predicate(entry) {
				return entry
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

// NotificationLogEntry represents a notification extracted from server logs.
type NotificationLogEntry struct {
	UserID string
	Title  string
}

// GetNotificationLogs returns workflow-relevant notification log entries from
// the captured logs. Membership notifications ("New member" pushes fired when
// preconditions seed users via registerUserByInvite) are setup side effects
// and excluded so per-workflow assertions stay scoped to the workflow under
// test. Use GetMembershipNotificationLogs for explicit assertions on member-
// join pushes.
func (lc *LogCapture) GetNotificationLogs() []NotificationLogEntry {
	all := lc.getAllNotificationLogs()
	result := all[:0]
	for _, n := range all {
		if n.Title == "New member" {
			continue
		}
		result = append(result, n)
	}
	return result
}

// GetMembershipNotificationLogs returns the notifications fired when new
// members joined the community (title "New member").
func (lc *LogCapture) GetMembershipNotificationLogs() []NotificationLogEntry {
	all := lc.getAllNotificationLogs()
	var result []NotificationLogEntry
	for _, n := range all {
		if n.Title == "New member" {
			result = append(result, n)
		}
	}
	return result
}

func (lc *LogCapture) getAllNotificationLogs() []NotificationLogEntry {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var result []NotificationLogEntry
	for _, entry := range lc.entries {
		// The logging package uses "message" as the key (not "msg")
		msg, _ := entry["message"].(string)
		// Look for "notification dispatched to user" from community subscriber
		if msg == "notification dispatched to user" {
			userID, _ := entry["user_id"].(string)
			title, _ := entry["title"].(string)
			result = append(result, NotificationLogEntry{
				UserID: userID,
				Title:  title,
			})
		}
	}
	return result
}

// GetNotificationLogsForUser returns notifications sent to a specific user.
func (lc *LogCapture) GetNotificationLogsForUser(userID string) []NotificationLogEntry {
	all := lc.GetNotificationLogs()
	var result []NotificationLogEntry
	for _, n := range all {
		if n.UserID == userID {
			result = append(result, n)
		}
	}
	return result
}

// WaitForNotificationLog waits for a notification log matching the predicate.
func WaitForNotificationLog(lc *LogCapture, predicate func(NotificationLogEntry) bool, timeout time.Duration) *NotificationLogEntry {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, n := range lc.GetNotificationLogs() {
			if predicate(n) {
				return &n
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

// notificationSettleWindow is how long the captured notification count must
// hold still before WaitForNotificationCount treats it as settled. Dispatch
// is a single subscriber goroutine emitting one log line per recipient with
// only small storage reads in between, so this much silence means the
// pipeline is idle — a straggler landing later would need a stall far beyond
// anything the suite produces.
const notificationSettleWindow = 750 * time.Millisecond

// WaitForNotificationCount waits until at least count notifications have
// been captured AND the count has stopped growing for
// notificationSettleWindow, then returns the settled count. The workflow
// tests assert exact equality against the contracts in docs/workflows/*.md;
// settling before the assert makes an over-emission (an N+1th push racing
// the assert, #2657) fail loudly instead of passing or failing on
// dispatch-lag luck. timeout bounds the total wait; on timeout the current
// count is returned whether or not it settled. The generous timeout is free
// on the happy path — the wait returns one settle window after the
// contract's last notification lands.
func WaitForNotificationCount(lc *LogCapture, count int, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	last := len(lc.GetNotificationLogs())
	lastChange := time.Now()
	for time.Now().Before(deadline) {
		if n := len(lc.GetNotificationLogs()); n != last {
			last = n
			lastChange = time.Now()
		}
		if last >= count && time.Since(lastChange) >= notificationSettleWindow {
			return last
		}
		time.Sleep(50 * time.Millisecond)
	}
	return len(lc.GetNotificationLogs())
}

// GetNoopNotificationLogs returns all noop notification log entries from the captured logs.
// These are logs from the noop provider when it "sends" a notification.
func (lc *LogCapture) GetNoopNotificationLogs() []map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var result []map[string]any
	for _, entry := range lc.entries {
		msg, _ := entry["message"].(string)
		if msg == "noop notification (not actually sent)" {
			result = append(result, entry)
		}
	}
	return result
}

// GetNotificationProcessingStartLogs returns logs indicating notification goroutine started.
func (lc *LogCapture) GetNotificationProcessingStartLogs() []map[string]any {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	var result []map[string]any
	for _, entry := range lc.entries {
		msg, _ := entry["message"].(string)
		if msg == "starting notification processing" {
			result = append(result, entry)
		}
	}
	return result
}

// newTestCommunityService constructs the community service plus the
// community-event bus + subscribers that production main.go wires (#510
// PR 3), returning both so tests can call bus.Drain to wait for in-flight
// async dispatch before assertions. Mirrors the order in server/main.go:
// the bus is built first, the service receives it, then the subscribers
// register against the now-existing service.
//
// Tests that need the notificationDone channel from
// NewWithNotificationSignal can ignore it via _; the bus's Drain is the
// replacement for that pattern.
func newTestCommunityService(t *testing.T, sqlStorage *storage.ProtoSQLStorage, bucket storage.BucketStorage, notif notifications.Service) (*community_svc.Service, *cebus.InProcessBus) {
	t.Helper()
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(sqlStorage, topic)
	svc := community_svc.New(sqlStorage, bucket, notif, bus, "localhost:8080")

	notifSub := commsub.New(sqlStorage, notif)
	notifSub.SetStreamChecker(svc.HasActiveUserStream)
	if _, err := bus.Subscribe(notifSub); err != nil {
		t.Fatalf("subscribe notification subscriber: %v", err)
	}
	if _, err := bus.Subscribe(svc.StreamSubscriber()); err != nil {
		t.Fatalf("subscribe stream subscriber: %v", err)
	}
	return svc, bus
}

// drainCommunityEventBus waits for in-flight bus dispatch to complete, with a
// 5s timeout. Used by tests as the replacement for the old `<-notifDone`
// pattern (G2 in docs/issues/510-community-event-pubsub-v2.md).
func drainCommunityEventBus(t *testing.T, bus *cebus.InProcessBus) {
	t.Helper()
	drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := bus.Drain(drainCtx); err != nil {
		t.Fatalf("drainCommunityEventBus: %v", err)
	}
}
