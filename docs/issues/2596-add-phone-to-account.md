# Issue #2596: Add a verified phone number to an existing email/OIDC account (#2492)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2596
**Filed:** 2026-06-30 | **Last activity:** 2026-06-30
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/registration_and_login.md`, `docs/single_player_mode.md`, `docs/issues/2492-phone-first-pivot.md` (epic), `docs/proto_conventions.md` (anchor), `docs/server/architecture.md` + `docs/server/conventions.md` (anchors), `docs/sms_notifications.md`, `docs/ad_hoc_communities.md`, `docs/client/architecture.md` + `docs/client/conventions.md` (anchors); review prompts: `api_design`, `security`, `error_handling`, `testing`

## Problem Statement

Part of epic #2492 (phone-first pivot). Today a Ripls account is established by exactly **one** method — phone (`PhoneRegister`), email (`EmailRegister`), or OIDC/Google (`OIDCRegister`) — and the single `User.auth_method` enum is treated as an exclusive lock: each login RPC rejects unless `auth_method` equals its own method. There is no way for an already-authenticated email/OIDC user to attach a phone number.

This matters for two pivot-shaped reasons:

1. **SMS reachability.** The off-app notification dispatcher falls back to phone→SMS only when `User.phone_number` is set. An email/OIDC user can never receive the SMS notifications the pivot is built around.
2. **Identity reconciliation.** When someone is invited **by phone number**, ID-1 seeds a phone-keyed `ProvisionalUser`. The promote-on-verify path (ID-2, `promoteProvisionalByPhone`) only runs at `PhoneRegister`. So an email/OIDC user who is also invited by phone ends up split across their real account **and** an orphan phone-keyed provisional that never reconciles.

This feature is the **inverse** of ID-2 (`promoteProvisionalByPhone`) and EMAIL-1b (`promoteProvisionalByEmail`): instead of *phone-first → claim by phone*, it is *email/OIDC-first → verify-and-attach a phone, then claim the phone-keyed provisional*.

### Confirmed decisions (maintainer, this planning session)

- **Niche, low-prominence upgrade path.** The pivot expects most users to sign up by phone; this is purely an upgrade for older email/OIDC accounts. Entry point lives **tucked away in the profile-edit screen**, not a prominent surface.
- **Auth methods are ADDITIVE, not exclusive.** Adding a phone makes phone OTP an *additional* active login method — it does **not** replace email/OIDC. After attaching, the user can sign in by their original method **or** by phone OTP (and gets phone as account recovery). `auth_method` is re-interpreted as the *primary / original / registration* method (kept for analytics, logging, and display), no longer an exclusive gate. This supersedes an earlier "switch `auth_method` to PHONE" idea raised mid-session.

### Why additive is cheap (the key codebase fact)

Mutual exclusivity is a **login-time policy, not a storage constraint**. `User` stores credentials as independent columns that can all coexist on one row: `password_hash` (field 10), `oidc_provider_subject` (field 12), `phone_number` (field 18). The "one method" rule is enforced by just **4 enum-equality gates** against `auth_method`:

| Gate | Location | Today |
| --- | --- | --- |
| `EmailLogin` | `authentication.go:44` | reject unless `auth_method == EMAIL_PASSWORD` |
| `OIDCLogin` | `authentication.go:144` | reject unless `auth_method == <provider>` |
| `PhoneLogin` | `phone_auth.go:268` | reject unless `auth_method == PHONE` |
| `RequestPasswordReset` | `password_reset.go:41` | reject unless `auth_method == EMAIL_PASSWORD` |

Every *other* read of `auth_method` is slog logging or human-readable error text (`getAuthMethodName`); the duplicate-email and phone-uniqueness checks are independent of it. The client doesn't model `auth_method` or `phone_number` **at all** today (confirmed: no such field on any current-user proto).

**Scope insight:** because this issue only lets a user add a *phone* (not add email/OIDC to a phone account), the only gate that must change is **`PhoneLogin`**. An email- or Google-registered user who attaches a phone keeps their original `auth_method`, so `EmailLogin`/`OIDCLogin` keep working unchanged; we only need phone login to start accepting them. And since `PhoneLogin` already looks the account up *by its verified phone number* (`GetUserByPhone`), finding a row **is** proof the account holds the phone credential — so the fix is simply to **delete the `auth_method != PHONE` rejection block**. The symmetric generalization (credential-presence for all four gates, enabling future *add-email* / *add-OIDC* to a phone account) falls out naturally later and is explicitly out of scope here.

**Security precondition (load-bearing — `security` review, §1/§4).** Relaxing `PhoneLogin` to "found-by-phone ⇒ may log in" is safe **only because every writer of `User.phone_number` is gated by a verified Firebase OTP**: today that is `PhoneRegister` alone; this feature adds `AddPhoneNumber` (also OTP-gated). `SaveUser` does **not** accept a phone number, and `MergeIntoUser` never writes `User.phone_number` (it migrates RSVPs/transfers only). The plan must preserve this invariant — **no unverified path may set `User.phone_number`** — or the relaxed gate becomes an account-takeover vector. Phase 1 includes an explicit test asserting `SaveUser` cannot set a phone, so a future regression that adds an unverified writer fails CI.

## Context from Comments

The issue has **no comments** — the body is a single, unusually complete maintainer-authored spec. The two design forks it raised were resolved by direct maintainer guidance in this session:

- **Where the RPC lives / how phone interacts with login** → additive (above).
- **Entry point** → profile-edit screen, deliberately low-prominence (this is an upgrade path, not a headline feature).

The epic plan (`docs/issues/2492-phone-first-pivot.md:124`) already lists #2596 as an open follow-up: *"add a verified phone number to an existing email/OIDC account — the inverse of ID-2/EMAIL-1b promote-on-verify; reconciles a phone-keyed provisional into the account."*

## What exists today (codebase research)

- **Storage slots already present.** `models.User.phone_number` (field 18, optional E.164) and `auth_method` (field 9; `AUTH_METHOD_PHONE = 4`). For an email/OIDC account `phone_number` is simply unset.
- **Phone verification is server-validated Firebase OTP.** The client runs the Firebase OTP flow and obtains a Firebase phone ID token; `server/auth/firebase.go` (`PhoneTokenVerifier.VerifyPhoneToken`) validates it, enforces `sign_in_provider == "phone"`, and returns the verified E.164 number. `login.Service` holds this via `s.phoneAuth` (nil when unconfigured — guarded by `isPhoneAuthConfigured`, the typed-nil-interface check).
- **Promote-on-verify already exists** as `login.Service.promoteProvisionalByPhone` (`server/services/login/phone_auth.go:169`): normalize phone → `provisional.FindUnclaimedByPhone` (cross-community) → for each, `auth.IsMemberOfCommunity` / `addUserToCommunity` + record `INVITATION_LINK_USED` event / `provisional.MergeIntoUser`. Best-effort, idempotent. `MergeIntoUser` (`server/provisional/merge.go`) migrates RSVPs + transfers and marks the provisional claimed.
- **Phone uniqueness is app-enforced, not a DB constraint.** `UserManager.GetUserByPhone` (`server/auth/users.go:119`) queries `phone_number` and errors on >1 match. The only DB unique index on a phone column is on `sms_opt_out`. So the attach RPC must check `GetUserByPhone` before writing (same TOCTOU posture as `PhoneRegister`). `ProvisionalUser` rows are **not** in the `User` table, so this check only ever returns real accounts — provisional collisions are handled by the merge, not this check.
- **Auth-context wiring is the load-bearing constraint.** `LoginService` is mounted **without** the auth middleware (`server/main.go:1813`, intentionally public for register/login). Every account-mutating service (`UserService` at main.go:1771) is mounted with `authMiddleware.Wrap(...)`. An RPC that needs `auth.RequireAuth(ctx)` to identify the current user **cannot** live on `LoginService` — it would never receive auth info.
- **RefreshToken regenerates claims from the live user row** (`server/services/login/refresh.go:78`, `GenerateToken(user.Id, user.Email, user.Role, user.GetPhoneNumber())`). Once `phone_number` is set, the next refresh mints a token carrying the phone claim — no token re-issue from the attach RPC is required, and since `auth_method` is untouched there is **zero session disruption**.
- **Client reuse points (from client research).**
  - `app/lib/presentation/screens/auth/phone_auth_screen.dart` has `enum PhoneAuthMode { register, login }`. The Firebase half (`_handleSendCode` → `_handleVerifyCode` → `_signInWithCredential`) is **mode-agnostic**; the mode branch is at the post-`getIdToken()` point (login → `_completePhoneLogin`; register → name step → `phoneRegister`). A third `attach` mode slots in there: skip the name step, call the new authenticated RPC, then `Navigator.pop` back to settings instead of `setAuthState` + navigate-home.
  - The **authenticated** client path is `app/lib/services/user_service.dart` → `UserServiceClient` (`_buildHeaders()` already attaches the access token + `onUnauthenticated` logout hook). `userServiceProvider` injects `getAccessToken` already. The new wrapper mirrors `saveUser`. (The existing `phoneRegister`/`phoneLogin` live on `auth_service.dart` → public `LoginService` with a deliberately *tokenless* header — the wrong home for an authenticated attach.)
  - Self-account hub: `app/lib/presentation/screens/profile/profile_settings_screen.dart`; "Update Profile" → `app/lib/presentation/screens/profile/profile_edit_screen.dart` (the intended tucked-away entry). Settings rows use `buildSettingsActionItem` (`app/lib/presentation/widgets/settings/settings_widgets.dart`).
  - **Client has no notion of `auth_method`/`phone_number` today.** `AuthStateData.user` is the thin display `User` proto (id/name/mediaId/…); even self-fetch `GetUserResponse` exposes `email` but not phone or auth_method. To show current phone state (or gate the tile), the server must surface `phone_number` on `GetUserResponse` **self-fetch only** (the same caller==target exception already used for `email`/locations, #2138).

## Potential Solutions

The auth-model fork is decided (additive). The remaining design choices are (a) where the new RPC lives and (b) how the shared promote-by-phone logic is reused.

### Option A: `AddPhoneNumber` on `UserService`, promote logic extracted to `server/provisional` (recommended)

- **Approach:**
  - New authenticated RPC `UserService.AddPhoneNumber(AddPhoneNumberRequest{ firebase_id_token }) → AddPhoneNumberResponse{ phone_number }`. Sets `user.phone_number` (leaves `auth_method` untouched), runs promotion.
  - Relax `PhoneLogin` by deleting the `auth_method != PHONE` block (found-by-phone ⇒ holds the credential).
  - Inject `auth.PhoneTokenVerifier` (and the `cebus.Publisher` for the join event) into `user.Service`.
  - Extract the promote orchestration out of `login.Service.promoteProvisionalByPhone` into reusable `provisional.PromoteByPhone(ctx, store, publisher, userID, phone)`; `PhoneRegister` and `AddPhoneNumber` both call it — one implementation, no drift.
- **Pros:** Correct auth wiring (UserService is auth-wrapped). Honors service independence — both services depend on the shared `provisional` package, not on each other. DRY: the issue explicitly wants the existing merge primitives reused; this guarantees identical cross-community claim behavior on both paths.
- **Cons:** Touches `user.Service`'s constructor (new deps) and refactors a working `login` helper; the extraction must thread the community-event publisher cleanly.
- **Effort:** Medium.

### Option B: `AddPhoneNumber` on `UserService`, promote logic duplicated

- **Approach:** Same RPC home, but copy the ~40-line promote helper into `user.Service`.
- **Pros:** No change to `login.Service`; smaller blast radius now.
- **Cons:** Two copies of a subtle idempotent best-effort cross-community claim loop that must stay byte-for-byte equivalent forever — violates DRY and the issue's "reuse the merge primitives" intent.
- **Effort:** Small now, larger later.

### Option C: Keep it on `LoginService` with a hybrid auth-optional mount

- **Approach:** Make one `LoginService` method auth-aware while the rest stay public.
- **Cons:** `authn.NewMiddleware` wraps a whole handler; per-procedure auth on the public service means splitting it or hand-rolling auth — fragile, and a security red flag for the public surface. Rejected.

## Recommendation

**Option A.** Host `AddPhoneNumber` on `UserService` (the only correct home given the auth-middleware wiring); set `phone_number`, leave `auth_method`; relax only the `PhoneLogin` gate; extract the promote-by-phone orchestration into `server/provisional` so registration and attach share one cross-community claim implementation.

Retain `email`, `password_hash`, and `oidc_provider_subject` untouched (additive model — every original method keeps working). Do **not** issue tokens from the RPC — rely on the existing refresh path to pick up the phone claim; the client may trigger one proactive refresh so the claim appears immediately. Surface `phone_number` on `GetUserResponse` (self-fetch only) so the edit screen can render current state.

This is the smallest design that is *correct* (auth context), *DRY* (no duplicated claim loop), *independent* (no service→service dependency), and *additive* (no lost sign-in method, phone-as-recovery).

## Open Questions

- **`AddPhoneNumberResponse` shape, given phone is normally PII-off-the-wire.** — Recommendation: `AddPhoneNumberResponse{ string phone_number = 1 }` echoing the just-verified E.164 **to its own authenticated owner only** (the self-fetch exception already used by `GetUser`). The client needs it to render "Phone added: +1 555…". No other user can call this for someone else.
- **Idempotent re-attach of the same phone the caller already has.** — Recommendation: if `GetUserByPhone(phone)` returns the caller's own user, return success (no-op) rather than erroring.
- **Phone already belongs to a *different full account*.** — Recommendation: reject with `CodeAlreadyExists`; full account↔account merge is out of scope. File a follow-up to track it (some users will hit it).
- **Caller already has a *different* phone (change-phone).** — Recommendation: allow it (same verify + conflict checks) — falls out of the same code — but keep it out of v1 UI copy; note in the doc.
- **Do we relax the Email/OIDC/password-reset gates too, for symmetry?** — Recommendation: **no, not in this issue.** Only `PhoneLogin` needs it (an attaching user's original `auth_method` is unchanged, so their original login keeps working). The full credential-presence generalization is the right move when *add-email* / *add-OIDC-to-phone-account* lands; do it then to avoid speculative changes to login enforcement now.
- **Member-joined notification on promotion.** — Recommendation: mirror `promoteProvisionalByPhone` exactly (record `INVITATION_LINK_USED`) so the attach path is indistinguishable from a phone registration to the communities involved. (This is why the extracted helper needs the publisher.)
- **Rate limiting / abuse.** — Recommendation: add a per-user `ratelimit` rule for `AddPhoneNumber`; Firebase already gates OTP cost and proves ownership, but a server budget keeps the verify path from being hammered. Mask the phone in all logs (`logging.MaskPhone`). Note the additive model widens the login surface (each active method is another way in — standard, and strictly more recoverable); the SIM-swap-into-an-email-account risk is accepted as industry-normal.

## Implementation Plan

### Phase 1: Server — attach RPC, additive login, shared promotion, tests
**Goal:** A fully-tested `UserService.AddPhoneNumber` that verifies a Firebase phone token, attaches the phone additively, and reconciles phone-keyed provisionals — validated entirely by Go tests (per `feedback_server_tests_run_locally`).
**Tasks:**
- [ ] Proto (`proto/ripls/api/user_service.proto`): add `AddPhoneNumberRequest { string firebase_id_token = 1; }`, `AddPhoneNumberResponse { string phone_number = 1; }`, and `rpc AddPhoneNumber(...)`. Add `phone_number` to `GetUserResponse` as **field 11** (next free number; declare it in source next to `email` per `feedback_proto_field_order`, even though the wire number appends). Dedicated request/response types, inline fields, no cross-RPC reuse. Write proto comments on the new RPC/messages/field matching the existing phone-auth style (the established convention already names `firebase_id_token` and references the OTP flow in `login_service.proto`). `npm run generate`.
- [ ] Extract `provisional.PromoteByPhone(ctx, store, publisher, userID, phone)` from `login.Service.promoteProvisionalByPhone`; rewire `PhoneRegister` to call it. Keep behavior byte-identical (cross-community find → join + `INVITATION_LINK_USED` event → merge; best-effort, idempotent, loud error logs).
- [ ] Wire `auth.PhoneTokenVerifier` + `cebus.Publisher` into `user.Service` (constructor + `main.go`). Guard with `isPhoneAuthConfigured`; return `CodeUnimplemented` when phone auth is unconfigured (mirror `PhoneRegister`).
- [ ] Implement `AddPhoneNumber`: `RequireAuth` (caller-only — the request carries **no** `user_id`, so there is no IDOR surface; the phone always attaches to the authenticated caller) → validate `firebase_id_token` non-empty (`CodeInvalidArgument`) → verify token (`CodeUnauthenticated` on failure) → `GetUserByPhone` conflict handling (self = idempotent no-op; other full account = `CodeAlreadyExists`; nil = proceed) → set `phone_number` (**leave `auth_method`**) → `UpdateUser` (`connecterr.Internal` on storage failure) → `PromoteByPhone` (best-effort) → return echoed phone. Log "phone added" with `user_id` + `logging.MaskPhone(phone)` (mirror `PhoneRegister`'s "user registered"). The `CodeAlreadyExists` enumeration leak is acceptable: the caller proved ownership of that number via OTP, so "this number you own is already on an account" reveals nothing new.
- [ ] **Relax `PhoneLogin`:** delete the `auth_method != PHONE` rejection block (`phone_auth.go:268`) — `GetUserByPhone` finding a row is sufficient proof of the phone credential. Leave Email/OIDC/password-reset gates unchanged.
- [ ] Surface `phone_number` on `GetUser` self-fetch (alongside `email`/locations).
- [ ] Add a per-user `ratelimit` rule for the new procedure.
- [ ] Tests (`server/services/user/` + login): happy path (phone set, `auth_method` **unchanged**); **additive login proof** — after attach, the original method (`EmailLogin`/`OIDCLogin`) *still* succeeds AND `PhoneLogin` *now* succeeds for the same account (integration); idempotent re-attach; conflict with another account; cross-community provisional merge (RSVPs + transfers migrated, communities joined); phone normalization; unconfigured-verifier path; unauthenticated caller rejected; `PhoneLogin` for an email-primary-with-phone account. **Security invariant test:** `SaveUser` cannot set `phone_number` (guards the relaxed `PhoneLogin` gate against a future unverified writer). **Read-path coverage (#1142):** assert `GetUser` self-fetch returns `phone_number` and a non-self fetch does **not** (update `server/services/user/roundtrip_test.go` / `service_test.go`).
**Validates:** The whole server contract — additive multi-method login + provisional reconciliation — without any client.

### Phase 2: Client — tucked-away profile-edit entry + OTP reuse
**Goal:** An existing email/OIDC user can add a phone from the profile-edit screen and immediately sign in by phone too.
**Tasks:**
- [ ] Add `addPhoneNumber({ firebaseIdToken })` to `app/lib/services/user_service.dart` (mirror `saveUser`: build request, `await _client.addPhoneNumber(request, headers: _buildHeaders())` inside `RpcUtils.executeRpc`). Optional `UserRepository` passthrough.
- [ ] Extend `PhoneAuthScreen` with `PhoneAuthMode.attach`: at the post-`getIdToken()` branch, skip the name step, call `addPhoneNumber`, then `Navigator.pop` with the result (no `setAuthState`/navigate-home).
- [ ] Add a low-prominence "Add phone number" row to `profile_edit_screen.dart` (`buildSettingsActionItem`), shown via the self-fetched `phone_number`: "Add phone number" when absent, masked "+1 555… ✓" when present. `onTap` pushes `PhoneAuthScreen(mode: attach)`.
- [ ] Positive pre-OTP copy: "Add your phone — sign in faster next time and get text reminders." (No "you'll lose Google" warning — additive.)
- [ ] On success: update the cached self-user so the row reflects the new phone, and trigger one **proactive token refresh** so the access token carries the phone claim immediately.
- [ ] Client tests: viewmodel/service test for the attach call + success-state transition (Mockito + ProviderContainer, `--reporter expanded`).
**Validates:** The end-to-end upgrade journey in the real app surface the maintainer specified.

### Phase 3: Hardening, docs, follow-ups
**Goal:** Ship-ready edges and durable docs.
**Tasks:**
- [ ] **Rewrite `docs/registration_and_login.md` → "Auth Method Enforcement"**: it currently states hard mutual exclusivity as an invariant. New model: methods are additive; `auth_method` is the *primary/original* method; `PhoneLogin` accepts any account holding a verified phone credential. Document `AddPhoneNumber`. Update `docs/single_player_mode.md` → *Claiming* (third promote trigger). Advance freshness stamps.
- [ ] File the follow-up for **phone-belongs-to-another-full-account** (account↔account merge) and reference it in the rejection path.
- [ ] Optional: a Playwright e2e if the attach flow is reachable on Flutter Web; otherwise note coverage rests on Go integration + client unit tests.
- [ ] (Forward pointer, not built here) Note the symmetric *add-email / add-OIDC to a phone account* generalization that would relax the remaining three gates to credential-presence.
**Validates:** The change is documented, the known out-of-scope collision is tracked, and the doc ratchet won't flag drift.

## Related Issues
- **#2492** — parent epic (phone-first pivot); listed follow-up at line 124.
- **ID-2 / `promoteProvisionalByPhone`** (in #2492) — the phone-first promote-on-verify this inverts; source of the extracted shared helper.
- **EMAIL-1b / `promoteProvisionalByEmail`** (#2492) — the email-side analog; same merge primitives.
- **#2571** — verified-email gate; pairs with this on the email side (the "email shadowing" accepted-risk counterpart).
- **registration_and_login.md "Future Enhancement #4: Account Linking"** — this is the first concrete step toward it (additive multi-method login).
- **(to file)** — phone belongs to another full account: account↔account merge, deliberately out of scope here.

---

## Audit notes (`/ripls-audit`)

**Scope:** both sides (server `services/user`, `services/login`, `provisional`, proto; client `app/lib/**`). Subsystems: auth/registration, provisional users, SMS reachability. Engaged dimensions: `api_design`, `security`, `error_handling`, `observability`, `testing`, `code_quality` (DRY), plus light `accessibility` (one settings tile).

**Refinements applied from the rubrics:**
- **security §1/§4 — the load-bearing finding:** added the *no-unverified-phone-writer* invariant as the precondition that makes relaxing the `PhoneLogin` gate safe (otherwise it's an account-takeover vector), plus a regression test that `SaveUser` cannot set a phone, plus the explicit caller-only/no-IDOR property and the OTP-gated reason the `AlreadyExists` enumeration leak is acceptable.
- **api_design §1/§2 — read-path coverage (#1142):** pinned `GetUserResponse.phone_number` to field 11 with `feedback_proto_field_order` source placement, required proto comments in the established phone-auth style, and added a round-trip test asserting self-fetch returns the field and non-self does not.
- **error_handling/observability:** enumerated the Connect error-code mapping on the implement step and the masked structured-log line.
- **markdown:** fixed the table to the repo's spaced-pipe style (MD060). Left the `**Goal:**`/`**Tasks:**` phase formatting as-is — it matches every existing plan in `docs/issues/` and markdownlint is not a CI gate here.

**No blocking conflicts found** with the architecture/convention anchors: the RPC home (UserService, auth-wrapped), the API/models separation, dedicated request/response types, service independence (shared `provisional` package, no service→service dependency), and PII-off-the-wire rules are all honored. The one documented decision this *intentionally revises* is `registration_and_login.md`'s "Auth Method Enforcement" invariant (hard exclusivity → additive), per maintainer direction; Phase 3 rewrites that section.

**Docs consulted:** `proto_conventions.md` (⚓), `server/architecture.md` (⚓), `server/conventions.md` (⚓), `client/architecture.md` (⚓), `client/conventions.md` (⚓), `registration_and_login.md`, `single_player_mode.md`, `sms_notifications.md`, `ad_hoc_communities.md`, `docs/issues/2492-phone-first-pivot.md`; review rubrics `reviews/security_review_prompt.md` + `reviews/api_design_review_prompt.md` ("Areas to Review").
