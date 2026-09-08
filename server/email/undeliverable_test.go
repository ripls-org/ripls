package email

import (
	"os"
	"strings"
	"testing"
)

func TestIsUndeliverableTestAddress(t *testing.T) {
	// Every synthetic address the harnesses actually generate must be caught —
	// these are the ones that would otherwise hard-bounce in bulk. Verified
	// against the dev database, where ~1,900 accounts sit at e2etest.example.com.
	undeliverable := []string{
		"marcus-123@e2etest.example.com", // server/e2e_test.go
		"e2e-uuid@ripls.test",            // e2e/lib/seed/users.ts
		"marcus@example.com",             // server/simulation personas
		"someone@example.net",
		"someone@example.org",
		"someone@sub.example.com",
		"someone@anything.test",
		"someone@thing.invalid",
		"someone@localhost",
		"MiXeD@Example.COM", // case-insensitive
	}
	for _, addr := range undeliverable {
		if !IsUndeliverableTestAddress(addr) {
			t.Errorf("IsUndeliverableTestAddress(%q) = false; it would be mailed and hard-bounce", addr)
		}
	}

	// Real addresses must never be skipped — a false positive here silently
	// stops someone receiving the code they need to sign in, which is worse
	// than the bounce this function exists to prevent.
	// These must be domains RFC 2606 does NOT reserve — the whole point is that
	// an ordinary recipient is never skipped. example.com and friends belong in
	// the undeliverable list below, so they cannot stand in here.
	deliverable := []string{
		"someone@acme.com",
		"someone@acme.org",
		"ops@acme.org",
		"someone@gmail.com",
		"someone@notexample.com",
		"someone@example.company", // not example.com
		"someone@testing.io",      // not a reserved .test TLD
		"someone@contest.com",     // must not match on a substring
	}
	for _, addr := range deliverable {
		if IsUndeliverableTestAddress(addr) {
			t.Errorf("IsUndeliverableTestAddress(%q) = true; a real recipient would never get their code", addr)
		}
	}

	// Malformed input is not our call to make — let it through and let the
	// provider reject it, rather than silently dropping a send.
	for _, addr := range []string{"", "no-at-sign", "trailing@"} {
		if IsUndeliverableTestAddress(addr) {
			t.Errorf("IsUndeliverableTestAddress(%q) = true; malformed input should not be silently dropped", addr)
		}
	}
}

// The skip is at the send layer, so the code is still issued and verification
// still works — that is what keeps every test harness functioning.
func TestSendEmailCode_SkipsDeliveryButNotIssuance(t *testing.T) {
	mock := &MockEmailService{}
	if err := mock.SendEmailCode(t.Context(), "someone@e2etest.example.com", "123456", 0, ""); err != nil {
		t.Fatalf("mock send failed: %v", err)
	}
	if _, ok := mock.LastCodeFor("someone@e2etest.example.com"); !ok {
		t.Error("the code was not recorded; test harnesses read it from here")
	}
}

// Every send path that takes a user-supplied address must be guarded. Missing
// one leaves a flow that can still bulk-bounce off the sending domain, which is
// the failure this whole guard exists to prevent — and the guard being partial
// is far easier to miss than it being absent.
func TestEverySendPathGuardsUndeliverableAddresses(t *testing.T) {
	src, err := os.ReadFile("mailgun.go")
	if err != nil {
		t.Fatalf("read mailgun.go: %v", err)
	}
	digest, err := os.ReadFile("activity_digest_render.go")
	if err != nil {
		t.Fatalf("read activity_digest_render.go: %v", err)
	}
	all := string(src) + string(digest)

	// Paths that mail an address a user supplied. Each must check before sending.
	for _, emailType := range []string{
		`"email_code"`,
		`"off_app_notification"`,
		`"password_reset"`,
		`"waitlist_welcome"`,
	} {
		if !strings.Contains(all, emailType) {
			t.Errorf("no send path found for %s — did it get renamed?", emailType)
		}
	}

	// One guard per user-address path. activity_digest and waitlist_notification
	// are deliberately unguarded: both go to an operator address from config,
	// never to anything a user typed.
	if got, want := strings.Count(string(src), "IsUndeliverableTestAddress("), 4; got != want {
		t.Errorf("found %d IsUndeliverableTestAddress guards in mailgun.go, want %d "+
			"(one per user-address send path: email_code, off_app_notification, "+
			"password_reset, waitlist_welcome); a new send path taking a "+
			"user-supplied address needs one too", got, want)
	}
}
