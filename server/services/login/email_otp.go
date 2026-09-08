package login

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"math/big"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/clock"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
)

const (
	// emailCodeLength is the number of digits in a mailed sign-in code. Six
	// matches the phone OTP the user may have just seen on the adjacent
	// screen; the guessing budget is bounded by emailCodeMaxAttempts and the
	// TTL, not by the code's length.
	emailCodeLength = 6

	// emailCodeTTL is how long a mailed code stays valid.
	emailCodeTTL = 10 * time.Minute

	// emailCodeMaxAttempts is how many wrong guesses a single issued code
	// tolerates before it is dead. The counter is per-code rather than
	// per-address so a legitimate retry after a fresh request is unaffected.
	emailCodeMaxAttempts = 5
)

// RequestEmailCode issues a one-time code to an email address and mails it.
//
// The response is deliberately identical whether or not an account exists for
// the address: this RPC is the entry point for both sign-up and sign-in, and
// distinguishing them here would turn it into an account-existence oracle.
// Which of EmailRegister/EmailLogin to call is decided after the caller has
// proven ownership, not before.
func (s *Service) RequestEmailCode(
	ctx context.Context,
	req *connect.Request[api.RequestEmailCodeRequest],
) (*connect.Response[api.RequestEmailCodeResponse], error) {
	email := auth.NormalizeEmail(req.Msg.Email)
	if email == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email is required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "RequestEmailCode",
		"user_email", logging.MaskEmail(email),
	)

	// Sign-in surfaces ask to be told up front when an address has no account,
	// rather than mailing a code that leads someone through code entry only to
	// be told there is nothing to sign in to.
	//
	// This makes the RPC disclose account existence to a caller who has proven
	// nothing, which the unflagged path deliberately does not. Accepted as a
	// trade: the flow it replaces was actively misleading, and the per-IP and
	// per-address budgets on this procedure bound how far probing scales.
	if req.Msg.RequireExistingAccount {
		existing, err := s.userManager.GetUserByEmail(ctx, email)
		if err != nil {
			return nil, connecterr.Internal(ctx, "RequestEmailCode", err, "detail", "failed to check existing account")
		}
		if existing == nil {
			logger.InfoContext(ctx, "no account for address on a sign-in surface; not sending a code")
			return nil, connect.NewError(connect.CodeNotFound,
				fmt.Errorf("no account found for that address"))
		}
	}

	// Test accounts short-circuit before anything is stored or sent: their
	// code is fixed in config, so there is no row to write and no mail to
	// deliver. Logged loudly so use of this path is visible in production.
	if _, ok := s.emailCodeTestAccounts[email]; ok {
		logger.InfoContext(ctx, "email code requested for test account",
			"test_account", true,
		)
		return connect.NewResponse(&api.RequestEmailCodeResponse{}), nil
	}

	code, err := generateEmailCode()
	if err != nil {
		return nil, connecterr.Internal(ctx, "RequestEmailCode", err, "detail", "failed to generate code")
	}

	codeHash, err := auth.HashOTPCode(code)
	if err != nil {
		return nil, connecterr.Internal(ctx, "RequestEmailCode", err, "detail", "failed to hash code")
	}

	// Retire any outstanding codes for this address first, so a re-request
	// invalidates its predecessor rather than leaving several live codes.
	s.consumeOutstandingEmailCodes(ctx, email)

	now := clock.UnixSec(ctx)
	pending := &models.PendingEmailCode{
		Id:               uuid.New().String(),
		Email:            email,
		CodeHash:         codeHash,
		CreatedAtUnixSec: now,
		ExpiresAtUnixSec: now + int64(emailCodeTTL.Seconds()),
	}
	if _, err := s.storage.Insert(ctx, pending); err != nil {
		return nil, connecterr.Internal(ctx, "RequestEmailCode", err, "detail", "failed to store code")
	}

	// A send failure is returned, not swallowed. RequestPasswordReset
	// deliberately swallows its send errors so a failure cannot be read as an
	// account-existence signal — but that reasoning does not transfer here.
	// This code is a blocking, interactive dependency, and a transport failure
	// says nothing about whether an account exists (the response is already
	// identical either way). Hiding it would strand the person at a code-entry
	// screen waiting for mail that is never coming.
	if err := s.emailService.SendEmailCode(ctx, email, code, emailCodeTTL, s.preferredLanguageFor(ctx, email)); err != nil {
		return nil, connecterr.Internal(ctx, "RequestEmailCode", err, "detail", "failed to send code")
	}

	logger.InfoContext(ctx, "email code issued")

	resp := &api.RequestEmailCodeResponse{}
	// Development mode only: hand the code back so automated tests and the
	// simulation harness can complete the flow without a mailbox. devAuth is
	// false in production, so this is unreachable there.
	if s.devAuth {
		resp.DevCode = &code
	}
	return connect.NewResponse(resp), nil
}

