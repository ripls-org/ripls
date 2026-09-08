package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// validConfig returns a Config that passes finalize's invariants, for tests
// that want to break exactly one rule at a time.
func validConfig() *Config {
	return &Config{
		DBURL:              "postgres://user:pass@localhost:5432/testdb",
		InviteLinkHostname: "test.example.com",
		JWTSigningSecret:   "test-jwt-signing-secret-not-real-32-or-more-bytes-required",
		MapProvider:        "mapbox",
		DevMode:            true,
	}
}

func TestFinalizeValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.finalize(nil, ""); err != nil {
		t.Fatalf("finalize on valid config: %v", err)
	}
	// Dev mode with no CORS origins defaults to wildcard.
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "*" {
		t.Errorf("CORSOrigins = %v; want [*] in dev mode", cfg.CORSOrigins)
	}
}

func TestFinalizeVertexProjectFallsBackToEnvironment(t *testing.T) {
	// --vertex-ai-project has no compiled-in default (that would name one
	// deployment), so an unset flag has to resolve from the environment or the
	// AI provider silently fails to configure — which is how this surfaced:
	// the e2e AI test started a dev server with no flag and got
	// "AI provider not configured". entrypoint.sh already forwards
	// GOOGLE_CLOUD_PROJECT the same way in production.
	tests := []struct {
		name string
		env  map[string]string
		flag string
		want string
	}{
		{"explicit flag wins", map[string]string{"VERTEX_AI_PROJECT": "from-env"}, "from-flag", "from-flag"},
		{"VERTEX_AI_PROJECT is used", map[string]string{"VERTEX_AI_PROJECT": "from-vertex-env"}, "", "from-vertex-env"},
		{"GOOGLE_CLOUD_PROJECT is the fallback", map[string]string{"GOOGLE_CLOUD_PROJECT": "from-gcp-env"}, "", "from-gcp-env"},
		{"VERTEX_AI_PROJECT takes precedence", map[string]string{
			"VERTEX_AI_PROJECT": "from-vertex-env", "GOOGLE_CLOUD_PROJECT": "from-gcp-env",
		}, "", "from-vertex-env"},
		{"neither set stays empty", nil, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{"VERTEX_AI_PROJECT", "GOOGLE_CLOUD_PROJECT"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg := validConfig()
			cfg.VertexAIProject = tt.flag
			if err := cfg.finalize(nil, ""); err != nil {
				t.Fatalf("finalize: %v", err)
			}
			if cfg.VertexAIProject != tt.want {
				t.Errorf("VertexAIProject = %q, want %q", cfg.VertexAIProject, tt.want)
			}
		})
	}
}

func TestFinalizeRequiresDBURL(t *testing.T) {
	cfg := validConfig()
	cfg.DBURL = ""
	err := cfg.finalize(nil, "")
	if err == nil || !strings.Contains(err.Error(), "--db flag is required") {
		t.Errorf("finalize without --db = %v; want required error", err)
	}
}

func TestFinalizeRequiresInviteLinkHostname(t *testing.T) {
	cfg := validConfig()
	cfg.InviteLinkHostname = ""
	err := cfg.finalize(nil, "")
	if err == nil || !strings.Contains(err.Error(), "--invite-link-hostname is required") {
		t.Errorf("finalize without hostname = %v; want required error", err)
	}
}

func TestFinalizeRequiresLongJWTSecret(t *testing.T) {
	cfg := validConfig()
	cfg.JWTSigningSecret = "short"
	err := cfg.finalize(nil, "")
	if err == nil || !strings.Contains(err.Error(), "--jwt-signing-secret") {
		t.Errorf("finalize with short JWT secret = %v; want length error", err)
	}
}

func TestFinalizeRejectsUnknownMapProvider(t *testing.T) {
	cfg := validConfig()
	cfg.MapProvider = "osm"
	err := cfg.finalize(nil, "")
	if err == nil || !strings.Contains(err.Error(), "--map-provider") {
		t.Errorf("finalize with unknown map provider = %v; want enum error", err)
	}
}

