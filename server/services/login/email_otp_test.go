package login

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/email"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// mockEmail reaches the captured-send recorder behind the service's email
// interface, matching the pattern in password_reset_test.go.
func mockEmail(t *testing.T, service *Service) *email.MockEmailService {
	t.Helper()
	mock, ok := service.emailService.(*email.MockEmailService)
	if !ok {
		t.Fatal("expected the service to hold a MockEmailService")
	}
	return mock
}

// requestCode issues a code and returns the value the mock captured.
func requestCode(t *testing.T, ctx context.Context, service *Service, addr string) string {
	t.Helper()
	if _, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: addr,
	})); err != nil {
		t.Fatalf("RequestEmailCode(%s) failed: %v", addr, err)
	}
	// The send goes to the normalized address, so that is the key the mock
	// recorded it under — look it up there rather than by whatever casing or
	// surrounding whitespace the caller passed in.
	code, ok := mockEmail(t, service).LastCodeFor(auth.NormalizeEmail(addr))
	if !ok {
		t.Fatalf("no code was sent to %s", addr)
	}
	return code
}

// proveEmail runs the full request→verify loop and returns the proof token,
// the credential every verified register/login path expects.
func proveEmail(t *testing.T, ctx context.Context, service *Service, addr string) string {
	t.Helper()

	code := requestCode(t, ctx, service, addr)
	resp, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	}))
	if err != nil {
		t.Fatalf("VerifyEmailCode(%s) failed: %v", addr, err)
	}
	return resp.Msg.EmailProofToken
}

func TestVerifyEmailCode_HappyPath(t *testing.T) {
	service, _, _, authTokenConfig := setupTestService(t)
	ctx := context.Background()
	const addr = "someone@example.com"

	code := requestCode(t, ctx, service, addr)

	resp, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	}))
	if err != nil {
		t.Fatalf("VerifyEmailCode failed: %v", err)
	}

	proven, err := authTokenConfig.ValidateEmailProofToken(resp.Msg.EmailProofToken)
	if err != nil {
		t.Fatalf("minted proof token did not validate: %v", err)
	}
	if proven != addr {
		t.Errorf("proof token attests to %q, want %q", proven, addr)
	}
	if resp.Msg.ExpiresAtUnixSec <= time.Now().Unix() {
		t.Error("proof token expiry is not in the future")
	}
}

// The address is normalized on both sides, so a code requested for one casing
// verifies with another — the same rule registration uses to match accounts.
func TestVerifyEmailCode_NormalizesAddress(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	code := requestCode(t, ctx, service, "mixed@example.com")

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: "  MiXeD@Example.COM  ",
		Code:  code,
	})); err != nil {
		t.Fatalf("VerifyEmailCode rejected a differently-cased address: %v", err)
	}
}

// The dev echo is the seam automated tests and the simulation harness use to
// complete the flow without a mailbox. It must be unreachable in production,
// where devAuth is false — this is the regression test that keeps it so.
func TestRequestEmailCode_DevCodeOnlyInDevMode(t *testing.T) {
	t.Run("suppressed when devAuth is false", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)

		resp, err := service.RequestEmailCode(context.Background(), connect.NewRequest(&api.RequestEmailCodeRequest{
			Email: "prod@example.com",
		}))
		if err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}
		if resp.Msg.DevCode != nil {
			t.Fatal("dev_code was populated on a non-dev server — the code leaked to the client")
		}
	})

	// The echo is ADDITIVE, not a substitute for sending. A dev server with a
	// real mail service configured — which the dev environment has — still
	// sends, so the delivery path can be exercised there for real. If this ever
	// short-circuits the send, testing the email workflow outside production
	// becomes impossible.
	t.Run("still sends the email in dev mode", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		service.devAuth = true
		const addr = "devsend@example.com"

		resp, err := service.RequestEmailCode(context.Background(), connect.NewRequest(&api.RequestEmailCodeRequest{
			Email: addr,
		}))
		if err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}

		sent, ok := mockEmail(t, service).LastCodeFor(addr)
		if !ok {
			t.Fatal("dev mode skipped the send; the mail path would be untestable outside production")
		}
		if sent != resp.Msg.GetDevCode() {
			t.Errorf("the mailed code %q differs from the echoed one %q", sent, resp.Msg.GetDevCode())
		}
	})

	t.Run("echoed when devAuth is true", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		service.devAuth = true

		resp, err := service.RequestEmailCode(context.Background(), connect.NewRequest(&api.RequestEmailCodeRequest{
			Email: "dev@example.com",
		}))
		if err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}
		if resp.Msg.GetDevCode() == "" {
			t.Fatal("dev_code was empty on a dev server")
		}

		// The echo must be the real code, or the harness it exists for is useless.
		if _, err := service.VerifyEmailCode(context.Background(), connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: "dev@example.com",
			Code:  resp.Msg.GetDevCode(),
		})); err != nil {
			t.Fatalf("the echoed dev_code did not verify: %v", err)
		}
	})
}

