---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Phone-first pivot workflow (epic #2492) — a non-app guest opens a per-item invite link, views the event on the web, registers via Firebase phone OTP, and their RSVP auto-fires as they join the community and any pre-seeded provisional identity is promoted to their real account.
  globs: [server/services/login/phone_auth.go, server/provisional/**, server/services/community/provisional_users.go, server/services/web/event_page.go, app/lib/presentation/screens/auth/phone_auth_screen.dart, app/lib/presentation/screens/experience/web_experience_screen.dart, app/lib/presentation/viewmodels/web_event_rsvp_handoff_view_model.dart, e2e/tests/workflows/**]
  triggers: [phone-first, phone-otp, provisional, promote-on-verify, web-rsvp, invite-link, pivot, auth-emulator]
  lens: [workflow, domain, client, server]
  domain: registration
freshness:
  verified_commit: "11503e4ea"
  verified_on: "2026-07-03"
---
# Phone-First Invite → Verify → RSVP Workflow

The phone-first pivot (epic #2492; canonical plan `docs/issues/2492-phone-first-pivot.md`)
lets a person with **no Ripls account and no app** receive an invite to an item, respond on the
**web**, verify ownership of their **phone**, and become a real member — all without installing the
app. This doc describes that loop for **events** (the v1 scope, WEB-2) and is the source of truth the
phone-first Playwright e2e exercises.

## The pieces this composes (all shipped earlier in the epic)

- **Provisional identity (ID-1)** — a contact-keyed (`phone`/`email`) placeholder member of a
  community, created by a host (`CreateProvisionalUser`).
- **Promote-on-verify (ID-2)** — on phone registration, `provisional.PromoteByPhone`
  (`server/provisional/promote.go`) claims every unclaimed provisional placeholder for that
  E.164 across communities, joins the real account to each, and merges activity history.
- **Ad-hoc / open invite links (COMM-1 / LINK-1)** — the per-item `ShareLink` + `/go/{code}` SSR
  landing; nameless (ad-hoc) communities render a graceful public label.
- **Web phone auth (WEB-1)** — Firebase phone OTP hardened for Flutter Web.
- **The web event-RSVP handoff (pre-existing)** — `WebExperienceScreen` +
  `WebEventRsvpHandoffNotifier` capture a pre-verify RSVP intention and auto-fire it after auth +
  community join. **Auth-method-agnostic** — fires identically after phone registration.

## How the loop runs

0. A host **creates an event** through the unified-create web UI (the two-phase Save-only flow,
   #2492): open create → Text → prompt → Generate → **Save Event**. On save the server provisions the
   event's **per-item community** and the share sheet auto-opens (`/go/{code}` link). See
   [experience.md](./experience.md#two-phase-creation--the-per-item-community-2492).
1. The host now has an event in its per-item community and a per-item share link (`/go/{code}`). The
   host may have pre-invited the guest by phone, creating a **provisional user** keyed to that number
   in that per-item community.
2. The guest opens `/go/{code}` → the SSR event landing (`server/services/web/event_page.go`) shows
   host / when / where / who's coming, with **I'm in / Maybe / I'm out** CTAs.
3. Tapping **I'm in** routes to the Flutter web app at `/event/{id}?rsvp=yes&code={code}`. Unauthenticated,
   the guest is sent to register; the `experience_id`, `rsvp`, and `code` are threaded through.
4. The guest registers via **phone OTP** (`PhoneAuthScreen`): enter phone → receive code → enter code
   → enter name → `PhoneRegister`.
5. Server: `PhoneRegister` creates the `AUTH_METHOD_PHONE` user, joins the invite link's community,
   and runs `provisional.PromoteByPhone` — claiming the guest's provisional placeholder(s) and merging
   history.
6. The web app returns to `/event/{id}?rsvp=yes&code={code}`; now authenticated, `WebExperienceScreen`
   joins the community (idempotent) and the handoff **auto-fires `RSVPToExperience`**.
7. The guest is now a real member who has RSVP'd — no app install.
8. The loop compounds (#2630): as a member of the event's per-item community, the guest can open the
   Who's-In roster's **Invite** action and get **their own `/go/{code}` reshare link** (`ShareItem`
   link-only calls are member-allowed; the share sheet hides the host-only audience rows for them).

## Workflow Example — Guest RSVPs to an event via phone OTP

Exercised by the Playwright e2e (`e2e/tests/workflows/phone-rsvp-full-loop.spec.ts`) against the
**Firebase Auth Emulator** (deterministic OTP) and a **deterministic in-process AI provider**
(`--mock-ai-provider`, so the host can drive the real unified-create UI without a live LLM); see
`docs/issues/2492-web2-pivot-e2e.md`.

### Preconditions

- A host is registered (the spec then drives the rest through the web UI).
- The guest is not registered and has no app.

### Steps

1. **Host** (web UI): creates an event via the unified-create flow (prompt → Generate → **Save
   Event**); the server provisions its per-item community and the share sheet auto-opens. The spec
   reads the event + its per-item community back via `ListMyExperiences`, mints the `/go/{code}` link,
   and pre-invites the guest by phone (a **provisional user** in the per-item community).
2. **Guest**: opens `/go/{code}` → sees the SSR event landing.
3. **Guest**: taps **I'm in** → lands on `/event/{id}?rsvp=yes&code={code}` → routed to register.
4. **Guest**: chooses **Register with Phone**, enters the phone number, receives + enters the OTP,
   enters a name, submits.
5. **Server**: `PhoneRegister` creates the user, joins the community, promotes the matching
   provisional placeholder.
6. **Client**: returns to the event view; joins the community + auto-fires the RSVP.
7. **Guest** (now a member): opens the event's **Who's In** roster → **Invite** → the share sheet
   loads their own `/go/` reshare link (#2630).

### Postconditions

- The guest is a real `AUTH_METHOD_PHONE` user and a member of the community.
- The seeded provisional user is `claimed` by the new account (its history merged).
- The event's `rsvpYesCount` reflects the guest's RSVP.
- The guest's share sheet shows a `/go/` link with Copy/Share, and **no** "Invite people" /
  "Invite community" rows (audience management stays host-only — #2630).
- No OTP is delivered via platform SMS — phone verification rides Firebase (the Auth Emulator in
  tests; real Firebase phone auth in prod). In **prod**, `PhoneRegister` now also fires the one-time
  opt-in **welcome** SMS/RCS on the platform channel (best-effort, #2492; see
  [registration_and_login.md](../registration_and_login.md) and
  [push_notifications.md](../push_notifications.md)); in this e2e that send is suppressed because the
  guest is a simulation identity (#2588), so no provider is reached.
