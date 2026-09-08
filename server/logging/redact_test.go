package logging

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestMaskEmail(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"standard email", "alice@example.com", "al***@example.com"},
		{"short local part 1 char", "a@example.com", "a***@example.com"},
		{"short local part 2 chars", "ab@example.com", "ab***@example.com"},
		{"long local part", "verylongemail@example.com", "ve***@example.com"},
		{"empty string", "", ""},
		{"no at sign", "invalid", "***@***"},
		{"no domain", "user@", "***@***"},
		{"no local part", "@domain.com", "***@domain.com"},
		{"multiple at signs", "user@domain@extra", "***@***"},
		{"subdomain", "user@mail.example.com", "us***@mail.example.com"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MaskEmail(tc.input)
			if result != tc.expected {
				t.Errorf("MaskEmail(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"long token", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature", "eyJhbGci..."},
		{"exactly 8 chars", "12345678", "12345678"},
		{"less than 8 chars", "short", "short"},
		{"empty string", "", ""},
		{"9 chars", "123456789", "12345678..."},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := MaskToken(tc.input)
			if result != tc.expected {
				t.Errorf("MaskToken(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestRedactedEmail_LogValue(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	email := RedactedEmail("alice@example.com")
	logger.Info("user action", "email", email)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["email"] != "al***@example.com" {
		t.Errorf("Expected masked email 'al***@example.com', got %v", logEntry["email"])
	}
}

func TestRedactedToken_LogValue(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	token := RedactedToken("eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.signature")
	logger.Info("auth event", "token", token)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["token"] != "eyJhbGci..." {
		t.Errorf("Expected masked token 'eyJhbGci...', got %v", logEntry["token"])
	}
}

func TestRedactedEmail_EmptyAndInvalid(t *testing.T) {
	var buf bytes.Buffer
	logger := NewLogger(Options{
		Level:  "info",
		Format: "json",
		Output: &buf,
	})

	// Test empty email
	emptyEmail := RedactedEmail("")
	logger.Info("test", "email", emptyEmail)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if logEntry["email"] != "" {
		t.Errorf("Expected empty string for empty email, got %v", logEntry["email"])
	}
}