// VerifyEmailCode exchanges a mailed code for a short-lived proof that the
// caller can receive mail at the address. The proof is what EmailRegister and
// EmailLogin accept in place of a password — the same shape as the phone flow,
// where a verified provider token is obtained first and then presented to
// whichever of register/login applies.
func (s *Service) VerifyEmailCode(
	ctx context.Context,
	req *connect.Request[api.VerifyEmailCodeRequest],
) (*connect.Response[api.VerifyEmailCodeResponse], error) {
	email := auth.NormalizeEmail(req.Msg.Email)
	if email == "" || req.Msg.Code == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("email and code are required"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "VerifyEmailCode",
		"user_email", logging.MaskEmail(email),
	)

	// Test accounts compare against their configured code. Membership in the
	// allowlist is what grants the bypass — a matching code for an address
	// that is not listed proves nothing and falls through to the real path.
	if want, ok := s.emailCodeTestAccounts[email]; ok {
		if subtle.ConstantTimeCompare([]byte(want), []byte(req.Msg.Code)) != 1 {
			logger.WarnContext(ctx, "email code verification failed",
				"reason", "test_account_code_mismatch",
				"test_account", true,
				"remote_addr", logging.RemoteAddrFromContext(ctx),
			)
			return nil, errInvalidEmailCode()
		}
		logger.InfoContext(ctx, "email code verified for test account", "test_account", true)
		return s.mintEmailProof(ctx, email)
	}

	pending, err := s.findOutstandingEmailCode(ctx, email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "VerifyEmailCode", err, "detail", "failed to look up code")
	}
	if pending == nil {
		logger.WarnContext(ctx, "email code verification failed",
			"reason", "no_outstanding_code",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, errInvalidEmailCode()
	}

	now := clock.UnixSec(ctx)
	if now > pending.ExpiresAtUnixSec {
		logger.WarnContext(ctx, "email code verification failed",
			"reason", "expired",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, errInvalidEmailCode()
	}

	if pending.Attempts >= emailCodeMaxAttempts {
		logger.WarnContext(ctx, "email code verification failed",
			"reason", "attempts_exhausted",
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, errInvalidEmailCode()
	}

	if err := auth.VerifyOTPCode(req.Msg.Code, pending.CodeHash); err != nil {
		// Record the failed attempt so the per-code guessing budget actually
		// shrinks. A storage failure here would silently restore unlimited
		// guesses, so it is logged at Error rather than ignored.
		pending.Attempts++
		if updateErr := s.storage.Update(ctx, pending); updateErr != nil {
			logger.ErrorContext(ctx, "failed to record failed email code attempt",
				"pending_email_code_id", pending.Id,
				"error", updateErr,
			)
		}
		logger.WarnContext(ctx, "email code verification failed",
			"reason", "wrong_code",
			"attempts", pending.Attempts,
			"remote_addr", logging.RemoteAddrFromContext(ctx),
		)
		return nil, errInvalidEmailCode()
	}

	// Consume before minting: a code is single-use, and a replay must fail
	// even if proof minting below were to fail.
	pending.ConsumedAtUnixSec = &now
	if err := s.storage.Update(ctx, pending); err != nil {
		return nil, connecterr.Internal(ctx, "VerifyEmailCode", err, "detail", "failed to consume code")
	}

	// code_age_sec is the issue→verify interval: delivery time plus however long
	// the person took to read and type it. It is a proxy, not a delivery
	// measurement — Mailgun tells us only that it *accepted* the message, and
	// nothing in the system observes when it lands in an inbox (the email analog
	// of the Twilio status callbacks from #2569 does not exist). A p95 in the
	// tens of seconds means delivery is fine; a p95 in minutes means it is not,
	// and that is the signal worth watching before passwords are removed.
	logger.InfoContext(ctx, "email code verified",
		"code_age_sec", now-pending.CreatedAtUnixSec,
	)
	return s.mintEmailProof(ctx, email)
}

// mintEmailProof issues the ownership proof for a verified address, and reports
// whether an account already exists for it.
//
// Reporting existence is safe at this point and nowhere earlier: the caller has
// just proven they receive mail at the address. RequestEmailCode, which anyone
// can call for any address, still reveals nothing. Without this the client has
// to guess, and guessing wrong means asking a returning member for a display
// name they chose long ago.
func (s *Service) mintEmailProof(ctx context.Context, email string) (*connect.Response[api.VerifyEmailCodeResponse], error) {
	token, expiresAt, err := s.authTokenConfig.GenerateEmailProofToken(email)
	if err != nil {
		return nil, connecterr.Internal(ctx, "VerifyEmailCode", err, "detail", "failed to mint email proof")
	}

	// A lookup failure is reported as "no account" rather than failing the
	// request: the proof is already valid, and the client's register-then-fall-
	// back-to-login path still lands correctly if this is wrong.
	existing, err := s.userManager.GetUserByEmail(ctx, email)
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to check account existence after code verification",
			"operation", "VerifyEmailCode",
			"user_email", logging.MaskEmail(email),
			"error", err,
		)
	}

	return connect.NewResponse(&api.VerifyEmailCodeResponse{
		EmailProofToken:  token,
		ExpiresAtUnixSec: expiresAt.Unix(),
		AccountExists:    existing != nil,
	}), nil
}

