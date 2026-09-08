---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Phone-first pivot workflow for requests (epic #2492, WEB-3) — a non-app guest opens a shared-request invite link, views the ask on the web, registers via Firebase phone OTP, and their OfferToFulfill auto-fires as they join the request's ad-hoc community and any pre-seeded provisional identity is promoted to their real account.
  globs: [server/services/web/request_page.go, server/services/request/offers.go, app/lib/presentation/screens/web/web_item_action_screen.dart, app/lib/presentation/viewmodels/web_action_handoff_view_model.dart, app/lib/presentation/screens/auth/phone_auth_screen.dart, e2e/tests/workflows/phone-request-offer-full-loop.spec.ts]
  triggers: [phone-first, phone-otp, provisional, promote-on-verify, web-request, offer-to-fulfill, help, invite-link, pivot, auth-emulator]
  lens: [workflow, domain, client, server]
  domain: registration
freshness:
  verified_commit: "7bc0dc33f"
  verified_on: "2026-06-30"
---
# Phone-First Invite → Verify → Offer to Help (Request) Workflow

The phone-first pivot (epic #2492; canonical plan `docs/issues/2492-phone-first-pivot.md`)
lets a person with **no Ripls account and no app** receive an invite to a neighbor's request, respond
on the **web**, verify ownership of their **phone**, and become a real member — all without installing
the app. This doc describes that loop for **requests** (WEB-3) — the offer-to-help analog of
[phone_first_rsvp.md](./phone_first_rsvp.md), reusing the same identity / link / promote machinery.

## The pieces this composes

- **Provisional identity (ID-1)** + **promote-on-verify (ID-2)** — identical to the event loop: a
  contact-keyed placeholder member is claimed by the real account on phone registration
  (`provisional.PromoteByPhone`, `server/provisional/promote.go`).
- **Per-item ad-hoc community** — `SubmitRequest` provisions the request's nameless per-item community
  and shares the request into it on create (#2492). Asserted by
  `e2e/tests/workflows/item-creation-adhoc-community.spec.ts` (ad-hoc community + host can invite members).
- **The SSR request landing (WEB-3, this work)** — `server/services/web/request_page.go` renders the
  public `/go/{code}` page for a `request_id` target: hero + a single **"Offer to help"** CTA linking
  to `/need/{requestId}?intent=offer&code={code}`.
- **The web request action handoff (WEB-3, this work)** — `WebItemActionScreen` (`/need/{requestId}`) +
  `WebRequestOfferHandoffNotifier` capture the intent and auto-fire `OfferToFulfill` after auth +
  community join. **Auth-method-agnostic**, mirroring the event handoff.
- **No server RPC change** — `OfferToFulfill` (`server/services/request/offers.go`) requires the caller
  to be an active member of a community the request is shared with; the request's ad-hoc community
  satisfies that like any other (no name-presence filter). Unlike `ExpressInterest`, `OfferToFulfill`
  needs an **explicit `community_id`** — the handoff resolves it from the share code
  (`CheckInvitation`), exactly as the RSVP handoff does.

## How the loop runs

0. A host **submits a request**. `SubmitRequest` (with no extra community) provisions the request's
   **per-item community** and shares the request in; the share sheet offers the `/go/{code}` link. The
   host may pre-invite the guest by phone (a **provisional user** in that community).
1. The guest opens `/go/{code}` → the SSR request landing (`server/services/web/request_page.go`) shows
   the request, the requester, and a single **Offer to help** CTA.
2. Tapping the CTA routes to the Flutter web app at `/need/{requestId}?intent=offer&code={code}`.
   Unauthenticated, the guest is sent to the phone-first verify screen (`/verify-phone`), which shows
   **"Confirm your phone to help with {item}"**; the `request_id`, `intent`, and `code` thread through.
3. The guest registers via **phone OTP** (`PhoneAuthScreen`): phone → code → name → `PhoneRegister`.
4. Server: `PhoneRegister` creates the `AUTH_METHOD_PHONE` user and runs `provisional.PromoteByPhone`
   — claiming the guest's provisional placeholder(s) and merging history.
5. The web app returns to `/need/{requestId}?intent=offer&code={code}`; now authenticated,
   `WebItemActionScreen` joins the request's community (idempotent `AcceptInvitationLink`), resolves the
   community from the share code, and the handoff **auto-fires `OfferToFulfill`**.
6. The guest is now a real member who has offered to help — no app install.

## Workflow Example — Guest offers to help on a request via phone OTP

Exercised by the Playwright e2e (`e2e/tests/workflows/phone-request-offer-full-loop.spec.ts`) against
the **Firebase Auth Emulator** (deterministic OTP). The host's request is RPC-seeded off-camera
(`SubmitRequest`); the unit under test is the guest's phone-first loop.

### Preconditions

- A host is registered and has submitted a request (its per-item community provisioned, request shared
  in).
- The guest is not registered and has no app; the host pre-invited the guest by phone (a provisional
  user in the request's community).

### Steps

1. **Host** (RPC seed): `SubmitRequest` → per-item community + request shared in; mint the `/go/{code}`
   request link; `CreateProvisionalUser` for the guest's phone.
2. **Guest**: opens `/go/{code}` → sees the SSR request landing.
3. **Guest**: taps **Offer to help** → lands on `/need/{requestId}?intent=offer&code={code}` → routed
   to `/verify-phone` ("Confirm your phone to help with {item}").
4. **Guest**: enters phone, receives + enters the OTP, enters a name, taps **Finish**.
5. **Server**: `PhoneRegister` creates the user and promotes the matching provisional placeholder.
6. **Client**: returns to the request view; joins the community + auto-fires `OfferToFulfill`.

### Postconditions

- The guest is a real `AUTH_METHOD_PHONE` user and a member of the request's community.
- The seeded provisional user is `claimed` by the new account (history merged).
- The request has a **RequestOffer** from the guest and transitions `ACTIVE → OFFERS_RECEIVED`.
- No platform SMS was sent (OTP via the emulator in tests; real Firebase phone auth in prod).
