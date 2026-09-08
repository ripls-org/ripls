# contact

Normalization and validation for **off-app contact handles** — the phone
numbers (and, by reference, emails) that identify a contact-keyed
`ProvisionalUser` and that the dispatcher routes to over SMS or email.

## Why this package exists

A phone number can enter the system formatted many ways — `(555) 123-4567`,
`555.123.4567`, `+1 555 123 4567`. If each call site normalized differently, the
same person would dedup as several provisional users and notifications could
route inconsistently. Centralizing the rules here means a handle is canonical no
matter where it enters (item-creation invite, share link, promotion-on-verify).

## Key files

| File | Purpose |
|------|---------|
| `contact.go` | `NormalizePhoneE164` — canonicalize a loosely-formatted number to E.164 (shape validation), or error. `IsDialableE164` — libphonenumber-backed *validity* check that rejects shape-valid-but-fake numbers (Firebase OTP test numbers, +1 555-area-code numbers) before an SMS provider call. |
| `doc.go` | Package comment. |

## When to add code here vs. elsewhere

- **Here:** provider-agnostic parsing/normalization/validation of contact
  handles (phone today; richer phone parsing later if needed).
- **`server/auth`:** email normalization already lives in `auth.NormalizeEmail`
  — call that for email handles rather than duplicating it here.
- **`server/logging`:** PII masking for logs (`MaskPhone`, `MaskEmail`) lives
  with the other redaction helpers, not here.
- **`server/notifications`:** channel selection and actual sending — this
  package only canonicalizes the handle string.
