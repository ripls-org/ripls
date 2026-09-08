package simulation

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.ripls.org/ripls/server/clock"
)

// TestSimulationTransport_InjectsHeaders verifies that the simulation transport
// correctly injects auth and timestamp headers into requests.
func TestSimulationTransport_InjectsHeaders(t *testing.T) {
	var capturedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	client.AsUser("test-token-123")
	simTime := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	client.SetTimestamp(simTime)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.httpClient().Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	// Verify auth header.
	authHeader := capturedHeaders.Get("Authorization")
	if authHeader != "Bearer test-token-123" {
		t.Errorf("Authorization header = %q, want %q", authHeader, "Bearer test-token-123")
	}

	// Verify simulation timestamp header.
	tsHeader := capturedHeaders.Get(clock.SimulationTimestampHeader)
	wantTS := fmt.Sprintf("%d", simTime.Unix())
	if tsHeader != wantTS {
		t.Errorf("Simulation timestamp header = %q, want %q", tsHeader, wantTS)
	}
}

// TestSimulationTransport_NoHeadersWhenUnset verifies that the simulation
// transport does not inject headers when token and timestamp are not set.
func TestSimulationTransport_NoHeadersWhenUnset(t *testing.T) {
	var capturedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.httpClient().Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	resp.Body.Close()

	if capturedHeaders.Get("Authorization") != "" {
		t.Error("Authorization header should be empty when no token is set")
	}
	if capturedHeaders.Get(clock.SimulationTimestampHeader) != "" {
		t.Error("Simulation timestamp header should be empty when no timestamp is set")
	}
}

// TestSimulationTransport_SwitchUser verifies that switching users changes
// the auth header for subsequent requests.
func TestSimulationTransport_SwitchUser(t *testing.T) {
	var capturedAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := NewClient(ts.URL)
	httpClient := client.httpClient()

	// First request as user A.
	client.AsUser("token-A")
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if capturedAuth != "Bearer token-A" {
		t.Errorf("First request: auth = %q, want %q", capturedAuth, "Bearer token-A")
	}

	// Second request as user B.
	client.AsUser("token-B")
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/test", nil)
	resp, err = httpClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if capturedAuth != "Bearer token-B" {
		t.Errorf("Second request: auth = %q, want %q", capturedAuth, "Bearer token-B")
	}
}

// TestClient_ServiceClients verifies that service client accessors return non-nil clients.
func TestClient_ServiceClients(t *testing.T) {
	client := NewClient("http://localhost:8080")

	if client.Login() == nil {
		t.Error("Login() returned nil")
	}
	if client.Gear() == nil {
		t.Error("Gear() returned nil")
	}
	if client.Community() == nil {
		t.Error("Community() returned nil")
	}
	if client.Transfer() == nil {
		t.Error("Transfer() returned nil")
	}
	if client.Request() == nil {
		t.Error("Request() returned nil")
	}
	if client.Experience() == nil {
		t.Error("Experience() returned nil")
	}
	if client.Chat() == nil {
		t.Error("Chat() returned nil")
	}
	if client.Media() == nil {
		t.Error("Media() returned nil")
	}
	if client.Admin() == nil {
		t.Error("Admin() returned nil")
	}
	if client.User() == nil {
		t.Error("User() returned nil")
	}
	if client.Search() == nil {
		t.Error("Search() returned nil")
	}
	if client.Impact() == nil {
		t.Error("Impact() returned nil")
	}
	if client.Location() == nil {
		t.Error("Location() returned nil")
	}
}
