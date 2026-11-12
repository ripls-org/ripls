# RCS-1 companion: RCS Business Messaging — compliance & registration (epic #2492, Phase 4)

**Parent plan:** [`2492-phone-first-pivot.md`](./2492-phone-first-pivot.md) → Phase 4 → **RCS-1**
**Companion to:** [`2492-sms-a2p-registration.md`](./2492-sms-a2p-registration.md) — RCS **reuses** that work's
approved Brand, opt-out store, inbound webhook, and the dev/prod Messaging Service split.
**Status:** 🟢 **SUBMITTED 2026-07-01** — Google RCS registration + US (Aegis) registration filed together.
Now in carrier review (~5–9 wk). Fees ($200/yr Aegis + $500 one-time) charged **at approval**. Per-recipient
gating (**#2609**) **CLOSED 2026-07-01**: cold no-opt-in messaging is parked, so every message goes to a
first-party-opted-in recipient → RCS-first has no cold-number exposure. *(If cold/no-opt-in messaging is
un-parked, the gating becomes required again before it ships — reopen #2609; the sender is live in the pool.)*
**Owner:** ops/founder + counsel (legal copy review)

> **Vendor names are fine in this doc** (contributor/ops-facing, like a README — see CLAUDE.md). The
> proto-comment vendor ban applies only to `.proto` files and user-facing message copy, where we say
> "Ripls" / "text message", never "Twilio" / "RCS provider".

---

## TL;DR — the load-bearing finding (read this first)

**RCS cannot be our cold-invite channel.** Sending a verified, branded RCS message to a recipient who
has **not opted in themselves** — including a number a host picked from their contacts — is **not
approvable** under Google or US-carrier rules. This was researched deeply (sources at the bottom); three
independent walls block it:

1. **Google RBM policy** requires *"consent prior to soliciting any end user"* and that you *"only
   communicate with users who have explicitly opted into receiving your messages."* Unverified agents can
   reach only test devices; violations are permanently suspended.
2. **CTIA opt-in is non-transferable and sender-specific** — it *"should not be transferable or
   assignable"* and exists specifically to stop texts to *"a phone number that does not belong to the
   Consumer who provided"* it. A host cannot consent **for** a guest. Shared/contributed contact lists are
   prohibited.
3. **The "host-as-initiator" theory (FCC *TextMe*/*Poshmark*) is a TCPA _liability_ defense only** — the
   FCC withdrew the very question of whether intermediary-supplied consent satisfies "prior express
   consent." Carriers treat **Ripls** as the Sender regardless. **Accessing contacts is not consent.**

**What Partiful actually does** (the reason this looked possible): cold invites are **plain SMS from a
CTIA short code, `39781` ("397-81")** — *not* RCS, *not* even 10DLC. Their only branded/verified channel
is **Apple Messages for Business (iMessage), which is opt-in and iPhone-only**. There is no evidence of a
branded Partiful RCS agent reaching cold recipients.

**Therefore the achievable architecture — and what every aggregator + invite platform actually does:**

- **Cold first touch → SMS** (the approved A2P 10DLC campaign; host-warranty + STOP/START). Already
  built. ✅
- **RCS → an enrichment _upgrade_ for recipients who opted in themselves** (registered/returning Ripls
  users). An **SMS opt-in carries over to RCS for the same program**, so once an invitee verifies their
  phone, *their* later invites/notifications can ride RCS. **RCS serves invites — just never the first
  cold touch to a never-seen number.**

> The thing that makes Partiful's cold invites feel premium is the **short code**, not RCS. That's a
> separate SMS initiative (see [Future: SMS short code](#future-option--sms-short-code-true-partiful-parity)),
> deferred.

---

## Locked decisions (Bryan, 2026-06-27)

1. **RCS scope = opted-in/returning users only.** Cold invites stay on SMS. RCS is the upgrade once a
   recipient has first-party-opted-in (verified their phone). *(Fact-forced by the TL;DR; not a
   preference we can override.)*
2. **SMS cold channel = the approved 10DLC long code** (no change). A CTIA short code is noted as a future
   option, not pursued now.
3. **Canonical customer-facing domain = `ripls.org`.** Legal pages + the "business website" stay on
   `ripls.org`; the sender-profile URLs and opt-in-page links all use `ripls.org`. ⚠️ **Residual risk:** the
   `/go/{code}` links in our sample messages live on **`ripls.app`** — a different registrable domain. It's
   a *branded* domain for "Ripls" (so defensible, not a third-party redirect), but reviewers cross-check
   link domains. See [Digital footprint](#8-digital-footprint--domain-consistency) for mitigations.
4. **In-app opt-in = keep the bundled CTA, add ToS + Privacy links on the same screen.** ⚠️ RCS reviewers
   are stricter than A2P on "consent must be separate and optional" — this may draw a fix request; we try
   it as-is first and fall back to a dedicated unchecked control if rejected.
5. **Timing = prep now, hold submission.** Complete all copy/opt-in/sender-profile/welcome-message prep
   immediately (no lead-time cost; most of it also hardens SMS compliance). Hold the Google + US submission
   (and the Aegis/carrier fees) until platform SMS is proven in prod or we choose to pay.

---

## How RCS rides on what we already built

RCS is **a capability of a Messaging Service**, not a separate service. Nearly all the plumbing is the
SMS plumbing:

| Asset | Reused for RCS? |
|---|---|
| Approved Brand "Ripls" / Type2Fun Foundation, PBC + EIN | ✅ identity reused (but Aegis runs its **own** brand verification — separate from TCR) |
| Opt-out store (`server/smsoptout`) + inbound webhook (`/sms/webhook`) | ✅ STOP/HELP/START + the persist-only mirror work for RCS unchanged |
| STOP/HELP/START Advanced Opt-Out copy | ✅ reused; add the RCS sender to the Messaging Service with Advanced Opt-Out enabled |
| Messaging Service split — **prod `MGa3f2884f…`** ("Ripls Invites and Updates Service") / **dev `MGe599f8c…`** ("Ripls Dev - Testing Only") | ✅ RCS sender `XE841…` is in the **prod** pool; SMS fallback uses `+15555550100`, already in the pool |
| Hosted opt-in proof + screenshots (`ripls.org/sms-opt-in.html`) | ✅ satisfies the "opt-in policy image URL" requirement |

**The one genuinely new engineering constraint — per-recipient channel gating.** A Twilio Messaging
Service with an RCS sender in its pool tries **RCS first for _every_ RCS-capable recipient**. That would
send branded RCS to cold, un-opted-in provisional users — a **policy violation**. So our dispatcher must
**decide the channel itself**, not delegate to Twilio's auto-RCS-first:

- **Registered/opted-in user, RCS-capable** → RCS (fallback SMS).
- **Provisional / unverified recipient** → **SMS only** (RCS disabled).

Implementation options (resolve at build): (a) a **separate RCS-enabled Messaging Service** for opted-in
traffic vs. an SMS-only Messaging Service for cold traffic; or (b) per-message channel override. Either
way the **constraint** — RCS only to first-party-opted-in users — is enforced in
`server/notifications` channel selection (the `service.go:92` off-app seam +
`off_app_sms.go`), never left to Twilio defaults.

---

## The two registrations

Launching RCS in the US needs **both**, submitted **together** (nothing reaches carriers until both are
complete):

1. **Google RCS registration** — required for all RCS senders, any country. Brand identity, opt-in
   process, use case. Done in Console → RCS → Google registration.
2. **United States registration** — required for US sending. **Aegis** (carrier-designated verifier)
   validates business identity + campaign + sample messages against CTIA standards. Console → RCS → United
   States registration.

**Fees & timeline** (Twilio vets for free first; you're only charged once vetted/approved):

| Item | Cost | When |
|---|---|---|
| Aegis brand verification (US) | **$200 / year** | at approval |
| US carrier onboarding | **$500 one-time** | at approval |
| RCS message pricing | per-message (see Twilio pricing) | at send |
| **End-to-end timeline** | **~5–9 weeks** (Twilio review 2–8 wk · Aegis ~1 wk · carrier review 3–6 wk) | after submission |

---

## Compliance gap analysis (current state → action)

Each item maps a Twilio-guide requirement to **what we have today** (grounded in our actual files) → the
**gap** → the **action**.

### 1. Sender profile (Twilio Console)
- **Display name:** "Ripls". ✅ have.
- **Description (≤100 chars, message-focused — not "we're a tech company"):** ❌ **new.** Draft:
  *"Event invites, RSVPs, and community activity updates for groups you're part of on Ripls."*
- **Banner image:** ❌ **new asset needed** (distinct from `logo.png`). Logo exists
  (`website/content/assets/logo.png`); a banner does not.
- **Privacy Policy URL / Terms of Service URL:** must be live, public, **on `ripls.org`**, and **match**
  the opt-in page + the in-app links. → `https://ripls.org/privacy-policy.html`,
  `https://ripls.org/sms-terms.html`.

### 2. Privacy Policy (`website/content/privacy-policy.html`)
- **No-sharing clause — TIGHTEN.** §3.2 today says we won't share opt-in consent *"…for their marketing
  or promotional purposes."* Carriers check for an **unconditional** statement; the qualifier can fail.
  → Replace with the carrier-preferred form **plus the aggregator carve-out** (we use Twilio + carriers):
  > *"Text-messaging originator opt-in data and consent are excluded from all of the above
  > information-sharing categories; this information will not be shared with any third parties, excluding
  > the aggregators and providers that deliver the text-message / RCS services."*
- **Add the messaging provider to the §3.1 third-party list** — today it lists Mailgun/FCM/Mapbox/etc.
  but **no SMS/RCS provider**. → add: *"SMS/RCS messaging platform — sends and receives text/RCS messages
  (receives phone numbers and message content)."*

### 3. Terms of Service (`website/content/sms-terms.html`)
Run against the guide's required-elements checklist — we're ~80% there:
- Program/brand name ✅ · specific program description ✅ (§1) · frequency "varies" ✅ · "Msg & data rates
  may apply" ✅ · HELP + support contact ✅ · Privacy Policy link ✅ · cart reminders N/A ✅.
- **Carrier-liability phrasing — FIX.** §4 says *"we are not responsible for delayed or undelivered
  messages."* The guide wants the literal **"Carriers are not liable for any delayed or undelivered
  messages."** → add that sentence.
- **Neutral number-change clause — ADD** (must NOT put opt-out liability on the user). →
  *"If you change or deactivate your mobile number, please update it in your Ripls account settings so
  messages aren't sent to a reassigned number."*
- **STOP prominence** — ensure "Reply STOP to cancel" renders **bold** (currently keyword spans; verify).

### 4. In-app opt-in surface (`app/lib/l10n/app_en.arb` → `phoneAuthConsent`)
- **Have:** brand ✅, frequency ✅, rates ✅, STOP ✅, HELP ✅ (the bundled "By clicking Send Code…" CTA).
- **Gap:** **no ToS + Privacy links on the opt-in screen** (RCS requires both on the *same* screen,
  pointing to the public `ripls.org` URLs).
- **Action:** add visible **Terms** (`ripls.org/sms-terms.html`) + **Privacy**
  (`ripls.org/privacy-policy.html`) links on the phone-entry screen, adjacent to the disclosure;
  re-shoot the opt-in screenshots (`sms-opt-in-app.png` / `sms-opt-in-web.png`) to show them.
- **Per decision 4** we keep the bundled CTA; flagged as the most likely RCS fix-request.

### 5. Welcome / opt-in-confirmation message — **NEW SEND, required for US RCS registration**
We don't send one today. Must include: program name, opt-out info, customer-care contact, frequency,
rates. → new l10n string, sent on first opt-in (phone verification). Draft:
> *"Welcome to Ripls — you'll get event invites & community updates. Msg & data rates may apply. Msg
> frequency varies. Reply HELP for help, STOP to opt out."*

### 6. Use case description (Google registration)
The Google registration's second page has three required fields — **triggers**, **use-case description**,
and a **use-case video URL**. Copy is **opt-in-first** to match the gated RCS scope (#2609): no cold
host-invite path here, because that first touch is SMS, not RCS.
- **What triggers messages (794/1000):** *"A recipient receives messages only after they have taken a
  first-party action on Ripls — they get nothing until they act. The FIRST message is triggered by the
  person's own action: they RSVP to an event, or verify their phone to join. Once they have opted in this
  way, later messages are triggered by activity on the specific things they are part of: someone RSVPs to
  their event, a neighbor offers an item for their request, a host posts an update in a group they joined,
  or a reminder fires for an event they said they would attend. Triggers are user actions inside Ripls, not
  external systems (there are no package-delivery-style automated events). There is no fixed date/time or
  recurring schedule; message frequency varies with the recipient's own activity and the groups they have
  joined."* ⚠️ This text *promises* opt-in-first RCS, so **#2609 must ship before approval** to keep it true.
- **Use case description (838/1000):** *"Ripls is a community app for organizing events and sharing goods
  among people who already know each other. All messaging is transactional and relationship-based, tied to
  a specific shared event or item — never marketing or promotional. PRIMARY interactions (most common):
  event invitations and RSVP confirmations; and community-activity updates on things you are part of (a new
  RSVP, a reminder before an event, a neighbor's offer on your request). SECONDARY interactions (possible
  but less common): giveaway and gear offers, a one-time welcome/opt-in confirmation when a person first
  verifies their phone, and lightweight event logistics. Recipients are always connected to the sender
  through a shared event, request, or community they were invited to or joined. Every message lets the user
  reply STOP to opt out, honored immediately."*
- **Use case video URL — ⚠️ REQUIRED (not optional).** The form marks this required (earlier assumption
  that it was optional for US-only was wrong). Record a ~30–90s screen capture showing: (1) **opt-in/consent**
  — open `ripls.app/go/<code>` → "I'm in" → the "Confirm your phone to RSVP" screen with the consent
  disclosure + Terms/Privacy links → enter number → enter code; (2) **core functionality** — the branded RCS
  welcome (and optionally an invite/RSVP) arriving in Google Messages; (3) **opt-out** — reply STOP (or the
  Google Messages Unsubscribe control) and show the STOP confirmation. Host at a public URL (unlisted
  YouTube is the usual accepted form; also fine: public GCS object or `ripls.org/assets/rcs-demo.mp4`).
  Record with a number **not** already a Ripls user (so opt-in hits `PhoneRegister` and the welcome fires).

### 7. Advanced Opt-Out for RCS
Add the RCS sender to the Messaging Service, enable Advanced Opt-Out, reuse the existing STOP/HELP/START
copy. Our persist-only webhook (`server/services/web/sms_webhook.go`) + opt-out store already mirror these
keywords — no change. STOP/HELP/START are Twilio's Advanced Opt-Out auto-replies (configured on the
Messaging Service, NOT our code). The **actual** live-tested copy submitted for the US registration:
- **HELP:** *"This number sends Ripls updates. Reply STOP to unsubscribe. Msg&Data Rates May Apply"*
- **STOP:** *"You have successfully been unsubscribed from Ripls text notifications. You will not receive
  any more messages from this number. Reply START to resubscribe."*
- **START:** *"You have successfully been re-subscribed to Ripls messages from this number. Reply HELP for
  help. Reply STOP to unsubscribe. Msg&Data Rates May Apply."*

**Per-message cadence (SMS vs RCS) — STOP/HELP does _not_ go on every message.** The STOP/HELP
**keywords** are honored on every message centrally (Advanced Opt-Out + the persist-only webhook +
`smsoptout`), independent of the rendered text. CTIA does **not** require the reminder *text* on every
message — it's required in the opt-in disclosure, the welcome / first message, and "conspicuously and
frequently" thereafter (common best practice **≈ monthly** for a recurring program). So for **SMS**, the
`Reply STOP to opt out, HELP for help.` footer (today appended to every off-app SMS via
`sms.notification.body`) is a *copy* decision: safe to keep on every message, or trim to
first-message-plus-periodic on high-frequency threads. For **RCS**, do **not** repeat "Reply STOP" in
every message — it defeats the rich UX; opt-out is surfaced via the in-chat keyword (always works),
suggested-reply opt-out buttons, and Google Messages' built-in **Unsubscribe** button. Only the
**SMS-fallback** path follows the SMS footer cadence. *(Sources: CTIA Messaging Principles, May 2023;
Bandwidth "RCS for Business opt-out methods"; Google RBM AUP.)*

### 8. Digital footprint / domain consistency
- **Entity/brand split is already cleared:** A2P approved **"Type2Fun Foundation, PBC"** (legal entity)
  with brand **"Ripls"** — the standard DBA pattern. Make it transparent with a **"Type2Fun Foundation,
  PBC DBA Ripls"** footer (already on the legal pages' footers — keep it).
- **Canonical domain = `ripls.org`** (decision 3). Ensure the sender profile, opt-in page, and in-app
  links **all** use `ripls.org`.
- ⚠️ **`/go` link-domain split (residual risk).** Sample messages carry `ripls.app/go/...` while the site
  is `ripls.org`. Mitigations (pick at submission): (a) declare **both** `ripls.app` and `ripls.org` as
  brand-owned in the registration / footer; (b) note in the use-case description that `ripls.app/go` is
  the brand's first-party link-shortener; (c) longer-term, move links to `go.ripls.org` or `ripls.org/go`.
- **Authorized-rep email** should be a corporate domain matching the brand → prefer a **`ripls.org`** address (or
  `@type2fun.org`), **never** a free webmail address.

---

## US registration data checklist (gather before submitting)

Mostly known from A2P; ⛳ = needs confirmation/new input (see interview):
- **Legal business name (exact CP 575):** Type2Fun Foundation, PBC *(provided 2026-06-27; confirm exact CP 575 punctuation before submit)*.
- **EIN:** on file (A2P Brand `BN4541bb2fd18e004461d814fbedb585ca`).
- **Address (must match CP 575):** 2957 11th Street, Boulder, CO 80304. ⚠️ *Differs from the old A2P Denver address — see [Registration inputs](#registration-inputs-provided-2026-06-27).*
- **Company type / industry:** **private corporation** (a public-benefit corporation / **PBC** is a
  *for-profit* corp, privately held — **do NOT select "non-profit"** despite the name "Foundation") /
  Technology · Social.
- **Brand contact phone (E.164):** +15555550100.
- **Authorized rep:** A. Maintainer · CTO · user2@example.org · +15555550100.
- **Traffic:** ~tens of thousands monthly `ripls.org` visits; existing short code = **No / N/A**; expected RCS volume **0–5k/month initially**.
- **Campaign description / message flow / HELP+STOP samples / welcome message:** drafts above.
- **Three compliance acknowledgments:** (PP no-sharing ✓ after §2 fix; ToS elements ✓ after §3 fix; CTIA
  handbook reviewed) — checkable once §2–§3 ship.
- **Notification email** (+ optional status-callback URL — could reuse the #2569 status-callback work).

---

## Engineering work (code deltas, when we build)

1. **Per-recipient RCS channel gating** (the keystone constraint) — **#2609 CLOSED 2026-07-01: not needed
   while cold no-opt-in messaging is parked** (every message goes to a first-party-opted-in recipient, so
   RCS-first has no cold-number exposure). If cold messaging is un-parked, reopen: opted-in/registered →
   RCS-capable path; provisional/unverified → SMS-only, in `server/notifications` channel selection. See
   [architecture](#how-rcs-rides-on-what-we-already-built).
2. **Welcome / opt-in-confirmation send** — ✅ **BUILT (2026-06-30)**. `notifications.Service.SendPhoneOptInWelcome`
   (`server/notifications/welcome.go`) renders `sms.welcome.body` (en+es) from the server l10n catalog and
   sends via the platform SMS/RCS channel (Messaging Service → RCS-first, SMS fallback). Hooked at
   `login.PhoneRegister` (after `provisional.PromoteByPhone`) — fires on **first** opt-in only (both in-app
   onboarding and the web event-RSVP flow route here; `PhoneLogin` does not). Best-effort (never blocks
   registration); always targets the phone (never push); bypasses quiet hours (real-time response to the
   user's own action); carries its own STOP/HELP disclosure (no footer appended) and records the disclosure
   timestamp; fails closed on STOP. Self-contained copy ends with a **generic `https://ripls.app`** link
   (the earlier *"Say hi at {item-chat}"* CTA was dropped — no chat-deep-link dependency). Tests: 6 unit
   (`welcome_test.go`) + e2e register→notification→mock-SMS and a login-does-not-welcome negative
   (`phone_auth_test.go`).
3. **RCS provider/sender wiring** — ✅ **DONE & VERIFIED.** `twilio.go` sends via `MessagingServiceSid`, so
   the RCS sender already in the **prod** Messaging Service (`MGa3f2884f…`) gives RCS-first-then-SMS-fallback
   with **no send-code change**. RCS sender `XE841f11e191e65c9c0a49f64a4c19088b` (`rcs:ripls_sm3u0sj8_agent`)
   was attached 2026-06-27; **RCS-first delivery verified 2026-06-30** — msg `SM1a095e4d567dbd2c2226ec00db18ad61`
   to test device `+15555550100` delivered via `rcs:ripls_sm3u0sj8_agent` (status `delivered`). Pre-approval,
   RCS reaches only allowlisted test devices; everyone else gets SMS fallback automatically.
   Note: the sender is live in the _prod_ pool. This was a gating concern (#2609) — **resolved by parking
   cold no-opt-in messaging** (2026-07-01): with no cold sends, approval can't send RCS to un-opted-in
   numbers. ⚠️ **If cold/no-opt-in messaging is un-parked, re-introduce the gating (or pull `XE841…` from the
   pool) before it ships** — otherwise it would send RCS to cold numbers (the TL;DR policy violation).
4. **In-app opt-in screen** — ✅ ToS/Privacy links added beside `phoneAuthConsent` (`phone_auth_screen.dart`, link semantics + Playwright ids); ✅ opt-in proof screenshots refreshed (app + web).
5. **Tests + workflow docs** — update `docs/workflows/*`, add coverage; keep the 1:1 system-test mapping
   (`/ripls-update-system-tests`).

---

## Phased task list

- **Phase A — copy & opt-in prep (now, no fees, also hardens SMS):**
  - [x] Privacy Policy no-sharing tighten + add messaging provider (§3.1/§3.2). **(done 2026-06-27)**
  - [x] ToS carrier-liability + number-change clause **(done 2026-06-27, `5e98d1aad`)**; STOP renders bold ✓.
  - [x] In-app opt-in: ToS/Privacy links added (`phone_auth_screen.dart`) + opt-in proof screenshots refreshed (`28ae56968`).
  - [x] Welcome-message copy **finalized, signed off, and BUILT (2026-06-30)** — `sms.welcome.body` en+es,
    sent on first opt-in (Engineering item 2). Final EN copy (216 chars, GSM-7 clean, blank line before the
    link): *"Welcome to Ripls! Let's lean on each other. You'll get event invites and community updates.
    Message & data rates may apply. Message frequency varies. Reply HELP for help, STOP to opt out.\n\nConnect
    at https://ripls.app"*. **Paste this exact text into the US-registration welcome field** so the declared
    message matches what we send.
  - [x] Adopt `support@ripls.org` across the website legal/consent pages. **(done 2026-06-27)**
- **Phase B — assets & sender profile:**
  - [x] Banner produced (`assets/ripls-rcs-banner-1440x448.png`). · [x] Sender description finalized (approved 2026-06-27).
  - [x] Google use-case + opt-in descriptions drafted (§6) and US-registration data gathered (checklist);
    assembled into the final submission packet 2026-06-30.
- **Phase C — registration:**
  - [x] Google RCS registration + US (Aegis) registration filled, **submitted together (2026-07-01)**.
        Google page 2 used the opt-in-first trigger/use-case copy + the hosted video
        (`ripls.org/assets/rcs-demo.mp4`); US campaign page used the campaign/message-flow copy, the actual
        Advanced Opt-Out samples, and service type **Account notification**.
  - [ ] Aegis email verification (from `user14@example.org`); fees charged at approval. Now in review (~5–9 wk).
- **Phase D — launch & code (after approval):**
  - [x] RCS sender `XE841…` added to the prod Messaging Service; RCS-first delivery verified (2026-06-30).
  - [x] Welcome-on-opt-in send built + deployed (dev+prod); full opt-in→RCS flow verified end-to-end.
  - [x] ~~Per-recipient gating (#2609)~~ **CLOSED 2026-07-01 — not needed while cold no-opt-in messaging is
        parked** (all sends go to opted-in recipients). Reopen #2609 if cold messaging is un-parked.

---

## Future option — SMS short code (true "Partiful parity")

Partiful's cold-invite deliverability comes from a **CTIA short code (`39781`)**, not RCS. If Ripls invite
volume scales, a short code is the real parity move for the **SMS** cold channel: best throughput/
deliverability, but ~**$1k/mo** lease + **8–12 wk** provisioning + its own CTIA vetting. **Deferred** —
the approved 10DLC long code is fine for beta/launch. Track separately if/when volume justifies it.

A related product nicety from Partiful worth considering for SMS (not a compliance item): a **per-host
opt-out** ("Reply X to stop invites from this host") layered on top of the mandatory global STOP — lets a
recipient mute one host without leaving the platform.

---

## Registration inputs (provided 2026-06-27)

All earlier interview items are answered; values feed the [US-registration checklist](#us-registration-data-checklist-gather-before-submitting) above.

- **Authorized rep / brand contact:** A. Maintainer · CTO · **user2@example.org** · **+15555550100**.
- **Legal name / address:** **Type2Fun Foundation, PBC** · **2957 11th Street, Boulder, CO 80304** (confirm exact CP 575 string before submitting).
- **Traffic:** ~tens of thousands monthly `ripls.org` visits; **0–5k RCS messages/month** initially; no short code.
- **Banner:** `website/content/assets/ripls-rcs-banner-1440x448.png` (1440×448).
- **`/go` domain:** ✅ both `ripls.app` and `ripls.org` registered as owned domains on Twilio — the link-domain split is covered.
- **Support address:** ✅ `support@ripls.org` adopted across the website legal/consent pages.

> ℹ️ **Entity conversion mop-up (non-blocking for RCS).** The entity converted **LLC → PBC, effective
> 2026-06-15**. The Twilio A2P 10DLC **brand is already approved as "Type2Fun Foundation, PBC"** (our
> earlier SMS-1 doc mis-recorded it as "LLC"), so **no brand re-vetting is needed and RCS is not
> blocked**. Remaining propagation of the new name/address where the old entity may linger (all
> non-urgent): verify the **A2P brand's address** is the current Boulder address; the **GCP billing
> account** (`Type2Fun, LLC Billing`); the **app-store dev account**; **IRS / SOS / CP-575**; and
> **bank / Stripe**.

## Submission packet — paste-ready field values (assembled 2026-06-30)

Exact verbiage to paste into the Twilio Console. Char counts are vs. the field limits noted.
Flow: **Messaging → RCS → create Sender/agent profile**, then **Register → Google + United States**,
submitted together. Field labels drift between Console versions — map by content.

### Block 1 — RCS Sender / agent profile
| Field | Value |
|---|---|
| Display name | `Ripls` |
| Description (92/100) | `Event invites, RSVPs, and community activity updates for the groups you're part of on Ripls.` |
| Logo | `website/content/assets/logo.png` |
| Banner (1440×448) | `website/content/assets/ripls-rcs-banner-1440x448.png` |
| Privacy Policy URL | `https://ripls.org/privacy-policy.html` |
| Terms of Service URL | `https://ripls.org/sms-terms.html` |
| Contact email | `support@ripls.org` |
| Contact phone (E.164) | `+15555550100` |
| Website | `https://ripls.org` |
| Messaging Service | prod `MGa3f2884f…` ("Ripls Invites and Updates Service") — RCS sender `XE841…` + SMS `+15555550100` already in pool |

### Block 2 — Google RCS registration

*Second page has three required fields — see the exact copy below (opt-in-first, no cold host-invite path).*

**What triggers messages (794/1000):**
```
A recipient receives messages only after they have taken a first-party action on Ripls — they get nothing until they act. The FIRST message is triggered by the person's own action: they RSVP to an event, or verify their phone to join. Once they have opted in this way, later messages are triggered by activity on the specific things they are part of: someone RSVPs to their event, a neighbor offers an item for their request, a host posts an update in a group they joined, or a reminder fires for an event they said they would attend. Triggers are user actions inside Ripls, not external systems (there are no package-delivery-style automated events). There is no fixed date/time or recurring schedule; message frequency varies with the recipient's own activity and the groups they have joined.
```

**Use case description (838/1000):**
```
Ripls is a community app for organizing events and sharing goods among people who already know each other. All messaging is transactional and relationship-based, tied to a specific shared event or item — never marketing or promotional.

PRIMARY interactions (most common): event invitations and RSVP confirmations; and community-activity updates on things you are part of (a new RSVP, a reminder before an event, a neighbor's offer on your request).

SECONDARY interactions (possible but less common): giveaway and gear offers, a one-time welcome/opt-in confirmation when a person first verifies their phone, and lightweight event logistics.

Recipients are always connected to the sender through a shared event, request, or community they were invited to or joined. Every message lets the user reply STOP to opt out, honored immediately.
```

**Use case video URL — ⚠️ REQUIRED, to be produced.** ~30–90s screen capture: opt-in/consent (`ripls.app/go/<code>` → "I'm in" → consent screen with Terms/Privacy → number → code) → branded RCS welcome in Google Messages → opt-out (STOP → confirmation). Host at a public URL (unlisted YouTube / public GCS / `ripls.org/assets/`). Use a number not already a Ripls user. See §6 for the full shot-list.

**Opt-in description (994/1000):**
```
Ripls collects express, per-person opt-in when someone gives their mobile number to receive event invites and community updates from groups they belong to. Both paths show the same consent disclosure, with links to our Terms (ripls.org/sms-terms.html) and Privacy Policy (ripls.org/privacy-policy.html), BEFORE the number is submitted:

1) Ripls mobile app: from sign-in the user chooses "continue with phone," lands on the phone-entry screen, reads the disclosure (agreeing to receive event invites and community updates from Ripls; frequency varies; message and data rates may apply; reply HELP for help, STOP to opt out), enters their number, and taps Send Code, then enters the one-time code to finish.

2) Ripls web app at ripls.app: the same phone-entry screen and disclosure appear; the user enters their number, requests a code, and enters it to finish.

Opt-in is not transferable; we never message a number that did not itself complete this flow. Summary at ripls.org/sms-opt-in.html.
```

**Opt-in policy image URL:** `https://ripls.org/assets/sms-opt-in-app.png`
*(literal hosted image of the in-app consent screen; `…/sms-opt-in-web.png` and the `…/sms-opt-in.html` page are also live)*

**Opt-out description (300/1000):**
```
Users opt out anytime by replying STOP (HELP for help, START to resubscribe); these keywords are honored on every message, and Google Messages also surfaces an Unsubscribe control. On opt-out the user receives: "You're unsubscribed from Ripls and won't get more messages. Reply START to resubscribe."
```

**Agent access Instructions (791/1000)** — exercises the real review invite (event-RSVP flow) end-to-end;
the welcome send is live (Engineering item 2):
```
Reviewers can exercise the full opt-in flow and receive a message:
1) On a phone, open this Ripls event invitation: https://ripls.app/go/Md5UQpRB
2) Tap "I'm in" on the event page. You're taken to "Confirm your phone to RSVP," which shows the consent disclosure with links to our Terms (ripls.org/sms-terms.html) and Privacy Policy (ripls.org/privacy-policy.html). Enter your number, request the verification code, and enter it to complete opt-in.
3) On completing opt-in you immediately receive Ripls' welcome message from this sender — delivered as rich RCS on devices allowlisted as testers for this agent, and as SMS otherwise.
To allowlist a specific device for rich RCS, contact A. Maintainer (user2@example.org, +15555550100). Opt-in is also documented at ripls.org/sms-opt-in.html.
```

### Block 3 — United States registration (Aegis)
| Field | Value |
|---|---|
| Legal business name | `Type2Fun Foundation, PBC` *(confirm exact CP-575 punctuation)* |
| Brand / DBA | `Ripls` |
| EIN | on file (A2P Brand `BN4541bb2fd18e004461d814fbedb585ca`) |
| Address | `2957 11th Street, Boulder, CO 80304` |
| Company type | **private corporation** (a PBC is for-profit — do **NOT** pick "non-profit") · Technology / Social |
| Authorized rep | `A. Maintainer · CTO · user2@example.org · +15555550100` |
| Brand contact phone | `+15555550100` |
| Expected volume | `0–5k messages/month` initially; no short code |
| Opt-in policy URL | `https://ripls.org/sms-opt-in.html` + app/web screenshots |

**Sample messages (3):**
```
Ripls: Sarah invited you to "Backyard BBQ" this Saturday. Tap to RSVP: ripls.app/go/AB12CD34
Ripls: Alex offered their camping tent for your request. View: ripls.app/go/EF56GH78
Ripls: Reminder — "Tool Library Meetup" starts in 2 hours. Details: ripls.app/go/IJ90KL12
```

**Keyword responses:**
```
These are the ACTUAL Twilio Advanced Opt-Out auto-replies (from live test messages) — what a reviewer sees:
HELP:   This number sends Ripls updates. Reply STOP to unsubscribe. Msg&Data Rates May Apply
STOP:   You have successfully been unsubscribed from Ripls text notifications. You will not receive any more messages from this number. Reply START to resubscribe.
START:  You have successfully been re-subscribed to Ripls messages from this number. Reply HELP for help. Reply STOP to unsubscribe. Msg&Data Rates May Apply.

Message service type submitted: Account notification.
```

**Welcome / opt-in-confirmation message (216 EN, GSM-7 clean)** — this is the *exact* text the system sends
(`sms.welcome.body`), so the declared message matches production. Note the blank line before the link:
```
Welcome to Ripls! Let's lean on each other. You'll get event invites and community updates. Message & data rates may apply. Message frequency varies. Reply HELP for help, STOP to opt out.

Connect at https://ripls.app
```
*If the field's helper text demands an explicit inline customer-care contact, append `or email support@ripls.org`
after "Reply HELP for help" — but then update `sms.welcome.body` (en+es) to match so sent == declared.*

## Still open (work items)

- ✅ **ToS** — carrier-liability + neutral number-change clause **added** (`5e98d1aad`).
- ✅ **In-app opt-in** — ToS/Privacy links added (`phone_auth_screen.dart`) + opt-in proof screenshots refreshed (app + web, `28ae56968`).
- ✅ **Welcome message** — finalized, signed off, and **BUILT** (2026-06-30): *"Welcome to Ripls! Let's lean
  on each other. You'll get event invites and community updates. Message & data rates may apply. Message
  frequency varies. Reply HELP for help, STOP to opt out.\n\nConnect at https://ripls.app"* (216 chars EN,
  GSM-7 clean). Send shipped (Engineering item 2); no chat-deep-link dependency (generic base-app link).
- ✅ **Sender description** (approved 2026-06-27): *"Event invites, RSVPs, and community activity updates for the groups you're part of on Ripls."*
- ✅ **Google use-case + trigger copy** — finalized opt-in-first (no cold host-invite path), 2026-06-30 (§6 / packet Block 2).
- ⬜ **Use-case video — REQUIRED, to produce.** Blocks completing the Google registration's second page. ~30–90s
  screen capture of opt-in → RCS welcome → STOP opt-out, hosted at a public URL (see §6 shot-list). Record with a
  number that isn't already a Ripls user.
- ⬜ **Submit** Google + US registrations together, then **land #2609 (gating) before approval**.

---

## Sources (research, 2026-06-27)

- [Google RBM Acceptable Use Policy](https://developers.google.com/business-communications/rcs-business-messaging/terms-and-policies/aup)
  — opt-in before soliciting; explicit-opt-in-only; no selling/distributing numbers.
- [CTIA Messaging Principles & Best Practices (May 2023)](https://api.ctia.org/wp-content/uploads/2023/05/230523-CTIA-Messaging-Principles-and-Best-Practices-FINAL.pdf)
  — §5.1.2 opt-in required; §5.1.2.2 non-transferable/sender-specific; §5.1.4 no shared lists.
- [FCC 15-72 (TextMe/Glide; footnote 130; GroupMe ¶48–49)](https://docs.fcc.gov/public/attachments/FCC-15-72A1.pdf)
  — host-as-maker is a TCPA liability question; intermediary-consent clarification was withdrawn.
- [Holland & Knight — Beyond TCPA: CTIA Messaging Principles (2026)](https://www.hklaw.com/en/insights/publications/2026/05/beyond-tcpa-compliance-why-ctia-messaging-principles)
  — carrier vetting is a separate regime from TCPA; documented consent expected.
- [Sinch — opt-in for RCS / SMS opt-in carries to RCS](https://community.sinch.com/t5/RCS/Is-opt-in-needed-to-send-RCS-messages/ta-p/16547)
- [Twilio — US RCS guidelines (opt-in flow reviewed)](https://www.twilio.com/en-us/guidelines/us/rcs)
- [Partiful — what number invites come from (short code 397-81)](https://help.partiful.com/hc/en-us/articles/37684658209051-What-number-do-Partiful-invite-texts-come-from)
- [Partiful — verified iMessage (Apple Messages for Business, opt-in, iPhone-only)](https://help.partiful.com/hc/en-us/articles/34049238292891-How-can-I-opt-into-verified-iMessage-notifications)
- [A2P brand best-practices — legal-entity match + DBA footer](https://help.gohighlevel.com/support/solutions/articles/155000000508-a2p-10dlc-brand-approval-best-practices)
