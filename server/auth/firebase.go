package auth

import (
	"context"
	"fmt"
	"reflect"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
)

// PhoneTokenVerifier validates Firebase ID tokens and extracts phone numbers.
type PhoneTokenVerifier interface {
	// VerifyPhoneToken validates a Firebase ID token and returns the verified
	// phone number. Returns an error if the token is invalid, expired, or was
	// not issued via phone auth.
	VerifyPhoneToken(ctx context.Context, idToken string) (phoneNumber string, err error)
}

// PhoneAuthConfigured reports whether phone auth is available behind v. It
// guards against Go's nil-interface trap: a typed nil (e.g.
// (*FirebaseAuth)(nil)) stored in an interface is non-nil, so a plain == nil
// check passes but calling any method panics with a nil pointer dereference.
// Shared by every RPC that depends on a PhoneTokenVerifier (PhoneRegister,
// PhoneLogin, AddPhoneNumber).
func PhoneAuthConfigured(v PhoneTokenVerifier) bool {
	if v == nil {
		return false
	}
	rv := reflect.ValueOf(v)
	return rv.Kind() != reflect.Pointer || !rv.IsNil()
}

// FirebaseAuth validates Firebase ID tokens for phone authentication.
type FirebaseAuth struct {
	client tokenVerifier
}

// tokenVerifier abstracts the Firebase auth client for testability.
type tokenVerifier interface {
	VerifyIDToken(ctx context.Context, idToken string) (*fbauth.Token, error)
}

// NewFirebaseAuth creates a FirebaseAuth from a Firebase app instance.
func NewFirebaseAuth(ctx context.Context, app *firebase.App) (*FirebaseAuth, error) {
	client, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get Firebase auth client: %w", err)
	}
	return &FirebaseAuth{client: client}, nil
}

// VerifyPhoneToken validates a Firebase ID token and returns the verified phone
// number in E.164 format. It rejects tokens that were not issued via phone auth
// to prevent impersonation via other Firebase sign-in providers.
func (fa *FirebaseAuth) VerifyPhoneToken(ctx context.Context, idToken string) (string, error) {
	if idToken == "" {
		return "", fmt.Errorf("firebase ID token is required")
	}

	token, err := fa.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return "", fmt.Errorf("invalid firebase token: %w", err)
	}

	// Verify the token was issued via phone sign-in provider.
	if token.Firebase.SignInProvider != "phone" {
		return "", fmt.Errorf("token was not issued via phone auth (provider: %s)", token.Firebase.SignInProvider)
	}

	// Extract the phone number from the token claims.
	phoneNumber, ok := token.Claims["phone_number"].(string)
	if !ok || phoneNumber == "" {
		return "", fmt.Errorf("token does not contain a phone number")
	}

	return phoneNumber, nil
}
