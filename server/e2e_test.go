package main

// End-to-end tests that exercise complete user journeys.
//
// Running modes:
//
//   # Run against local server (default - starts automatically)
//   go test -v ./server -run TestEndToEnd -timeout 10m
//
//   # Run against deployed dev server (for CI)
//   E2E_SERVER_URL=https://server-dev.example.com go test -v ./server -run TestEndToEnd -timeout 10m
//

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/storage"
)

// e2eBinaryOnce ensures the server binary is built only once per e2e test run.
var (
	e2eBinaryOnce sync.Once
	e2eBinaryPath string
	errE2EBuild   error
)

// buildE2EServerBinary builds the server binary once and returns its path.
func buildE2EServerBinary(t *testing.T) string {
	e2eBinaryOnce.Do(func() {
		tmpDir := os.TempDir()
		e2eBinaryPath = filepath.Join(tmpDir, "ripls-e2e-server")
		cmd := exec.Command("go", "build", "-o", e2eBinaryPath, ".")
		cmd.Dir = "."
		if output, err := cmd.CombinedOutput(); err != nil {
			errE2EBuild = fmt.Errorf("failed to build server: %w\n%s", err, output)
		}
	})
	if errE2EBuild != nil {
		t.Fatal(errE2EBuild)
	}
	return e2eBinaryPath
}

// Shared server state for all e2e tests when running locally.
var (
	e2eServerOnce sync.Once
	e2eServerURL  string
	e2eServerCmd  *exec.Cmd
)

// lockedBuffer is a goroutine-safe sink for the local e2e server's stdout/stderr,
// so tests can assert on its structured (JSON) log lines while the same output is
// also streamed to the test's stderr. Only populated in local mode (no
// E2E_SERVER_URL) — against a deployed server we cannot read its process logs.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// e2eServerLog captures the local server's logs for behavioral assertions.
var e2eServerLog lockedBuffer

// e2eLogLines parses the captured server log into JSON objects, skipping any
// non-JSON lines (library output, panics, etc.).
func e2eLogLines() []map[string]any {
	var out []map[string]any
	for _, line := range strings.Split(e2eServerLog.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err == nil {
			out = append(out, m)
		}
	}
	return out
}

// logLineMatches reports whether a parsed log line has every given string field.
func logLineMatches(m map[string]any, fields map[string]string) bool {
	for k, v := range fields {
		got, ok := m[k].(string)
		if !ok || got != v {
			return false
		}
	}
	return true
}

// waitForServerLog polls the captured server log until a JSON line matches all
// the given field=value pairs, or the timeout elapses. Off-app dispatch is
// asynchronous (notifications fire on background goroutines), so callers poll
// rather than read once.
func waitForServerLog(timeout time.Duration, fields map[string]string) bool {
	deadline := time.Now().Add(timeout)
	for {
		for _, m := range e2eLogLines() {
			if logLineMatches(m, fields) {
				return true
			}
		}
		if time.Now().After(deadline) {
			return false
		}
		//nolint:forbidigo // Backoff inside a bounded poll over captured server
		// log lines; the subprocess writes them asynchronously with nothing the
		// test can synchronize on.
		time.Sleep(100 * time.Millisecond)
	}
}

// countServerLog returns how many captured log lines match every field=value pair.
func countServerLog(fields map[string]string) int {
	n := 0
	for _, m := range e2eLogLines() {
		if logLineMatches(m, fields) {
			n++
		}
	}
	return n
}

// TestMain cleans up the shared server and database after all tests.
func TestMain(m *testing.M) {
	code := m.Run()

	// Cleanup local server if started (graceful shutdown via SIGTERM)
	if e2eServerCmd != nil && e2eServerCmd.Process != nil {
		_ = e2eServerCmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- e2eServerCmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = e2eServerCmd.Process.Kill()
			<-done
		}
	}

	// Cleanup shared PostgreSQL container
	storage.CleanupSharedPostgreSQL()

	os.Exit(code)
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

// getTestServerURL returns the server URL for e2e tests.
// If E2E_SERVER_URL is set, uses that deployed server.
// Otherwise, starts a shared local server (once) with testcontainers database.
func getTestServerURL(t *testing.T) string {
	if url := os.Getenv("E2E_SERVER_URL"); url != "" {
		return url
	}

	// Start local server once, reuse for all tests
	e2eServerOnce.Do(func() {
		e2eServerURL = startSharedE2EServer(t)
	})

	return e2eServerURL
}

// startSharedE2EServer starts a local server with a test database.
// Called once and shared across all e2e tests.
func startSharedE2EServer(t *testing.T) string {
	binary := buildE2EServerBinary(t)

	// Setup test database - the cleanup is handled by TestMain via CleanupSharedPostgreSQL
	dbURL, _ := storage.SetupTestDatabase(t)

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	serverURL := fmt.Sprintf("http://localhost:%s", port)
	mediaStoragePath := t.TempDir()

	args := []string{
		"-db", dbURL, "-port", port, "-dev-mode", "-log-format", "json",
		"-embedding-model-path", "../model_tuning/ripls_embedding.onnx",
		"-embedding-vocab-path", "../model_tuning/ripls_embedding_tokenizer/vocab.txt",
		"-local-media-storage", mediaStoragePath,
		"-invite-link-hostname", "test.example.com",
		"-jwt-signing-secret", "test-jwt-signing-secret-not-real-32-or-more-bytes-required",
		// Enable the off-app email channel (backed by the mock email sender, since
		// no Mailgun key is supplied) so notifications to deviceless recipients
		// reach off-app dispatch — exercising the simulation-suppression path
		// (#2588) that TestEndToEnd_CompleteRequestLifecycle asserts on.
		"-off-app-email-enabled",
	}

	cmd := exec.Command(binary, args...)
	cmd.Dir = "."
	// Capture the server's JSON logs for behavioral assertions while still
	// streaming them to the test's stderr for debugging.
	cmd.Stdout = io.MultiWriter(os.Stderr, &e2eServerLog)
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Store for cleanup in TestMain
	e2eServerCmd = cmd

	if err := waitForServerReady(serverURL, 60*time.Second); err != nil {
		_ = cmd.Process.Kill()
		t.Fatalf("Server failed to start: %v", err)
	}

	t.Logf("Started local e2e server at %s", serverURL)
	return serverURL
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
			resp, err := client.Get(serverURL + "/")
			if err == nil {
				resp.Body.Close()
				return nil
			}
		}
	}
}