func TestVerifyEmailCode_WrongCodeIsRejectedAndCounted(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()
	const addr = "wrong@example.com"

	requestCode(t, ctx, service, addr)

	_, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  "000000",
	}))
	if err == nil {
		t.Fatal("VerifyEmailCode accepted a wrong code")
	}
	if got := connect.CodeOf(err); got != connect.CodeUnauthenticated {
		t.Errorf("error code = %v, want %v", got, connect.CodeUnauthenticated)
	}

	pending := onlyPendingCode(t, ctx, sqlStorage, addr)
	if pending.Attempts != 1 {
		t.Errorf("attempts = %d after one wrong guess, want 1", pending.Attempts)
	}
}

// Once the per-code budget is spent the code is dead, even if the guess that
// follows is the right one. Without this the TTL alone would allow far more
// guesses than a 6-digit space can absorb.
func TestVerifyEmailCode_AttemptCapKillsTheCode(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()
	const addr = "capped@example.com"

	code := requestCode(t, ctx, service, addr)

	for i := 0; i < emailCodeMaxAttempts; i++ {
		if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: addr,
			Code:  "000000",
		})); err == nil {
			t.Fatalf("wrong guess %d was accepted", i+1)
		}
	}

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	})); err == nil {
		t.Fatal("the correct code was still accepted after the attempt cap was reached")
	}
}

func TestVerifyEmailCode_ExpiredCodeIsRejected(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()
	const addr = "expired@example.com"

	code := requestCode(t, ctx, service, addr)

	pending := onlyPendingCode(t, ctx, sqlStorage, addr)
	pending.ExpiresAtUnixSec = time.Now().Add(-time.Minute).Unix()
	if err := sqlStorage.Update(ctx, pending); err != nil {
		t.Fatalf("failed to age the pending code: %v", err)
	}

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	})); err == nil {
		t.Fatal("an expired code was accepted")
	}
}

func TestVerifyEmailCode_IsSingleUse(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()
	const addr = "replay@example.com"

	code := requestCode(t, ctx, service, addr)

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	})); err != nil {
		t.Fatalf("first verification failed: %v", err)
	}

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  code,
	})); err == nil {
		t.Fatal("a consumed code was accepted a second time")
	}
}

// Re-requesting retires the previous code, so a person who requests twice and
// then types the code from the first email is correctly rejected rather than
// having two live codes outstanding.
func TestRequestEmailCode_RetiresThePriorCode(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()
	const addr = "reissue@example.com"

	first := requestCode(t, ctx, service, addr)
	second := requestCode(t, ctx, service, addr)

	if first == second {
		t.Fatal("re-request returned the same code; codes must be freshly generated")
	}

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  first,
	})); err == nil {
		t.Fatal("the superseded code was still accepted")
	}

	if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
		Email: addr,
		Code:  second,
	})); err != nil {
		t.Fatalf("the current code was rejected: %v", err)
	}
}

