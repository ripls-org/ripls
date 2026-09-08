# notifications/sms

Thin Twilio REST client for platform-originated text messages, plus inbound
webhook signature validation. No SDK — a single authenticated `POST` to the
Messages API and an HMAC-SHA1 check are the entire Twilio surface the platform
needs, so this mirrors the `server/location` Mapbox/Google thin clients rather
than pulling `twilio-go` (which carries deprecated transitive deps and typed
clients for Voice/Verify/Lookups we don't use).

## Key files

- `twilio.go` — `TwilioSMSSender` (sends via a **Messaging Service SID**, which
  routes to the registered sender pool + A2P campaign) and `ValidateSignature`
  (verifies `X-Twilio-Signature` on inbound STOP/HELP/START webhooks).
- `mock.go` — `MockSMSSender`, the dev/test stand-in that records sends.

## Isolation boundary

`TwilioSMSSender` satisfies `notifications.SMSSender` **structurally** — it does
not import the `notifications` package, and no Twilio type leaves this directory.
The notification service depends only on the interface, so swapping providers
(or adopting the SDK later) is contained to this package. PII discipline: the
recipient number is masked (`logging.MaskPhone`) and the message body is never
logged.

## When to add code here vs. elsewhere

Message *content* and channel *selection* (who to text, what to say) live in the
`notifications` package and the services that trigger notifications — not here.
This package only knows how to put a given string on a given E.164 number and
how to authenticate an inbound Twilio request. Opt-out state lives in
`server/smsoptout`.
