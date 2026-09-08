# services/login

The `login` service implements the LoginService RPC interface: user registration, email/password authentication, phone-number authentication, OIDC (third-party identity provider) login, JWT token refresh, and password reset flows. It also enforces invitation gating during registration.

## Key files

- `service.go` — service struct and constructor; holds auth config, OIDC manager, email service, and storage.
- `authentication.go` — `Login`, `OIDCLogin`: credential verification and JWT issuance.
- `registration.go` — `Register`: new user creation, invitation code validation, profile image handling.
- `phone_auth.go` — `VerifyPhoneToken`: phone-number based login.
- `refresh.go` — `RefreshToken`: issues new JWTs from a valid refresh token.
- `password_reset.go` — `RequestPasswordReset`, `ResetPassword`: secure reset token generation and email delivery.
- `email_otp.go` — `RequestEmailCode`, `VerifyEmailCode`: the mailed one-time code that proves someone can receive mail at an address, and the short-lived proof token it mints. Also holds `stampVerifiedEmailIfMissing` and the test-account allowlist parser.
- `helpers.go` — shared invitation and user-creation helpers.

## Email ownership proof (#2571)

Phone auth proves ownership via a Firebase OTP token; email/password proved
nothing, which is why `promoteProvisionalByEmail` could be triggered by someone
who did not own the address ("email shadowing"). `email_otp.go` closes that by
giving email the same shape phone has: obtain a portable proof first, then
present it to whichever of register/login applies.

- `RequestEmailCode` mails a 6-digit code (bcrypt-hashed at rest, 10-minute TTL,
  single-use, 5 attempts per issued code). Its response is identical for known
  and unknown addresses — it is the entry point for both sign-up and sign-in, so
  distinguishing them would make it an account-existence oracle.
- Unlike `RequestPasswordReset`, a **send failure is returned, not swallowed**.
  That handler hides send errors so they cannot leak account existence; here the
  code is a blocking interactive dependency and a transport failure says nothing
  about existence, so hiding it would only strand the user.
- `VerifyEmailCode` returns an `auth.GenerateEmailProofToken` JWT carrying a
  `purpose` claim and no `user_id`. It is not a session token: `NewAuthFunc`
  rejects any token carrying a `purpose`, and `ValidateEmailProofToken` rejects
  any token without one, so the two cannot be swapped in either direction.

Two separate escape hatches exist for testing, and they cover different cases:

| | `dev_code` echo | `EmailCodeTestAccounts` |
|---|---|---|
| Where it works | dev-mode servers only (`--dev-mode`) | any environment, including production |
| Which addresses | any, ad hoc | only addresses on the configured allowlist |
| For | integration tests, e2e, the simulation harness (which mints thousands of unique personas) | app-store review and manual QA |

The allowlist grants the bypass, **not** the code: an address must be listed
*and* its configured code must match. A fixed code accepted for any address
would be a universal backdoor — exactly the vulnerability this closes.

**The `dev_code` echo does not replace the send.** `RequestEmailCode` mails the
code first and echoes it second, so a dev server with a real mail service
configured — which the dev environment has, since `mailgun-api-key` is in its
Secret Manager set — still delivers a genuine email. That is what makes the
delivery path testable outside production: point at dev, use an address you can
actually read, and the message arrives.

Two consequences worth knowing when testing there:

- **Reaching the code step already proves the send succeeded.** A send failure
  fails the RPC rather than being swallowed (see above), so the client never
  advances to code entry on a broken mail path.
- **The app prefills the code from the echo**, so tapping straight through does
  not read the inbox. Clear the field and type what actually arrived when the
  message itself — template, subject, deliverability — is the thing under test.

Only the test-account allowlist skips sending, deliberately: those addresses are
placeholders nobody reads.

## Rate limiting

In production mode (`--dev-mode=false`), the LoginService handler is wrapped with a Connect interceptor that enforces per-(procedure, IP) and per-(procedure, email) token-bucket limits. The budget table lives in `ratelimit.DefaultLoginRules()` (`server/middleware/ratelimit/`). Dev mode bypasses rate limiting entirely so simulation runs and integration tests are not artificially throttled.

## When to add code here vs. elsewhere

Authentication and registration RPCs belong here. Auth primitives (token formats, hashing, OIDC verification) belong in `server/auth`. Email delivery belongs in `server/email`. Session or device management belongs in `server/services/device`.
