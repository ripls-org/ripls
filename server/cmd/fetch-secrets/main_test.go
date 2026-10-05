package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeResponse is what the fake Secret Manager returns for one secret.
type fakeResponse struct {
	status int
	body   string
}

func secretValue(value string) fakeResponse {
	data := base64.StdEncoding.EncodeToString([]byte(value))
	return fakeResponse{http.StatusOK, fmt.Sprintf(`{"name":"projects/p/secrets/s/versions/1","payload":{"data":%q}}`, data)}
}

func apiError(code int, status, message string) fakeResponse {
	return fakeResponse{code, fmt.Sprintf(`{"error":{"code":%d,"message":%q,"status":%q}}`, code, message, status)}
}

// newFetcher starts a fake Secret Manager serving responses by secret name and
// returns a fetcher pointed at it, plus the requested paths and the log.
func newFetcher(t *testing.T, responses map[string]fakeResponse) (*fetcher, *[]string, *bytes.Buffer) {
	t.Helper()
	var (
		mu    sync.Mutex
		paths []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		const prefix, suffix = "/v1/projects/proj/secrets/", "/versions/latest:access"
		name, ok := strings.CutPrefix(r.URL.Path, prefix)
		name, ok2 := strings.CutSuffix(name, suffix)
		resp, known := responses[name]
		if !ok || !ok2 || !known {
			resp = apiError(http.StatusNotFound, "NOT_FOUND", "Secret [projects/proj/secrets/"+name+"] not found or has no versions.")
		}
		w.WriteHeader(resp.status)
		io.WriteString(w, resp.body)
	}))
	t.Cleanup(srv.Close)
	log := &bytes.Buffer{}
	return &fetcher{client: srv.Client(), endpoint: srv.URL, project: "proj", log: log}, &paths, log
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestParseSpec(t *testing.T) {
	tests := []struct {
		raw        string
		wantFile   string
		wantSecret string
		wantErr    bool
	}{
		{raw: "openai-api-key", wantFile: "openai-api-key", wantSecret: "openai-api-key"},
		{raw: "github-app-private-key=github-app-private-key-base64", wantFile: "github-app-private-key", wantSecret: "github-app-private-key-base64"},
		{raw: "db-password=Mixed_Case-Secret", wantFile: "db-password", wantSecret: "Mixed_Case-Secret"},
		{raw: "", wantErr: true},
		{raw: "../escape", wantErr: true},
		{raw: "Upper", wantErr: true},
		{raw: "-leading-dash", wantErr: true},
		{raw: "has space", wantErr: true},
		{raw: "file=", wantErr: true},
		{raw: "file=a/b", wantErr: true},
		{raw: "=secret", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseSpec(tt.raw, true)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseSpec(%q) = %+v, want error", tt.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSpec(%q): %v", tt.raw, err)
			}
			if got.file != tt.wantFile || got.secret != tt.wantSecret || !got.optional {
				t.Errorf("parseSpec(%q) = %+v, want file %q secret %q optional", tt.raw, got, tt.wantFile, tt.wantSecret)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	cfg, err := parseArgs([]string{
		"-project=proj", "-dir=/tmp/x", "-timeout=5s",
		"-optional=twilio-auth-token", "-optional=feedback-signer-key",
		"jwt-signing-secret", "github-app-private-key=github-app-private-key-base64",
	}, io.Discard)
	if err != nil {
		t.Fatalf("parseArgs: %v", err)
	}
	if cfg.project != "proj" || cfg.dir != "/tmp/x" || cfg.timeout != 5*time.Second {
		t.Errorf("parseArgs config = %+v", cfg)
	}
	want := []secretSpec{
		{file: "jwt-signing-secret", secret: "jwt-signing-secret"},
		{file: "github-app-private-key", secret: "github-app-private-key-base64"},
		{file: "twilio-auth-token", secret: "twilio-auth-token", optional: true},
		{file: "feedback-signer-key", secret: "feedback-signer-key", optional: true},
	}
	if fmt.Sprint(cfg.specs) != fmt.Sprint(want) {
		t.Errorf("specs = %+v, want %+v", cfg.specs, want)
	}

	failures := map[string][]string{
		"missing project":       {"-dir=/tmp/x", "a"},
		"missing dir":           {"-project=p", "a"},
		"no secrets":            {"-project=p", "-dir=/tmp/x"},
		"duplicate file":        {"-project=p", "-dir=/tmp/x", "-optional=a", "a"},
		"duplicate via rename":  {"-project=p", "-dir=/tmp/x", "a", "a=other"},
		"invalid spec":          {"-project=p", "-dir=/tmp/x", "Bad"},
		"non-positive timeout":  {"-project=p", "-dir=/tmp/x", "-timeout=0s", "a"},
		"unknown flag":          {"-project=p", "-dir=/tmp/x", "-nope", "a"},
		"flag after positional": {"-project=p", "-dir=/tmp/x", "a", "-optional=b"},
	}
	for name, args := range failures {
		t.Run(name, func(t *testing.T) {
			if cfg, err := parseArgs(args, io.Discard); err == nil {
				t.Errorf("parseArgs(%q) = %+v, want error", args, cfg)
			}
		})
	}
}

func TestFetchAllWritesTrimmedValuesWithRestrictivePermissions(t *testing.T) {
	pem := "-----BEGIN KEY-----\nline one\nline two\n-----END KEY-----"
	f, paths, _ := newFetcher(t, map[string]fakeResponse{
		"jwt-signing-secret":            secretValue("  leading space kept\r\n\n"),
		"github-app-private-key-base64": secretValue(pem + "\n"),
	})
	dir := filepath.Join(t.TempDir(), "secrets")

	err := f.fetchAll(context.Background(), dir, []secretSpec{
		{file: "jwt-signing-secret", secret: "jwt-signing-secret"},
		{file: "github-app-private-key", secret: "github-app-private-key-base64"},
	})
	if err != nil {
		t.Fatalf("fetchAll: %v", err)
	}

	if got := readFile(t, filepath.Join(dir, "jwt-signing-secret")); got != "  leading space kept" {
		t.Errorf("jwt-signing-secret = %q, want trailing whitespace stripped only", got)
	}
	if got := readFile(t, filepath.Join(dir, "github-app-private-key")); got != pem {
		t.Errorf("github-app-private-key = %q, want embedded newlines kept", got)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("dir mode = %o, want 700", perm)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("dir holds %d entries, want exactly the 2 secrets (no leftover temp files)", len(entries))
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o400 {
			t.Errorf("%s mode = %o, want 400", e.Name(), perm)
		}
	}

	wantPaths := []string{
		"/v1/projects/proj/secrets/jwt-signing-secret/versions/latest:access",
		"/v1/projects/proj/secrets/github-app-private-key-base64/versions/latest:access",
	}
	if fmt.Sprint(*paths) != fmt.Sprint(wantPaths) {
		t.Errorf("requested %v, want %v", *paths, wantPaths)
	}
}

func TestFetchAllOptionalAbsentWritesEmptyFile(t *testing.T) {
	tests := map[string]fakeResponse{
		"not found":         apiError(http.StatusNotFound, "NOT_FOUND", "not found"),
		"permission denied": apiError(http.StatusForbidden, "PERMISSION_DENIED", "denied"),
		"disabled version":  apiError(http.StatusBadRequest, "FAILED_PRECONDITION", "version is disabled"),
		"empty value":       secretValue("\n"),
	}
	for name, resp := range tests {
		t.Run(name, func(t *testing.T) {
			f, _, log := newFetcher(t, map[string]fakeResponse{"twilio-auth-token": resp})
			dir := t.TempDir()

			err := f.fetchAll(context.Background(), dir, []secretSpec{{file: "twilio-auth-token", secret: "twilio-auth-token", optional: true}})
			if err != nil {
				t.Fatalf("fetchAll: %v", err)
			}
			if got := readFile(t, filepath.Join(dir, "twilio-auth-token")); got != "" {
				t.Errorf("file = %q, want empty", got)
			}
			if !strings.Contains(log.String(), "feature disabled") {
				t.Errorf("log = %q, want a feature-disabled notice", log.String())
			}
		})
	}
}

func TestFetchAllFailures(t *testing.T) {
	tests := []struct {
		name     string
		resp     fakeResponse
		optional bool
		wantErr  string
	}{
		{name: "required not found", resp: apiError(http.StatusNotFound, "NOT_FOUND", "gone"), wantErr: "roles/secretmanager.secretAccessor"},
		{name: "required permission denied", resp: apiError(http.StatusForbidden, "PERMISSION_DENIED", "denied"), wantErr: "PERMISSION_DENIED"},
		{name: "required empty value", resp: secretValue(" \n"), wantErr: "is empty"},
		// A transient failure must not silently disable an optional feature.
		{name: "optional server error", resp: apiError(http.StatusInternalServerError, "INTERNAL", "boom"), optional: true, wantErr: "HTTP 500 INTERNAL"},
		{name: "optional bad request", resp: apiError(http.StatusBadRequest, "INVALID_ARGUMENT", "bad"), optional: true, wantErr: "INVALID_ARGUMENT"},
		{name: "optional non-JSON error", resp: fakeResponse{http.StatusBadGateway, "<html>proxy</html>"}, optional: true, wantErr: "HTTP 502"},
		{name: "malformed JSON", resp: fakeResponse{http.StatusOK, "not json"}, wantErr: "decode response"},
		{name: "malformed base64", resp: fakeResponse{http.StatusOK, `{"payload":{"data":"!!!"}}`}, wantErr: "decode payload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, _ := newFetcher(t, map[string]fakeResponse{"s": tt.resp})
			dir := t.TempDir()

			err := f.fetchAll(context.Background(), dir, []secretSpec{{file: "s", secret: "s", optional: tt.optional}})
			if err == nil {
				t.Fatal("fetchAll succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tt.wantErr)
			}
			if _, statErr := os.Stat(filepath.Join(dir, "s")); !os.IsNotExist(statErr) {
				t.Errorf("a failed secret left a file behind (stat err %v)", statErr)
			}
		})
	}
}

// A restarted container keeps its filesystem, so the 0400 files from the last
// start must be replaced rather than refused.
func TestFetchAllReplacesReadOnlyFileFromEarlierRun(t *testing.T) {
	f, _, _ := newFetcher(t, map[string]fakeResponse{"jwt-signing-secret": secretValue("new")})
	dir := t.TempDir()
	old := filepath.Join(dir, "jwt-signing-secret")
	if err := os.WriteFile(old, []byte("old"), 0o400); err != nil {
		t.Fatal(err)
	}

	if err := f.fetchAll(context.Background(), dir, []secretSpec{{file: "jwt-signing-secret", secret: "jwt-signing-secret"}}); err != nil {
		t.Fatalf("fetchAll: %v", err)
	}
	if got := readFile(t, old); got != "new" {
		t.Errorf("file = %q, want the new value", got)
	}
}

func TestFetchAllNeverLogsValues(t *testing.T) {
	const value = "sk-do-not-print-this-value"
	f, _, log := newFetcher(t, map[string]fakeResponse{
		"present": secretValue(value),
		"missing": apiError(http.StatusInternalServerError, "INTERNAL", "boom"),
	})

	err := f.fetchAll(context.Background(), t.TempDir(), []secretSpec{
		{file: "present", secret: "present"},
		{file: "missing", secret: "missing"},
	})
	if err == nil {
		t.Fatal("fetchAll succeeded, want the missing secret to fail it")
	}
	if strings.Contains(log.String(), value) || strings.Contains(err.Error(), value) {
		t.Errorf("a secret value leaked into the log or error: log %q, err %q", log.String(), err)
	}
}

func TestFetchAllStopsAtDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	f := &fetcher{client: srv.Client(), endpoint: srv.URL, project: "proj", log: io.Discard}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := f.fetchAll(ctx, t.TempDir(), []secretSpec{{file: "s", secret: "s", optional: true}})
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Errorf("fetchAll error = %v, want a request failure at the deadline", err)
	}
}
