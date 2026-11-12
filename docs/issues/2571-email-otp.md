# Issue #2571: Email auth: passwordless email-OTP (or OTP + optional password) with a mockable verifier

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2571
**Filed:** 2026-06-27 | **Last activity:** 2026-06-27
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors — `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/client/architecture.md`, `docs/client/conventions.md`, `docs/proto_conventions.md`, `docs/testing/architecture.md`, `docs/server/observability.md`; leaves — `docs/registration_and_login.md`, `docs/single_player_mode.md`, `docs/invitations.md`, `docs/server/l10n.md`, `docs/client/testing.md`, `docs/client/deep_linking.md`, `docs/issues/2492-phone-first-pivot.md`, `docs/issues/2596-add-phone-to-account.md`, `docs/reviews/security_review_prompt.md`, `docs/reviews/api_design_review_prompt.md`

> **Framing from the requester (2026-08-06):** *"we want to migrate to full OTP (no
> password for email), but need to handle existing users with passwords."* That
> settles the issue's first design fork in favour of **full passwordless**, and makes
> the password-holder migration a first-class deliverable rather than a footnote.

## Status — Phases 1–3 implemented (branch `email_otp`, 2026-08-06)

| Phase | State |
|---|---|
| 1 · Verified-email substrate | ✅ implemented, tests green |
| 2 · OTP as a register/login credential | ✅ implemented, tests green |
| 3 · OTP default, password deprecated | ✅ implemented; **soak clock starts at the next shipped release** |
| 4 · Remove passwords | ⏸ **blocked on the soak** — see below |

**Phase 4 is deliberately not started.** It removes API fields and RPCs, which
`docs/proto_conventions.md` gates behind *one shipped release plus ~24 hours*,
counted from the release that carries the client change. That release has not
shipped, so doing the removals now would break every installed build. The soak is
the whole mechanism; short-circuiting it is the failure mode it exists to prevent.

**What to do when the soak closes:** work the Phase 4 checklist below. Two of its
steps do not have automation behind them and are easy to skip — the by-hand
no-caller proof for the three RPCs (`lint:proto:breaking` does not police RPC
deletion) and the `reserved` lines on every deleted field (the gate *does* reject
that one, so it will fail loudly rather than silently).

**Deviations from the plan as written**, all deliberate:

- **Phase 3's UI landed with Phase 2.** The plan sequenced "password behind a
  toggle" as a separate step, but registration had nowhere sensible to put a
  password once the code flow existed, so `EmailRegisterForm` was replaced
  outright rather than kept alongside. Login does keep the password form, behind
  an explicit "Use my password instead" — that is where the 22 existing holders
  actually need it.
- **`auth.HashOTPCode`/`VerifyOTPCode` added** rather than reusing
  `auth.HashPassword`, which enforces an 8-character minimum a 6-digit code cannot
  meet. Weakening that rule for passwords would have been the wrong fix, and the
  split means the code path survives Phase 4's deletion of `password.go` without
  inheriting a rule written for a credential that no longer exists.
- **A second test seam was needed.** The plan had only the `dev_code` echo; app
  store review runs against production, where the echo is off by construction, so
  the fixed-code allowlist was added alongside it. See the settled Open Question.
- **`login_method` now distinguishes `email_code` from `email_password`.** The
  plan assumed a dashboard could tell them apart; the log line hardcoded
  `"email_password"` for both, so watching password sign-ins fall to zero — the
  signal that gates Phase 4 — would have been impossible.

---

## Problem Statement

Every authentication path in Ripls proves something about the credential holder
except one.

| Path | Ownership proof | Promotion gate |
|---|---|---|
| `PhoneRegister` / `PhoneLogin` | Firebase phone OTP — server validates the ID token and enforces `sign_in_provider == "phone"` | `provisional.PromoteByPhone` is safe: only the verified owner claims a number's placeholders |
| `OIDCRegister` / `OIDCLogin` | Provider-signed ID token carrying a verified email | `promoteProvisionalByEmail` is the true analog |
| `EmailRegister` / `EmailLogin` | **none** — a bcrypt password over a self-asserted address, plus a forwardable 8-char invite code | `promoteProvisionalByEmail` runs anyway |

`server/services/login/registration.go:142` calls `promoteProvisionalByEmail` at the
end of `EmailRegister`. That helper claims *every* unclaimed email-keyed provisional
placeholder across *all* communities, joins the new account to each, and merges the
placeholder's activity history. Nothing in the path establishes that the registrant
owns the address. This is "email shadowing," accepted on #2492 because `/go` links
are not email-specific and ad-hoc communities are open-link-join, so a claimed
placeholder grants no data the link didn't already expose. It is an identity hole,
not a data hole — but it is still a hole, and there is currently **no `email_verified`
signal anywhere on `models.User`** to gate it on even if we wanted to.

Two structural consequences follow from that missing signal:

1. **The gate the issue asks for cannot be written today.** "Condition EMAIL-1b on a
   verified-email signal" requires the signal to exist first. It does not.