// This RPC is the entry point for both sign-up and sign-in, so its response
// must not reveal whether an address already has an account.
func TestRequestEmailCode_DoesNotDiscloseAccountExistence(t *testing.T) {
	service, sqlStorage, _, _ := setupTestService(t)
	ctx := context.Background()

	createTestUser(t, ctx, sqlStorage, "known@example.com", "Known Person")

	known, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: "known@example.com",
	}))
	if err != nil {
		t.Fatalf("RequestEmailCode failed for a known address: %v", err)
	}

	unknown, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: "stranger@example.com",
	}))
	if err != nil {
		t.Fatalf("RequestEmailCode failed for an unknown address: %v", err)
	}

	if (known.Msg.DevCode == nil) != (unknown.Msg.DevCode == nil) {
		t.Error("responses differ between a known and an unknown address")
	}
}

// A send failure must reach the caller. RequestPasswordReset deliberately
// swallows its send errors for enumeration parity, and copying that here would
// leave the person waiting at a code-entry screen for mail that never arrives.
func TestRequestEmailCode_SurfacesSendFailure(t *testing.T) {
	service, _, _, _ := setupTestService(t)
	ctx := context.Background()

	mockEmail(t, service).SendEmailCodeErr = errors.New("mailgun unavailable")

	_, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
		Email: "undeliverable@example.com",
	}))
	if err == nil {
		t.Fatal("a send failure was swallowed; the caller would wait for a code that never arrives")
	}
	if got := connect.CodeOf(err); got != connect.CodeInternal {
		t.Errorf("error code = %v, want %v", got, connect.CodeInternal)
	}
}

func TestEmailCodeTestAccounts(t *testing.T) {
	const (
		listedAddr = "review@example.com"
		fixedCode  = "424242"
	)

	newServiceWithTestAccount := func(t *testing.T) *Service {
		t.Helper()
		service, _, _, _ := setupTestService(t)
		service.emailCodeTestAccounts = map[string]string{listedAddr: fixedCode}
		return service
	}

	t.Run("fixed code verifies without mail or a stored code", func(t *testing.T) {
		service := newServiceWithTestAccount(t)
		ctx := context.Background()

		if _, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{
			Email: listedAddr,
		})); err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}
		if len(mockEmail(t, service).EmailCodes) != 0 {
			t.Error("a test account was mailed a code")
		}

		if _, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: listedAddr,
			Code:  fixedCode,
		})); err != nil {
			t.Fatalf("the configured fixed code was rejected: %v", err)
		}
	})

	t.Run("a wrong code is still rejected for a listed address", func(t *testing.T) {
		service := newServiceWithTestAccount(t)

		if _, err := service.VerifyEmailCode(context.Background(), connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: listedAddr,
			Code:  "999999",
		})); err == nil {
			t.Fatal("a wrong code was accepted for a test account")
		}
	})

	// The allowlist grants the bypass, not the code. A code that happens to
	// match a configured one proves nothing about an address that is not
	// listed — otherwise the fixed code would be a universal backdoor.
	t.Run("the fixed code does nothing for an unlisted address", func(t *testing.T) {
		service := newServiceWithTestAccount(t)

		if _, err := service.VerifyEmailCode(context.Background(), connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: "someone-else@example.com",
			Code:  fixedCode,
		})); err == nil {
			t.Fatal("the fixed code verified an address that is not on the allowlist")
		}
	})
}

