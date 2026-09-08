package auth

import (
	"context"
	"fmt"
	"strings"
	"testing"

	fbauth "firebase.google.com/go/v4/auth"
)

// mockTokenVerifier implements tokenVerifier for testing.
type mockTokenVerifier struct {
	token *fbauth.Token
	err   error
}

func (m *mockTokenVerifier) VerifyIDToken(_ context.Context, _ string) (*fbauth.Token, error) {
	return m.token, m.err
}

func TestPhoneAuthConfigured(t *testing.T) {
	t.Run("nil interface is not configured", func(t *testing.T) {
		if PhoneAuthConfigured(nil) {
			t.Error("a nil verifier must report unconfigured")
		}
	})

	t.Run("typed-nil pointer is not configured", func(t *testing.T) {
		// The nil-interface trap: a (*FirebaseAuth)(nil) stored in the
		// interface is itself non-nil, so a plain == nil check passes but a
		// method call would panic. PhoneAuthConfigured must catch this.
		var typedNil *FirebaseAuth
		if PhoneAuthConfigured(typedNil) {
			t.Error("a typed-nil verifier must report unconfigured")
		}
	})

	t.Run("a real verifier is configured", func(t *testing.T) {
		var v PhoneTokenVerifier = &FirebaseAuth{}
		if !PhoneAuthConfigured(v) {
			t.Error("a non-nil verifier must report configured")
		}
	})
}

func TestFirebaseAuth_VerifyPhoneToken(t *testing.T) {
	tests := []struct {
		name       string
		idToken    string
		mockToken  *fbauth.Token
		mockErr    error
		wantPhone  string
		wantErrMsg string
	}{
		{
			name:    "valid phone token",
			idToken: "valid-token",
			mockToken: &fbauth.Token{
				Firebase: fbauth.FirebaseInfo{
					SignInProvider: "phone",
				},
				Claims: map[string]interface{}{
					"phone_number": "+15551234567",
				},
			},
			wantPhone: "+15551234567",
		},
		{
			name:       "empty token",
			idToken:    "",
			wantErrMsg: "firebase ID token is required",
		},
		{
			name:       "expired token",
			idToken:    "expired-token",
			mockErr:    fmt.Errorf("ID token has expired"),
			wantErrMsg: "invalid firebase token",
		},
		{
			name:    "non-phone provider (Google)",
			idToken: "google-token",
			mockToken: &fbauth.Token{
				Firebase: fbauth.FirebaseInfo{
					SignInProvider: "google.com",
				},
				Claims: map[string]interface{}{
					"email": "user@example.com",
				},
			},
			wantErrMsg: "token was not issued via phone auth (provider: google.com)",
		},
		{
			name:    "non-phone provider (password)",
			idToken: "password-token",
			mockToken: &fbauth.Token{
				Firebase: fbauth.FirebaseInfo{
					SignInProvider: "password",
				},
				Claims: map[string]interface{}{
					"email": "user@example.com",
				},
			},
			wantErrMsg: "token was not issued via phone auth (provider: password)",
		},
		{
			name:    "missing phone number claim",
			idToken: "no-phone-token",
			mockToken: &fbauth.Token{
				Firebase: fbauth.FirebaseInfo{
					SignInProvider: "phone",
				},
				Claims: map[string]interface{}{},
			},
			wantErrMsg: "token does not contain a phone number",
		},
		{
			name:    "empty phone number claim",
			idToken: "empty-phone-token",
			mockToken: &fbauth.Token{
				Firebase: fbauth.FirebaseInfo{
					SignInProvider: "phone",
				},
				Claims: map[string]interface{}{
					"phone_number": "",
				},
			},
			wantErrMsg: "token does not contain a phone number",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fa := &FirebaseAuth{
				client: &mockTokenVerifier{
					token: tt.mockToken,
					err:   tt.mockErr,
				},
			}

			phone, err := fa.VerifyPhoneToken(context.Background(), tt.idToken)

			if tt.wantErrMsg != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrMsg)
				}
				if got := err.Error(); !strings.Contains(got, tt.wantErrMsg) {
					t.Errorf("error = %q, want containing %q", got, tt.wantErrMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if phone != tt.wantPhone {
				t.Errorf("phone = %q, want %q", phone, tt.wantPhone)
			}
		})
	}
}