// generateSimulationID creates a unique simulation ID for tagging test data.
// All entities created with this ID can be cleaned up via CleanupSimulation.
func generateSimulationID(testName string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("e2e-%s-%s", testName, hex.EncodeToString(b))
}

// registerSimulationCleanup registers a t.Cleanup function that calls
// CleanupSimulation for the given simulation ID. Set E2E_SKIP_CLEANUP=true
// to skip cleanup for debugging leftover test data.
func registerSimulationCleanup(t *testing.T, serverURL, simulationID string) {
	t.Cleanup(func() {
		if os.Getenv("E2E_SKIP_CLEANUP") != "" {
			t.Logf("Skipping cleanup for simulation %s (E2E_SKIP_CLEANUP set)", simulationID)
			return
		}

		adminClient := apiconnect.NewAdminServiceClient(http.DefaultClient, serverURL)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		resp, err := adminClient.CleanupSimulation(ctx, connect.NewRequest(&api.CleanupSimulationRequest{
			SimulationId: simulationID,
		}))
		if err != nil {
			t.Logf("Warning: CleanupSimulation failed for %s: %v", simulationID, err)
			return
		}

		t.Logf("Cleaned up simulation %s: %d users, %d communities, %d records (%dms)",
			simulationID,
			resp.Msg.UsersDeleted,
			resp.Msg.CommunitiesDeleted,
			resp.Msg.RecordsDeleted,
			resp.Msg.DurationMilliseconds)
	})
}

// createUniqueTestUser creates a test user with a unique email address.
// The simulationID tags the user for cleanup via CleanupSimulation.
// Returns the auth token and user ID.
func createUniqueTestUser(t *testing.T, serverURL, namePrefix, simulationID string) (string, string) {
	// Create unique email using timestamp and random component
	uniqueID := fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Unix()%10000)
	email := fmt.Sprintf("%s-%s@e2etest.example.com", namePrefix, uniqueID)
	name := fmt.Sprintf("%s Test User %s", namePrefix, uniqueID)

	loginClient := apiconnect.NewLoginServiceClient(http.DefaultClient, serverURL)

	ctx := context.Background()

	// Prove ownership via the dev-mode code echo rather than setting a
	// password: registration is passwordless since #2571, and the echo is the
	// seam that exists so tests can complete the loop without a mailbox.
	codeResp, err := loginClient.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: email,
	}))
	if err != nil {
		t.Fatalf("Failed to request an email code for %s: %v", email, err)
	}
	verifyResp, err := loginClient.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: email,
		Code:  codeResp.Msg.GetDevCode(),
	}))
	if err != nil {
		t.Fatalf("Failed to verify the email code for %s: %v", email, err)
	}

	registerReq := connect.NewRequest(&api.EmailRegisterRequest{
		Email:           email,
		Name:            name,
		EmailProofToken: verifyResp.Msg.EmailProofToken,
		SimulationId:    proto.String(simulationID),
	})
	registerResp, err := loginClient.EmailRegister(ctx, registerReq)
	if err != nil {
		t.Fatalf("Failed to register test user %s: %v", email, err)
	}

	authToken := registerResp.Msg.Tokens.AccessToken
	if authToken == "" {
		t.Fatalf("Expected auth token from registration for %s", email)
	}

	userID := registerResp.Msg.User.Id
	if userID == "" {
		t.Fatalf("Expected user ID from registration for %s", email)
	}

	t.Logf("Created test user: %s (ID: %s, sim: %s)", email, userID, simulationID)

	return authToken, userID
}

// uploadTestImage uploads a test image from the test_data directory.
// Returns the media ID. Cleanup is handled by CleanupSimulation.
func uploadTestImage(t *testing.T, client apiconnect.MediaServiceClient, filename string) string {
	testImagePath := filepath.Join("test_data", filename)
	imageData, err := os.ReadFile(testImagePath)
	if err != nil {
		t.Fatalf("Failed to read test image %s: %v", filename, err)
	}

	ctx := context.Background()
	addResp, err := client.AddMedia(ctx, connect.NewRequest(&api.AddMediaRequest{
		EncodedBytes: imageData,
		ContentType:  "image/jpeg",
		Filename:     filename,
		Description:  fmt.Sprintf("E2E test image: %s", filename),
	}))
	if err != nil {
		t.Fatalf("Failed to upload test image %s: %v", filename, err)
	}

	mediaID := addResp.Msg.Id
	t.Logf("Uploaded test image %s: media ID %s", filename, mediaID)

	return mediaID
}