func TestParseEmailOTPTestAccounts(t *testing.T) {
	t.Run("empty config disables the path", func(t *testing.T) {
		got, err := ParseEmailOTPTestAccounts("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})

	t.Run("parses and normalizes entries", func(t *testing.T) {
		got, err := ParseEmailOTPTestAccounts(" Review@Example.Com:123456 , qa@example.com:654321 ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{
			"review@example.com": "123456",
			"qa@example.com":     "654321",
		}
		if len(got) != len(want) {
			t.Fatalf("got %d entries, want %d", len(got), len(want))
		}
		for addr, code := range want {
			if got[addr] != code {
				t.Errorf("code for %s = %q, want %q", addr, got[addr], code)
			}
		}
	})

	// Every rejection here is a misconfiguration that would otherwise be
	// discovered as a failed app-store review.
	for name, raw := range map[string]string{
		"missing separator": "review@example.com",
		"empty address":     ":123456",
		"short code":        "review@example.com:123",
		"duplicate address": "review@example.com:123456,Review@example.com:654321",
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			if _, err := ParseEmailOTPTestAccounts(raw); err == nil {
				t.Fatalf("ParseEmailOTPTestAccounts(%q) succeeded, want an error", raw)
			}
		})
	}
}

// The client uses this to decide whether to ask for a display name. Getting it
// wrong showed a returning member "What should we call you?" on the login
// screen — a name they had already chosen, on a surface that cannot register.
func TestVerifyEmailCode_ReportsAccountExistence(t *testing.T) {
	t.Run("false for an address with no account", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		ctx := context.Background()
		const addr = "brand-new@example.com"

		code := requestCode(t, ctx, service, addr)
		resp, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: addr, Code: code,
		}))
		if err != nil {
			t.Fatalf("VerifyEmailCode failed: %v", err)
		}
		if resp.Msg.AccountExists {
			t.Error("reported an existing account for an address that has none")
		}
	})

	t.Run("true for an address that already has one", func(t *testing.T) {
		service, sqlStorage, _, _ := setupTestService(t)
		ctx := context.Background()
		const addr = "returning@example.com"

		createTestUser(t, ctx, sqlStorage, addr, "Returning Person")

		code := requestCode(t, ctx, service, addr)
		resp, err := service.VerifyEmailCode(ctx, connect.NewRequest(&api.VerifyEmailCodeRequest{
			Email: addr, Code: code,
		}))
		if err != nil {
			t.Fatalf("VerifyEmailCode failed: %v", err)
		}
		if !resp.Msg.AccountExists {
			t.Error("did not report the existing account; the client would ask a returning member for a name")
		}
	})

	// The disclosure is confined to the post-proof response. RequestEmailCode,
	// which anyone may call for any address, must still reveal nothing.
	t.Run("existence is not disclosed before ownership is proven", func(t *testing.T) {
		service, sqlStorage, _, _ := setupTestService(t)
		ctx := context.Background()

		createTestUser(t, ctx, sqlStorage, "known2@example.com", "Known")

		known, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{Email: "known2@example.com"}))
		if err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}
		unknown, err := service.RequestEmailCode(ctx, connect.NewRequest(&api.RequestEmailCodeRequest{Email: "stranger2@example.com"}))
		if err != nil {
			t.Fatalf("RequestEmailCode failed: %v", err)
		}
		if (known.Msg.DevCode == nil) != (unknown.Msg.DevCode == nil) {
			t.Error("RequestEmailCode responses differ between a known and unknown address")
		}
	})
}

func TestEmailRegister_WithProofToken(t *testing.T) {
	const addr = "verified-signup@example.com"

	t.Run("creates a passwordless, verified account", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter-otp@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}
		comm := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, comm.Id, inviter.Id)

		resp, err := service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:           addr,
			Name:            "Verified Person",
			EmailProofToken: proveEmail(t, ctx, service, addr),
			ShortCode:       invitation.ShortCode,
		}))
		if err != nil {
			t.Fatalf("EmailRegister failed: %v", err)
		}

		created, err := userManager.GetUserByEmail(ctx, addr)
		if err != nil || created == nil {
			t.Fatalf("failed to load the created account: %v", err)
		}
		if created.Id != resp.Msg.User.Id {
			t.Fatalf("stored account id %q does not match the response %q", created.Id, resp.Msg.User.Id)
		}
		if created.PasswordHash != "" {
			t.Error("a code-verified registration stored a password hash")
		}
		if created.EmailVerifiedAtUnixSec == nil {
			t.Error("a code-verified registration was not stamped as verified")
		}
	})

	// Proving one address must not let a caller register a different one.
	t.Run("rejects a proof token for another address", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		inviter, err := userManager.CreateUser(ctx, "inviter-otp2@example.com", "Inviter", models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("create inviter: %v", err)
		}
		comm := createTestCommunity(t, ctx, sqlStorage, inviter.Id)
		invitation := createTestInvitation(t, ctx, sqlStorage, comm.Id, inviter.Id)

		if _, err := service.EmailRegister(ctx, connect.NewRequest(&api.EmailRegisterRequest{
			Email:           "victim@example.com",
			Name:            "Impostor",
			EmailProofToken: proveEmail(t, ctx, service, "attacker@example.com"),
			ShortCode:       invitation.ShortCode,
		})); err == nil {
			t.Fatal("a proof token for one address registered a different address")
		}
	})

	t.Run("requires some credential", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)

		if _, err := service.EmailRegister(context.Background(), connect.NewRequest(&api.EmailRegisterRequest{
			Email: "nocredential@example.com",
			Name:  "Nobody",
		})); err == nil {
			t.Fatal("registration succeeded with neither a code nor a password")
		}
	})
}

