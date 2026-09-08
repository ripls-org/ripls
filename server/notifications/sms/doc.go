// Package sms delivers platform-originated text messages through Twilio's
// Messages REST API and validates inbound Twilio webhook signatures.
//
// It is a thin net/http client — no SDK — mirroring the location package's
// Mapbox/Google clients. The only Twilio surface the platform needs is a single
// authenticated POST to send a message and an HMAC-SHA1 check on inbound
// webhooks; the official SDK's value (typed clients for Voice/Verify/Lookups)
// does not apply, and it pulls deprecated transitive dependencies.
//
// TwilioSMSSender satisfies the notifications.SMSSender interface structurally,
// so no Twilio type escapes this package: callers depend on the interface, and
// swapping in a different provider (or the SDK) touches only this directory.
package sms
