---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Phone-first pivot workflow for gear (epic #2492, WEB-4) — a non-app guest opens a shared-gear invite link, views the item on the web, registers via Firebase phone OTP, and their ExpressInterest auto-fires as they join the gear's ad-hoc community and any pre-seeded provisional identity is promoted to their real account.
  globs: [server/services/web/gear_page.go, server/services/transfer/interest.go, server/services/transfer/queries.go, app/lib/presentation/screens/web/web_item_action_screen.dart, app/lib/presentation/viewmodels/web_action_handoff_view_model.dart, app/lib/presentation/screens/auth/phone_auth_screen.dart, e2e/tests/workflows/phone-gear-interest-full-loop.spec.ts, e2e/tests/workflows/phone-gear-giveaway-full-loop.spec.ts]
  triggers: [phone-first, phone-otp, provisional, promote-on-verify, web-gear, express-interest, borrow, giveaway, invite-link, pivot, auth-emulator]
  lens: [workflow, domain, client, server]
  domain: registration
freshness:
  verified_commit: "7bc0dc33f"
  verified_on: "2026-06-30"
---
# Phone-First Invite → Verify → Express Interest (Gear) Workflow

The phone-first pivot (epic #2492; canonical plan `docs/issues/2492-phone-first-pivot.md`)
lets a person with **no Ripls account and no app** receive an invite to a shared item, respond on the
**web**, verify ownership of their **phone**, and become a real member — all without installing the
app. This doc describes that loop for **gear** (a loan or a giveaway, WEB-4) — the non-RSVP analog of
[phone_first_rsvp.md](./phone_first_rsvp.md), reusing the same identity / link / promote machinery.

## The pieces this composes

- **Provisional identity (ID-1)** + **promote-on-verify (ID-2)** — identical to the event loop: a
  contact-keyed placeholder member is claimed by the real account on phone registration
  (`provisional.PromoteByPhone`, `server/provisional/promote.go`).
- **Per-item ad-hoc community** — `SaveGear` provisions the gear's nameless per-item community and
  shares the gear into it (defaulting `FOR_LOAN`) on create (#2492), so a new gear is born with its own
  audience. Asserted by `e2e/tests/workflows/item-creation-adhoc-community.spec.ts` (ad-hoc community +
  host can invite members).
- **The SSR gear landing (WEB-4, this work)** — `server/services/web/gear_page.go` renders the public
  `/go/{code}` page for a `gear_id` or `transfer_id` target: hero + a single primary CTA that adapts
  to **"Ask to borrow"** (loan) or **"I want this"** (giveaway), linking to
  `/item/{gearId}?intent=interest&code={code}`. A transfer-flavored link resolves to its gear.
- **The web gear action handoff (WEB-4, this work)** — `WebItemActionScreen` (`/item/{gearId}`) +
  `WebGearInterestHandoffNotifier` capture the intent and auto-fire `ExpressInterest` after auth +
  community join. **Auth-method-agnostic**, mirroring the event handoff.
- **No server RPC change** — `ExpressInterest` (`server/services/transfer/interest.go`) derives the
  community + transfer type from the gear's shared communities (`findMatchingCommunityAndType`,
  `queries.go`); an ad-hoc (nameless) community satisfies it like any other (no name-presence filter).

## How the loop runs

0. A host **creates a gear item** and shares it for **loan** or **giveaway**. `SaveGear` provisions
   the gear's **per-item community** and shares the gear in; the share sheet offers the `/go/{code}`
   link. The host may pre-invite the guest by phone (a **provisional user** in that community).
1. The guest opens `/go/{code}` → the SSR gear landing (`server/services/web/gear_page.go`) shows the
   item, owner, and a single CTA — **Ask to borrow** (loan) / **I want this** (giveaway).
2. Tapping the CTA routes to the Flutter web app at `/item/{gearId}?intent=interest&code={code}`.
   Unauthenticated, the guest is sent to the phone-first verify screen (`/verify-phone`), which shows
   **"Confirm your phone to claim {item}"**; the `gear_id`, `intent`, and `code` thread through.
3. The guest registers via **phone OTP** (`PhoneAuthScreen`): phone → code → name → `PhoneRegister`.
4. Server: `PhoneRegister` creates the `AUTH_METHOD_PHONE` user and runs `provisional.PromoteByPhone`
   — claiming the guest's provisional placeholder(s) and merging history.
5. The web app returns to `/item/{gearId}?intent=interest&code={code}`; now authenticated,
   `WebItemActionScreen` joins the gear's community (idempotent `AcceptInvitationLink`) and the handoff
   **auto-fires `ExpressInterest`** — a loan auto-selects the guest as recipient; a giveaway records
   their interest for the owner to choose.
6. The guest is now a real member with a transfer started — no app install.

## Workflow Example — Guest borrows a shared gear via phone OTP

Exercised by the Playwright e2e (`e2e/tests/workflows/phone-gear-interest-full-loop.spec.ts`) against
the **Firebase Auth Emulator** (deterministic OTP). The host's gear is RPC-seeded off-camera
(`SaveGear` carrying `availability: FOR_LOAN`); the unit under test is the guest's phone-first loop. The
**giveaway** twin (`phone-gear-giveaway-full-loop.spec.ts`) seeds `FOR_GIVEAWAY`, drives the
**"I want this"** CTA (aria "Tell {owner} you want {item}"), and asserts the guest lands in the
transfer context's `pending_requests` (`INTEREST_EXPRESSED`) rather than on `transfer.recipient` — a
giveaway does not auto-select a recipient.

### Preconditions

- A host is registered and has shared a gear item for loan (its per-item community provisioned).
- The guest is not registered and has no app; the host pre-invited the guest by phone (a provisional
  user in the gear's community).

### Steps

1. **Host** (RPC seed): `SaveGear` → per-item community, carrying **FOR_LOAN** into that
   community's first share (availability is item-wide, #2687); mint the `/go/{code}` gear
   link; `CreateProvisionalUser` for the guest's phone.
2. **Guest**: opens `/go/{code}` → sees the SSR gear landing.
3. **Guest**: taps **Ask to borrow** → lands on `/item/{gearId}?intent=interest&code={code}` → routed
   to `/verify-phone` ("Confirm your phone to claim {item}").
4. **Guest**: enters phone, receives + enters the OTP, enters a name, taps **Finish**.
5. **Server**: `PhoneRegister` creates the user and promotes the matching provisional placeholder.
6. **Client**: returns to the gear view; joins the community + auto-fires `ExpressInterest`.

### Postconditions

- The guest is a real `AUTH_METHOD_PHONE` user and a member of the gear's community.
- The seeded provisional user is `claimed` by the new account (history merged).
- A **transfer** exists for the gear with the guest as recipient (a loan is auto-selected to
  `RECIPIENT_SELECTED`; a giveaway sits at `INTEREST_EXPRESSED` awaiting the owner's choice).
- No platform SMS was sent (OTP via the emulator in tests; real Firebase phone auth in prod).