func TestFinalizeCORSProductionRules(t *testing.T) {
	t.Run("empty origins rejected in production", func(t *testing.T) {
		cfg := validConfig()
		cfg.DevMode = false
		cfg.CORSOrigins = nil
		err := cfg.finalize(nil, "")
		if err == nil || !strings.Contains(err.Error(), "--cors-allowed-origins is required") {
			t.Errorf("finalize = %v; want required error", err)
		}
	})
	t.Run("wildcard rejected in production", func(t *testing.T) {
		cfg := validConfig()
		cfg.DevMode = false
		cfg.CORSOrigins = []string{"https://example.com", "*"}
		err := cfg.finalize(nil, "")
		if err == nil || !strings.Contains(err.Error(), "wildcard CORS origin") {
			t.Errorf("finalize = %v; want wildcard error", err)
		}
	})
	t.Run("explicit allowlist accepted in production", func(t *testing.T) {
		cfg := validConfig()
		cfg.DevMode = false
		cfg.CORSOrigins = []string{"https://example.com"}
		if err := cfg.finalize(nil, ""); err != nil {
			t.Errorf("finalize = %v; want nil", err)
		}
	})
}

func TestFinalizeResolvesSecretFiles(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(keyPath, []byte("file-secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := validConfig()
	path := keyPath
	err := cfg.finalize([]secretFile{
		{"mailgun-api-key", &cfg.MailgunAPIKey, &path},
	}, "")
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if cfg.MailgunAPIKey != "file-secret-value" {
		t.Errorf("MailgunAPIKey = %q; want trimmed file contents", cfg.MailgunAPIKey)
	}
}

func TestFinalizeDecodesGitHubPrivateKey(t *testing.T) {
	const fakePEM = "-----BEGIN RSA PRIVATE KEY-----\nabc\n-----END RSA PRIVATE KEY-----\n"
	cfg := validConfig()
	cfg.GitHubAppPrivateKey = base64.StdEncoding.EncodeToString([]byte(fakePEM))
	if err := cfg.finalize(nil, ""); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if cfg.GitHubAppPrivateKey != fakePEM {
		t.Errorf("GitHubAppPrivateKey = %q; want decoded PEM", cfg.GitHubAppPrivateKey)
	}

	cfg = validConfig()
	cfg.GitHubAppPrivateKey = "not-valid-base64!!!"
	err := cfg.finalize(nil, "")
	if err == nil || !strings.Contains(err.Error(), "not valid base64") {
		t.Errorf("finalize with bad base64 = %v; want decode error", err)
	}
}

func TestDecodeBase64PEM(t *testing.T) {
	const fakePEM = "-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----\n"

	encoded := base64.StdEncoding.EncodeToString([]byte(fakePEM))
	got, err := decodeBase64PEM(encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != fakePEM {
		t.Errorf("got %q, want %q", got, fakePEM)
	}
}

func TestDecodeBase64PEMInvalidInput(t *testing.T) {
	_, err := decodeBase64PEM("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64, got nil")
	}
}

func TestParseCORSOrigins(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{name: "empty", input: "", want: nil},
		{name: "single", input: "https://example.com", want: []string{"https://example.com"}},
		{name: "multiple", input: "https://example.com,https://dev.example.com", want: []string{"https://example.com", "https://dev.example.com"}},
		{name: "whitespace trimmed", input: " https://example.com , https://dev.example.com ", want: []string{"https://example.com", "https://dev.example.com"}},
		{name: "wildcard", input: "*", want: []string{"*"}},
		{name: "localhost pattern", input: "http://localhost:*", want: []string{"http://localhost:*"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCORSOrigins(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("parseCORSOrigins(%q) = %v; want %v", tc.input, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("parseCORSOrigins(%q)[%d] = %q; want %q", tc.input, i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseWeekday(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Weekday
		wantErr bool
	}{
		{input: "Monday", want: time.Monday},
		{input: "monday", want: time.Monday},
		{input: " tue ", want: time.Tuesday},
		{input: "TUES", want: time.Tuesday},
		{input: "Wed", want: time.Wednesday},
		{input: "thurs", want: time.Thursday},
		{input: "fri", want: time.Friday},
		{input: "sat", want: time.Saturday},
		{input: "sun", want: time.Sunday},
		{input: "someday", wantErr: true},
		{input: "", wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseWeekday(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseWeekday(%q) = %v; want error", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseWeekday(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ParseWeekday(%q) = %v; want %v", tc.input, got, tc.want)
			}
		})
	}
}