func TestEmailLogin_WithProofToken(t *testing.T) {
	t.Run("signs in an email/password account without its password", func(t *testing.T) {
		service, sqlStorage, _, _ := setupTestService(t)
		ctx := context.Background()

		user := createTestUser(t, ctx, sqlStorage, "legacy@example.com", "Legacy Person")

		resp, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:           "legacy@example.com",
			EmailProofToken: proveEmail(t, ctx, service, "legacy@example.com"),
		}))
		if err != nil {
			t.Fatalf("EmailLogin failed: %v", err)
		}
		if resp.Msg.User.Id != user.Id {
			t.Errorf("signed in as %q, want %q", resp.Msg.User.Id, user.Id)
		}
	})

	// The settled decision on #2571: sign-in is gated on holding a verified
	// email, not on how the account was originally created. This is what
	// removes the "please login with Google" dead end.
	t.Run("signs in an OIDC account", func(t *testing.T) {
		service, sqlStorage, _, _ := setupTestService(t)
		ctx := context.Background()

		user := createTestUser(t, ctx, sqlStorage, "googler@example.com", "Googler")
		user.AuthMethod = models.AuthMethod_AUTH_METHOD_GOOGLE
		user.PasswordHash = ""
		if err := sqlStorage.Update(ctx, user); err != nil {
			t.Fatalf("failed to convert the test user to OIDC: %v", err)
		}

		if _, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:           "googler@example.com",
			EmailProofToken: proveEmail(t, ctx, service, "googler@example.com"),
		})); err != nil {
			t.Fatalf("an OIDC account could not sign in by code: %v", err)
		}
	})

	// Signing in by code proves the address, so an account that predates the
	// field acquires the stamp on the way through.
	t.Run("stamps the verified signal on a legacy account", func(t *testing.T) {
		service, sqlStorage, userManager, _ := setupTestService(t)
		ctx := context.Background()

		createTestUser(t, ctx, sqlStorage, "unstamped@example.com", "Unstamped")

		if _, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:           "unstamped@example.com",
			EmailProofToken: proveEmail(t, ctx, service, "unstamped@example.com"),
		})); err != nil {
			t.Fatalf("EmailLogin failed: %v", err)
		}

		after, err := userManager.GetUserByEmail(ctx, "unstamped@example.com")
		if err != nil || after == nil {
			t.Fatalf("failed to reload the account: %v", err)
		}
		if after.EmailVerifiedAtUnixSec == nil {
			t.Error("signing in by code did not stamp the account as verified")
		}
	})

	t.Run("still rejects an unknown address", func(t *testing.T) {
		service, _, _, _ := setupTestService(t)
		ctx := context.Background()

		if _, err := service.EmailLogin(ctx, connect.NewRequest(&api.EmailLoginRequest{
			Email:           "nobody@example.com",
			EmailProofToken: proveEmail(t, ctx, service, "nobody@example.com"),
		})); err == nil {
			t.Fatal("signed in an address with no account")
		}
	})
}

// onlyPendingCode returns the single unconsumed pending code for an address.
func onlyPendingCode(t *testing.T, ctx context.Context, sqlStorage *storage.ProtoSQLStorage, addr string) *models.PendingEmailCode {
	t.Helper()

	rows, err := sqlStorage.QueryByField(ctx, "email", addr, &models.PendingEmailCode{})
	if err != nil {
		t.Fatalf("failed to query pending email codes: %v", err)
	}

	var live []*models.PendingEmailCode
	for _, row := range rows {
		pending := row.(*models.PendingEmailCode)
		if pending.ConsumedAtUnixSec == nil {
			live = append(live, pending)
		}
	}
	if len(live) != 1 {
		t.Fatalf("got %d live pending codes for %s, want 1", len(live), addr)
	}
	return live[0]
}
