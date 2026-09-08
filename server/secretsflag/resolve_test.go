package secretsflag

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolve_ValueOnly(t *testing.T) {
	got, err := Resolve("openai-api-key", "sk-test-123", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "sk-test-123" {
		t.Errorf("got %q, want %q", got, "sk-test-123")
	}
}

func TestResolve_FileOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("sk-from-file\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Resolve("openai-api-key", "", path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "sk-from-file" {
		t.Errorf("got %q, want %q (trailing newline should be trimmed)", got, "sk-from-file")
	}
}

func TestResolve_FileWithoutTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, []byte("sk-no-newline"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Resolve("openai-api-key", "", path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "sk-no-newline" {
		t.Errorf("got %q, want %q", got, "sk-no-newline")
	}
}

func TestResolve_FileMultilinePEM(t *testing.T) {
	// PEM payloads embed newlines that must be preserved; only the trailing
	// run of whitespace is trimmed.
	pem := "-----BEGIN PRIVATE KEY-----\nABCDEF\nGHIJKL\n-----END PRIVATE KEY-----\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Resolve("github-app-private-key", "", path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := strings.TrimSuffix(pem, "\n")
	if got != want {
		t.Errorf("multi-line content not preserved.\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestResolve_TrimsAllTrailingWhitespace covers the cases the old single-\n
// TrimSuffix missed: CRLF line endings, repeated trailing newlines, and
// trailing spaces/tabs — all common Secret Manager upload artifacts that
// silently corrupt exact-match credentials.
func TestResolve_TrimsAllTrailingWhitespace(t *testing.T) {
	cases := map[string]string{
		"single newline":      "sk-key\n",
		"crlf":                "sk-key\r\n",
		"double newline":      "sk-key\n\n",
		"trailing spaces":     "sk-key   ",
		"trailing tabs":       "sk-key\t\t",
		"mixed trailing":      "sk-key \t\r\n",
		"no trailing":         "sk-key",
		"crlf after newlines": "sk-key\n\r\n",
	}
	dir := t.TempDir()
	for name, raw := range cases {
		t.Run(name+" (file)", func(t *testing.T) {
			path := filepath.Join(dir, "key")
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			got, err := Resolve("openai-api-key", "", path)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != "sk-key" {
				t.Errorf("file path: got %q, want %q", got, "sk-key")
			}
		})
		t.Run(name+" (value)", func(t *testing.T) {
			got, err := Resolve("openai-api-key", raw, "")
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != "sk-key" {
				t.Errorf("value path: got %q, want %q", got, "sk-key")
			}
		})
	}
}

// TestResolve_PreservesLeadingAndEmbeddedWhitespace guards that only the
// trailing run is trimmed — leading whitespace and interior newlines survive.
func TestResolve_PreservesLeadingAndEmbeddedWhitespace(t *testing.T) {
	raw := "\tline-one\n  line-two\n"
	want := "\tline-one\n  line-two"
	dir := t.TempDir()
	path := filepath.Join(dir, "multi")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := Resolve("jwt-signing-secret", "", path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolve_NeitherSet(t *testing.T) {
	got, err := Resolve("openai-api-key", "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestResolve_BothSet(t *testing.T) {
	_, err := Resolve("openai-api-key", "sk-test-123", "/tmp/anything")
	if err == nil {
		t.Fatal("expected error when both flags set")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error %q should mention mutual exclusivity", err)
	}
}

func TestResolve_FileMissing(t *testing.T) {
	_, err := Resolve("openai-api-key", "", "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error when file missing")
	}
	// Error must reference the flag name and path so operators can debug.
	if !strings.Contains(err.Error(), "openai-api-key") {
		t.Errorf("error %q should include the flag name", err)
	}
	// The error must NOT include any decoded contents — there are no
	// contents to leak, but if we ever change the wrap pattern this guards
	// against accidental disclosure.
	if strings.Contains(err.Error(), "sk-") {
		t.Errorf("error %q looks like it might leak secret material", err)
	}
}

func TestInjectPostgresPassword_NoFile(t *testing.T) {
	got, err := InjectPostgresPassword("postgres://user:pass@host/db", "")
	if err != nil {
		t.Fatalf("InjectPostgresPassword: %v", err)
	}
	if got != "postgres://user:pass@host/db" {
		t.Errorf("URL changed when path empty: got %q", got)
	}
}

func TestInjectPostgresPassword_InjectsFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dbpw")
	if err := os.WriteFile(path, []byte("s3cret\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got, err := InjectPostgresPassword("postgres://user@host:5432/db?sslmode=require", path)
	if err != nil {
		t.Fatalf("InjectPostgresPassword: %v", err)
	}
	want := "postgres://user:s3cret@host:5432/db?sslmode=require"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInjectPostgresPassword_RejectsURLPasswordWhenFileSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dbpw")
	if err := os.WriteFile(path, []byte("s3cret"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := InjectPostgresPassword("postgres://user:embedded@host/db", path)
	if err == nil {
		t.Fatal("expected error when URL has password AND file flag is set")
	}
	if !strings.Contains(err.Error(), "embeds a password") {
		t.Errorf("error %q should explain the conflict", err)
	}
}

func TestInjectPostgresPassword_RejectsURLWithoutUsername(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dbpw")
	if err := os.WriteFile(path, []byte("s3cret"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := InjectPostgresPassword("postgres://host/db", path)
	if err == nil {
		t.Fatal("expected error when URL has no username")
	}
}

func TestInjectPostgresPassword_RejectsMalformedURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dbpw")
	if err := os.WriteFile(path, []byte("s3cret"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	_, err := InjectPostgresPassword("::not a url::", path)
	if err == nil {
		t.Fatal("expected error on malformed URL")
	}
}