// errInvalidEmailCode is the single failure returned for every rejected
// verification — wrong code, expired code, exhausted attempts, or no code at
// all. Collapsing them keeps the response from revealing whether an address
// has an outstanding code, which would otherwise leak who is mid-sign-in.
// The specific reason is recorded in the server log.
func errInvalidEmailCode() error {
	return connect.NewError(connect.CodeUnauthenticated,
		fmt.Errorf("that code isn't valid — request a new one"))
}

// findOutstandingEmailCode returns the newest unconsumed code for an address,
// or nil when there is none. Consumed rows are skipped rather than deleted so a
// replay is distinguishable from a never-issued code in the logs.
func (s *Service) findOutstandingEmailCode(ctx context.Context, email string) (*models.PendingEmailCode, error) {
	rows, err := s.storage.QueryByField(ctx, "email", email, &models.PendingEmailCode{})
	if err != nil {
		return nil, fmt.Errorf("failed to query pending email codes: %w", err)
	}

	var newest *models.PendingEmailCode
	for _, row := range rows {
		pending := row.(*models.PendingEmailCode)
		if pending.ConsumedAtUnixSec != nil {
			continue
		}
		if newest == nil || pending.CreatedAtUnixSec > newest.CreatedAtUnixSec {
			newest = pending
		}
	}
	return newest, nil
}

// consumeOutstandingEmailCodes retires every live code for an address so a
// freshly issued one is the only usable code. Best-effort: failing to retire a
// predecessor leaves a second live code rather than blocking a legitimate
// sign-in, so it is logged loudly and the request continues.
func (s *Service) consumeOutstandingEmailCodes(ctx context.Context, email string) {
	rows, err := s.storage.QueryByField(ctx, "email", email, &models.PendingEmailCode{})
	if err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to query prior email codes for retirement",
			"operation", "RequestEmailCode",
			"user_email", logging.MaskEmail(email),
			"error", err,
		)
		return
	}

	now := clock.UnixSec(ctx)
	for _, row := range rows {
		pending := row.(*models.PendingEmailCode)
		if pending.ConsumedAtUnixSec != nil {
			continue
		}
		pending.ConsumedAtUnixSec = &now
		if err := s.storage.Update(ctx, pending); err != nil {
			logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to retire prior email code",
				"operation", "RequestEmailCode",
				"pending_email_code_id", pending.Id,
				"error", err,
			)
		}
	}
}