2. **Email is a dead end for account recovery.** `EmailLogin` hard-gates on
   `auth_method == AUTH_METHOD_EMAIL_PASSWORD` (`authentication.go:44`), so a Google
   user who loses their Google account gets "please login with Google" and nothing
   else. `PhoneLogin` already moved to credential-presence (#2596); email did not.

Adjacent defects surfaced while reading the code, both of which disappear with
passwords:

- `PendingPasswordReset.token` is stored **raw** (`password_reset.go:54`,
  `proto/ripls/models/user.proto:225`) — unlike `RefreshToken`, which stores only a
  SHA-256 hash. A database read discloses live reset tokens.
- `CheckResetPasswordToken` returns the account's email address to any caller holding
  a token (`password_reset.go:140`), rate-limited only per-IP at 30/min.

### What "handle existing users with passwords" actually costs — measured

Queried against production Metabase on **2026-08-06** (`analytics.ripls.org`,
database 2). Recorded here because the size of this population is the single fact
that decides between "careful dovetail" and "just migrate everyone":

| `auth_method` | Accounts | Hold a password | Hold a phone | Active in last 90d |
|---|---|---|---|---|
| `EMAIL_PASSWORD` (1) | **22** | 22 | 0 | 17 |
| `GOOGLE` (2) | 8 | 0 | 0 | 7 |
| `PHONE` (4) | 8 | 0 | 8 | 6 |
| **Total** | **38** | 22 | 8 | 30 |

`public.provisional_user` in production: **0 rows** (total, of any handle type).

Dev, for contrast: 1,952 `EMAIL_PASSWORD` + 4 `PHONE` — the email population there is
simulation-generated (`server/simulation/runner.go:240,377` registers personas via
`EmailRegister` with a literal password).

**Three things follow.**

- The password-holder migration is **22 accounts, 17 of them active**. Full
  passwordless is affordable outright. There is no population that justifies keeping
  a password credential permanently.
- The shadowing risk is **currently theoretical in production** — zero provisional
  rows means nothing is there to shadow yet. The phone-first pivot is what starts
  creating them, so the window to close this before it is load-bearing is now.
- The load-bearing consumer of email/password is not users, it is **test and
  simulation infrastructure**: `server/simulation/runner.go`,
  `server/integration_tests/test_helpers.go:195,239`, `server/e2e_test.go:348`,
  `e2e/lib/seed/users.ts`, `e2e/lib/ui/email-register.ts`. Deleting the password field
  without a replacement seed path breaks the entire integration suite. This is the
  real migration problem, and it is exactly what the issue's "mockable verifier"
  requirement is for.

---

## Context from Comments

**The issue has no comments.** Ordinarily that is a signal the issue needs more input
before planning, but not here: the body is unusually complete (it names the design
forks, the key files, and the accepted-risk rationale), and the surrounding decisions
live in `docs/issues/2492-phone-first-pivot.md` rather than in issue threads. The
substantive context, gathered from that plan and from the code:

- **EMAIL-1b shipped knowingly unverified** (#2572, merged). `2492-phone-first-pivot.md:393`
  records it as done; `registration.go:505-524` carries a long comment naming #2571 as
  the proper fix. The risk acceptance is deliberate and documented, not an oversight.
- **#2596 already set the precedent for this change.** It relaxed `PhoneLogin` from an
  `auth_method` gate to credential-presence and established the security invariant that
  makes it safe: *the only writers of the credential column are OTP-gated.*
  `docs/registration_and_login.md:566-572` explicitly names relaxing the email/OIDC
  gates the same way as "the symmetric future work." That is this issue.
- **#2602** tracks full account↔account merge and is out of scope here, the same way
  it was out of scope for `AddPhoneNumber`.
- **The requester's framing supersedes the issue's first fork.** The issue offered
  "full passwordless *or* OTP + optional password dovetail"; the instruction is full
  passwordless. The dovetail survives only as a time-boxed migration window, not as an
  end state.

**Skipping `ripls-ui-investigate`:** this is not a UI-observable misbehaviour report —
there is no wrong-thing-on-a-screen to reproduce. It is a missing server-side
capability plus a migration. The skill's bisect (client render / client state / server
/ data / UX gap) has nothing to bisect.

---

## Potential Solutions

### Option A: Own 6-digit email OTP over Mailgun, mirroring the phone flow

**Approach.** Build the verifier in-repo. Two new RPCs issue and check a code, and the
successful check mints a **short-lived signed email-proof token** that is then passed
to `EmailRegister` / `EmailLogin` in place of the password. This is a deliberate,
structural mirror of phone: today the client obtains a `firebase_id_token` and hands
that portable proof to whichever of `PhoneRegister`/`PhoneLogin` applies. Substituting
`email_proof_token` for `firebase_id_token` reuses the entire register-vs-login branch,
the invite-`short_code` plumbing, the `alreadyExists` → fall-back-to-login pattern in
`PhoneAuthScreen._handlePhoneRegister`, and the `AddPhoneNumber`-shaped RPC for
attaching an email to an existing account later.

- Code storage: new `PendingEmailCode` model, bcrypt-hashed code (reuses
  `server/auth/password.go`), 10-minute TTL, single-use, per-row attempt counter.
- Delivery: `email.Service.SendEmailCode`, Mailgun, `email_code_template_{en,es}.{html,txt}`
  following the existing password-reset template pattern, copy in
  `server/l10n/source/{en,es}.toml`. Honours the render boundary (server renders email).
- Mockability: `email.MockEmailService` captures the code for Go tests; a dev-mode-only
  echo field on the response covers e2e and the simulation harness.

**Pros.**
- **UX parity with phone.** Type a 6-digit code — identical to the phone screen, which
  matters because the phone-first pivot made that the primary registration surface and
  both options appear on the same guest screen (#2595).
- **No new vendor, no new dependency.** Mailgun, bcrypt, protosql, the l10n catalog,
  and `MockEmailService` all already exist. Aligns with the project's minimal-dependency
  rule.
- **Full control of the message** — branded shell, CAN-SPAM footer, `en`/`es` locale
  resolution, `logging.MaskEmail` discipline. All already built for password-reset mail.
- **The mock is deterministic and single-seam**, which is precisely what the issue asks
  for and what the integration/simulation suites need.
- Per-email rate limiting comes free: the interceptor keys off a `GetEmail()` getter
  (`ratelimit.go:80-83`), so naming the field `email` inherits the existing budget
  machinery.

**Cons.**
- We own the security-sensitive code path: enumeration parity, attempt caps, timing,
  expiry, single-use. Roughly 300 lines of server code that must be right.
- We own deliverability. An OTP that lands in 90 seconds is a broken flow, and dev+prod
  share one Mailgun quota (a known 429 risk).
- Two round-trips where there was one, so every email-register seed helper changes.

**Effort:** medium.

### Option B: Firebase Auth email-link (magic link), reusing the phone verifier seam

**Approach.** Use Firebase's passwordless email-link sign-in. The client requests a
link, the user clicks it, Firebase returns an ID token with
`sign_in_provider == "emailLink"`, and `server/auth/firebase.go` validates it with a
near-copy of `VerifyPhoneToken`.

**Pros.**
- Almost no new server code — one more verifier method on an existing seam.
- **The Auth Emulator already boots in the e2e harness** (`e2e/global-setup.ts:79`) and
  exposes `oobCode`s over REST, so the mockable-verifier requirement is satisfied by
  infrastructure that is already running.
- Firebase owns rate limiting, abuse prevention, and delivery.

**Cons.**
- **Click-not-type diverges from phone** — the issue itself flags this. Two adjacent
  buttons on one guest screen would behave differently.
- **Firebase Dynamic Links shut down in August 2025.** Email-link sign-in on mobile now
  requires self-hosting the continue-URL handoff and re-plumbing it into our existing
  App/Universal Link routing (`docs/client/deep_linking.md`). The "almost no new code"
  advantage largely evaporates here, and it lands in the most fragile part of the client.
- **Loses the email pipeline we already built.** Firebase sends from its own domain with
  its own templates unless custom SMTP is configured — which means a second sender, a
  second set of DNS/DKIM concerns against `docs/project_email_auth_dns.md`'s existing
  Mailgun setup, no `en`/`es` catalog, no CAN-SPAM shell, no branded template.
- A magic link in an inbox is a bearer credential with a longer, less controllable life
  than a 6-digit code the user retypes.

**Effort:** medium (small server, large client + infra).

### Option C: Keep passwords; bolt on a separate "confirm your email" step

**Approach.** Leave `EmailRegister`/`EmailLogin` untouched. Add a post-registration
confirmation email, stamp `email_verified_at_unix_sec` when it is clicked, and gate
`promoteProvisionalByEmail` on that stamp.

**Pros.** Smallest diff; nothing existing breaks; closes the literal shadowing gap.

**Cons.**
- **Does not deliver what was asked for.** Passwords stay forever.
- Verification becomes asynchronous and skippable, so promotion silently doesn't happen
  for anyone who ignores the email — a worse failure mode than today's (wrong person
  promoted → nobody promoted, with no signal).
- Leaves the `EmailLogin` `auth_method` dead-end and the raw-token password-reset
  surface in place.
- Two credentials on one address forever: more code, more support, more attack surface.

**Effort:** small.

---

## Recommendation

**Option A, staged so passwords are removed rather than removed-and-hoped.**

The deciding facts:

1. **Parity is the point.** The issue's own success criterion is "mirror the phone flow
   as closely as possible." Option A mirrors it exactly — same UX, same
   proof-token-then-register/login shape, same mock seam. Option B mirrors the *plumbing*
   while diverging on the thing users touch.
2. **Option B's cost moved.** It would have been the obvious choice while Firebase
   Dynamic Links existed. Post-shutdown, the mobile continue-URL handoff is ours to build
   anyway — in the deep-link router, the most breakage-prone client surface — and we'd
   still be maintaining two email senders.
3. **Option C is excluded by the instruction**, and its failure mode is worse than the
   status quo.
4. **22 accounts.** The dovetail the issue offered as a hedge isn't needed as an end
   state. Ship OTP alongside passwords for one release, then delete the password path
   outright.

**The load-bearing design decision** is that `VerifyEmailCode` returns a proof token
rather than returning session tokens directly. It costs one extra RPC and buys: the
register-vs-login branch stays where it already is; invite `short_code` / `name` /
`gear_id` plumbing is untouched; the proven `alreadyExists` → fall-back-to-login client
pattern is reused verbatim; and a future `AddEmailAddress` (the `AddPhoneNumber` inverse)
consumes the same proof with no new verification path. A `VerifyEmailCode`-returns-tokens
design would need a parallel registration path and a second place where invite codes are
redeemed.

**Second decision: drop the `auth_method` gate on `EmailLogin`**, matching what #2596 did
to `PhoneLogin`. Sign-in by email OTP becomes conditional on the account holding a
*verified* email, not on how it was originally created. This is what
`docs/registration_and_login.md:566-572` already names as the symmetric future work; it
falls out of this change for free, and it is what closes the Google-user recovery dead
end. Its one real implication — Google accounts become reachable by email OTP — is
called out as an open question below rather than buried.

**Third decision: no `password_hash` backfill-to-verified.** Existing password holders
are *not* stamped verified at migration time. Their address becomes verified the first
time they complete an OTP. Anything else would mint 22 verified-email signals from
credentials that never proved ownership — reintroducing the exact defect this issue
closes, at the moment of closing it.

**What this does not include** (deliberately, to keep the shippable increments honest):
`AddEmailAddress` for phone-first accounts, and account↔account merge (#2602). Both are
natural follow-ups; neither is needed to close #2571.

---

## Open Questions

- ~~**Should a Google-registered account be able to sign in by email OTP?**~~
  **SETTLED 2026-08-06 — yes, allowed.** Dropping the `auth_method` gate makes it
  possible, since OIDC emails are provider-verified; Google's own account recovery
  already flows through that same inbox, so the security floor is unchanged for the
  realistic case, and it removes a recovery dead end that today has no exit. Gated
  strictly on the verified-email stamp being present, so the capability is
  credential-derived and never inferred from `auth_method`.

- ~~**How do manually created test accounts sign in?**~~ **SETTLED 2026-08-06 — a
  fixed-code test-account allowlist, mirroring Firebase test phone numbers.** This is
  a *production* requirement, not a dev convenience: `docs/release/apple_checklist.md:175`
  asks for demo account credentials, and store review hits prod, where the `dev_code`
  echo is off by construction. Design:

  - Config `EMAIL_OTP_TEST_ACCOUNTS`, `address:code` pairs, empty by default, sourced
    from Secret Manager in prod (`docs/secrets.md`). Entries may share one code — that
    is fine and is the expected setup for a handful of QA accounts.
  - **The allowlist grants the bypass, not the code.** Both the address *and* its
    configured code must match. A single global magic code accepted for *any* address
    would be a total backdoor — it would let anyone verify an address they don't own,
    which is precisely the vulnerability this issue exists to close. The code alone is
    never sufficient.
  - `RequestEmailCode` for a listed address sends no mail and writes no row;
    `VerifyEmailCode` constant-time-compares against the configured code and then mints
    an ordinary proof token. Nothing downstream knows the difference.
  - Startup validation rejects malformed entries and codes shorter than 6 digits; the
    count is logged at startup, never the codes. Every use logs at `Info` with
    `test_account=true` and a masked address, so the path is visible in prod logs and
    alertable if it fires unexpectedly.

  **This does not replace the `dev_code` echo — the two cover different cases.** The
  allowlist is a small fixed set that works in prod (store review, manual QA). The echo
  is `devAuth`-gated and handles arbitrary ad-hoc addresses, which is what the
  simulation harness needs — it mints ~2,000 unique personas per run and cannot be
  allowlisted.

- **Code parameters: length, TTL, attempt cap.** — **Recommendation: 6 digits, 10-minute
  TTL, single-use, 5 attempts per row, then the row is consumed.** Matches the phone OTP
  the user just saw on the adjacent screen. bcrypt (cost 12) over a 6-digit space costs
  an offline attacker ~69 hours per code against a 10-minute window, so a DB disclosure
  does not yield live codes — this is why bcrypt over SHA-256 is worth the ~250 ms
  verify latency here, and that latency doubles as a natural throttle.

- **How does e2e / the simulation harness read the code?** — **Recommendation: a
  `dev_code` field on `RequestEmailCodeResponse`, populated only when `s.devAuth` is
  true.** One field, no new endpoint, structurally impossible to leak in production
  because production runs `DevAuth=false`. Mirrors how the Auth Emulator lets e2e read
  the phone OTP. Ship it with a regression test asserting the field is empty when
  `devAuth` is off — that test is the whole safety argument, so it is not optional.
  (Rejected alternative: a fixed `000000` for `@example.com` addresses — cheaper, but it
  is a production code path with a magic constant in it.)

- **Does `AUTH_METHOD_EMAIL_PASSWORD` get renamed or joined by `AUTH_METHOD_EMAIL_OTP`?**
  — **Recommendation: neither.** `auth_method` records the *original* method an account
  was created with, for analytics and display. Rewriting it falsifies history; adding a
  value creates a second thing every gate must check. Since the gates become
  credential-based, the enum stops being load-bearing for auth entirely. Leave it, and
  say so in the proto comment.

- **Do the 22 password holders get a heads-up?** — **Recommendation: yes, one plain
  notice when Phase 4 lands**, sent to the 17 active ones at minimum. It is 22 people;
  the cost is one template and the alternative is someone finding their password box
  gone with no explanation. Fold it into the Phase 4 checklist rather than building a
  campaign.

- ~~**Is Mailgun fast enough to be an interactive auth dependency?**~~
  **RESEARCHED 2026-08-06 — we cannot currently answer this, and now know why.**

  What exists: `server/email/mailgun.go` logs `duration_ms` on a successful send.
  That is the **Mailgun API accept** latency, not time-to-inbox. Production has
  **11 samples in 90 days** (85–358 ms, all `off_app_notification` and one
  `password_reset`); dev has none, because the off-app email channel is flag-gated
  off. So the only email-latency number we have is both tiny-sample and measuring
  the wrong thing.

  What is missing: **nothing in the codebase consumes Mailgun delivery events.**
  SMS has exactly this — #2569 built Twilio status callbacks — but email never got
  the counterpart. Filed as **#2862**.

  What was added instead (Phase 1's SLI, which the plan listed but the first
  implementation pass never actually built — now in
  `terraform/modules/monitoring/log_metrics.tf`):

  - `email_code_age_{env}` — issue→verify seconds. A **proxy**: it bundles delivery
    with however long the person took to read and type, and is only observable for
    codes that were used. Read it as a threshold, not a number — tens of seconds
    means delivery is not the bottleneck; minutes means it is.
  - `email_code_failures_{env}` by reason. The reasons are deliberately
    indistinguishable to the caller but separated here. A rise in
    `no_outstanding_code`/`expired` points at delivery; `wrong_code` alongside
    steady successes is ordinary typing; `attempts_exhausted` suggests guessing.
  - `email_signins_by_credential_{env}` — `email_code` vs `email_password`. This is
    the Phase 4 gate.

  Standing caveat: dev and prod share one Mailgun quota, so a dev simulation run
  can 429 production mail. **Before Phase 4**, read `email_code_age` — after the
  password path goes, the mailed code is the *only* way into an email account, so a
  slow send stops being an annoyance and becomes a lockout.

- **What happens to reset links already in flight when Phase 4 deletes `/reset-password`?**
  — **Recommendation: the one-release soak covers it** (reset tokens live 1 hour;
  `password_reset.go:56`). Delete the route in the same release that deletes the RPCs, not
  earlier.

---

## Implementation Plan

Four phases. Phases 1–2 are additive and independently shippable; 3 flips the default;
4 removes. The proto-deprecation soak (`docs/proto_conventions.md:347` — one shipped
release plus ~24 h) sits between 3 and 4.

### Phase 1: Verified-email substrate

**Goal:** a server-side verified-email signal and a working code verifier, with zero
behaviour change to any existing flow.

**Tasks:**
- [ ] `models.User.email_verified_at_unix_sec` — **`optional int64`**, next free field
      **23** (`off_app_email_opted_out` is 21, `preferred_language` 22, `13` is reserved).
      `optional` is required by `docs/proto_conventions.md` § *Field presence*: a
      timestamp that only exists after a state transition is that section's literal
      example, and it removes the "0 means unverified" / "0 means never set" ambiguity.
      Go reads become `user.GetEmailVerifiedAtUnixSec()` / nil checks, not `!= 0`.
- [ ] New `models.PendingEmailCode`: `id`, `email` (normalized), `code_hash` (bcrypt),
      `created_at_unix_sec`, `expires_at_unix_sec`, `attempts`, and **`optional`**
      `consumed_at_unix_sec`.
- [ ] `LoginService.RequestEmailCode` / `VerifyEmailCode` with dedicated request/response
      types (never shared — `docs/proto_conventions.md`). **Both** request types name the
      address field `email`, so the rate limiter's `GetEmail()` extraction
      (`ratelimit.go:80-83`) applies to each.
- [ ] **Proto comments carry no vendor or mechanism.** No "Mailgun", no "Firebase", no
      "bcrypt", no description of how the server produces the code
      (`docs/proto_conventions.md` § *Proto comments*). Those belong in the Go doc
      comments and `server/services/login/README.md`.
- [ ] Server code lands in a **new sibling file** `server/services/login/email_otp.go`,
      not appended to `registration.go` (673 lines, against the 1,000-line
      `npm run lint:go:size` gate) — per `docs/server/architecture.md` §3, `service.go`
      keeps the struct/constructor wiring and feature files are named after functionality.
- [ ] Issue path: invalidate prior unconsumed codes for the address, then insert + send.
      Verify path: constant-shape failures, attempt cap, single-use consume, then mint the
      proof token. Proof token = short-lived signed JWT, ~10 min, **audience-scoped to a
      distinct purpose so it can never be presented as an access token**, carrying the
      verified address; issued from `server/auth/tokens.go`.
- [ ] **Send failure must fail loudly, unlike `RequestPasswordReset`.** That handler
      deliberately swallows send errors (`password_reset.go:73-79`) so a failure can't be
      read as an enumeration signal. Copying it here would be a bug: an OTP is a blocking,
      interactive dependency, and a swallowed failure leaves the user staring at a code
      entry box for a code that will never arrive. A *transport* failure is not an
      existence oracle — the address is self-asserted and the response is already
      identical for known and unknown addresses — so return `connecterr.Internal(...)`
      on send failure while still returning success when the address simply has no
      account. Per `docs/server/conventions.md` § *Error Handling*, this is the fail-fast
      default; the enumeration-parity carve-out applies to *lookup* results only.
- [ ] `email.Service.SendEmailCode` + `MockEmailService` implementation +
      `email_code_template_{en,es}.{html,txt}` + `[email.email_code.*]` keys in
      **both** `server/l10n/source/en.toml` and `es.toml`, following the
      `email.password_reset` pattern (`en.toml:251`). Gated by `lint:go:l10n-parity` and
      `lint:go:no-inline-user-strings`; the server renders email copy per the
      `docs/server/l10n.md` render boundary — none of this copy goes in the client ARB.
- [ ] Stamp `email_verified_at_unix_sec` on `OIDCRegister` and `OIDCLogin` (the claim is
      provider-verified). No backfill for `EMAIL_PASSWORD` rows.
- [ ] Rate limits in `DefaultLoginRules()`: `RequestEmailCode` 5/min per IP + 3/hour per
      email (mirroring `RequestPasswordReset`); `VerifyEmailCode` 10/min per IP **plus a
      per-email budget** — the per-row attempt cap bounds guesses against one code, the
      email budget bounds re-request-and-reguess cycling.
- [ ] `dev_code` echo field on `RequestEmailCodeResponse`, gated on `s.devAuth`, **with
      the regression test asserting it is empty when `devAuth` is false.**
- [ ] Test-account allowlist (`EMAIL_OTP_TEST_ACCOUNTS`, `address:code` pairs) per the
      settled Open Question — address *and* code must both match; startup validation;
      `test_account=true` logging. Covers store review and manual QA in prod.
- [ ] **Reject proof tokens at the access-token boundary.** The proof token is signed
      with the same HS256 secret, so add a `purpose` claim and make `NewAuthFunc`
      (`server/auth/tokens.go:163`) reject any token carrying one. A proof token already
      fails today's `userID == ""` check, but that is incidental — an explicit guard plus
      a token-confusion test is what keeps it true.
- [ ] Logging discipline: `logging.MaskEmail` on every address, `logging.MaskToken` on the
      proof token, and **the code itself is never logged at any level** — including the
      dev-mode path, so a dev log is not a credential store.
- [ ] Log-based SLI for issue→verify latency + a `ripls_email_code_*{outcome}` counter,
      defined in `terraform/modules/monitoring` alongside the existing log-based metrics
      (`docs/server/observability.md`).
- [ ] Go unit tests (`storage.SetupTestStorage(t)`, testcontainers per
      `docs/server/conventions.md` § *Server Testing*): happy path, expiry, replay,
      attempt cap, enumeration parity (unknown address returns the same shape and does not
      disclose existence), send-failure surfaces an error, proof-token audience/expiry
      rejection, and the `devAuth=false` echo-suppression regression.

**Explicitly not in scope:** `email_verified_at_unix_sec` is **not** added to `api.User`.
`toAPIUser` (`helpers.go:128`) returns only `id`/`name`/`media_id`, no client surface needs
the flag, and adding it would pull in the PII-self-only rule that already governs `email`
and `phone_number` on `GetUser` for no benefit. Revisit only if a settings-screen
"verified" indicator is actually specified.

**Validates:** that a verified-email signal can exist and be produced deterministically
in tests — the precondition every later phase depends on. Also produces the first real
Mailgun latency data, which is the input to the open question about transport.

### Phase 2: Email OTP as a register/login credential

**Goal:** email OTP works end to end; passwords still work; the shadowing gate closes.

**Tasks:**
- [ ] `EmailRegisterRequest.email_proof_token` and `EmailLoginRequest.email_proof_token`
      (new fields; `password` untouched and still accepted). Server accepts either,
      rejects both-empty.
- [ ] `EmailRegister` via proof token: set `email_verified_at_unix_sec`, no
      `password_hash`. Registration by password continues to leave both unset.
- [ ] `EmailLogin`: drop the `auth_method == EMAIL_PASSWORD` gate for the proof-token
      path; accept any account holding a verified email. Password path keeps its existing
      gate unchanged. Mirror the `PhoneLogin` comment explaining why credential-presence
      is safe.
- [ ] **Gate `promoteProvisionalByEmail` on the verified-email stamp being present** —
      with a regression test. This is the issue's third bullet and the reason it exists.
      Keep the helper's existing best-effort/logged posture for genuine failures; the new
      gate is a precondition check that returns early, not an error swallow.
- [ ] Client: new `app/lib/presentation/screens/auth/email_auth_screen.dart` (address →
      code → name), extracting the shared OTP-entry widget out of `phone_auth_screen.dart`.
      **Do not grow the phone screen** — it is 998 lines against a 1,000-line CI gate
      (`npm run lint:dart:size`).
- [ ] Client: `email_register_form.dart` switches to the code flow; reuse the
      `alreadyExists` → `EmailLogin` fallback exactly as `PhoneAuthScreen._handlePhoneRegister`
      does, so a returning member typing their address is logged in rather than errored.
- [ ] Client conventions on the new screen: strings in `app_en.arb` + `app_es.arb` only
      (`lint:dart:i18n`, `check_l10n_parity.js`); **viewmodels never resolve strings** —
      state carries `UserError?` via `RpcErrorHandler.classify`, the widget calls
      `localize(...)` in `build()` (`docs/client/i18n.md`, `app/lib/core/errors/README.md`).
- [ ] Accessibility on the new screen (`docs/client/accessibility.md`, gated by
      `npm run check:a11y`): built from the accessibility primitives with l10n-sourced
      semantic labels; the code field is labelled and its validation error announced via
      `LiveRegion` rather than colour alone; ≥48 dp targets on resend/submit; focus moves
      to the code field when the address step completes.
- [ ] **Distinct `Key`s on the address and code fields.** They swap `keyboardType`
      (`emailAddress` → `number`) within one screen, which is the exact condition that
      needs a `ValueKey` to avoid the keyboard-type-stuck bug seen previously.
- [ ] **Migrate the test/seed fleet off passwords** onto the dev-code path:
      `server/integration_tests/test_helpers.go:195,239`, `server/e2e_test.go:348`,
      `server/simulation/runner.go:240,377`, `e2e/lib/seed/users.ts`,
      `e2e/lib/ui/email-register.ts`.
- [ ] Rewrite `e2e/tests/workflows/phone-guest-email-register.spec.ts` and
      `phone-gear-email-register.spec.ts`. Both carry comments asserting they need no
      Auth Emulator because `EmailRegister` is a single RPC — still true (no emulator),
      but it is now two RPCs, so update the comments, not just the code.
- [ ] `app/lib/services/auth_service.dart` + ViewModel tests per `docs/client/testing.md`;
      `semanticsIdentifier`s on the new screen per `docs/client/testing/semantics_identifiers.md`.

**Validates:** the full register→verify→promote loop is e2e-testable without sending
real mail — the issue's explicit second requirement — and email-provisional promotion
now requires proven ownership. **#2571 is functionally closed at the end of this phase;**
3 and 4 deliver the requester's "no password" instruction.

### Phase 3: OTP becomes the default; password becomes legacy

**Goal:** nobody chooses a password; the 22 holders discover OTP naturally.

**Tasks:**
- [ ] `RegisterScreen` and the phone-first guest screen offer only OTP for email.
      `EmailRegister` rejects `password` server-side (new accounts are passwordless).
- [ ] `LoginScreen` leads with OTP; the password field moves behind an explicit
      "Use my password instead" affordance.
- [ ] Mark `EmailRegisterRequest.password` and `EmailLoginRequest.password`
      `[deprecated = true]` with `TODO(#2571):` markers, and keep populating/accepting
      them. Same markers on `RequestPasswordReset` / `CheckResetPasswordToken` /
      `ResetPassword`. Each TODO body must stand alone — what the code does today, what
      would let it be removed, what blocked doing it now (CLAUDE.md § *TODO Comments*).
      **Start the soak clock on the release that ships this.**
- [ ] Read the Phase 1 latency SLI and record the number in this doc before proceeding.
- [ ] Dashboard panel: password vs. OTP sign-ins, to watch the 17 active holders convert.

**Validates:** that real users complete email OTP without support load, and that the
password path is genuinely idle before it is deleted.

### Phase 4: Remove passwords

**Goal:** one email credential.

**Tasks:**
- [ ] Wait one shipped release + ~24 h from Phase 3 (`docs/proto_conventions.md:347`).
      Magnitude does not matter — a fix release counts. Count from the release that
      shipped Phase 3, not from the deprecation marker; if that release is rolled back,
      the clock restarts. Confirm from the dashboard that password sign-ins are zero.
- [ ] Send the heads-up notice to remaining password holders.
- [ ] Delete `RequestPasswordReset` / `CheckResetPasswordToken` / `ResetPassword`.
      **`lint:proto:breaking` green is not evidence this is safe** — `RPC_NO_DELETE` is
      `FILE`/`PACKAGE`-only, so RPC deletion passes the gate silently
      (`docs/proto_conventions.md` § *Removing an RPC*). Prove no callers by hand:
      repository and service wrappers are not callers, so grep for a viewmodel or screen
      that actually invokes them. An old build hitting a removed RPC gets `UNIMPLEMENTED`
      — a broken screen, not a blank field. Reserve nothing; RPC names are not reused.
- [ ] Delete the `models.PendingPasswordReset` message (retires the raw-token storage and
      the email-disclosure surface noted in the Problem Statement). A whole top-level
      message needs no `reserved` line — only fields within a surviving message do.
- [ ] Delete `password` from `EmailRegisterRequest` (field 3) and `EmailLoginRequest`
      (field 2) and **`reserved 3;` / `reserved 2;` respectively**. Deleting a field
      without reserving its number is one of the things the `WIRE` gate rejects outright.
      **Check all four proto-reader surfaces** — Go, `flutter analyze`, the e2e
      protobuf-es types (`npm run lint:e2e:types`), and the web bundle — a Dart compile
      error is the real liveness gate.
- [ ] Delete `forgot_password_screen.dart`, `reset_password_screen.dart`, the
      `/reset-password` and `/forgot-password` routes, and the password-reset templates
      + `[email.password_reset.*]` keys from **both** locale files (parity gate).
- [ ] One-shot job to clear `password_hash` on all rows; **file the cleanup issue before
      the `TODO(#NNNN)` lands** (CLAUDE.md), and delete the job once prod-verified (the
      #2643 precedent). Then remove `password_hash` from `models.User` and **`reserved 10;`**.
      Storage protos have no version skew, so the field can go as soon as no code reads it
      — no second soak. Delete `server/auth/password.go` **unless** bcrypt is still backing
      `PendingEmailCode.code_hash`, in which case keep the package and narrow its doc
      comment.
- [ ] Update `server/services/login/README.md` (multi-file directories must document
      purpose/key types/when-to-add-here, per CLAUDE.md).
- [ ] Refresh `docs/registration_and_login.md` (the anchor — Auth Method Enforcement,
      Security Considerations, the "Email Verification" future-enhancement entry),
      `docs/single_player_mode.md` → *Claiming*, and the EMAIL-1b accepted-risk note in
      `docs/issues/2492-phone-first-pivot.md`. Advance the `freshness:` stamps.

**Validates:** the accepted risk recorded on #2492 is retired, and email auth holds the
same invariant phone does — *the only writers of the credential are ownership-gated.*

---

## Related Issues

- **#2492** — parent epic (phone-first pivot). This issue is the email arm of its
  identity model; EMAIL-1b's accepted risk is what #2571 retires.
- **#2596** — add a phone to an existing account. Established the credential-presence
  gate and the OTP-gated-writer invariant this plan copies for email. Its natural
  inverse, `AddEmailAddress`, is a follow-up to this work.
- **#2602** — full account↔account merge. Out of scope here, exactly as it was for
  `AddPhoneNumber`.
- **#2572** — shipped EMAIL-1b (the unverified cross-community email promotion this
  issue gates).
- **#2595** — inline email/phone auth on the phone-first guest screen. That screen hosts
  `EmailRegisterForm` below the fold, so Phase 2's client change lands there too.

---

## Audit Record

Audited 2026-08-06 against the documentation context map. Engaged dimensions:
`security`, `api_design`, `error_handling`, `observability`, `testing`,
`code_quality`, `accessibility`, `tech_debt`. Findings were applied in place above;
this section records what changed and why, so a reviewer can see the basis.

**Corrections applied:**

1. **`optional` presence, not zero-sentinel.** `email_verified_at_unix_sec` and
   `consumed_at_unix_sec` were specified as plain `int64` with `!= 0` checks.
   `docs/proto_conventions.md` § *Field presence* names "timestamps that only exist
   after a state transition" as the canonical case for `optional`. Corrected in Phase 1,
   Phase 2's promotion gate, and the Open Questions answer.
2. **Missing `reserved` on field deletion.** Phase 4 deleted
   `EmailRegisterRequest.password` (3), `EmailLoginRequest.password` (2), and
   `models.User.password_hash` (10) with no `reserved` lines. Deleting a field without
   reserving its number is one of the four things the `WIRE` gate rejects outright — the
   plan as written would have failed `lint:proto:breaking`.
3. **RPC removal has no gate.** Phase 4 deletes three RPCs. `RPC_NO_DELETE` is
   `FILE`/`PACKAGE`-only, so a green breaking-change check says nothing here. Added the
   manual no-caller proof and the `UNIMPLEMENTED`-not-blank failure-mode note.
4. **Send-failure handling was a latent bug.** The plan implied following
   `RequestPasswordReset`, which deliberately swallows Mailgun errors for enumeration
   parity. Copying that for an OTP would strand users at a code box forever. Separated
   the two concerns: lookup results stay indistinguishable, transport failures fail fast
   via `connecterr.Internal` — the `docs/server/conventions.md` default.
5. **Server file placement.** New code was unplaced; `registration.go` is 673 lines
   against a 1,000-line CI gate. Directed to a new `email_otp.go` sibling per
   `docs/server/architecture.md` §3.
6. **Proto comment hygiene.** Added the explicit vendor/mechanism ban — the plan's
   phrasing invited "verified via Firebase/Mailgun" comments, which
   `docs/proto_conventions.md` § *Proto comments* forbids in `.proto` files (while
   CLAUDE.md permits them in READMEs and Go doc comments).
7. **l10n gates and the render boundary.** Named `lint:go:l10n-parity`,
   `lint:go:no-inline-user-strings`, `lint:dart:i18n`, and `check_l10n_parity.js`, and
   stated that email copy goes to `server/l10n/source` while screen copy goes to the ARB
   files — the boundary this feature straddles.
8. **Accessibility was absent** for a brand-new auth screen. Added the primitives,
   l10n-sourced semantic labels, `LiveRegion` error announcement, 48 dp targets, focus
   movement, and the `npm run check:a11y` gate.
9. **Client error/string conventions.** Added the `UserError` +
   `RpcErrorHandler.classify`/`localize` shape and the viewmodels-never-resolve-strings
   rule.
10. **`ValueKey` on swapping keyboard types.** The screen swaps `emailAddress` → `number`
    within one flow, the known trigger for the stuck-keyboard-type bug.
11. **Logging discipline.** Added `MaskEmail`/`MaskToken` and an explicit never-log-the-code
    rule covering the dev echo path.
12. **`VerifyEmailCode` needed a per-email budget**, not IP-only: the per-row attempt cap
    bounds guesses against a single code but not request-and-reguess cycling.
13. **API surface decision made explicit.** The plan was silent on whether the verified
    stamp reaches `api.User`; recorded as deliberately out of scope with the reason
    (`toAPIUser` exposes no PII; the self-only rule would apply for no benefit). This is
    the #1142 read-path-coverage check from the API-design rubric.
14. **Test infrastructure named** (`storage.SetupTestStorage(t)`, testcontainers) and the
    `README.md` update requirement for `server/services/login/` added.

**No contradictions found** with existing documented decisions. The plan's central move —
relaxing `EmailLogin` from an `auth_method` gate to credential-presence — is the work
`docs/registration_and_login.md:566-572` already names as symmetric future work, and it
preserves the #2596 invariant (*the only writers of a credential column are
ownership-gated*) rather than weakening it.

**Docs consulted:** `docs/llms.txt`, `docs/context_map.md`; anchors —
`docs/proto_conventions.md`, `docs/server/architecture.md`, `docs/server/conventions.md`,
`docs/client/architecture.md`, `docs/client/conventions.md`, `docs/server/observability.md`,
`docs/testing/architecture.md`; review rubrics —
`docs/reviews/security_review_prompt.md`, `docs/reviews/api_design_review_prompt.md`,
`docs/reviews/accessibility_review_prompt.md`; leaves —
`docs/registration_and_login.md`, `docs/single_player_mode.md`, `docs/client/i18n.md`,
`docs/client/accessibility.md`, `docs/client/testing.md`,
`docs/client/testing/semantics_identifiers.md`, `docs/client/deep_linking.md`,
`docs/server/l10n.md`, `docs/invitations.md`, `docs/issues/2492-phone-first-pivot.md`;
plus `CLAUDE.md` (TODO, file-size, README, and internal-docs vendor rules).
