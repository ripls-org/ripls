package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	riplsfirebase "go.ripls.org/ripls/server/firebase"
)

// TestFirebaseAuth_EmulatorPhoneToken proves the production phone-token verifier
// (NewApp → NewFirebaseAuth → VerifyPhoneToken) accepts a real Firebase phone ID
// token issued by the Auth Emulator, with NO real credentials. This is the
// server half of the WEB-2 phone-first e2e: the e2e server runs against the
// emulator (FIREBASE_AUTH_EMULATOR_HOST), so a token the Flutter web client
// obtains from the emulator must validate here.
//
// It is gated on FIREBASE_AUTH_EMULATOR_HOST, so it is a no-op in the normal
// unit-test run and only executes under `firebase emulators:exec` (which sets
// that env). Run it with:
//
//	npx firebase-tools@15 emulators:exec --only auth --project demo-ripls \
//	  'cd server && go test ./auth/ -run TestFirebaseAuth_EmulatorPhoneToken -v'
func TestFirebaseAuth_EmulatorPhoneToken(t *testing.T) {
	emulatorHost := os.Getenv("FIREBASE_AUTH_EMULATOR_HOST")
	if emulatorHost == "" {
		t.Skip("FIREBASE_AUTH_EMULATOR_HOST not set; run under `firebase emulators:exec`")
	}

	const (
		projectID = "demo-ripls"
		phone     = "+15555550001"
	)
	ctx := context.Background()

	// Build the production verifier exactly as the server does — no credentials.
	app, err := riplsfirebase.NewApp(ctx, projectID)
	if err != nil {
		t.Fatalf("NewApp against emulator: %v", err)
	}
	fa, err := NewFirebaseAuth(ctx, app)
	if err != nil {
		t.Fatalf("NewFirebaseAuth: %v", err)
	}

	// Mint a phone ID token through the emulator's identitytoolkit REST API
	// (the same calls the Firebase client SDK makes under the hood).
	idToken := mintEmulatorPhoneToken(t, emulatorHost, projectID, phone)

	got, err := fa.VerifyPhoneToken(ctx, idToken)
	if err != nil {
		t.Fatalf("VerifyPhoneToken on an emulator-issued phone token: %v", err)
	}
	if got != phone {
		t.Errorf("VerifyPhoneToken = %q, want %q", got, phone)
	}
}

// mintEmulatorPhoneToken drives the emulator's phone sign-in: request a
// verification code, read it back from the emulator's debug endpoint, then
// exchange it for an ID token.
func mintEmulatorPhoneToken(t *testing.T, emulatorHost, projectID, phone string) string {
	t.Helper()
	base := "http://" + emulatorHost
	// key is ignored by the emulator but required by the endpoint shape.
	const key = "fake-api-key"

	// 1. Request a verification code.
	var sendResp struct {
		SessionInfo string `json:"sessionInfo"`
	}
	emulatorPost(t,
		base+"/identitytoolkit.googleapis.com/v1/accounts:sendVerificationCode?key="+key,
		map[string]any{"phoneNumber": phone},
		&sendResp)
	if sendResp.SessionInfo == "" {
		t.Fatal("emulator returned empty sessionInfo")
	}

	// 2. Read the auto-generated code from the emulator's debug endpoint.
	var codesResp struct {
		VerificationCodes []struct {
			PhoneNumber string `json:"phoneNumber"`
			Code        string `json:"code"`
		} `json:"verificationCodes"`
	}
	emulatorGet(t, base+"/emulator/v1/projects/"+projectID+"/verificationCodes", &codesResp)
	code := ""
	for _, c := range codesResp.VerificationCodes {
		if c.PhoneNumber == phone {
			code = c.Code
		}
	}
	if code == "" {
		t.Fatalf("no verification code for %s in emulator", phone)
	}

	// 3. Exchange (sessionInfo, code) for an ID token.
	var signInResp struct {
		IDToken string `json:"idToken"`
	}
	emulatorPost(t,
		base+"/identitytoolkit.googleapis.com/v1/accounts:signInWithPhoneNumber?key="+key,
		map[string]any{"sessionInfo": sendResp.SessionInfo, "code": code},
		&signInResp)
	if signInResp.IDToken == "" {
		t.Fatal("emulator returned empty idToken")
	}
	return signInResp.IDToken
}

func emulatorPost(t *testing.T, url string, body, out any) {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	readEmulatorResponse(t, url, resp, out)
}

func emulatorGet(t *testing.T, url string, out any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	readEmulatorResponse(t, url, resp, out)
}

func readEmulatorResponse(t *testing.T, url string, resp *http.Response, out any) {
	t.Helper()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s -> %d: %s", url, resp.StatusCode, string(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s response %q: %v", url, string(body), err)
	}
}
