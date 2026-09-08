---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Platform-originated SMS — the thin Twilio provider behind the SMSSender seam, the off-app SMS channel for deviceless recipients, the per-number opt-out store, the STOP/HELP/START webhook, the outbound delivery status-callback handler, l10n copy, the secret/feature-flag wiring that keeps it inert until A2P clears, and the as-approved A2P 10DLC campaign registration record (the reuse template for RCS).
  globs: [server/notifications/sms/**, server/notifications/off_app_sms.go, server/notifications/off_app_email.go, server/smsoptout/**, server/quiethours/**, server/services/web/sms_webhook.go, server/services/web/sms_status.go]
  triggers: [sms, text-message, twilio, opt-out, stop-help-start, a2p, 10dlc, messaging-service, platform-sms]
  lens: [server, domain]
  domain: notifications
freshness:
  verified_commit: "9a2e20c14"
  verified_on: "2026-08-12"
---
# Platform SMS Notifications

Platform-originated text messages let the server reach a recipient who has **no
active app device** but **does** have a phone number — the SMS counterpart to the
off-app email channel ([`docs/server/l10n.md`](server/l10n.md) defines the same
client/server render boundary for both). It is provider-backed by Twilio and
gated behind the `-platform-sms-enabled` feature flag, currently **off** (A2P +
consent strategy approved; flips once the off-app email backup lands — see
[`docs/issues/2492-sms-a2p-registration.md`](issues/2492-sms-a2p-registration.md)).

> **Status.** Shipped (#2566). Secrets + Terraform grants applied in dev+prod;
> **outbound delivery and inbound STOP/START/HELP both verified end-to-end
> against real Twilio** (the inbound webhook live on the dev deployment's `/sms/webhook`,
> signature-validating). Consent/attestation strategy **approved** (2026-06-27).
> The flag (`-platform-sms-enabled`) stays **off** until the off-app **email
> backup** send is finished — so a deviceless recipient still gets email until
> the flag flips, even though the dispatcher would otherwise prefer SMS for a
> phone handle. Outbound delivery **status callbacks** are complete end to end
> (#2569): handled in code at `POST /sms/status`, posted to by the Messaging
> Service, and counted by `sms_delivery_outcomes_{env}` with the
> `SMS Delivery Failures High` alert over it.

## Architecture at a glance

```
NotifyUser(userID, notification)
  └─ no active device? → dispatchOffApp  (off_app_email.go)
       load recipient, then choose by handle:
       ├─ phone handle → sendOffAppSMS    (off_app_sms.go)
       │     ├─ not dialable?      → fall back to email   (contact.IsDialableE164)
       │     ├─ quiet hours?       → fall back to email   (server/quiethours)
       │     ├─ smsoptout.IsOptedOut? → fall back to email (server/smsoptout)
       │     ├─ render via l10n (notification_content.RenderSMS; chat→sms.notification.body)
       │     └─ SMSSender.SendSMS(toE164, body) → ok? done
       │           └─ TwilioSMSSender (POST Messages.json)  (server/notifications/sms)
       └─ otherwise (no phone / SMS off / SMS didn't send) → sendOffAppEmail

Inbound:  Twilio (opt-out mgmt owns the reply) → POST /sms/webhook → HandleSMSWebhook (server/services/web)
            ├─ verify X-Twilio-Signature (sms.ValidateSignature)
            ├─ STOP/START → smsoptout.Record{OptOut,OptIn}   (persist-only mirror)
            └─ ack with empty <Response/> — no app reply

Status:   Twilio (async delivery lifecycle) → POST /sms/status → HandleSMSStatusCallback (server/services/web)
            ├─ verify X-Twilio-Signature (sms.ValidateSignature)
            ├─ log MessageStatus/ErrorCode by outcome (ERROR failed/undelivered, INFO delivered, DEBUG else)
            └─ ack 204 — observability-only, no state change
```

## Channel selection (deviceless recipients)

`dispatchOffApp` loads the recipient once and **prefers SMS for a phone-bearing
recipient** (the native channel for the phone-first audience), falling back to
email so the person is still reached. SMS falls back to email when: the SMS
channel is off, the recipient has no phone, the number is **not dialable**, it's
the recipient's **quiet hours** (22:00–07:00 local via `User.preferred_timezone`;
real-time sends can't defer like the scheduled dispatcher, so they switch channel
rather than buzz at 3am), the number **opted out**, or the **send failed**. The
not-dialable check (`contact.IsDialableE164`, libphonenumber) catches a
shape-valid-but-fake number — a Firebase Phone Auth test number or any +1
555-area-code number — that passes `NormalizePhoneE164` at sign-up but a carrier
would reject (Twilio 21211). Skipping it before the provider call avoids a
metered send, an ERROR-level provider rejection in the logs, and a wasted retry;
sign-up still accepts such numbers (Firebase test numbers must keep working). If neither channel can deliver
(e.g. opted out of SMS and no email), the notification is suppressed. The flag
gates only whether SMS is *attempted*; with it off, every deviceless recipient
gets email as before.

**Simulation recipients never reach a provider.** Once a channel is selected, a
recipient tagged with `User.simulation_id` (the E2E suite or the load-test
harness register users this way) is suppressed *before* the provider call, on
whichever channel was chosen (`outcome="suppressed_simulation"`). This keeps test
traffic — which uses synthetic, deviceless accounts — from burning the shared
the Mailgun sending domain's daily quota or the metered A2P SMS path (#2588). The check
lives in `sendOffAppSMS`/`sendOffAppEmail` so the recorded channel is the one the
notification actually routed to.

## The provider seam

`notifications.SMSSender` is a one-method interface (`SendSMS(ctx, toE164, body)`).
SMS has **no `UserDevice` row**, so it does not fit the
`providers map[DevicePlatform]Provider` model; it is wired as a separate
`SMSChannel` on the service via `WithSMS(...)`.

`server/notifications/sms.TwilioSMSSender` is the only implementation. It is a
**thin `net/http` client**, not the `twilio-go` SDK — the entire Twilio surface
we need is one authenticated `POST` plus an HMAC-SHA1 signature check, and the
SDK pulls deprecated transitive dependencies and typed clients for products
(Voice/Verify/Lookups) we don't use. The decision mirrors the existing
`server/location` Mapbox/Google thin clients. **No Twilio type escapes the
`sms` package** — swapping providers (or adopting the SDK later) is contained
there.

Sends route through a **Messaging Service SID** (not a bare number), which is
how Twilio attaches the registered sender pool and the approved A2P campaign.

## Opt-out: global, per-number, fail-closed

`server/smsoptout` holds the global opt-out state keyed by **normalized E.164**
(`models.SmsOptOut`, unique index on `phone_number`):

- `IsOptedOut` is consulted **before every send**; an opted-out number is never
  texted, even on behalf of a different host.
- `RecordOptOut` (STOP) / `RecordOptIn` (START) upsert on **transition only**,
  so repeat keywords don't churn the row.
- The send path **fails closed**: if the opt-out check itself errors, the send
  is suppressed rather than risk texting an opted-out number.

## Inbound webhook (persist-only opt-out sync)

`POST /sms/webhook` → `HandleSMSWebhook` (`server/services/web/sms_webhook.go`).
**Twilio's Messaging Service opt-out management owns the carrier-compliant
STOP/HELP/START replies and the authoritative opt-out list** (configured in the
Twilio console). This handler is a **secondary mirror**: it keeps our `smsoptout`
store in sync so the send path can suppress *before* ever calling Twilio, and it
**never sends its own reply** (that would double-text the recipient).

1. **Signature first.** `sms.ValidateSignature` recomputes the HMAC-SHA1
   `X-Twilio-Signature` over the public URL + sorted POST params using the
   Twilio auth token. An invalid signature is `403` **before any state change**,
   so a forged POST cannot toggle anyone's stored preference. An empty auth
   token disables verification (dev/local only — production sets it).
2. **Classify** the keyword from Twilio's opt-out-management `OptOutType` field
   when present, else the first word of the body, against the CTIA keyword sets
   (`STOP/STOPALL/UNSUBSCRIBE/CANCEL/END/QUIT`, `START/YES/UNSTOP`,
   `HELP/INFO`).
3. **Persist** — STOP/START upsert `smsoptout` state; HELP changes nothing;
   unrecognized keywords are a no-op. Every recognized keyword is logged
   with a structured `keyword` field ("sms webhook: synced keyword to
   opt-out store").
4. **Acknowledge** with a bare `<Response></Response>` — no app-level reply, so
   only Twilio's opt-out management texts the recipient.

Enforcement remains correct even before our store knows: Twilio blocks sends to
an opted-out number at the API (error 21610), which our send path logs. The
mirror just lets us suppress earlier and surface opt-out state in-app.

## Localization

SMS is a **server-rendered surface**: all copy goes through the go-i18n catalog
(`server/l10n/source/{en,es}.toml`), never inline `fmt.Sprintf`. The two new
SMS-rendering files (`off_app_sms.go`, `sms_webhook.go`) are registered in
`scripts/check_no_inline_user_strings.js`. Keys:

- `notif.offapp.{kind}.message` / `notif.offapp.{kind}.cta`, `sms.brand_prefix`,
  `sms.optout` — the copy for **community-event** SMS, composed by
  `notification_content.RenderSMS` (`notifications/notification_content/`): the
  `Ripls:` brand prefix (every message), the message, the STOP/HELP footer
  (gated — see below), and a CTA verb on the resolved `/go` link. `kind` is
  mapped from the event type by `kindForEventType`.

  **`.message` is not SMS-specific.** It is the one self-contained sentence for
  that kind, and push renders it too (`Content.PushCopy`), adding only
  `notif.community_event.{kind}.title` as its category label. Off-app adds only
  the CTA. Writing a sentence twice — once as a push body, once as an off-app
  message — is what let the two drift apart, and what let system-generated
  notifications have push copy and no off-app copy at all (#2896).

  **Every event type that can reach a recipient must map to a kind**, including
  the synthetic strings the reminder/nudge producers mint (they are not
  `CommunityEventType` enum members). `notification_content.NotificationEventTypes`
  is the canonical list, `BusEventTypesWithoutCopy` records the enum values that
  deliberately notify nobody, and `TestEveryNotificationEventTypeHasCopy` fails
  the build on anything unaccounted for. An unmapped type at runtime falls
  through to `default` and logs a warn-once
  `"notification copy: event type has no kind mapping"` line — the signal that
  was missing when #2896 shipped a text reading `Ripls:  posted an update in `.

  **No template slot can render blank.** Names reach the renderers from lookups
  that can miss, so `Content.params` substitutes a localized stand-in
  (`notif.standin.*`) for any empty name, and the `default` line picks one of
  four wordings from the names the payload genuinely carries — bottoming out at
  "You have an update on Ripls" when there is no actor and no community.
- `sms.notification.body` — the **chat / legacy fallback** wrapper (community
  events use the keys above): `{{.Message}}{{if .Url}} {{.Url}}{{end}}`.
  `Message` is the already-localized notification content; `Url` is a short
  `/go/{code}` deep link to the notification's entity (experience/gear/request),
  resolved via the community share-link infra (`notifications/deeplink.go` →
  `community.ShortLinkCodeForEntity`), falling back to the bare app URL when no
  entity applies. The same resolved `/go` link is the email channel's CTA URL.
  The `sms.optout` footer is gated to first-message-plus-monthly — see
  [Per-message STOP/HELP disclosure cadence](#per-message-stophelp-disclosure-cadence).

Inbound STOP/HELP/START **auto-replies are not in our catalog** — Twilio's
opt-out management sends those (configured in the Twilio console), so the webhook
renders no reply. The `STOP/HELP` keywords in the outbound footer stay literal
even in translated copy — they are fixed, carrier-recognized SMS commands.

## Per-message STOP/HELP disclosure cadence

The `Reply STOP to opt out, HELP for help.` footer is **not** appended to every
off-app SMS. CTIA does **not** require the reminder *text* on every message; it
requires it in the opt-in disclosure, the welcome / first message of an
interaction, and "conspicuously and frequently" thereafter (the common carrier
best practice is **≈ monthly** for a recurring program). What must hold on *every*
message is that the STOP/HELP **keywords are honored** — which they are, centrally
and independent of the rendered text, via Twilio Advanced Opt-Out + the
[persist-only webhook](#inbound-webhook-persist-only-opt-out-sync) + the
[opt-out store](#opt-out-global-per-number-fail-closed). So the footer is a
**copy** decision, not a functional one.

**Implemented (#2572).** `RenderSMS`'s `includeOptOut` gates the footer:
`shouldDiscloseOptOut` (`off_app_sms.go`) returns true for the recipient's
**first** platform SMS (`User.sms_optout_disclosed_at_unix_sec == 0`) and then
only once `optOutDisclosureInterval` (~monthly) has elapsed; a disclosing send
records the timestamp on the user. The `Ripls:` brand prefix still rides every
message. (RCS drops the footer entirely — see below.)

**RCS differs:** do **not** repeat "Reply STOP" in every RCS message — it defeats
the rich UX. RCS opt-out is surfaced by the in-chat keyword (always works),
suggested-reply opt-out buttons, and Google Messages' built-in Unsubscribe
button; the same Advanced-Opt-Out Messaging Service honors STOP cross-channel.
Only when RCS **falls back to SMS** does the SMS footer cadence apply (the
fallback message is an SMS). Detail:
[`docs/issues/2492-rcs-business-messaging.md`](issues/2492-rcs-business-messaging.md).

Sources: CTIA Messaging Principles & Best Practices (May 2023); Bandwidth "RCS
for Business mobile user opt-out methods"; Google RBM Acceptable Use Policy.

## Observability

- Every SMS dispatch decision logs a structured line with
  `channel="sms"` + `outcome`, queryable in Cloud Logging (the in-process
  Prometheus counters were removed with the `/metrics` endpoint —
  #1613/#1614). Outcomes:
  `sent`, `failed`, `suppressed_opted_out`, `suppressed_no_handle`,
  `suppressed_disabled`, `suppressed_quiet_hours` (SMS-only),
  `suppressed_invalid_number` (SMS-only — number not dialable, e.g. a Firebase
  OTP test number or a 555-area-code number; skipped before the provider call,
  falls back to email), and
  `suppressed_simulation` (recipient is a simulation/test user — the E2E suite or
  load-test harness, tagged via `User.simulation_id`; suppressed before any
  provider call so test traffic never burns the shared Mailgun quota or the
  metered A2P SMS path — #2588; recorded on whichever channel was selected). The
  non-SMS-only outcomes are shared with the email channel.
- Inbound `stop`/`start`/`help` keyword events log with a `keyword` field
  from the webhook handler.
- **Outbound delivery** is observed through **structured logs**, not a Prometheus
  counter (deliverability alerting rides the log-based-metric pipeline that already
  backs `rpc_errors_{env}`; see [`docs/server/observability.md`](server/observability.md)).
  `HandleSMSStatusCallback` (`POST /sms/status`) logs each Twilio status callback
  with `message_sid` / `message_status` / `twilio_code` (the Twilio `ErrorCode`,
  present only on failure) / masked `recipient_phone`, at a level chosen by
  outcome: **ERROR** for `failed`/`undelivered` (trips the "Server Error Logged"
  alert), **INFO** for `delivered`, **DEBUG** for intermediate lifecycle states.
  Those logs feed `sms_delivery_outcomes_{env}` in
  `terraform/modules/monitoring/log_metrics.tf` — a single counter over the two
  terminal outcomes, labelled `message_status` and `twilio_code` — and the
  `SMS Delivery Failures High` policy in `alerts.tf`, which alerts on the
  failed/undelivered fraction of that counter with a minimum-rate floor so one
  failure in a quiet window cannot fire it.
- **PII:** phone numbers are masked in every log (`logging.MaskPhone`); the
  message body is never logged. The status callback carries no `From`/`Body` — its
  only PII is the `To` number, which is masked.

## Configuration & enablement

Three secrets in Secret Manager, in every environment that sends:
`twilio-account-sid`, `twilio-auth-token`, `twilio-messaging-service-sid`. They
flow through `server/entrypoint.sh` (optional fetches) into the `-twilio-*-file`
flags. The channel is constructed only when **all three** are present; the
`-platform-sms-enabled` flag (env `PLATFORM_SMS_ENABLED`) then gates whether a
wired channel actually sends.

**Order matters** when turning it on (don't crash the server): define the flags,
add the entrypoint optional fetches, create the GSM secrets, then uncomment the
Terraform `secretAccessor` grant (`terraform/environments/{dev,prod}/main.tf`).
Granting IAM on a non-existent secret fails `terraform apply`. Full ops runbook,
campaign copy, and the legal brief live in
[`docs/issues/2492-sms-a2p-registration.md`](issues/2492-sms-a2p-registration.md).

## A2P 10DLC campaign registration

Sending application-to-person SMS in North America requires the operator to
register a brand and a campaign with the carriers, and to host the consent
proof pages the registration cites. Both are properties of *who is sending*,
not of this code, so the approved copy, the registration IDs, and the proof
URLs are deployment-specific and are not recorded here.

What the code requires of any such registration: the opt-out keywords are
honored centrally (see *Opt-out* above) regardless of the copy, every message
carries the STOP/HELP disclosure on the cadence described in *Per-message
STOP/HELP disclosure cadence*, and the sender is a Messaging Service rather
than a bare number so the registered sender pool applies.

## Testing

Unit tests fake Twilio with an `httptest` server + a `MockSMSSender`; the opt-out
store and webhook run against a real Postgres test container. They cover request
shaping/auth, error→code mapping (no PII leak), the signature algorithm against
Twilio's worked-example vector, the opt-out state machine, l10n rendering, and the
webhook keyword→state logic including forged-request rejection.

**Against live Twilio (no charge, no real SMS):** `twilio_integration_test.go`
(`//go:build integration`) hits the real Messages API with **Twilio TEST
credentials** and **magic numbers** — `+15005550006` succeeds, `+15005550001`
→ error 21211, `+15005550002` → 21612 — validating request shaping, auth, and
error mapping end-to-end. The credentials come from **Secret Manager** the same
way as every other secret (`twilio-test-account-sid` / `twilio-test-auth-token`
in the dev project; see [`docs/secrets.md`](secrets.md)):
- **Local:** `npm run test:server:integration:sms` (fetches via `fetch_secret.sh <dev-project>`).
- **CI:** `test_go.yaml` fetches them into `TWILIO_TEST_*` in the test step; tolerant of a not-yet-created secret (empty ⇒ self-skip).

Test mode rejects `MessagingServiceSid`, so the sender uses a magic `From` number
there (production routes through the Messaging Service).

**Live send (manual, real SMS):** `twilio_live_test.go` (`TestTwilioLiveSend`,
also `//go:build integration`) drives the real `TwilioSMSSender` through the live
account + **Messaging Service** — the production `MessagingServiceSid` path the
test-credential test can't reach. It **sends a real message** (fractions of a
cent) and is **not** wired into CI; it self-skips unless `TWILIO_LIVE_TO` is set.
Run on demand against a number you own:

```
TWILIO_LIVE_TO=+15551234567 npm run test:server:integration:sms:live
```

(The npm script fetches the live creds from the prod project's Secret Manager; you
supply the recipient.) No app flag needed — both integration tests construct the
sender directly, so they validate delivery without enabling `-platform-sms-enabled`.

Real inbound STOP/HELP/START with a live `X-Twilio-Signature` has been verified
end-to-end against the webhook on the dev deployment's `/sms/webhook` (#2566). The
outbound delivery **status-callback** handler (`POST /sms/status`, #2569) is
covered by `sms_status_test.go` — signature accept/reject, per-status log level,
masked recipient, and the malformed-form path — driven with a captured `slog`
handler injected via `logging.WithLogger`, since the handler records no state and
its only observable output is the log line.

## When to add code here vs. elsewhere

- A different SMS provider → a new implementation of `notifications.SMSSender`
  in `server/notifications/sms` (or a sibling package); nothing else changes.
- *What* to text and *when* → the services that trigger notifications, exactly
  as for push; this subsystem only delivers and records opt-out state.
- New inbound keywords we mirror → `classifyKeyword`. The user-facing
  STOP/HELP/START replies live in Twilio's opt-out-management config, not here.

## Outbound delivery status callbacks (#2569)

Implemented in code. Twilio reports a message's delivery lifecycle
(`queued` → `sent` → `delivered`, or `undelivered`/`failed` with an `ErrorCode`)
asynchronously to a **separate** status-callback URL — distinct from the inbound
`/sms/webhook`; the payload is `MessageSid`/`MessageStatus`/`ErrorCode`/`To` with
no `From`/`Body`. `HandleSMSStatusCallback` (`server/services/web/sms_status.go`,
`POST /sms/status`) signature-verifies it the same way as the inbound webhook and
[logs the outcome](#observability); it records no state and sends no reply
(acks `204`). This closes the gap between "Twilio accepted the send" and "the
carrier actually delivered it", surfacing carrier filtering (30007), opted-out
recipients (21610), and unreachable numbers (30003) that otherwise pass silently.

**Remaining (ops, not code):**

- ✅ Messaging Service **Delivery Status Callback URL** configured to
  `https://<host>/sms/status` (the same host as the inbound webhook —
  the dev host in dev). Next: confirm a real send produces `delivered` (and,
  against a magic-fail number, `failed`) callbacks in the logs — mirroring the
  inbound-webhook end-to-end check.
- ✅ Cloud **log-based metric** (`sms_delivery_outcomes_{env}`, labelled by
  `message_status` and `twilio_code`) + the `SMS Delivery Failures High`
  failure-rate alert, both in `terraform/modules/monitoring`. Built long after
  the handler — see [Observability](#observability) for the shape and why it is
  one counter rather than the email side's two metrics.
- Optional follow-up: feed repeated hard failures back into suppression (with a
  threshold/decay so a transient 30007 doesn't permanently suppress).
