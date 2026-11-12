# SMS-1 companion: Twilio account, A2P 10DLC registration, and "text authorization"

**Parent plan:** [`2492-phone-first-pivot.md`](./2492-phone-first-pivot.md) → Phase 0 → **SMS-1**
**Status:** Phase 0 — ✅ **Brand + Campaign `APPROVED` (2026-06-26). Platform SMS is unblocked.**
Remaining before SMS can send: Twilio secrets → Secret Manager + `entrypoint.sh` (§6), counsel sign-off,
then build SMS-2/SMS-3.
**Owner:** _(ops/founder)_ + counsel for the legal track

### Progress log

> **Secrets are never recorded here.** Account SID, Auth Token, and Messaging Service SID live only in
> Secret Manager (per [§6](#6-code-wiring-spec-secrets--feature-flag)). The IDs below are non-secret
> registration identifiers (the kind referenced in a Twilio support ticket), safe to track.

- **2026-06-18 — Hosted consent pages live.** `/sms-opt-in.html` (opt-in / CTA proof), refreshed
  `/sms-terms.html`, and the `/privacy-policy.html` mobile-data carve-out shipped via **PR #2508
  (merged)** and are live on `ripls.org`.
- **2026-06-18 — Twilio account + number + Messaging Service.** Account created, first 10DLC long code
  purchased, Messaging Service **"Ripls – Invites & Notifications (prod)"** created with the number in
  its sender pool. _(Messaging Service SID → stored as the `twilio-messaging-service-sid` GSM secret.)_
- **2026-06-18 — Brand `APPROVED`.** Standard Brand **"Ripls"**, registered to **Type2Fun Foundation,
  PBC** (for-profit public benefit corporation + EIN), auto-approved on submission. Brand Registration SID
  `BN4541bb2fd18e004461d814fbedb585ca`.
- **2026-06-18 — Campaign `PENDING_REVIEW`.** Submitted `2026-06-18T22:39:22Z`, use case **MIXED**
  (covers invitations + activity notifications + verification — a cleaner fit than SOCIAL for Ripls's
  message mix), attached to the prod Messaging Service. Campaign SID
  `CMd396e39ffd9e022b22dc8b332f2f5f40`.
- **2026-06-26 — Campaign `APPROVED`. ✅** The resubmit with real screenshots of the in-app consent
  surfaces (phone-entry SMS disclosure #2530; host invite-attestation checkbox #2527) cleared the earlier
  30909 rejection. **Platform-originated SMS is now unblocked** — SMS-2/SMS-3 are buildable; only the
  secret wiring (§6) + counsel sign-off gate the flag flip.
- **2026-06-26 — §6 code wiring landed (branch `sms2-twilio-provider`).** ✅ The
  [§6](#6-code-wiring-spec-secrets--feature-flag) secret + feature-flag scaffold is built: `twilio-*`
  flags + `resolveSecret` + `-platform-sms-enabled` (default off) in `main.go`; `entrypoint.sh`
  optional fetches + env-gated flag + `-file` exec args; Terraform `PLATFORM_SMS_ENABLED` env (var
  false) + the three secret names staged **commented** in dev+prod `server_runtime_secrets`. SMS-2
  (the provider, opt-out store, and STOP/HELP/START webhook) landed on the same branch. The code is
  **inert** until the secrets exist and the flag flips.
- **2026-06-26 — GSM secrets created.** The **live trio** in **ripls-prod**: `twilio-account-sid`,
  `twilio-auth-token`, and `twilio-messaging-service-sid` (`MGa3f2884f…`, the account's one Messaging
  Service, looked up via the API). Twilio **test** credentials (`twilio-test-account-sid`,
  `twilio-test-auth-token`) in **ripls-dev + ripls-prod** — the SMS provider integration test now runs
  green against real Twilio (magic numbers; `npm run test:server:integration:sms`). CI's
  `github-actions@ripls-dev` SA has project-level `secretAccessor`, so no per-secret grant was needed.
  *(No live creds in ripls-dev yet — dev sends would need a dev set; the test path uses test creds + a
  magic From, so it doesn't need them.)*
- **2026-06-26 — Terraform grants applied (prod + dev).** The live trio is in `server_runtime_secrets`
  for both envs; `terraform apply` added the 3 `secretAccessor` grants + `PLATFORM_SMS_ENABLED=false` to
  each server. Prod revision `server-00116-7fh`, dev `server-dev-00993-7gq`, both 100% / `/readyz` 200 /
  flag off. **Dev shares prod's Twilio account, campaign, and Messaging Service** (a 10DLC campaign binds
  to one Messaging Service, so a separate dev campaign isn't worth it; real sends route via the Messaging
  Service SID, not a `From` number) — the live trio was mirrored prod→dev in GSM. The servers fetch the
  creds and construct the SMS sender, but **send nothing** until the flag flips. *(Dev apply was
  **targeted** to avoid an unrelated pre-existing delete of orphaned `feedback_url_signer` resources —
  removed from config, dev's cleanup apply still pending; tracked separately.)*
- **2026-06-27 — Consent/attestation strategy APPROVED.** ✅ Company leadership (acting as counsel)
  signed off on the host-attestation consent model + state-law posture. The legal gate is cleared.
- **2026-06-27 — Inbound + outbound verified end-to-end on real Twilio.** Live delivery to a real
  handset (outbound) and real STOP/START/HELP through `dev.ripls.app/sms/webhook` (inbound,
  signature-validated, persist-only — Twilio sends the replies). Confirmed via `server-dev` logs.
- **2026-06-27 — Dev gets its own Messaging Service (env isolation).** Decided to give **dev** a
  dedicated Messaging Service + **Low-Volume Standard** campaign (under the shared "Ripls" Brand, same
  account) so dev/prod have **independent webhooks + opt-out lists** — a Messaging Service has only one
  inbound webhook URL, so the prior shared-MS setup could feed STOP/HELP to only one env. Dev MS
  **`MGe599f8c77c0959bb976ad7f09c6f0c47`** created; its campaign is **PENDING approval**. The
  `twilio-messaging-service-sid` secret in **ripls-dev** was swapped from the prod MS (`MGa3f2884f…`)
  to the dev MS (version 2; account-sid/auth-token unchanged — same account). Inert until the dev
  campaign approves **and** dev's `platform-sms-enabled` flips. **Supersedes** the "dev shares prod's
  Messaging Service" note in the 2026-06-26 entries.
- **Remaining before the flag flips:** finish the **off-app email backup send** (the email fallback
  for deviceless recipients) — then flip `platform-sms-enabled` per env (`platform_sms_enabled = true` +
  apply). Each Messaging Service now carries its **own** inbound webhook (prod →
  `https://ripls.app/sms/webhook`, dev → `https://dev.ripls.app/sms/webhook`), so there's no
  shared-webhook cutover — just confirm the **prod** MS webhook points at `ripls.app` (not the dev URL)
  before prod go-live. Optional: rename the Messaging Services from their default names; rotate the live
  auth token (it transited a chat transcript during setup).

This is the working companion for **SMS-1**. It expands the plan's SMS-1 checklist into:

1. [Account + number setup](#1-account--number-setup-from-zero) — click-by-click, from no account.
2. [Brand registration data](#2-brand-registration-the-legal-entity) — for-profit / PBC.
3. [Campaign registration content](#3-campaign-registration-the-text-authorization) — the actual "text
   authorization": opt-in flow, **ready-to-paste sample messages**, STOP/HELP/opt-out copy.
4. [Hardening + Messaging Service](#4-hardening--messaging-service).
5. [Legal / consent brief for counsel](#5-legal--consent-brief-for-counsel).
6. [Code-wiring spec](#6-code-wiring-spec-secrets--feature-flag) — secrets + feature flag, exact file
   touch-points, mirroring the Mailgun pattern.

> **Vendor names are fine in this doc** (it's contributor/ops-facing, like a README — see CLAUDE.md).
> The proto-comment vendor ban applies only to `.proto` files and user-facing SMS copy, where we say
> "Ripls" / "text message", never "Twilio".

> **Why this runs in parallel and blocks nothing in Phase 1.** A2P 10DLC review is the ~2–3 week
> external pole. The events loop (Phase 1) ships on **email (EMAIL-1, no 10DLC) + host-relayed
> links**, so engineering never waits on this. Platform SMS (Phase 2) is a flag-flip once this clears.

---

## 1. Account + number setup (from zero)

- [ ] **Create the account** at twilio.com. Verify your email and your own mobile number. In
      onboarding pick: product **SMS**, "what do you want to do first" → **send an SMS**, "how do you
      want to build" → **with code**.
- [ ] **Upgrade to a paid account** and add billing. A trial account **cannot** register A2P 10DLC or
      send to numbers you haven't manually verified — it's a dead end for this work.
- [x] **Decide dev vs. prod sending.** ✅ **Decided 2026-06-27 — dev gets its _own_ Messaging
      Service** (not a shared one), so dev and prod have **independent inbound webhooks and opt-out
      lists**:
  - **Prod** → the real 10DLC long code + the MIXED Standard Campaign, Messaging Service
    `MGa3f2884f…`, inbound webhook `https://ripls.app/sms/webhook`.
  - **Dev** → its own dev long code + a **Low-Volume Standard** campaign under the **same approved
    "Ripls" Brand**, in the **same Twilio account** (so it shares `twilio-account-sid` /
    `twilio-auth-token`), Messaging Service **`MGe599f8c77c0959bb976ad7f09c6f0c47`** (created
    2026-06-27, **campaign PENDING approval**), inbound webhook `https://dev.ripls.app/sms/webhook`.
  - **Why split the service:** a Messaging Service has exactly **one** inbound webhook URL and **one**
    carrier opt-out list, so a *shared* service can only feed STOP/HELP to one environment — a second
    Messaging Service is the only way to get two webhooks. Cost ~$1.50/mo + $15 one-time + ~$1.15/mo
    number. *(Supersedes the earlier "dev shares prod's Messaging Service" posture in the 2026-06-26
    progress entries.)*
  - **Unit/integration tests** still use **Twilio test credentials + Magic Numbers** (e.g.
    `+15005550006`) — no registration, no real send — so SMS-2 stays "buildable against test creds."
    The dev Messaging Service is for **manual real-handset dev testing**, not the automated suite.
- [ ] **Buy the prod number:** Console → Phone Numbers → **Buy a number** → US **Local**, **SMS**
      capability. One long code is enough for v1 (short code is the documented upgrade path, not v1).
- [ ] **Note the cost shape** (approx, confirm live): Brand ~$4 one-time · Campaign ~$15/mo (SOCIAL) ·
      optional external vetting ~$40 one-time (higher trust/throughput) · number ~$1.15/mo + per-msg +
      carrier fees. Approval: a few days to ~2 weeks depending on vetting.

The three credentials you'll harvest for [§6](#6-code-wiring-spec-secrets--feature-flag):
**Account SID**, **Auth Token** (Console home), and **Messaging Service SID** (created in [§4](#4-hardening--messaging-service)).

---

## 2. Brand registration (the legal entity)

Console → **Messaging → Regulatory Compliance → A2P 10DLC** → create a **Business/Customer Profile**,
then register a **Standard Brand**. Have this ready (you chose **for-profit / PBC** — public benefit corporation):

- [ ] **Legal business name** — exact, as it appears on the EIN/IRS letter (mismatches are the #1
      rejection cause).
- [ ] **Business type:** Private for-profit / **PBC** (public benefit corporation).
- [ ] **EIN / Tax ID** (US).
- [ ] **Registered legal address** (must match IRS records).
- [ ] **Website:** `https://ripls.app`.
- [ ] **Business industry / vertical** (e.g. Technology / Online services / Social).
- [ ] **Authorized representative:** name, business email, phone, job title.
- [ ] **Stock exchange / ticker:** N/A (private).
- [ ] **(Optional) external vetting** — pay ~$40 for a third-party vetting score if you want higher
      daily throughput / better deliverability. Not required to launch v1 volume.

> The Brand is vetted **once** by The Campaign Registry (TCR). Get this exactly right before
> submitting the Campaign — a Brand rejection resets the clock on the critical path.

---

## 3. Campaign registration (the "text authorization")

This is the part reviewers actually read. Console → A2P 10DLC → **Create a Campaign** under the
approved Brand. Values below are locked by the plan ([Decision 4](./2492-phone-first-pivot.md#decision-4--delivery-sequencing--channels)).

### 3.1 Use case
- [ ] **Primary use case:** `SOCIAL` — "communication within and between members of closed
      communities; event invitations, RSVPs, and activity updates among people a host already knows."
- [ ] **Fallback / sub-use-cases:** `MIXED` + `ACCOUNT_NOTIFICATION` (covers RSVP/offer
      confirmations and community-activity pings to deviceless members).

### 3.2 Opt-in flow description (paste-ready)

> Ripls sends a single transactional invite or activity update only after a person's number enters
> the system through one of two consented paths:
>
> 1. **Account-holder opt-in.** When a person signs up for or logs into Ripls with their phone
>    number, the phone-entry screen displays: *"By continuing, you agree to receive event invites and
>    community updates from Ripls. Message frequency varies. Msg & data rates may apply. Reply STOP to
>    cancel, HELP for help."* Submitting the number is the opt-in.
> 2. **Host-attested invite.** A Ripls host creating an event, request, or giveaway may invite people
>    they personally know by phone number. Before any number is sent to, the host completes a logged,
>    timestamped attestation: *"I know these people and they're OK receiving an invite from me on
>    Ripls."* The recipient receives **one** message that names the host and Ripls and carries STOP/
>    HELP, e.g. "Alex invited you to … via Ripls."
>
> Every recipient can opt out of all future messages at any time by replying STOP; HELP returns
> support contact info. Opt-outs are honored globally at the data layer and are never re-contacted,
> even by a different host.

- [ ] **Opt-in screenshot / URL.** TCR/carriers want proof. Host a public page (or screenshot) of the
      phone-entry consent disclosure and the host attestation UI; supply the URL. _(Ties to the CREATE-1
      consent surface and the WEB-1 phone-entry screen — those must render the disclosure verbatim.)_

### 3.3 Sample messages (paste-ready; ≥2 required, give all four)

> 1. **Event invite (host-relayed/A2P):**
>    `Alex invited you to "Backyard BBQ" on Ripls — Sat Jun 21, 4pm. View & RSVP: https://ripls.app/go/AB12CD34  Reply STOP to opt out, HELP for help.`
> 2. **RSVP / offer confirmation (account notification):**
>    `Ripls: Sam RSVP'd yes to "Backyard BBQ". See who's coming: https://ripls.app/go/AB12CD34  Reply STOP to opt out.`
> 3. **Community activity update:**
>    `New on Ripls for "Backyard BBQ": Jordan offered to bring chairs. https://ripls.app/go/XR42KQ7  Reply STOP to opt out.`
> 4. **Request/giveaway invite:**
>    `Maria needs a ladder on Ripls and thought you might help. View: https://ripls.app/go/9KD3MQ2  Reply STOP to opt out, HELP for help.`

Every sample carries **brand (Ripls) + host identity + the item + a `ripls.app/go/` link + STOP** —
matching the single-transactional-invite content rule in [§5](#5-legal--consent-brief-for-counsel).
Use **our own branded `ripls.app` link domain**, never a public shortener (carrier filtering /
campaign requirement).

### 3.4 Opt-out / help auto-replies & keywords
- [ ] **Enable Advanced Opt-Out** on the Messaging Service; set the auto-reply copy:
  - **STOP** (also CANCEL, END, QUIT, UNSUBSCRIBE) → `You're unsubscribed from Ripls and won't get more messages. Reply START to resubscribe.`
  - **HELP** (also INFO) → `Ripls: event invites & community updates. Help: support@ripls.org. Reply STOP to opt out. Msg & data rates may apply.`
  - **START** (also YES, UNSTOP) → `You're resubscribed to Ripls messages. Reply STOP to opt out.`
- [ ] **Message frequency:** "Recurring; frequency varies by community activity."
- [ ] **Embedded link:** yes (`ripls.app/go/…`). **Embedded phone number:** no. **Age-gated content:** no.
      **Affiliate marketing:** no.

> The SMS-2 webhook (`OptOutType`) and the per-E.164 opt-out store persist these globally; the
> auto-reply copy above must match what SMS-2 renders via the l10n catalog. **Honor revocation within
> 10 business days.**

---

## 4. Hardening + Messaging Service

- [ ] **Create a Messaging Service**, add the prod number to its **sender pool**, and **link the
      approved Campaign**. (Sends route through the Messaging Service SID, not the bare number.)
- [ ] **Geo permissions:** lock sending to your launch countries only (US first); disable all others.
- [ ] **SMS Pumping Protection:** enable (paired with CREATE-1's per-host/IP/recipient rate-limiting).
- [ ] **Advanced Opt-Out:** confirm enabled with the §3.4 copy.
- [ ] Capture the **Messaging Service SID** for [§6](#6-code-wiring-spec-secrets--feature-flag).

---

## 5. Legal / consent brief for counsel

> **Compliance research, not legal advice.** Counsel must approve the attestation wording and the
> state-law posture **before the `platform-sms-enabled` flag is flipped**. Email (EMAIL-1, CAN-SPAM)
> is materially lower-risk and ships first; this brief is specifically about **gating SMS**.

**What we're doing.** Platform-originated A2P SMS that delivers (a) one transactional event/request/
giveaway **invite** per recipient chosen by a host, and (b) **account/activity notifications** to
members who have no active app device. No marketing, no drip, no re-nagging.

**Why it's federally defensible (necessary, not sufficient).** Under the FCC's *TextMe* (2015) ruling
and the *Poshmark* line, the **person who picks recipients and timing is the "maker/initiator"** of
the message — here, the host — not the platform. This is why comparable platforms (Partiful short code
397-81, Evite 296-94, Paperless Post, Punchbowl, Luma) carry a **host representation-and-warranty** in
their ToS. Model language (Paperless Post): *"you represent and warrant that you have the Guests' prior
express consent, permission or invitation."* We keep each send a **single transactional invite** (host
+ item + link, no marketing) with **STOP/HELP** honored globally.

**The real exposure is SMS + state law.** A ToS attestation is **not a full shield**. Block (Cash App)
settled **$12.5M** under **Washington's CEMA** over "Invite Friends" referral texts — TCPA was held
*inapplicable* to the assister, but **CEMA reached Block anyway** ("even if the company did not
physically press send"). Federal TCPA defensibility is necessary but **not sufficient**; **WA CEMA,
FL FTSA, CA CIPA** add independent exposure, and CTIA's carrier layer treats opt-in as
non-transferable at campaign-registration time.

**Asks for counsel:**
1. Review/approve the **host-attestation wording** (§3.2) and the in-app **consent disclosure** copy.
2. Draft the **ToS rep-and-warranty clause** binding hosts to "prior express consent."
3. Opine on **state-law exposure** (esp. WA CEMA, FL FTSA, CA CIPA) and whether to **geo-exclude**
   the highest-risk states for the initial SMS launch.
4. Confirm the **single-transactional-invite content rules** (one message per recipient per item; no
   marketing copy; clear sender identity) are sufficient.
5. Confirm **CAN-SPAM mechanics** for EMAIL-1 (accurate headers, physical postal address, one-click
   unsubscribe ≤10 business days) — low-risk, but get it on record.

**Sources:** Partiful help center; Paperless Post & Evite ToS; FCC *TextMe* / *Poshmark* (Lexology);
FTC CAN-SPAM compliance guide; Block/Cash App CEMA settlement (National Law Review / Top Class
Actions); CTIA Messaging Principles; Airbnb email-only referral (CloudSponge).

---

## 6. Code-wiring spec (secrets + feature flag)

The only code SMS-1 needs: **secret plumbing + the gate flag**. It's inert until the secrets exist and
SMS-2 builds the provider — empty creds ⇒ SMS disabled (the Mailgun `main.go:864` pattern), flag
defaults off. **Sequencing matters** (don't crash the server):

> **Safe order:** (a) define the Go flags first, (b) add the entrypoint *optional* fetches + exec args,
> (c) create the GSM secrets, (d) *then* add the Terraform `secretAccessor` grant. Adding an exec flag
> the Go binary doesn't define → flag-parse crash-loop; granting IAM on a secret that doesn't exist →
> `terraform apply` failure. Optional fetches (`fetch_secret_to_file_optional`) tolerate the gap in
> between, exactly like `google-maps-api-key-server`.

**Secret names** (GSM, both `ripls-dev` and `ripls-prod`): `twilio-account-sid`, `twilio-auth-token`,
`twilio-messaging-service-sid`. Per the plan all three live in Secret Manager for uniformity.

### (a) `server/main.go` — flags + cfg fields (mirror Mailgun)
```go
// Twilio / platform SMS (#2492 SMS-1 wiring; consumed by SMS-2/SMS-3). Empty
// creds ⇒ SMS disabled, like Mailgun. -platform-sms-enabled gates the path.
twilioAccountSID        := flag.String("twilio-account-sid", "", "Twilio Account SID for platform SMS (empty disables SMS)")
twilioAccountSIDFile    := flag.String("twilio-account-sid-file", "", "Path to a file containing the Twilio Account SID")
twilioAuthToken         := flag.String("twilio-auth-token", "", "Twilio Auth Token")
twilioAuthTokenFile     := flag.String("twilio-auth-token-file", "", "Path to a file containing the Twilio Auth Token")
twilioMessagingSID      := flag.String("twilio-messaging-service-sid", "", "Twilio Messaging Service SID")
twilioMessagingSIDFile  := flag.String("twilio-messaging-service-sid-file", "", "Path to a file containing the Twilio Messaging Service SID")
platformSMSEnabled      := flag.Bool("platform-sms-enabled", false, "Enable platform-originated SMS (#2492). Off until 10DLC clears + counsel sign-off; email needs no flag.")
```
Then resolve the file variants and store on `cfg`, mirroring `resolveSecret("mailgun-api-key", …)`:
```go
resolveSecret("twilio-account-sid", twilioAccountSID, twilioAccountSIDFile)
resolveSecret("twilio-auth-token", twilioAuthToken, twilioAuthTokenFile)
resolveSecret("twilio-messaging-service-sid", twilioMessagingSID, twilioMessagingSIDFile)
// cfg: twilioAccountSID, twilioAuthToken, twilioMessagingSID string; platformSMSEnabled bool
```

### (b) `server/entrypoint.sh` — optional fetches + exec args
```sh
# After the existing fetch_secret_to_file_optional google-maps... line:
fetch_secret_to_file_optional twilio-account-sid            "$SECRETS_DIR/twilio-account-sid"
fetch_secret_to_file_optional twilio-auth-token             "$SECRETS_DIR/twilio-auth-token"
fetch_secret_to_file_optional twilio-messaging-service-sid  "$SECRETS_DIR/twilio-messaging-service-sid"
```
```sh
# Added to the exec ./server flag list:
  -twilio-account-sid-file="$SECRETS_DIR/twilio-account-sid" \
  -twilio-auth-token-file="$SECRETS_DIR/twilio-auth-token" \
  -twilio-messaging-service-sid-file="$SECRETS_DIR/twilio-messaging-service-sid"
```
```sh
# platform-sms-enabled rides as an env-gated flag like the digest toggles:
[ "${PLATFORM_SMS_ENABLED:-}" = "true" ] && set -- "$@" -platform-sms-enabled
```

### (c) Terraform — grant (uncomment *after* the GSM secrets exist)
In **both** `terraform/environments/{dev,prod}/main.tf`, add to `local.server_runtime_secrets`:
```hcl
# Platform SMS (#2492). Uncomment once the secrets exist in GSM, else the
# secretAccessor grant fails on a non-existent secret.
"twilio-account-sid",
"twilio-auth-token",
"twilio-messaging-service-sid",
```
Create the secrets first (per env), e.g.:
```
gcloud secrets create twilio-auth-token --project=ripls-prod
printf '%s' "<token>" | gcloud secrets versions add twilio-auth-token --data-file=- --project=ripls-prod
```

### (d) `PLATFORM_SMS_ENABLED` env (Terraform containers module)
Add a `PLATFORM_SMS_ENABLED` env var to `terraform/modules/containers/main.tf` (alongside the digest
toggles), wired to a tfvars boolean — kept **false** until counsel signs off and 10DLC clears.

> **Where the provider plugs in (SMS-2/SMS-3, not now):** a new `SMSSender` field on the notification
> `service` struct (`server/notifications/service.go:43`) — SMS has no `UserDevice` row so it can't
> join `providers map[DevicePlatform]Provider` — and the off-app channel branch extends the
> `len(devices)==0` no-op at `service.go:92`.

---

## SMS-1 exit criteria
Platform-originated SMS is **legally and operationally cleared**: Brand + Campaign approved, number in
a Messaging Service with geo + pumping protection, secrets in GSM, the gate flag wired, and counsel
sign-off on the attestation + state-law posture recorded. The flag stays **off** until Phase 2.
