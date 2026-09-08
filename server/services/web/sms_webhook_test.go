package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/smsoptout"
	"go.ripls.org/ripls/server/storage"
)

// twilioSignature recomputes the X-Twilio-Signature Twilio would send: HMAC-SHA1
// of the request URL with each POST param (sorted by key) appended as key+value,
// base64-encoded. Mirrors Twilio's documented algorithm so the test proves the
// handler accepts a genuinely-signed request, not just rejects a forged one.
func twilioSignature(token, fullURL string, params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(fullURL)
	for _, k := range keys {
		sb.WriteString(k)
		sb.WriteString(params.Get(k))
	}
	mac := hmac.New(sha1.New, []byte(token))
	mac.Write([]byte(sb.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func newWebServiceForTest(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	t.Helper()
	store, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	svc, err := New(store, nil, "test.example.com", []byte("unsub-secret"))
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return svc, store
}

// postKeyword fires the webhook with the given form values (no signature; the
// service is left with an empty auth token so verification is disabled).
func postKeyword(t *testing.T, svc *Service, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	svc.HandleSMSWebhook(rec, req)
	return rec
}

func TestSMSWebhook_StopOptsOut(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	rec := postKeyword(t, svc, url.Values{"From": {"+15551234567"}, "Body": {"STOP"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// Persist-only: we acknowledge but never reply (Twilio owns the auto-reply).
	if strings.Contains(rec.Body.String(), "<Message>") {
		t.Errorf("handler must not send its own reply: %q", rec.Body.String())
	}
	out, err := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if err != nil || !out {
		t.Errorf("number should be opted out after STOP, got out=%v err=%v", out, err)
	}
}

func TestSMSWebhook_StartOptsBackIn(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	ctx := context.Background()
	if err := smsoptout.RecordOptOut(ctx, store, "+15551234567"); err != nil {
		t.Fatalf("seed opt-out: %v", err)
	}

	rec := postKeyword(t, svc, url.Values{"From": {"+15551234567"}, "Body": {"START"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	out, err := smsoptout.IsOptedOut(ctx, store, "+15551234567")
	if err != nil || out {
		t.Errorf("number should be opted back in after START, got out=%v err=%v", out, err)
	}
}

func TestSMSWebhook_HelpNoStateChange(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	rec := postKeyword(t, svc, url.Values{"From": {"+15551234567"}, "Body": {"HELP"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	// HELP is acknowledged but never altered or replied to here (Twilio replies).
	if strings.Contains(rec.Body.String(), "<Message>") {
		t.Errorf("handler must not reply to HELP: %q", rec.Body.String())
	}
	out, _ := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if out {
		t.Errorf("HELP must not change opt-out state")
	}
}

func TestSMSWebhook_OptOutTypeFieldWins(t *testing.T) {
	// Twilio Advanced Opt-Out posts OptOutType; honor it over the body text.
	svc, store := newWebServiceForTest(t)
	rec := postKeyword(t, svc, url.Values{
		"From": {"+15551234567"}, "Body": {"please leave me alone"}, "OptOutType": {"STOP"},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	out, _ := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if !out {
		t.Errorf("OptOutType=STOP should opt the number out")
	}
}

func TestSMSWebhook_UnrecognizedIsNoop(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	rec := postKeyword(t, svc, url.Values{"From": {"+15551234567"}, "Body": {"hello there"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	// Bare acknowledgement, no <Message>.
	if strings.Contains(rec.Body.String(), "<Message>") {
		t.Errorf("unrecognized keyword should not auto-reply: %q", rec.Body.String())
	}
	out, _ := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if out {
		t.Errorf("unrecognized keyword must not opt out")
	}
}

func TestSMSWebhook_ValidSignatureAccepted(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	svc.SetSMSAuthToken("a-real-token") // enables verification

	form := url.Values{"From": {"+15551234567"}, "Body": {"STOP"}}
	// The handler verifies against "https://<hostname>/sms/webhook"; hostname is
	// "test.example.com" in newWebServiceForTest.
	sig := twilioSignature("a-real-token", "https://test.example.com/sms/webhook", form)

	req := httptest.NewRequest(http.MethodPost, "/sms/webhook", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", sig)
	rec := httptest.NewRecorder()
	svc.HandleSMSWebhook(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a correctly-signed request", rec.Code)
	}
	out, err := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if err != nil || !out {
		t.Errorf("a validly-signed STOP should opt the number out, got out=%v err=%v", out, err)
	}
}

func TestSMSWebhook_InvalidSignatureRejected(t *testing.T) {
	svc, store := newWebServiceForTest(t)
	svc.SetSMSAuthToken("a-real-token") // enables verification

	req := httptest.NewRequest(http.MethodPost, "/sms/webhook",
		strings.NewReader(url.Values{"From": {"+15551234567"}, "Body": {"STOP"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Twilio-Signature", "obviously-wrong")
	rec := httptest.NewRecorder()
	svc.HandleSMSWebhook(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	out, _ := smsoptout.IsOptedOut(context.Background(), store, "+15551234567")
	if out {
		t.Errorf("a forged webhook must not change opt-out state")
	}
}