// resolveEmailCredential decides which email credential a request presented and
// checks it, returning whether address ownership was proven.
//
// It is the single place the two credentials meet, so the precedence rule lives
// in one spot: a proof token wins outright, and a password is only consulted in
// its absence. During the migration window both are accepted; once passwords
// are removed this collapses to the proof branch.
//
// allowPassword says whether the password fallback is available at all.
// Sign-in passes true — the 22 accounts that still hold a password must keep
// working until #2864 retires them. Registration passes devAuth, so a new
// account can only ever be created by proving ownership of the address (#2864).
// That makes the set of password-holding accounts closed: it can shrink as
// people migrate, and nothing can add to it. The dev exception exists solely so
// the e2e suite can still build a pre-#2571 account shape to exercise the login
// screen's password fallback; it leaves with the password path.
//
// A proof token for a *different* address than the request names is rejected.
// Without that check, proving ownership of one address would let a caller
// register or sign in as another.
func (s *Service) resolveEmailCredential(ctx context.Context, email, proofToken, password string, allowPassword bool) (emailVerified bool, err error) {
	if proofToken != "" {
		proven, err := s.authTokenConfig.ValidateEmailProofToken(proofToken)
		if err != nil {
			logging.LoggerWithContext(ctx).WarnContext(ctx, "authentication_failed",
				"reason", "email_proof_invalid",
				"user_email", logging.MaskEmail(email),
				"remote_addr", logging.RemoteAddrFromContext(ctx),
			)
			return false, connect.NewError(connect.CodeUnauthenticated,
				fmt.Errorf("email verification failed — request a new code"))
		}
		if auth.NormalizeEmail(proven) != email {
			logging.LoggerWithContext(ctx).WarnContext(ctx, "authentication_failed",
				"reason", "email_proof_address_mismatch",
				"user_email", logging.MaskEmail(email),
				"remote_addr", logging.RemoteAddrFromContext(ctx),
			)
			return false, connect.NewError(connect.CodeUnauthenticated,
				fmt.Errorf("email verification failed — request a new code"))
		}
		return true, nil
	}

	if !allowPassword || password == "" {
		return false, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("a verification code is required"))
	}
	return false, nil
}

// stampVerifiedEmailIfMissing records the verified-email signal on an account
// that does not already carry one, when the caller has just established
// ownership. It is a no-op when ownership was not proven or the account is
// already stamped, so it is safe to call on every sign-in.
//
// Best-effort by design: this runs after authentication has already succeeded,
// so failing the request over a bookkeeping write would lock a user out for no
// security gain. The failure is logged loudly and the next sign-in retries.
func (s *Service) stampVerifiedEmailIfMissing(ctx context.Context, user *models.User, verified bool) {
	if !verified || user.EmailVerifiedAtUnixSec != nil {
		return
	}

	now := clock.UnixSec(ctx)
	user.EmailVerifiedAtUnixSec = &now
	if err := s.userManager.UpdateUser(ctx, user); err != nil {
		logging.LoggerWithContext(ctx).ErrorContext(ctx, "failed to stamp verified email",
			"operation", "stampVerifiedEmailIfMissing",
			"user_id", user.Id,
			"user_email", logging.MaskEmail(user.Email),
			"error", err,
		)
	}
}

// preferredLanguageFor returns the stored language preference of the account
// behind an address, or "" when there is no account — in which case the email
// service falls back to the request's Accept-Language. A lookup failure is not
// fatal: it only costs the recipient a localized subject line.
func (s *Service) preferredLanguageFor(ctx context.Context, email string) string {
	user, err := s.userManager.GetUserByEmail(ctx, email)
	if err != nil || user == nil {
		return ""
	}
	return user.GetPreferredLanguage()
}

// generateEmailCode returns a uniformly random numeric code. crypto/rand with
// a rejection-free big.Int bound avoids the modulo bias that a naive
// rand%1000000 would introduce.
func generateEmailCode() (string, error) {
	upperBound := big.NewInt(1)
	for i := 0; i < emailCodeLength; i++ {
		upperBound.Mul(upperBound, big.NewInt(10))
	}

	n, err := rand.Int(rand.Reader, upperBound)
	if err != nil {
		return "", fmt.Errorf("failed to generate email code: %w", err)
	}
	return fmt.Sprintf("%0*d", emailCodeLength, n), nil
}

// ParseEmailOTPTestAccounts turns the configured "address:code,address:code"
// string into a lookup map. Entries are normalized the same way registration
// normalizes an address, so a differently-cased config entry still matches.
//
// It is strict on purpose: a malformed entry, a duplicate address, or a code
// shorter than a real one is a configuration mistake that would otherwise be
// discovered as a failed app-store review. Callers should treat the error as
// fatal at startup.
func ParseEmailOTPTestAccounts(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	accounts := make(map[string]string)
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		addr, code, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("email OTP test account entry must be address:code")
		}

		normalized := auth.NormalizeEmail(strings.TrimSpace(addr))
		code = strings.TrimSpace(code)
		if normalized == "" {
			return nil, fmt.Errorf("email OTP test account entry has an empty address")
		}
		if len(code) < emailCodeLength {
			return nil, fmt.Errorf("email OTP test account code must be at least %d characters", emailCodeLength)
		}
		if _, dup := accounts[normalized]; dup {
			return nil, fmt.Errorf("email OTP test account listed more than once")
		}
		accounts[normalized] = code
	}
	return accounts, nil
}
