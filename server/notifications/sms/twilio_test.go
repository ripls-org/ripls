package sms

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSendSMS_Success(t *testing.T) {
	gotAuthUser, gotAuthOK := "", false
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuthUser, _, gotAuthOK = r.BasicAuth()
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		if r.URL.Path != "/2010-04-01/Accounts/AC123/Messages.json" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SM999","status":"queued","error_code":null}`))
	}))
	defer srv.Close()

	s := NewTwilioSMSSender("AC123", "tok", "MG456")
	s.baseURL = srv.URL

	if err := s.SendSMS(context.Background(), "+15551234567", "hello there"); err != nil {
		t.Fatalf("SendSMS: %v", err)
	}
	if !gotAuthOK || gotAuthUser != "AC123" {
		t.Errorf("basic auth user = %q ok=%v, want AC123", gotAuthUser, gotAuthOK)
	}
	if gotForm.Get("MessagingServiceSid") != "MG456" {
		t.Errorf("MessagingServiceSid = %q, want MG456", gotForm.Get("MessagingServiceSid"))
	}
	if gotForm.Get("To") != "+15551234567" {
		t.Errorf("To = %q", gotForm.Get("To"))
	}
	if gotForm.Get("Body") != "hello there" {
		t.Errorf("Body = %q", gotForm.Get("Body"))
	}
}

func TestSendSMS_ProviderError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":21610,"message":"Attempt to send to unsubscribed recipient","status":400}`))
	}))
	defer srv.Close()

	s := NewTwilioSMSSender("AC123", "tok", "MG456")
	s.baseURL = srv.URL

	err := s.SendSMS(context.Background(), "+15551234567", "hi")
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
	if !strings.Contains(err.Error(), "21610") {
		t.Errorf("error %q should carry the twilio code", err.Error())
	}
	// The error must not leak the recipient number or body.
	if strings.Contains(err.Error(), "5551234567") || strings.Contains(err.Error(), "hi") {
		t.Errorf("error leaks PII: %q", err.Error())
	}
}

func TestValidateSignature(t *testing.T) {
	// Twilio's documented worked example: this URL + params signed with token
	// "12345" yields the known signature below.
	const token = "12345"
	const publicURL = "https://mycompany.com/myapp.php?foo=1&bar=2"
	params := url.Values{
		"CallSid": {"CA1234567890ABCDE"},
		"Caller":  {"+14158675309"},
		"Digits":  {"1234"},
		"From":    {"+14158675309"},
		"To":      {"+18005551212"},
	}
	const want = "RSOYDt4T1cUTdK1PDd93/VVr8B8="

	if !ValidateSignature(token, publicURL, params, want) {
		t.Errorf("valid signature rejected")
	}
	if ValidateSignature(token, publicURL, params, "bogus") {
		t.Errorf("invalid signature accepted")
	}
	// Empty token disables validation (caller decides acceptability).
	if !ValidateSignature("", publicURL, params, "anything") {
		t.Errorf("empty token should disable validation")
	}
}

func TestMockSMSSender(t *testing.T) {
	m := &MockSMSSender{}
	if err := m.SendSMS(context.Background(), "+15550001111", "yo"); err != nil {
		t.Fatalf("mock send: %v", err)
	}
	msgs := m.Messages()
	if len(msgs) != 1 || msgs[0].To != "+15550001111" || msgs[0].Body != "yo" {
		t.Errorf("unexpected captured messages: %+v", msgs)
	}
}
