# Issue #2492: Account-optional, phone-first progressive accounts + SMS-native invitations (Partiful-style pivot)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2492
**Filed:** 2026-06-17 | **Last activity:** 2026-06-17
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** `docs/users/priorities.md`, `docs/proto_conventions.md`, `docs/push_notifications.md`, `docs/registration_and_login.md`, `docs/invitations.md`, `docs/single_player_mode.md`, `docs/client/sharing.md`, `docs/realtime_updates.md`, `docs/community_health.md`, `docs/users/competitor_analysis.md`

> **This is the canonical, living plan for epic #2492.** It consolidates the 15 child
> issues (originally #2493–#2507), which have been **closed** — their content lives here so we
> iterate on one markdown doc rather than across many GitHub issues. The work is still phased and
> ships as **multiple PRs**; progress is tracked in the [Progress](#progress--status) table and
> the per-task checkboxes below. Epic #2492 remains open as the umbrella tracker.

---

## ⏯️ Resume / handoff (updated 2026-07-01)

**Read this first if you're picking the epic back up after a context flush.**

### 🚦 Where things stand right now (the 30-second version)

- **Merged to `main`:** phase one + create/share/phone-first journeys + DISPLAY-1 + **WEB-3/WEB-4**
  (#2560) + **SMS-2/SMS-3** (#2566) + **the off-app email backup, channel enablement, and the NOTIF-1
  server copy rework** (#2572, merged 2026-06-28). The full phone-first arm (events, gear, requests),
  the SMS + email off-app channels, and the reworked off-app copy are all in `main`; SMS was verified
  end-to-end on real Twilio.
- **✅ #2572 (branch `offapp-notif-shortlinks`) is MERGED — off-app backup + send-enablement + copy
  rework, all live on next deploy:** (1) **off-app notifications carry a `/go/{code}` short link** to
  the entity they're about (was the bare homepage) — `community.ShortLinkCodeForEntity` +
  `notifications/deeplink.go`; (2) **EMAIL-1b** — `FindUnclaimedByEmail` + `promoteProvisionalByEmail`
  on `EmailRegister`/`OIDCRegister` (email analog of ID-2; accepted email-shadowing risk, proper
  verified-email fix → **#2571**); (3) **CAN-SPAM postal address filled** (single config value
  `mailgun_postal_address`; the default was corrected Denver→**Boulder** in follow-up #2575) and **both
  off-app channels enabled in Terraform** (`off_app_email_enabled = true`, `platform_sms_enabled = true`,
  dev+prod) — takes effect on next deploy; (4) **NOTIF-1 P1 (server copy rework)** — `notification_content`
  package (`NotifyContent` + per-surface `RenderPush`/`RenderSMS`/`RenderEmail`), `Ripls:`-prefixed
  SMS with entity-inline message + CTA-with-`/go`-link layout, per-kind message/CTA catalog, the
  `&#39;` text/plain fix (`mailgun.go` → `text/template`), and the render-all utility (shipped as
  `server/cmd/notification_examples/`, not the planned `notif_examples/`); (5) entity **renamed
  LLC→PBC** on active legal/consent pages. See [What's left](#-whats-left-the-pending-work).
- **✅ A2P APPROVED + consent + legal all cleared; postal address in place; copy reworked.** With the
  flags flipped, the next deploy turns on platform SMS + email backup with the reworked copy. **Remaining
  NOTIF-1 work is a deferred client picker (P2 device-contacts — 🛑 DEFERRED, host-relay suffices) + a
  product flip (P3 un-gate cold SMS — now PARKED, Bryan 2026-07-01).** **RCS-1 (Phase 4) is SUBMITTED
  (2026-07-01):** Google + US/Aegis registration filed together, now in carrier review (~5–9 wk, fees at
  approval); the welcome-on-opt-in send is built + deployed and RCS-first delivery was verified end-to-end in
  prod; demo video hosted at `ripls.org/assets/rcs-demo.mp4`. **#2609 (per-recipient gating) CLOSED** — moot
  while cold no-opt-in messaging is parked. Canonical detail: `2492-rcs-business-messaging.md`.
- **✅ MERGED — **PR #2589** (2026-06-30) — the in-app "Shared with" surface +
  more e2e + invitee fixes:** (1) **`SharedWithCard`** on the gear + request read shells ("Shared with N
  people" + invited-face cluster → `AccessSheet` / `ItemShareSheet`), so sharing reads the same across all
  item types; server adds **`invited_individuals`** to `GetGearResponse`/`Request` (shared
  `services.FetchInvitedIndividuals`). (2) **After share, the app drops into the new gear/request** (not
  just events) and the **`AccessSheet` title is "Shared with"**, lists the item's own ad-hoc community +
  invited people **by name**, and **updates in place** when a community is added. (3) **Bug fixes (one
  root cause each):** the access surfaces no longer re-filter the server's already-caller-scoped
  `sharedCommunities` against the lagging client `communitiesProvider` (fixed the blank panel **and** the
  silently-failing community-add); request's silent `refreshRequest()` now keeps `sharedCommunities`
  fresh; **a returning member's phone now logs them in** (`PhoneAuthScreen` falls back to `PhoneLogin` on
  `AlreadyExists`) instead of being rejected as a duplicate. (4) **New e2e:** gear/request
  authoring→share→Shared-with, gear-**giveaway** phone-first, returning-member phone login, + a
  `registerPhoneUser` emulator-OTP seed helper. All green locally (`mobile-chromium`); recorded videos show
  the full flows incl. the item hero on the final frame. See [What's left](#-whats-left-the-pending-work)
  items 1 + the e2e note.

### ✅ Phase one + the create / share / phone-first journeys + DISPLAY-1 are all MERGED to `main`

Six integration PRs carry everything **merged**: #2520, #2530, #2527, #2543 (below), plus #2560
(WEB-3/WEB-4) and #2572 (off-app email backup + channel enablement + NOTIF-1 server copy rework).

- **[PR #2520](https://github.com/Type2Fun-Foundation/ripls/pull/2520)** (merged 2026-06-20) — the
  **phase-one foundation stack**: ID-1, COMM-1, ID-2, EMAIL-1 (channel), LINK-1 (server), WEB-1, WEB-2.
- **[PR #2530](https://github.com/Type2Fun-Foundation/ripls/pull/2530)** (merged 2026-06-21) — SMS
  opt-in consent disclosure + SSR `/go/` landing fixes.
- **[PR #2527](https://github.com/Type2Fun-Foundation/ripls/pull/2527)** (merged 2026-06-23) —
  **CREATE-1** (Save-only creation + per-item communities), **INVITE-1** (glass share sheet + Who's-In
  host management), **DISPLAY-1** (partial), **COMM-2** (partial), the **phone-first web onboarding**,
  a load-bearing **cache-coherency fix**, the **marketing-video harness**, and **forced-dark SSR**.
- **[PR #2543](https://github.com/Type2Fun-Foundation/ripls/pull/2543)** (merged 2026-06-25) —
  **DISPLAY-1 full**: group-text **name + `GroupAvatar` cluster** for nameless communities everywhere
  (switcher, AccessSheet, selection pickers, item header, portfolio); the **promote nudge** reworked —
  switcher rows now *select* and show a **"from {item}"** origin label (new server field
  `CommunityItem.origin_item_name`), with naming moved to the **overview header "Name this group" pill**;
  a persistent **Invite pill**; the **pager re-align** fix; and the **light-mode phone-RSVP** theming fix.
  Two follow-ups filed: **#2547** (community gen: drop AI name/description, keep image) and **#2548**
  (per-item community membership policy).

The consolidated [What's left](#-whats-left-the-pending-work) list lives at the end of this section.

The phase-one (#2520) "as built" facts (kept for reference):
- **ID-1** — `ShadowUser`→`ProvisionalUser` rename + contact-keyed handle + idempotent
  `MigrateProvisionalUserRename`.
- **COMM-1** — ad-hoc (nameless) per-item communities; `origin_item` provenance;
  `ProvisionAdHocCommunity`; promote-by-naming. **+ audience dedup** (this round — see Review round).
- **ID-2** — promote-on-verify across communities in `PhoneRegister`.
- **EMAIL-1** — off-app email channel + signed one-click unsubscribe, `--off-app-email-enabled`
  (default off). CAN-SPAM postal address still a `TODO(#2492)` placeholder.
- **LINK-1** — nameless-community public label, **now localized** (this round).
- **WEB-1 / WEB-2** — web phone-auth hardening + the full-loop Auth-Emulator Playwright e2e in CI.

### Review round (tescobar on the closed precursors → all addressed in #2520, 2026-06-19)
- **Ad-hoc audience dedup** — new server-only `Community.member_signature` (canonical hash of host +
  invitees by user-id / normalized contact handle). `ProvisionAdHocCommunity` **reuses** a live ad-hoc
  community with the same audience (keeps the first `origin_item`); audience-less items skip dedup; **no
  auto-merge** on later open-link drift. Its signature changed to take **`AdHocAudience{MemberUserIDs,
  ContactHandles}`**. Doc: `docs/ad_hoc_communities.md` §"Deduplication by audience".
- **SSR i18n** — the nameless-community label is localized via the server `l10n` catalog
  (`common.adhoc_community_name`, en+es); `/go/` routes wrapped in `middleware.AcceptLanguage`.
  Remaining SSR template strings → **#2090**.
- **Email opt-out reversibility** — deferred to **#2524** (in-app toggle + get/set RPC) with a
  `TODO(#2524)` on the field; reversible at the data layer already.

### Unblocked / open
- **SMS-1 (Phase 0)** — ✅ **A2P Campaign APPROVED (2026-06-26).** Brand + Campaign cleared (the resubmit
  with real screenshots of the in-app consent surfaces — host invite-attestation checkbox, #2527; phone-entry
  SMS disclosure, #2530 — passed). **Platform SMS is now unblocked.** Remaining ops wiring before flipping
  any SMS flag: Twilio secrets → Secret Manager + `entrypoint.sh` / `scripts/fetch_secret.sh`, and counsel
  sign-off on the host-attestation consent + state-law posture. **SMS-2 and SMS-3 are now buildable** (they
  were the only items blocked on approval). A mockup attempt (PR #2522) was
  **closed**. Full state in
  [`2492-sms-a2p-registration.md`](./2492-sms-a2p-registration.md).
- **Open follow-up issues:** **#2090** (localize remaining SSR templates), **#2524** (in-app email
  opt-out toggle), **#2526** (converge sharing onto `ShareItem`), **#2531** (verify live host-RSVP push
  cross-client), **#2548** (per-item community membership policy — **decided + implemented for events in
  PR #2589**: events-only implicit join on yes/maybe RSVP; see What's-left item 7),
  **#2569** (SMS delivery status callbacks), **#2571** (verified-email gate), **#2573** (drop STOP/HELP
  footer on non-first SMS), **#2596** (add a verified phone number to an existing email/OIDC account —
  the inverse of ID-2/EMAIL-1b promote-on-verify; reconciles a phone-keyed provisional into the account).
  **Parked/deferred features:** **#2592** (COMM-2 full), **#2593** (NOTIF-1 P2 contacts picker),
  **#2594** (in-app read-only pre-verify view), **#2595** (inline email / retire `RegisterScreen`).
  **Closed:** **#2529** (deprecate `SubmitRequest.community_id`), **#2547**
  (community gen: drop AI name/description).

**Foundation facts a fresh session needs (as built in #2512):**
- The placeholder identity is now **`ProvisionalUser`** (full rename from `ShadowUser` — message,
  RPCs, reference columns `provisional_user_id` / `provisional_recipient_id`, `server/provisional`
  package, Flutter client, DB). Field numbers preserved.
- Contact handle = **`oneof contact { phone_number | email }`** on the model +
  `CreateProvisionalUserRequest`; normalized via **`server/contact.NormalizePhoneE164`** /
  `auth.NormalizeEmail`; **deduped one-per-handle-per-community**; masked in logs
  (`logging.MaskPhone`), **never on the API**.
- Idempotent startup **DB migration `MigrateProvisionalUserRename`** (`server/storage/migrations.go`)
  copies the legacy `shadow_user` table + columns into the renamed ones (it intentionally still
  references the legacy column names — that's what it renames *from*). `TODO(#2492)` to delete after
  it runs in dev+prod.
- **Deferred out of ID-1 (do these next):** off-app **recipient resolution + dispatch → EMAIL-1**
  (the `server/notifications/service.go:92` seam); **promote-on-verify + cross-community dedup → ID-2**.

**COMM-1 facts a fresh session needs (as built in #2512):**
- An ad-hoc community is just a **nameless `Community`** — no `kind` enum. **Name-presence is the
  discriminator**; `community.IsAdHoc(c)` (`server/services/community/adhoc.go`) is `c.Name == ""`.
- Origin provenance is a **`oneof origin_item { origin_experience_id=12 | origin_gear_id=13 |
  origin_request_id=14 | origin_transfer_id=15 }`** on `models.Community` (mirrors `ShareLink.target`;
  the variant carries the kind, so no separate kind field). It is **provenance only**, retained
  across promotion — *not* the ad-hoc signal.
- **Provisioning:** `Service.ProvisionAdHocCommunity(ctx, hostUserID, AdHocOrigin{…},
  AdHocAudience{MemberUserIDs, ContactHandles})` (`adhoc.go`). `AdHocOrigin` is an exported struct with
  exactly one id set (maps to the unexported oneof internally). It shares a `createCommunity` core
  **extracted from `CreateCommunity`** (which now delegates to it). **CREATE-1 wires the item-creation
  callers** — COMM-1 only provides the mechanism. `MemberUserIDs` seeds *real-user* members;
  `ContactHandles` (provisional invitees' phone/email) feed the dedup signature only — provisional
  users still attach via the provisional-user path. **Audience dedup (this round):** the host + the
  full invitee set hash to `Community.member_signature`; an existing live ad-hoc community with the
  same signature is **reused** (original `origin_item` preserved) instead of creating a duplicate.
- **Promote-by-naming reuses `UpdateCommunity`** (no new RPC): the first unnamed→named transition
  emits **`COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED` (=46)** once (not on rename or description-only
  update); `origin_item` survives. The event value is in **both** the `models` and `api`
  `CommunityEventType` enums (events.go/streaming.go cast model→api by direct numeric value), and is
  classified **`ClassificationClientOnly`** in `server/undo/registry.go`.
- **⚠️ Adding any new `CommunityEventType` requires an entry in `server/undo/registry.go`** —
  `TestEveryEventTypeClassified` is a hard gate in the *full* Go suite (not the community package), and
  client-facing/lifecycle events should be mirrored into the `api` enum too. (Both bit this PR; fixed.)
- **ID-2 (Phase 1c) — ✅ DONE (server)**, **[PR #2515](https://github.com/Type2Fun-Foundation/ripls/pull/2515)**
  (branch **`id2-promote-on-verify`**, stacked on `id1-provisional-identity`/#2512). Phone verification now promotes the provisional
  identity across **all** communities: `provisional.FindUnclaimedByPhone` (cross-community lookup by
  normalized E.164) + `login.(*Service).promoteProvisionalByPhone` (idempotent community join +
  `MergeIntoUser` per match), wired into `PhoneRegister`. Cross-community dedup + idempotent re-entry
  covered; `MergeIntoUser` unchanged (RSVP + transfer are the only activity references). **Client
  pre-verify response-intention replay carrier deferred to WEB-2/LINK-1** — the server already
  *migrates* a provisional RSVP's recorded intention; the choose-before-verify *auto-fire* is the
  client web arm. Tests at both layers; `go test ./provisional/... ./services/login/...` green.
- **EMAIL-1 channel (Phase 1d) — 🟡 channel DONE**, **[PR #2516](https://github.com/Type2Fun-Foundation/ripls/pull/2516)**
  (branch **`email1-email-channel`**, stacked on `id2-promote-on-verify`/#2515). The off-app **email
  dispatch channel** is built + tested: `NotifyUser`'s no-device branch now calls `dispatchOffApp`
  (`notifications/off_app_email.go`) → resolves `User.email`/locale/opt-out → `email.SendNotification`
  (reusing the already-localized `Notification.Title/Body`, wrapped in `notification_template_{en,es}`).
  Opt-out = `User.off_app_email_opted_out` (field 21) set by the signed one-click unsubscribe endpoint
  `GET /email/unsubscribe` (`auth.SignUnsubscribeToken`/`VerifyUnsubscribeToken`). Metric
  `ripls_off_app_notifications_total{channel,outcome}`. **Flag-gated off** (`--off-app-email-enabled`,
  default false) — enabling it for all deviceless users is an ops decision, and the CAN-SPAM **postal
  address is a `TODO(#2492)` placeholder** that must be filled first. **Deferred to EMAIL-1b:** the
  email-provisional *cross-community* promotion (the email analog of ID-2: `FindUnclaimedByEmail` +
  `promoteProvisionalByEmail` on `EmailRegister`) — small + symmetric with ID-2.
- **LINK-1 server (Phase 1b) — 🟡 server DONE**, **[PR #2517](https://github.com/Type2Fun-Foundation/ripls/pull/2517)**
  (branch **`link1-adhoc-open-links`**, stacked on `email1-email-channel`/#2516). Recon found ad-hoc
  per-item open links **already mint/resolve/preview/join** via existing infra (`GetOrCreateShareLink`
  per-item target, `fetchItemPreview`, `AcceptInvitationLink`) — the only gap was the public `/go` SSR
  landing rendering an empty `Community.Name`. Fixed with `communityDisplayName` (a generic public
  label "a Ripls group" for nameless communities, applied at both invite + event landing builders;
  no PII, zero template churn). **Client response-intention carrier deferred to WEB-2**; host-relay
  share-sheet/`sms:` pre-fill deferred to CREATE-1. Tests: unit + invite-page + event-page nameless
  landings.
- **WEB-2 milestone (Phase 1c) — ✅ GREEN** on branch **`web2-pivot-loop-e2e`** (stacked on
  `link1-adhoc-open-links`/#2517). The full phone-first loop e2e passes reliably (~8s): SSR invite →
  Flutter web → **real phone OTP via the Auth Emulator** → `PhoneRegister` promotes the seeded
  provisional user → auto-RSVP → `rsvpYesCount > 0`. Bundles WEB-1 (web phone-auth hardening), the
  **Auth-Emulator e2e foundation** (server+harness+client, firebase-tools@15), the LINK-1 response
  carrier (reused as-is — the handoff was already auth-agnostic, no new wiring), the
  `docs/workflows/phone_first_rsvp.md` doc, and the Playwright spec. Full detail + the two big traps
  (`//go:embed` server-rebuild requirement; Flutter-web `.fill()`→`pressSequentially`) + the
  exit-143 teardown known-issue in the companion plan [`2492-web2-pivot-e2e.md`](./2492-web2-pivot-e2e.md).
- **Open-link join** reuses `AcceptInvitationLink` unchanged (no ad-hoc special-casing).
- **Surfacing:** `ListCommunities` (`membership.go`) has **no name-presence filter** — ad-hoc
  communities already flow through with an empty name; **group-text client rendering is DISPLAY-1**.
- **Cap exemption is DEFERRED** (see COMM-1 *Deferred* below): ad-hoc communities keep the 32-cap for
  now. Enforcing sites to touch later: `invitations.go:164`, `login/registration.go:502`.
- **Decisions locked (Bryan):** `oneof` over flat id+kind fields; reuse `UpdateCommunity` over a new
  promote RPC. Doc: `docs/ad_hoc_communities.md`. Tests: `server/services/community/adhoc_test.go`.

### ✅ #2527 — create / share / phone-first journeys (MERGED 2026-06-23)

**[PR #2527](https://github.com/Type2Fun-Foundation/ripls/pull/2527) merged to `main`** (branch
`create1-share-item-experiences` pruned). It delivered **CREATE-1**, **INVITE-1**, **DISPLAY-1
(partial)**, **COMM-2 (partial)**, the phone-first web onboarding, a cache-coherency fix, the
marketing-video harness, and forced-dark SSR. The detail below is what a fresh session needs; the
residual work is in [What's left](#-whats-left-the-pending-work).

**Server — every item is born with its own audience (CREATE-1).**
- Community creation lives in the shared **`server/community`** library (`CreateCommunity` /
  `ProvisionPerItemCommunity`, keyed by `origin_item`) — item services call it directly, the community
  RPC service delegates to it; **no service→service dependency** (`docs/server/architecture.md` §1).
- `SaveExperience` / `SubmitRequest` / **`SaveGear`** provision the host-only per-item community at the
  create RPC, share the item in, and return `item_community_id`. *(Gear was originally lazy on first
  `ShareItem`; `SaveGear` now provisions at creation too — `WEB-3/WEB-4` round — via an injected
  provisioner wired in `main.go`, defaulting the share to `FOR_LOAN`. **transfers inherit
  `Transfer.community_id`**.)* `SubmitRequest.community_id` is now **optional** (deprecation → #2529).
- **`ShareItem`** is the single audience entry point: finds the per-item community by origin
  (provision-if-missing), attaches phone/email invitees as provisional members, shares to extra
  communities, mints the `/go/{code}` link, sends compliant off-app email (suppression + unsubscribe +
  consent record), PII masked. Supersedes per-type `Share*` (→ #2526). The `member_signature` audience
  dedup is vestigial here (one community per item, keyed by origin).

**Client — creation is just *Save*; invite is additive (CREATE-1 + INVITE-1).**
- Unified-create button → **"Save {type}"**; at-creation Share UI removed (`share_audience_sheet.dart` +
  `preview_invite_selector.dart` deleted). After Save, the redesigned **glass `item_share_sheet.dart`**
  auto-opens (also from "Who's In"): QR + open link, Copy/Share, **Invite people** (chip-complete
  existing Ripls members across the host's communities via `DarkPersonSearch`,
  `sharing/invite_members_sheet.dart`) + **Invite community** (add not-yet-shared communities,
  `excludeCommunityIds`). Glass is the standard — `CompletionColors` pinned dark; selected affordances
  use `experienceSageGreen`.
- Texting device contacts on the host's behalf + per-link Revoke are **removed for now** — re-add with
  SMS-2/3 once A2P clears.

**"Who's In" = flat roster + host management (INVITE-1 / COMM-2 partial).**
- Flat list with a per-row status pill (Going/Maybe/Not going/Invited); a Communities section shows each
  shared community + its "N hasn't responded" count (`GetExperience.community_no_reply_counts`,
  caller-scoped, excludes host). Directly-invited individuals show as "Invited"
  (`GetExperience.invited_individuals`). `RosterEntry.isHost` separates the host flag from the pill.
- **Host control RPCs** (`services/experience/member_management.go`): `SetExperienceMemberRSVP`
  (owner-only upsert; UNSPECIFIED resets to invited) + `RemoveExperienceMember` (owner-only soft-delete +
  clears RSVP; can't remove host). **Re-invite after remove** works: `community/share_item.go
  addMembersToCommunity` **restores** a soft-deleted membership (zero-valued `DeletedMetadata{}`) instead
  of Insert-colliding.
- Invited individuals see the event on **home/calendar** (`portfolio/fetch.go` includes origin-community
  experiences in `activeExpIDs`, intention-aware subtitle).

**DISPLAY-1 (partial).** Host-only *single-member* nameless per-item communities are **suppressed** from
pickers; the rest render group-chat-style by their first few members' first names (server
`CommunityItem.member_count` + `member_preview_first_names`, `community/membership.go`; client
`core/utils/community_display.dart`). **The "name this group" promote nudge is built** (branch
`display1-name-group-nudge`, PR #2543 — see below). Full group-text visuals + provisional-member
visuals are still pending.

**Name-this-group promote nudge (branch `display1-name-group-nudge`).** Two owner-facing entry points
turn a nameless per-item community into a real one via the rich create/AI modal in **promote mode**:
- **Workshop community switcher** (the "Ripls"-tab community list, opened via the "Switch circle" pill,
  `workshop_switcher_dropdown.dart`) — `ListCommunities` now sorts nameless communities **after** named
  ones (`membership.go`, stable partition); the owner's nameless rows show a pencil **"needs a name" cue**
  and **tap into the namer** instead of selecting the community.
- **Event roster** — a **"Name this group…"** link sits between the attendees and the Communities section
  when the origin per-item community is still nameless and the viewer is the host
  (`experience_pitching_in_screen.dart`).
- Both open `CommunityCreationModal.show(promoteCommunityId:)`; the only branch is
  `GenCommunityNotifier.createCommunity()` → **`UpdateCommunity`** (name + photo + description) instead of
  `CreateCommunity`, so the AI-assist + preview flow is reused as-is. The server emits `COMMUNITY_NAMED`
  and the community becomes persistent. Tests: gen-VM promote branch, switcher affordance, roster-link
  visibility, `ListCommunities` ordering, and the **two-flow web e2e**
  (`community-promote-nudge.spec.ts`).

**Phone-first web onboarding (the guest arm) — 3 screens → 1.** Tapping "I'm in"/"Maybe" on the SSR
landing routes straight to a phone-first **`/rsvp-phone`** route (`app_router.dart`) rendering
`PhoneAuthScreen` ("Confirm your phone to RSVP to {event}"); the old "Sign in to RSVP" gate and the
"You're Invited" register hero are gone. The route **participates in the auth-aware redirect**
(`isGoingToRsvpPhone`) — **trap:** don't `Navigator.push` `PhoneAuthScreen` over `WebExperienceScreen`
then `context.go('/event')`; the auto-RSVP won't fire. Event name + hero are threaded from the SSR
(`event.html` → `/event` → `WebExperienceScreen` → `/rsvp-phone`); `img` gated on `HasHeroImage`.

**🐛 Cache-coherency fix (load-bearing, reusable).** `StashCacheManager.invalidate/remove/clear` dropped
the stored value but **not the in-flight fetch**, so an "invalidate-then-refetch" racing an in-flight
fetch (same key) coalesced onto the **stale** result — surfaced as post-RSVP "You haven't replied yet".
Fix = a **per-key invalidation epoch** (`stash_cache_manager.dart`): invalidating bumps it + drops the
in-flight marker; a stale fetch that resolves later neither writes nor evicts the newer slot. Regression
tests prove bug + fix. Companion: `refresh()` in `experience_view_model.dart` now resolves the place name
when unset (fixed "WHERE: TBD").

**Real-time RSVP streaming.** `SetExperienceMemberRSVP` publishes `EXPERIENCE_RSVP_*` to the **origin
community** (`actor=host`, `ObjectUserId` empty = no push); the client routes it → `content` → refreshes
the open roster. Proven by `event-rsvp-streaming-multi-client.spec.ts`. **#2531 addressed** (issue kept
open to formally verify).

**Forced-dark SSR.** Public `/go/` event + invite landings render **always dark** via a `theme-dark` class
on `<html>` remapping `--color-*` → fixed `--dark-*` tokens (`website/content/css/tokens.css`); no
hardcoded hex. The Flutter registration flow still **respects device theme**.

**Marketing-video harness.** `@marketing` Playwright project `marketing-video` +
`phone-onboarding-video.spec.ts` (films only the guest; host/event/roster RPC-seeded off-camera); the
standard projects `grepInvert` it so it's **on-demand, not a CI gate**. Hermetic runner
`e2e/scripts/run_marketing_video.sh` (throwaway Postgres testcontainer). **HI-DPI learning:** Playwright's
video recorder ignores the context `deviceScaleFactor`; only the browser launch flag
`--force-device-scale-factor=2` raises screencast resolution (CanvasKit rasterizes text at the forced 2×).

**Decisions locked (Bryan).** Glass is the standard (completion/people surfaces always dark); selected
affordances use `experienceSageGreen`; per-item communities suppress only host-only single-member nameless
ones, name the rest by member preview; RSVP stream → origin community, `actor=host`, no push; phone-first
is a full-screen `/rsvp-phone` route (not a sheet); SSR landings always dark, registration respects device
theme; marketing runs hermetic per-run; chase the **real** root cause of flakes (the cache bug) over
band-aids. **Commit with `git commit --no-verify`** (pre-commit's stash holds `.git/index.lock` and races
the IDE; gitleaks still runs in CI).

**Local e2e gotcha.** The embedded web bundle (`server/services/web/app_assets/`) is overwritten to a
*dev* build when the app runs locally (`npm run start:server` / `build:web:dev`), failing local e2e on a
login screen. Rebuild with **`npm run build:web:e2e` then `go build -o tmp/server ./server`** before local
e2e. CI builds its own e2e bundle. Local e2e DB auto-provisioned by `npm run db:start`
(`scripts/ensure_e2e_db.sh`). Single spec: `cd e2e &&
E2E_DB_URL='postgres://ripls:ripls_dev@localhost:5432/ripls_e2e_proof?sslmode=disable' E2E_PORT=8090 npx
playwright test tests/workflows/<spec> --project=mobile-chromium`.

### E2E coverage (finish_pivot)

New/updated Playwright workflow specs (all green on `mobile-chromium`; paired with `docs/workflows/`):

- **Authoring → share → "Shared with"** (NEW): `gear-authoring-share.spec.ts` + `request-authoring-share.spec.ts`
  — host creates a gear/request via the **real unified-create UI** (new `e2e/lib/ui/create-item.ts`
  `createGearViaWebUI`/`createRequestViaWebUI`), invites people from the auto-opened share sheet, lands in the
  item (no reload), inspects the `SharedWithCard` → `AccessSheet` (asserts the invited people show **by name**,
  not a faceless aggregate), then **adds a community from the open panel** and asserts it appears **in place**
  + lands server-side. Doc: `docs/workflows/item_sharing.md`.
- **Phone-first invitee** — gear-**giveaway** (NEW, `phone-gear-giveaway-full-loop.spec.ts`: "I want this" →
  `INTEREST_EXPRESSED` pending-request), plus the existing gear-loan + request-offer + RSVP loops (real-name
  cast + Unsplash heroes; a `waitForTimeout` settle so the recorded last frame shows the in-app item hero).
- **Returning member** (NEW, `phone-existing-member-login.spec.ts`): registers a phone account off-camera
  (`e2e/lib/seed/phone-user.ts` `registerPhoneUser` — emulator OTP REST → `PhoneRegister`), then drives the
  in-app phone-first invite with the same number and asserts it **logs in** (not rejected) + the auto-action
  fires.
- **Gotchas captured (CanvasKit/e2e):** unified-create classifier is keyword-based (`provider_e2e.go` —
  keep gear cues out of request prompts); mock AI titles aren't the prompt (resolve items by audience);
  `ListMyRequests`/`listUserGear` omit audience fields (call `GetRequest`/`GetGear` per item); the
  `SharedWithCard` is **one a11y button** (use `toHaveAccessibleName`); `InviteMembersSheet` lists members as
  `"{first} {initials} {first}"` buttons; inspect a video via `E2E_KEEP_VIDEOS=1` then
  `ffmpeg -sseof -1.5 -i v.webm -update 1 out.png`.

### 🚧 What's left (the pending work)

Branch new work off `main`. In rough priority order:

1. **Platform SMS (Phase 2) — ✅ built + ENABLED.** SMS-2 (Twilio provider + STOP/HELP/opt-out) and SMS-3
   (off-app channel selection) **merged (#2566)**; A2P + consent + legal all approved. **`platform_sms_enabled`
   is now flipped `true` in Terraform (dev+prod) — the next deploy turns SMS on**, with the email backup as
   fallback. *The device-contacts "Invite people" path (NOTIF-1 P2) is **🛑 DEFERRED** — host-relay
   already covers cold contacts and a contacts-permission prompt is too invasive (item 4).* See the SMS-2/SMS-3
   sections + `2492-sms-a2p-registration.md`.
2. **Gear / request web-response (Phase 3, WEB-3/WEB-4)** — ✅ **DONE** (branch
   `web34-item-web-response`, stacked on #2552): SSR gear + request landings (`gear_page.go`/`request_page.go`),
   the generic auth-exempt `WebItemActionScreen` (`/item/{id}`, `/need/{id}`), and post-verify
   `ExpressInterest` / `OfferToFulfill` auto-fire — the phone-first arm now covers **events, gear, and
   requests**. No web-response RPC change; **`SaveGear` now provisions its per-item community at creation**
   too. See the WEB-3/WEB-4 sections + `docs/workflows/phone_first_{gear_interest,request_offer}.md`.
   *Item-detail in-app share/invite for the non-RSVP types + invitee polish — ✅ **DONE** (branch
   `finish_pivot`, NOT yet PR'd). The full set: (a) a reusable **`SharedWithCard`** ("Shared with N" +
   invited-face cluster → `AccessSheet` / `ItemShareSheet`) on the gear + request read shells (gear's
   `AccessSheet` plumbing built from scratch); server adds **`invited_individuals`** to
   `GetGearResponse`/`Request` via shared `services.FetchInvitedIndividuals`, so the panel lists invited
   people **by name**. (b) **After share, the app drops into the new gear/request** (was events-only); the
   `AccessSheet` title is **"Shared with"** and **updates in place** when a community is added. (c) **Bug
   fixes** — the access surfaces no longer re-filter the server's already-caller-scoped `sharedCommunities`
   against the lagging client `communitiesProvider` (one root cause behind the blank panel **and** the
   silently-failing community-add; `invitableSharedCommunities` + the `AccessSheet` partition now trust the
   server); request's silent `refreshRequest()` keeps `sharedCommunities` fresh; **a returning member's
   phone logs them in** (`PhoneAuthScreen` → `PhoneLogin` on `AlreadyExists`) instead of being rejected.
   (d) **e2e** — see the [e2e note](#e2e-coverage-finish_pivot) below. An optional in-app read-only
   pre-verify content view was sized and **filed as #2594** (large + largely redundant — the `/go/{code}`
   SSR landing already is the read-only pre-verify view the guest sees first).*
3. **Off-app email backup — ✅ DONE & MERGED (#2572).** **EMAIL-1b** built:
   `provisional.FindUnclaimedByEmail` + `promoteProvisionalByEmail` on `EmailRegister`/`OIDCRegister`
   (symmetric with ID-2). The proper verified-email gate is deferred (accepted email-shadowing risk, no new
   hole since ad-hoc communities are open-link join) → **#2571**. The **CAN-SPAM postal address** is filled
   as a single config value (`mailgun_postal_address`, was 4 placeholders; default corrected Denver→Boulder
   in #2575) and **`off_app_email_enabled` is flipped `true` in Terraform**. Also this round: **off-app
   SMS/email carry a `/go/{code}` short link** to the notification's entity
   (`community.ShortLinkCodeForEntity`), and the entity was renamed **LLC→PBC** on active legal/consent
   pages. The channels are enabled *and* the NOTIF-1 server copy rework shipped in the same PR, so messaging
   goes live with the reworked copy.
4. **NOTIF-1 · Off-app notification copy rework + cold-contact invites — 🟡 P1 (server) DONE & MERGED
   (#2572); P2 (client) DEFERRED; P3 (product flip) remains.** **Shipped (P1):** uniform `NotifyContent` +
   polymorphic per-surface renderers (push/SMS/email) in the new `server/notifications/notification_content`
   package; per-kind message + CTA-verb catalog; `Ripls:` prefix + entity-inline + CTA-with-`/go`-link SMS
   layout; the `&#39;` text/plain fix; the render-all utility (`server/cmd/notification_examples/`). Plus
   in-flight refinements beyond the original spec: STOP/HELP opt-out footer now only on the first message +
   ~monthly (not every text; investigate dropping further → **#2573**), stopped notifying on
   request-fulfilled / offer-selected, and a copy polish pass (articles, no quotes, push giveaway/loan
   split, de-jargoned transfers). **Still open:** **P2 — 🛑 DEFERRED** (device-contacts selection in the
   invite modal via `flutter_contacts`): host-relay already covers cold contacts and the
   contacts-permission prompt is too invasive for the marginal gain (Bryan, 2026-06-29); revisit only if
   host-relay proves insufficient — now tracked as **#2593**. **P3 — 🛑 PARKED (Bryan 2026-07-01):** the
   deliberate product flip to un-gate platform-sent *cold* SMS invites (built + review-ready, gated off) — a
   product call, not a coding step. *(RCS relies on this staying parked; un-parking it re-opens the #2609
   RCS-gating requirement.)* Full spec in
   the [NOTIF-1 section](#notif-1--off-app-notification-copy-rework--cold-contact-invites--feature-new-2026-06-27).
5. **Real-time / live-update follow-ups — ✅ DONE (PR #2589).** Home "Up next" now live-updates on RSVP
   (RSVP cases route to the portfolio provider in `event_router.dart`); host *reset-to-invited* /
   *remove-member* stream via the new no-notification `EXPERIENCE_ROSTER_CHANGED` event; and roster/RSVP
   events fan out to every community the experience is shared to (`publishRosterChangedToOthers`), so a
   co-invitee viewing through a sibling named community sees the change live.
6. **COMM-2 full — parked, tracked as #2592.** Require-approval gate + capacity + guest-list visibility on
   the open-link model (the Who's-In set/remove host controls already landed). The next substantial feature
   in the core pivot; deliberately parked 2026-06-30 to land the in-app sharing + real-time + #2548 work first.
7. **DISPLAY-1 follow-ups** — **#2547** (community gen: drop AI name/description, two-field input, keep LLM
   only for image keywords) is **CLOSED**; **#2548** (do community-shared invitees implicitly join the
   per-item community?) — **policy decided + implemented for events (PR #2589):** **no** implicit join in
   general, **except** a user who can see an *event* only through a named community it's shared to and then
   RSVPs **yes/maybe** is promoted into the event's ad-hoc origin community (`ensureRSVPerInOriginCommunity`
   in `experience/member_management.go`), so the host sees them by name in Who's-In rather than as a
   collapsed count. Intuitive because their name already pops out individually in the RSVP UI. Gear/requests
   get **no** implicit join — there's no individual-roster surface for a name to pop into. (The "what happens
   when it's named?" sub-question is moot: the join targets the origin community whether or not it's since
   been named.)
8. **Phone-screen polish** — inlining the email secondary path into `PhoneAuthScreen` + retiring
   `RegisterScreen` was sized and **filed as #2595** (the inline form is ~1hr, but retiring `RegisterScreen`
   is a real refactor — it still homes `/invite` deep links, first-launch, and manual invite-code entry).
   Separately: bump marketing capture to 3× if max CanvasKit crispness is wanted.
9. **Follow-up issues** — open: #2090 (SSR i18n), #2524 (in-app email opt-out toggle), #2526 (`Share*`
   convergence), #2531 (verify live host-RSVP push), #2569 (SMS delivery status callbacks), #2571
   (verified-email gate), #2573 (drop STOP/HELP footer on non-first SMS). Closed: #2529
   (`SubmitRequest.community_id` deprecation), #2547 (community-gen rework).
10. **Phase 4 — RCS-1 · ✅ SUBMITTED 2026-07-01, in carrier review (~5–9 wk).** Compliance prep (#2574)
    merged; then the Business Messaging channel was built + registered: welcome-on-opt-in send
    (`notifications.SendPhoneOptInWelcome`, deployed dev+prod), RCS sender `XE841…` live in the prod
    Messaging Service, RCS-first delivery verified end-to-end, and Google + US/Aegis registration filed
    together (video hosted at `ripls.org/assets/rcs-demo.mp4`). Fees ($200/yr + $500) at approval.
    **#2609 (per-recipient gating) CLOSED** — not needed while cold no-opt-in messaging is parked. Canonical:
    `2492-rcs-business-messaging.md`.

---

## Problem Statement

Today every Ripls participant must register and join a bounded (≤32) community before they can
see or respond to anything. That gate is the single biggest source of invite friction and the
reason a host can't pull in someone who isn't already on the app.

This epic pivots Ripls toward an **account-optional, contact-based, progressive** identity and
notification model, inspired by Partiful. A host creates any item (event, request, giveaway, loan)
through the existing flows, but can now invite **individuals identified only by a phone number or
email** (or existing members by account) via a **shareable open link**, **email**, and/or **SMS** —
not only existing communities. Invitees are notified in the most native way available (push for app
users; email / SMS / link for everyone else), respond on the **web app**, verify ownership of their
phone (OTP) or email (tokenized link), and become real Ripls users **progressively**.

Two infrastructural pillars make this possible:

1. **Off-app messaging as first-class notification channels** alongside FCM — **email** (Mailgun,
   ships first; lowest compliance risk) and **SMS** (Twilio; RCS later). Both plug in behind the
   existing provider-agnostic notification abstraction; the dispatcher chooses the channel from
   user metadata (active app device? else phone → SMS, email → email).
2. **Ad-hoc, per-item communities** created at item-creation time so that **every participant,
   even a brand-new phone user, is always part of some community**, and visibility is cleanly
   "the audience of this item." Items can still also be shared with existing communities, as
   today.

**Strategic note (product fit).** This pivot is in tension with the founding constraint in
`docs/priorities.md` / `docs/community_health.md` — invite-only, bounded-at-32, no-strangers — so
the departure is **scoped deliberately to ad-hoc per-item communities only**. Persistent
communities keep the cap and the thesis. See [Decision 2](#decision-2--ad-hoc-community-model)
and [Decision 3](#decision-3--invitee-model-open-link-emergent).

---

## Context & Confirmed Decisions

The epic has **no GitHub comments** — it was scoped through a live planning discussion. The
load-bearing context is the set of design decisions confirmed there (recorded here because they
shape every phase). Each was chosen against explicit alternatives; see
[Potential Solutions](#potential-solutions) for the options weighed.

1. **Identity — phone-capable provisional member.** Evolve the existing shadow-user concept into
   a phone-bearing *provisional* identity that is a member of the ad-hoc community, can be
   SMS-notified, and is **promoted to a full `auth_method=PHONE` `User` on phone verification**,
   reusing `server/shadow/merge.go:MergeIntoUser`.
2. **Ad-hoc communities — an *unnamed* `Community`; naming promotes it to "real".** No `kind`
   enum: a per-item community is simply **nameless**, and it **becomes a real, persistent community
   the moment someone gives it a name** — the natural "this one-off turned into a standing thing"
   path, which is a product goal. Add only an optional `origin_item_id` provenance back-reference;
   everything else (sharing junctions, the `community_event_bus`, streams, push recipients,
   visibility) already keys off `community_id`. **The 32-member cap does not apply to unnamed
   communities**; on naming, an over-cap group is grandfathered (don't block the evolve path) with
   a soft nudge — persistent named communities otherwise keep the cap. See
   [Open Questions](#open-questions).
3. **Open-link / emergent membership — the existing ShareLink behavior, scoped tighter.** This is
   *not* a thesis departure: today's `/go/{code}` links already join you to the whole persistent
   community **and** surface the item. A per-item ad-hoc community makes that *more* restrictive
   (single-item audience). Membership is emergent (anyone with the link can respond → join),
   reusing `AcceptInvitationLink`.
4. **Delivery — two mechanics; host-relay first, platform A2P in parallel.**
   - **Share/copy link** → the *host* relays via their own Messages/WhatsApp. No 10DLC. Ships
     first.
   - **Contact-picker invite** → replicating Partiful's contact picker requires **our own
     platform-originated A2P send** (Partiful sends these itself from short code 397-81; it does
     *not* relay through the host). Gated on 10DLC. **Phase-1 fallback:** the picker opens the
     host's Messages app pre-filled (`sms:` URI), then switches to silent A2P once SMS is live.
5. **v1 scope — events first.** The event web-view + verify-and-RSVP handoff already exists and
   is e2e-tested. Reuse it; extend to requests / giveaways / loans afterward (they need new
   web-response screens).

### Architecture leverage already in place (from codebase research)

- **Notifications are provider-agnostic.** `NotificationService.NotifyUser(userID, notification)`
  (`server/notifications/service.go`) funnels all push (community events, chat, scheduled jobs)
  through one method. The SMS-fallback seam is the existing silent `len(devices)==0` no-op at
  **`server/notifications/service.go:92`** — one branch there covers all 8 `NotifyUser` call
  sites.
- **Phone auth exists.** `PhoneRegister` / `PhoneLogin` (Firebase OTP, E.164); `User.phone_number`
  (field 18, set only for `AUTH_METHOD_PHONE` users).
- **Shadow users** (`proto/ripls/models/shadow_user.proto`, `server/services/community/shadow_users.go`)
  are community-scoped, name-only, **no phone**, not chat participants, and already mergeable on
  registration — the precursor to the provisional identity.
- **Polymorphic `ShareLink`** already carries `shadow_user_id` merged on registration, and a
  per-item `oneof target`.
- **The event web-response arm is built**: SSR event landing (`server/services/web/event_page.go`,
  `templates/event.html`, Yes/Maybe/No CTAs), `WebExperienceScreen`, the guest-RSVP handoff
  (`WebEventRsvpHandoffNotifier`, `EventInviteJoinGate`), and a passing Playwright e2e
  (`e2e/tests/guest-rsvp-confirm.spec.ts`). **Gear and requests have no equivalent.**
- **SMS structural mismatch:** the `Provider` interface is device-token + `DevicePlatform`-keyed;
  SMS recipients have **no `UserDevice` row**, so SMS does not fit the
  `providers map[DevicePlatform]Provider` model — add a separate `SMSSender` on the `service`
  struct.

---

## Potential Solutions

The genuine solution space is a set of architecture forks. Each is recorded with the options
weighed and the choice made, so the rationale survives iteration.

### Decision 1 — Identity model for phone invitees

- **Option A — Phone-capable provisional member (CHOSEN).** Evolve shadow users into a
  phone-bearing identity that's a member of the ad-hoc community and is promoted to a full PHONE
  `User` on verification (reuse `MergeIntoUser`). *Pros:* reuses merge infra; the invitee is a
  real member (group-text display, accrues the invite) before verifying; "always part of a
  community" holds. *Cons:* requires extending the shadow model + recipient-resolution.
  *Effort:* medium.
- **Option B — Full unverified `User` at invite time.** *Pros:* first-class member immediately.
  *Cons:* unverified rows in the users table; complicates auth-method enforcement and dedup.
- **Option C — Phone on the invite only; `User` created at verify.** *Pros:* smallest model
  change. *Cons:* invitee isn't a real member/participant until they respond — weakens the
  membership guarantee and group-text display.

### Decision 2 — Ad-hoc community model

- **Option A — Unnamed `Community`; naming = promotion to "real" (CHOSEN, revised).** A per-item
  community is just a **nameless** `Community`; adding a name promotes it to a persistent one.
  *Pros:* no `kind` enum to thread through readers; `name` already isn't required; gives users a
  one-tap "turn this one-off into a real group" path, which is a product goal. *Cons:* a few
  readers branch on name-presence; cap-on-naming needs a grandfather rule. *Effort:* medium.
- **Option B — Explicit `kind` (REAL/AD_HOC) discriminator.** *Pros:* unambiguous. *Cons:* a
  second enum to special-case everywhere, and it fights the evolve-to-real goal — you'd flip a
  kind, not just add a name. Rejected in favor of name-presence.
- **Option C — New "audience/thread" entity.** *Cons:* re-plumbs sharing, visibility, the event
  bus, streams, and notifications for a second container type. *Effort:* large.

### Decision 3 — Invitee model (open-link emergent)

- **Option A — Open link, emergent membership (CHOSEN).** Anyone with the link can respond → join;
  no 32-cap for ad-hoc; optional approval gate later. *Pros:* matches the *existing* ShareLink
  behavior (reuses `AcceptInvitationLink`); maximally viral / lowest friction; the no-install
  growth loop actually spreads. *Cons:* larger abuse surface (mitigated in SMS-1/SMS-2 + the
  number-entry rate-limiting). *Effort:* small-medium (reuse).
- **Option B — Closed host-specified list.** Strongest on-thesis, simplest to keep compliant, but
  forgoes the viral spread that is the point of the pivot.
- **Option C — Hybrid (closed default + optional open link).** Reasonable, but the open path is
  the existing behavior anyway; defaulting closed adds friction without a clear win. Host controls
  land as an opt-in follow-up (COMM-2) instead.

### Decision 4 — Delivery sequencing & channels

Three delivery channels, in ascending order of compliance risk (see
[Consent precedent & model](#consent-precedent--model)):
1. **Email** (platform-sent, no 10DLC) — CAN-SPAM (opt-out, no prior consent); *lowest-risk default
   channel.* Ships Phase 1.
2. **Host-relayed link** (host sends from their own Messages/WhatsApp) — no platform send, no
   10DLC. Ships Phase 1.
3. **Platform-originated SMS** — needs 10DLC; carries TCPA + state-law risk. Phase 2.

- **Option A — Email + host-relay first; platform SMS in parallel (CHOSEN).** Ship email and the
  link-share / pre-filled-picker paths immediately (no 10DLC); run 10DLC registration in parallel;
  flip on silent A2P SMS when it clears. *Pros:* a **complete invite loop ships now** (email +
  relayed links); de-risks the ~2–3 wk external critical path; SMS is an additive channel.
- **Option B — Platform SMS only.** Nothing ships until 10DLC + provider land, full TCPA burden
  from day one. Rejected.
- **Option C — Everything together after 10DLC.** Most complete launch, slowest first slice —
  email has no reason to wait. Rejected.

### Decision 5 — v1 item-type scope

- **Option A — Events first (CHOSEN).** Reuse the built RSVP web arm; close the loop with minimal
  client work; extend to requests/giveaways/loans after. *Effort:* small for events, then medium
  per additional type.
- **Option B — All four types in v1.** Most complete, but requires building unauthenticated
  web-response screens + post-verify handoffs for gear/requests up front.

---

## Recommendation

Proceed with **Options A across all five decisions** (the choices above), with two revisions
folded in: the provisional identity is keyed by **phone _or_ email** (Decision 1), and the
per-item community is an **unnamed** `Community` that becomes "real" when named (Decision 2). The
synthesis: a contact-keyed provisional identity is a member of an unnamed (ad-hoc) community whose
open link reuses the existing join path (Decision 3); the host invites by relayed link, **email
(platform-sent, no 10DLC)**, or platform SMS once 10DLC clears (Decision 4); and we prove the
whole loop on **events** first (Decision 5), where the web-response arm already exists.

The phasing in [Implementation Plan](#implementation-plan) front-loads the **no-external-dependency
events loop** (Phase 1) while the **10DLC registration runs in parallel** (Phase 0), so the
slowest external item never blocks engineering. Platform SMS (Phase 2) is a flag flip once
registration lands. Other item types and host controls (Phase 3) and RCS (Phase 4) follow.

---

## Open Questions

- **Provisional identity: extend `ShadowUser` vs. introduce `ProvisionalUser`?** — ✅ **RESOLVED
  (ID-1, PR #2512):** kept the single model but **renamed `ShadowUser` → `ProvisionalUser`**
  outright (Bryan's call) rather than just relabeling conceptually — a full rename across protos
  (message, RPCs, and reference fields `shadow_user_id`→`provisional_user_id` /
  `shadow_recipient_id`→`provisional_recipient_id`, field numbers preserved), `server/shadow`→
  `server/provisional`, the Flutter client, and the DB (idempotent copy-then-drop migration
  `MigrateProvisionalUserRename`). Contact handle is a `oneof contact { phone_number | email }`. All
  remaining "shadow user" references were then scrubbed from the live surface.
- **How does `NotifyUser` address a provisional member with no `User` row?** — Recommendation:
  resolve the contact handle (phone **or** email) in the recipient-resolution path
  (`server/notifications/community_subscriber/recipients.go`) rather than minting a placeholder
  `User`; keep the users table clean until verification.
- **Ad-hoc community visibility — where do they surface?** — **Decided:** show them in the
  carousel/workshop **like any community** — the only difference is the missing name, rendered
  group-text style (DISPLAY-1), plus a light **"add a name"** nudge. No special-case filtering;
  this reinforces the evolve-to-real path.
- **Ad-hoc community lifecycle on item completion/deletion?** — Recommendation: archive (not hard
  delete) alongside the item, reusing `DeletedMetadata` + the existing cascade patterns, so
  history/impact survives.
- **View-before-verify on web?** — Recommendation: render read-only item content pre-verify via
  the SSR landing (the event landing already shows host/when/where/attendees); requiring
  verification before *seeing* the item hurts conversion. Build the SSR read path for gear/requests
  in Phase 3.
- **Legal sign-off on host-attestation consent.** — Recommendation: a hard blocker for
  *platform-originated SMS* (Phase 2) and **state-law exposure** (WA CEMA et al.) — **not** for
  host-relayed links or **platform email** (Phase 1, CAN-SPAM). Counsel reviews the SMS attestation
  + state posture during Phase 0. See [Consent precedent & model](#consent-precedent--model).
- **Default ad-hoc capacity / approval?** — Recommendation: no cap, no approval by default
  (Partiful parity); ship host controls as opt-in in COMM-2 (Phase 3).
- **32-cap when an unnamed community is named?** — Recommendation: **never block naming.** Enforce
  the cap only when adding a *new* member to an already-named community; a group that grew over-cap
  while unnamed is **grandfathered** on naming, with a soft "larger than the usual 32" nudge.
  Communities created directly via the explicit named "Create community" flow enforce the cap as
  today.
- **Email-provisional verification / promotion path?** — Recommendation: the **unique tokenized
  invite link sent to the email address is itself the email-possession proof** (parity with phone
  OTP) — clicking it promotes the provisional identity to a light account; a full
  `AUTH_METHOD_EMAIL_PASSWORD` account can be set later. Avoids building a separate email
  magic-link OTP. Confirm against `registration_and_login.md` auth-method enforcement.

---

## Implementation Plan

Each work item below corresponds to a now-closed child issue (its number noted as `was #NNNN` for
traceability). Items are independently tackleable; **dependencies** are stated per item. Check
boxes track progress; one or more PRs per item.

### Conventions & cross-cutting requirements (apply to every phase)

Grounded in the project's documented standards (folded in by `/ripls-audit` — see
[Audit notes](#audit-notes) for sources):

- **Proto conventions** (`docs/proto_conventions.md`): new model fields (`Community.origin_item_id`,
  the provisional identity's `phone_number` / `email`) and any new API fields use
  **`optional`** for conditionally-present values; **proto comments must not name vendors** — write
  "SMS" / "text message" / "email", never "Twilio" / "RCS" / "Mailgun" (same for enum value names).
  Each RPC keeps
  **dedicated request/response types** — do not introduce a shared invitee/invite message across
  `SaveExperience` / `SaveGear` / `SubmitRequest`. Conversion stays in the service layer; `api` and
  `models` never import each other. Any API field the client sets and reads back gets a
  **round-trip test** (`server/services/*/roundtrip_test.go`).
- **SMS is a server-rendered surface** — `docs/server/l10n.md`'s render boundary explicitly lists
  SMS. All SMS copy goes through the go-i18n catalog (`server/l10n/source/{en,es}.toml`) via
  `Localizer.T(key, params)`, **never inline `fmt.Sprintf`** (extend
  `scripts/check_no_inline_user_strings.js::SCANNED_FILES` for the new SMS sender/subscriber).
  Locale resolves like **email** (recipient's stored `preferred_language` wins); since provisional
  invitees have **no `preferred_language`**, seed locale from the inviting request's
  `Accept-Language` (fallback `en`) and capture it up-front if the send is backgrounded.
- **PII & observability** (`docs/server/observability.md`, `docs/server/metrics.md`, security
  review): phone numbers are PII — **mask E.164 in all structured logs** (never log raw numbers);
  add `ripls_`-prefixed metrics for SMS sends / failures / opt-outs; keep ERROR logs loud for
  genuine failures. `CheckInvitation` and any public landing must **never return a provisional
  member's phone number**.
- **Error handling** (`docs/server/conventions.md`): SMS send failures, opt-out-blocked sends, and
  the STOP/HELP webhook fail-fast with typed errors; user-visible failures use
  `connecterr.UserVisible` + paired ARB keys.
- **Testing & workflow docs** (`docs/testing/update_system_tests.md`): the new invite/respond flows
  change the documented workflows — update `docs/workflows/*.md`, add matching
  `server/integration_tests/` + `e2e/tests/workflows/` coverage, and run
  `/ripls-update-system-tests` to keep the 1:1 mapping. Client work stays ViewModel-first.
- **Accessibility** (`docs/client/accessibility.md`): the new web-response screens (WEB-2/3/4), the
  creation invite surface (CREATE-1), and group-text display (DISPLAY-1) use the accessibility
  primitives with l10n-sourced semantic labels and `semanticsIdentifier`s for Playwright;
  `npm run lint` (a11y + i18n + dart-size gates) must pass.
- **Dependencies** (`docs/reviews/dependency_review_prompt.md`, CLAUDE.md "minimal dependencies"):
  the Twilio SMS API is a single authenticated POST — **evaluate a thin `net/http` REST client
  before pulling the full `twilio-go` SDK**; route the chosen dependency through Renovate /
  `versions.env` and `govulncheck`.

### Phase 0 — SMS provisioning & legal (start immediately, runs in parallel)

#### SMS-1 · A2P 10DLC registration, Twilio setup & legal sign-off — *Task* · was #2493
**Goal:** clear the ~2–3 week external critical path so platform SMS can turn on the moment the
code (SMS-2/SMS-3) is ready. Mostly ops/legal; minimal code (secret wiring + feature flag).
**Companion:** [`2492-sms-a2p-registration.md`](./2492-sms-a2p-registration.md) — step-by-step Twilio
setup, paste-ready campaign content (opt-in flow, sample messages, STOP/HELP copy), the counsel brief,
and the exact secret + feature-flag code-wiring spec.
**Status (2026-06-26):** ✅ **Brand + Campaign APPROVED.** (The earlier submission was REJECTED — Twilio
error 30909, CTA/opt-in unverifiable; the resubmit with real screenshots of the in-app consent surfaces —
phone-entry SMS disclosure #2530; host invite-attestation checkbox, CREATE-1 Share sheet #2527 — cleared
it.) **Platform SMS is unblocked.** Remaining before the SMS flag flips: Twilio secrets → GSM +
`entrypoint.sh`, counsel sign-off. A mockup attempt (PR #2522) was closed. Live progress + non-secret IDs
in the [companion progress log](./2492-sms-a2p-registration.md#progress-log).
**Tasks:**
- [~] Twilio account + Messaging Service (dev + prod); numbers provisioned; SID/auth-token/
      messaging-service-SID in Secret Manager and wired via `server/entrypoint.sh` /
      `scripts/fetch_secret.sh`. — *prod account + Messaging Service "Ripls – Invites & Notifications
      (prod)" + 10DLC number done; **§6 code wiring DONE** (branch `sms2-twilio-provider`: flags +
      `entrypoint.sh` optional fetches/exec args + Terraform stubs); **creating the 3 GSM secrets +
      uncommenting the Terraform grant still pending**.*
- [x] Standard **Brand** registration via Twilio → The Campaign Registry. — *Brand "Ripls" / Type2Fun
      Foundation, PBC **APPROVED** 2026-06-18.*
- [x] **Campaign** registration; opt-in description + sample messages + opt-out copy. — *submitted
      2026-06-18 as **MIXED** (covers invites + notifications + verification); **PENDING_REVIEW**.*
- [x] Sender identity: **10DLC long code** for v1; document the short-code upgrade path.
- [ ] **Geo permissions** locked to sending countries; **SMS Pumping Protection** enabled.
- [x] **Legal:** sign-off on the **host-attestation consent model** and **state-law
      exposure** (esp. WA CEMA, FL FTSA, CA CIPA — a ToS attestation did *not* shield Block/Cash
      App from a **$12.5M CEMA settlement**); draft ToS/consent + single-transactional-invite
      content rules. See [Consent precedent & model](#consent-precedent--model). — *✅ **APPROVED by
      company leadership (acting as counsel), 2026-06-27.** Legal gate cleared; remaining before the
      flag flips is finishing the off-app email backup.*
- [x] `platform SMS enabled` **feature flag** to gate the SMS path (email needs no flag). — *`-platform-sms-enabled`
      (env `PLATFORM_SMS_ENABLED`), default off; branch `sms2-twilio-provider`.*
**Validates:** that platform-originated SMS is legally and operationally cleared.
**Dependencies:** none (start now). Unblocks *enablement* of SMS-2/SMS-3, not their build. **Email
(EMAIL-1) does not depend on this** — it ships in Phase 1.

### Consent precedent & model

> Compliance research, **not legal advice** — counsel must review the SMS attestation wording and
> state-law posture before SMS is enabled. The live Partiful/Evite ToS pages block automated fetch,
> so verify quoted language against the live pages.

**This pattern is mainstream, and Partiful does exactly it.** Partiful's platform sends the invite
SMS from its own short code (397-81) to numbers with no prior Partiful relationship, and also offers
**email invites** (up to 500 email / 1,000 combined). Comparable platform-sent invite flows:
**Evite** (short code 296-94, SMS+email), **Paperless Post**, **Punchbowl**, **Luma**, **Eventbrite**
(email), **Cash App / PayPal**. Lower-risk host-relayed variants: **WhatsApp / Signal** (draft opens
in the user's *own* Messages app), **Venmo** ("only share your link with people you know").

**Why SMS is defensible:** under the FCC's *TextMe* (2015) ruling and the *Poshmark* line, the
**user who picks recipients + timing is the "maker/initiator"**, not the platform — which is why
these apps carry a **host rep-and-warranty** in their ToS (model: Paperless Post — *"you represent
and warrant that you have the Guests' prior express consent, permission or invitation"*). Keep each
send a **single transactional invite** (host + item + link; no marketing; no re-nagging) with
**STOP/HELP** opt-out honored globally.

**But a ToS attestation is not a full shield — the real risk is SMS + state law.** Block (Cash App)
settled **$12.5M** under **Washington's CEMA** for "Invite Friends" referral texts: TCPA was held
*inapplicable* to the assister, but **CEMA reached Block anyway** ("even if the company did not
physically press send"). Federal TCPA defensibility (maker/initiator + single message + STOP) is
necessary but **not sufficient**; state laws (WA CEMA, FL FTSA, CA CIPA) add exposure, and CTIA's
carrier layer treats opt-in as non-transferable at campaign-registration time.

**Email is materially lower-risk.** Email invites fall under **CAN-SPAM** (opt-out, *no prior
consent required*; a friend's single event invite is strongly transactional/relationship). The
requirements are mechanical: accurate headers, a physical postal address, one-click unsubscribe
honored ≤10 business days. **Airbnb deliberately chose email-only referral to dodge SMS risk.** This
is why the plan makes **email the default, ship-first channel** (EMAIL-1) and gates SMS.

**Our consent model — the design rules these platforms converge on:**
1. **Host owns the relationship** — capture a **logged, timestamped attestation** ("I know these
   people and they're OK receiving an invite") before sending to phone numbers.
2. **One transactional message** per recipient per item — no drip, no growth-marketing copy.
3. **Clear sender identity** ("Alex invited you to … via Ripls") so it reads as a personal invite.
4. **Frictionless, global opt-out** — SMS STOP/HELP, email one-click unsubscribe — suppressed at the
   data layer and never re-contacted, even via a different host.
5. **No address-book harvesting** — store only the contacts actually invited.
6. **Prefer email; gate SMS** behind 10DLC + CTIA + counsel-reviewed attestation + state-law review.

Sources: Partiful help center (invite number / email invites), Paperless Post & Evite ToS, FCC
*TextMe* / *Poshmark* (Lexology), FTC CAN-SPAM compliance guide, Block/Cash App CEMA settlement
(National Law Review / Top Class Actions), CTIA Messaging Principles, Airbnb referral (CloudSponge).

### Phase 1 — Events end-to-end via host-relayed links (first shippable slice)

#### Phase 1a — Foundation

#### ID-1 · Contact-keyed provisional member identity — *Feature* · was #2496 — ✅ **DONE (PR #2512)**
**Goal:** a lightweight **contact-keyed** identity (phone **or** email) that is a member of the
unnamed community and is notifiable off-app (SMS or email); foundation for everything else.
**As built (PR #2512, branch `id1-provisional-identity`):** the name-only "shadow user" was
**renamed to `ProvisionalUser`** across protos/server/client + DB (Open Question resolved — see
below), and the contact handle was added.
**Tasks:**
- [x] Add a **contact handle** — `oneof contact { phone_number | email }` (phone-XOR-email, Bryan's
      call) on `models.ProvisionalUser` + `CreateProvisionalUserRequest`; field numbers preserved;
      merge infra reused (`server/provisional/merge.go`).
- [x] Dedup by handle — `CreateProvisionalUser` normalizes (`server/contact.NormalizePhoneE164` /
      `auth.NormalizeEmail`) and finds-or-creates **one provisional user per handle per community**.
      *(Cross-community dedup deferred to ID-2 promotion.)*
- [~] Addressable by recipient-resolution — **deferred to EMAIL-1**, which introduces the off-app
      send seam at `server/notifications/service.go:92` together with the first real sender (no inert
      plumbing landed in ID-1).
- [x] Count as members (visibility/display) but not chat participants — unchanged (provisional users
      are community-scoped, already exempt from the 32-cap and chat).
- [x] Extend `ShareLink.provisional_user_id` (renamed) merge-on-registration to the contact case.
- [x] Update `docs/single_player_mode.md` (+ globs/freshness). PII: handle masked in logs
      (`logging.MaskPhone`), **never exposed on the API**. DB migration `MigrateProvisionalUserRename`
      (copy-then-drop; protosql ensures schema at construction). All shadow-user references eliminated
      from the live surface (rename commit).
**Validates:** that a deviceless phone-or-email person can be a member + (once EMAIL-1 lands) an
off-app recipient. **Next on this foundation:** ID-2 (promote-on-verify, carries the handle onto the
real User + cross-community dedup), EMAIL-1 (recipient resolution + first complete loop), COMM-1.
**Dependencies:** none. Unblocks COMM-1, CREATE-1, DISPLAY-1, ID-2, EMAIL-1, and the SMS audience.
**Key files:** `proto/ripls/models/shadow_user.proto`, `proto/ripls/api/shadow_user.proto`,
`server/services/community/shadow_users.go`, `server/shadow/merge.go`,
`server/notifications/community_subscriber/recipients.go`,
`app/lib/presentation/widgets/shadow_user_avatar.dart`.

#### COMM-1 · Ad-hoc per-item community model + open-link emergent membership — *Feature* · was #2498
**Goal:** every item gets a **nameless** per-item community (its audience); open-link join reuses
existing infra; **naming it promotes it to a real persistent community**. The unnamed-community
**cap exemption is deferred** (see *Deferred* below) — for the first cut an ad-hoc community is
subject to the same 32-member cap as any community, which is fine for the initial events slice.
**Tasks:**
- [x] Add an optional origin-item provenance field to `proto/ripls/models/community.proto`. **No
      `kind` enum** — name-presence is the ad-hoc-vs-real signal (`name == ""` ⇒ ad-hoc). *As built:
      a `oneof origin_item { origin_experience_id | origin_gear_id | origin_request_id |
      origin_transfer_id }` (Bryan's call), mirroring `ShareLink.target`; the variant carries the
      kind so no separate kind field. `community.IsAdHoc(c)` is the canonical check.*
- [x] Provision an unnamed community — *`Service.ProvisionAdHocCommunity(ctx, host, AdHocOrigin,
      extraMembers…)` (`server/services/community/adhoc.go`); shares a `createCommunity` core
      extracted from `CreateCommunity`. **Wiring the item-creation flows to call it is CREATE-1.***
- [x] **Promote-by-naming** — *reused `UpdateCommunity` (Bryan's call): the first unnamed→named
      transition emits `COMMUNITY_EVENT_TYPE_COMMUNITY_NAMED` (once; origin survives). No new RPC.*
- [x] Open-link join via `AcceptInvitationLink` — *reused unchanged; ad-hoc targets need no
      special-casing (confirmed).*
- [x] **Surface unnamed communities in the carousel/workshop** — *server confirmed: `ListCommunities`
      has no name-presence filter and passes the empty name through. Group-text client rendering is
      DISPLAY-1.*
- [x] Lifecycle: an unnamed community **persists** — *default behavior (nothing auto-deletes them).
      Archive-with-item cascade integrates when item-deletion flows are wired (CREATE-1 / item
      lifecycle).*
- [x] New `docs/ad_hoc_communities.md` (+ regenerated `docs/llms.txt`).
**Deferred (future work, was part of COMM-1):** **exempt unnamed communities from
`MaxCommunityMembers`.** Requires auditing every cap-enforcement site to branch on name-presence
(the two enforcing sites today are `server/services/community/invitations.go:164`
`AcceptInvitationLink` and `server/services/login/registration.go:502` register-via-invite; the
`MaxMembers:` reporters in `share_links.go`, `web/event_page.go`, `web/service.go`, `lifecycle.go`,
`registration.go` are display-only). Plus the **promote-by-naming grandfather rule** (an unnamed
group that grew over-cap is grandfathered on naming with a soft nudge; never block the evolve path)
and the `docs/community_health.md` note documenting the exemption + grandfather. Until this lands,
ad-hoc communities hit the 32-cap on open-link join like any community.
**Validates:** that "every participant is always in a community" holds, and that a one-off audience
can become a standing community by simply gaining a name.
**Dependencies:** ID-1. Unblocks CREATE-1, DISPLAY-1, LINK-1, COMM-2.
**Key files:** `proto/ripls/models/community.proto`, `server/services/community/lifecycle.go`,
`server/community/constants.go`, `server/services/community/invitations.go`,
`server/services/feed/service.go`.

#### Phase 1b — Creation, surfacing, link & web auth

> **🔁 PIVOT (Bryan, 2026-06-20): split creation and invitation into two phases.** CREATE-1 below is
> re-scoped to **standalone creation** (just *Save*, no sharing); the invitation work moves to the new
> **INVITE-1** (a per-item Share sheet + "Who's In"). The **server** built in #2527 (`ShareItem`
> orchestration + provisional attach + compliant email + `InviteConsent`) **all stays** — it is the
> backend for INVITE-1. What's **reverted** is only the *at-creation* client UI from #2527 (the unified
> "Share" button + `ShareAudienceSheet`). #2527 is the landing PR for the kept server work; the revert
> + INVITE-1 continue on it (or a fresh branch off it).

#### CREATE-1 · Per-item communities provisioned at item creation — *Feature* · was #2500 — ✅ **DONE & MERGED (#2527)**
> **As built:** see the [#2527 consolidated section](#-2527--create--share--phone-first-journeys-merged-2026-06-23).
> The task list below is the original design spec, kept for traceability; the as-built summary is authoritative.

**Goal:** creating an item just **saves** it; the **create RPC itself provisions the item's nameless
per-item community** (host the sole member) and shares the item into it, so every item is born with
its audience community. Invitation is a separate action (INVITE-1), but **the Share sheet auto-opens
immediately after Save**. The unified-create primary button reads **"Save Event" / "Save Request" /
"Save …"** (per type), never "Share".

**Server (the heart of this item):**
- [ ] Community service: `findAdHocByOrigin(origin)` + `provisionPerItemCommunity(host, origin)`
      (find-or-create host-only nameless community keyed by `origin_item`, idempotent) +
      per-type `Provision{Experience,Gear,Request}ItemCommunity` (provision **and** share the item in).
- [ ] Extract `ShareGearToCommunity` (carrying `availability`) + `ShareRequestToCommunity` cores and
      **wire the Gear + Request `ItemSharer`s** (`VerifyOwner` + `ShareToCommunity`) — only Experience
      was wired in #2527.
- [ ] Inject a narrow `perItemCommunityProvisioner` into the experience / gear / request services
      (wired in `main.go`; no import cycle — community service already takes item-service func values).
- [ ] `SaveExperience` / `SaveGear` / `SubmitRequest`: on **insert**, provision + share into the
      per-item community; **return the per-item `community_id`** in the response.
- [ ] **Decouple `SubmitRequest`** from a mandatory `community_id` (now optional; if set, also share
      there). Move the junction/conversation creation into the shared `ShareRequestToCommunity` core.
- [ ] **Rework `ShareItem`** to find the per-item community by origin (provision-if-missing fallback)
      and *add* to it, instead of provisioning a fresh ad-hoc community.

**Client:**
- [ ] Unified-create primary button → **"Save {type}"** — revert `unifiedCreateShareButton`; drop the
      `ShareAudienceSheet` open and the at-creation community/invitee collection
      (`unified_create_modal.dart` `onShare`, `unified_preview_card.dart`). `_saveEvent`/`_saveGear`/
      `_saveRequest` (`unified_create_save_actions.dart`) drop the share loop and just call the create
      RPC, reading back the per-item `community_id`.
- [ ] **Auto-open the INVITE-1 Share sheet right after a successful Save** (every item type), against
      the returned per-item community. Dismiss → item already saved.
- [ ] Revert the experience-preview-modal invite surface + `PreviewInviteSelector`. **Keep**
      `InvitePeopleSection` + `experience_host_relay.dart` — INVITE-1 reuses them.
- [ ] Remove `selectedCommunityIds`/`invitees`/`inviteConsent` from the *creation* path
      (`UnifiedCreateState`); retire `share_audience_sheet.dart`.
**Kept as-is from #2527 (server — do NOT revert):** `ShareItem` RPC shape, `Invitee` /
`HostInviteConsent` / `InviteConsent` protos, `email.InviteSender` + `EmailSuppression`,
`GetOrCreateShareLink`, the #2526 `Share*` deprecation.
**Validates:** every item is created with its own per-item community (host-only); `ShareItem` adds to it.
**Dependencies:** COMM-1.

#### INVITE-1 · Per-item invitation: Share sheet (QR) + "Who's In" — *Feature* (NEW, 2026-06-20) — ✅ **DONE & MERGED (#2527)**
> **As built (diverged from the spec below):** shipped as an **additive invite** — the share sheet's two
> actions are **Invite people** (chip-complete existing Ripls members) + **Invite community** (add
> not-yet-shared communities), *not* the "Choose contacts"/"Add Community" split below. The
> **device-contacts picker + host-relay SMS** ("Choose contacts", `flutter_contacts`) is **deferred to
> SMS-2/3** (A2P-gated). "Who's In" shipped as a **flat roster + host management** (set RSVP / remove),
> not the roll-up design below. Full detail in the
> [#2527 consolidated section](#-2527--create--share--phone-first-journeys-merged-2026-06-23).

**Goal:** invitation is a distinct action on a saved item, surfacing the same share bottom sheet
centered on the **per-item community's** QR/link. Because every invite now leads into the **per-item
ad-hoc community** (COMM-1), the sheet no longer asks the host to pick which community the invite leads
into.
**Entry points (all open the same redesigned sheet):**
1. **Auto-open immediately after Save** (CREATE-1) — every item type.
2. **"Who's In" → "Invite"** — RSVP item types (events). The "Invite" button already exists.
3. **Item-detail sharing CTA** — for item types **without RSVP** (gear, requests) there's no "Who's
   In"; put a prominent **Share / Invite** call-to-action on the item-detail screen itself.
**Reference:** current sheet = `item_share_sheet.dart` (community picker on top → QR → Copy Link /
Share). Target redesign (Bryan):
**Share-sheet redesign (`item_share_sheet.dart`):**
- [ ] **Remove the top community-picker section** (`community_picker_button.dart` dropdown +
      "N spots left"). The QR/link is **always the per-item community** invite — mint via
      `GetOrCreateShareLink` (community = the item's ad-hoc community, per-item target).
- [ ] **QR code + link text** stay centered; **"Copy Link" + "Share"** move **up, right below** the
      QR/link text (no longer pinned at the bottom).
- [ ] **New "Choose contacts"** button → device contact picker → text a **selected subset** of
      contacts an invite (SMS via host-relay `sms:` pre-fill; each becomes a **provisional member** of
      the per-item community via `ShareItem` invitees — reuses provisional + `InviteConsent` + masking +
      the host-attestation consent that's already built). Adds `flutter_contacts`.
- [ ] **New "Add Community"** button → share the item with an **entire existing community**
      (`ShareItem` `share_to_community_ids`). This is the **only** remaining place that picks a
      community (reuse `community_picker_button.dart` / `CommunitySelectionSheet` here).
- [ ] Keep "Anyone with link can join · Revoke".
**Entry-point surfaces:**
- [ ] Auto-open the sheet after Save (CREATE-1) + keep the "Who's In" **Invite** button (events).
- [ ] **Item-detail sharing CTA for non-RSVP types** (gear, requests) — no "Who's In" there, so add a
      prominent Share/Invite affordance on the item-detail screen that opens the same sheet.
**"Who's In" display redesign (RSVP item types):**
- [ ] Show **communities as single entries** (the per-item ad-hoc audience + any communities added via
      "Add Community") instead of exploding every member; **members who have explicitly RSVP'd are
      listed individually** (the current GOING list). Invited-but-not-yet-RSVP'd roll up under their
      community entry.
**Reuses (server, from #2527):** `ShareItem` (contacts → provisional + SMS/email; Add Community →
`share_to_community_ids`), `GetOrCreateShareLink` (QR), `InviteConsent` + PII masking,
`email.InviteSender`, `InvitePeopleSection` + `experience_host_relay.dart` (for "Choose contacts").
**Key files:** `app/lib/presentation/widgets/sharing/item_share_sheet.dart`,
`.../community_picker_button.dart` (now only under "Add Community"), the experience/request "Who's In"
panel (`experience_content_view.dart`, `experience_pitching_in_screen.dart`, the needs/RSVP
viewmodels), `experience_host_relay.dart`, `invite_people_sheet.dart`.
**Open design Qs:** per-item community provisioned at Save vs lazily at first Invite; "Choose contacts"
SMS copy + whether it routes through `ShareItem` (provisional + host-relay) or a lighter link-only
text; does "Add Community" go through `ShareItem` or the per-type `Share*`.
**Validates:** from a saved item, a host can (a) show a QR / copy-or-share a link into the per-item
community, (b) text a chosen set of contacts, (c) add a whole community — and "Who's In" reads as
communities + individually-RSVP'd people.
**Dependencies:** CREATE-1, COMM-1, LINK-1; `flutter_contacts`; Phase-2 silent A2P stays SMS-2/SMS-3.

#### DISPLAY-1 · Group-text-style display of unnamed communities + provisional members — *Feature* · was #2501 — ✅ **DONE & MERGED (#2543, 2026-06-25)**
**Goal:** nameless communities render group-text style **everywhere they appear (incl. the
carousel)**, with a light "add a name" nudge to promote to real; provisional members are visually
distinct.
**As built (PR #2543, merged 2026-06-25):** all surfaces below are done. A nameless
ad-hoc community now reads as a group everywhere — group-text **name** ("You and Ada, Sam + N others"),
group-chat **avatar** cluster (`GroupAvatar`), and an **origin label** ("from {item}") — and an owner
promotes it to a real community from the **overview header action pill** (not by tapping its switcher
row, which now just selects). Requires the server (`origin_item_name`) — won't render against a
backend that predates this branch.
**Tasks:**
- [x] Shared group-text helper **+ stacked-avatar cluster**, used by the switcher, AccessSheet, the
      community-selection pickers, and the item header.
      *Done — name: `communityDisplayName`/`communityLabel` (carousel/switcher/pickers) plus a new
      `communityDisplayNameFromList` (`community_display.dart`) for call sites that hold only a community
      id + name + member count and resolve the member-preview names from the loaded community list. The
      AccessSheet accordion (`access_sheet.dart`) and the content **item header**
      (`community_context_header.dart`) now render "You and Ada, Sam + N others" for a nameless community
      instead of a blank label. Avatar: a new **`GroupAvatar`** (`group_avatar.dart`) depicts a nameless
      community as a group-chat-style cluster of up to four member circles (viewer first, then
      member-preview faces; photo or colored-initial fallback) — wired into the Workshop switcher, the
      AccessSheet row, and the shared `CommunitySelectionTile` (post-creation / bulk / invite pickers).
      Named communities keep their single avatar. (Inbox has no community-name surface; the feed's
      `community_event_card` already omits an empty name rather than rendering blank.)*
- [x] Provisional-member visual — distinct avatar + status subtitle, handle-agnostic. *Done earlier:
      `ProvisionalUserAvatar` (greyed avatar + `person_off` badge) is used in Manage Members with an
      "Invite pending"/"Joined" subtitle; the treatment keys off the display name, not the phone/email
      handle. (Provisional members surface only via the dedicated `ProvisionalUser` list — they are not
      embedded in RSVP/`CommunityMember` API responses — so the roster/access surfaces have no provisional
      rows to distinguish.)*
- [x] `AccessRing` / `AccessSheet` render unnamed audiences by faces/names (distinct-member count
      intact). *Done: AccessRing already shows the overlapped avatar stack + the server's deduped people
      count; the AccessSheet now leads a nameless row with a stack of up to 3 member **faces** (instead of
      the placeholder community avatar) and the group-text **name**.*
- [x] **Render unnamed communities in the carousel/workshop** with an affordance that promotes the
      community to real. *Done (branch `display1-name-group-nudge`):*
      - **Switcher row** (`workshop_switcher_dropdown.dart`) — identifies a nameless community by what
        spawned it: a **"from {item}"** subtitle (`workshopGroupFromItem`), driven by the new server field
        `CommunityItem.origin_item_name` (resolved in `ListCommunities` from the experience/gear name,
        request title, or — for a transfer — the gear behind it). **Tapping just selects** it (no
        surprising force-name); nameless communities sort after named ones.
      - **Overview** (`workshop_centered_overview.dart`) — the owner names it from a **header action
        pill** ("Name this group", owner-gated), which opens the create/AI modal in promote mode (→
        `UpdateCommunity`). The eyebrow identifies a nameless community by origin too: **"FROM {item} · N
        MEMBERS"**. The Invite CTA was made a **persistent** header pill ("Invite people" when nameless,
        "Invite to {name}" when named) — it previously vanished once a group had impact (it lived only in
        the zero-state `_CrewInvitation`).
      - **Pager desync fix** (`workshop_screen.dart`) — after promoting, `ListCommunities` re-sorts, so
        the pinned community's PageView index moves; a `ref.listenManual` re-aligns the pager to the pin
        (otherwise the overview showed a *different* community than the pill named).
      - Plus a second surface — a "Name this group…" link on the event roster. All verified end-to-end by
        `community-promote-nudge.spec.ts` (asserts the overview names the just-promoted community).
- [x] Widget tests for the helper + the name nudge. *Done: `community_display_test` (helper +
      `communityDisplayNameFromList`), `access_sheet_test` (nameless group-text + faces, named unchanged),
      `community_context_header_test` (nameless vs named chip), the promote-nudge unit tests (switcher
      affordance, roster-link visibility, ListCommunities ordering), and the two-flow web e2e
      `community-promote-nudge.spec.ts` — which now also asserts the switcher renders "You and Ada".*
- [x] Portfolio impact list renders nameless communities group-text (was raw `community_id`).
      *Done client-side: `community_card_list.dart`'s `_CommunityCard` is now a `ConsumerWidget` and
      resolves the label via `communityDisplayNameFromList` from the loaded community list (the user's own
      ranked communities are in it, so member-preview names are available) — no `RankedUserCommunity`
      proto change needed.*
**Validates:** unnamed communities are legible without a name and offer a one-tap path to real.
**Dependencies:** ID-1, COMM-1.
**Follow-up issues (filed, not started):** **#2547** — community gen should stop AI-generating the
name/description and only fetch a relevant image (the promote modal currently invents both). **#2548** —
decide the membership policy for community-shared invitees (do they implicitly join the per-item
community, and what happens at naming?). Today community shares grant item *visibility* only; direct
invitees join.
**Key files:** `app/lib/core/utils/community_display.dart`,
`app/lib/presentation/widgets/group_avatar.dart`,
`app/lib/presentation/widgets/workshop/workshop_switcher_dropdown.dart`,
`.../workshop_centered_overview.dart`, `.../workshop_metrics_sentence.dart`,
`app/lib/presentation/screens/workshop/workshop_screen.dart`,
`app/lib/presentation/widgets/portfolio/community_card_list.dart`,
`app/lib/presentation/widgets/content/access_sheet.dart`, `.../community_context_header.dart`,
`server/services/community/membership.go` (`origin_item_name`),
`app/lib/presentation/widgets/provisional_user_avatar.dart`.

#### WEB-1 · Harden Firebase phone auth on Flutter Web — *Task* · was #2502
**Goal:** make web phone OTP the reliable verify path for SMS/link recipients (prerequisite for
all web response).
**Tasks:**
- [ ] reCAPTCHA on web (verify `firebase_auth_web` invisible auto-reCAPTCHA or add explicit
      setup).
- [ ] Add web hostnames to Firebase authorized-domains.
- [ ] Guard the unconditional mobile-only `getAPNSToken()` call in `phone_auth_screen.dart`
      (~line 336) with `kIsWeb`.
- [ ] Make the router's reCAPTCHA-callback logic (`app_router.dart:60`–`72`) web-aware.
- [ ] Playwright e2e for web phone verification (Firebase test numbers); add a Web row to the
      platform-behavior table in `docs/registration_and_login.md`.
**Validates:** phone OTP completes on Flutter Web.
**Dependencies:** none (parallelizable). Blocks WEB-2/WEB-3/WEB-4.

#### LINK-1 · Ad-hoc open invite links: web view + open-join + response intention — *Feature* · was #2503 — 🟡 **server DONE; client carrier → WEB-2**
**Goal:** the shareable open link lets a non-app recipient view the item + join the ad-hoc
community, carrying a response intention through verification — reusing ShareLink/Accept/`/go`.
**As built (branch `link1-adhoc-open-links`, stacked on `email1-email-channel`):** the recon
(folded below) found the ad-hoc per-item open link **already mints, resolves, previews, and joins via
existing infra** — the only server gap was the public SSR landing rendering an empty `Community.Name`.
That gap is fixed; the client response-intention carrier is deferred to WEB-2 (its natural home).
**Tasks:**
- [x] Mint a per-item `ShareLink` via `GetOrCreateShareLink` with `community_id` = ad-hoc community +
      item target. *Already supported — `GetOrCreateShareLink` accepts any community + a per-item
      target oneof; no change needed.* (Tying `provisional_user_id` for a *seeded* invitee is the
      CREATE-1 closed-invite path, not the open link.)
- [x] Confirm the `/go/{code}` landing + public resolution for ad-hoc targets. *Lookup + item preview
      already target-agnostic; `CheckInvitation`/`fetchItemPreview` resolve pre-auth without leaking
      PII. **Fixed the one gap:** `communityDisplayName` substitutes a generic public label
      ("a Ripls group") for a nameless community at both landing builders (`service.go`,
      `event_page.go`) so titles/OG cards/body copy never render empty.*
- [x] Open-join on response → `AcceptInvitationLink` (ad-hoc community). *Joins by `community_id`;
      confirmed unchanged for ad-hoc in COMM-1.*
- [~] Generalize the event response-intention carrier (`?rsvp=…&code=…`, `WebEventRsvpHandoffNotifier`,
      `EventInviteJoinGate`) — **deferred to WEB-2** (client; per-type RSVP/offer auto-fire fires there).
- [~] Host-relay delivery: native share sheet + `sms:` pre-fill — **deferred to CREATE-1** (it consumes
      this at the creation/invite surface).
**Validates:** a link recipient reaches a non-broken public view of an ad-hoc item + can join the
ad-hoc community. (The pre-verify→post-verify response replay lands with WEB-2.)
**Dependencies:** COMM-1, ID-1; WEB-1 for verify; pairs with WEB-2.
**Key files:** `server/services/community/share_links.go`, `.../invitations.go`,
`server/services/web/service.go`, `event_page.go`, `app/lib/core/router/app_router.dart`,
`app/lib/data/repositories/share_link_repository.dart`.

#### Phase 1c — Close the loop (events v1 milestone)

#### ID-2 · Promote provisional identity to a full PHONE account on verification — *Feature* · was #2497 — ✅ **DONE (server)**
**Goal:** verifying a phone promotes the provisional identity to a real `auth_method=PHONE` user,
migrating membership + history and replaying the pending response.
**As built (branch `id2-promote-on-verify`, stacked on `id1-provisional-identity`/#2512):** a new
cross-community lookup + a phone-keyed promotion step on `PhoneRegister`. The client-side
pre-verify-choice replay carrier is deferred to WEB-2/LINK-1 (see *Deferred*).
**Tasks:**
- [x] Extend `PhoneRegister` + reuse `MergeIntoUser` so a verified phone matching a provisional
      identity merges it (migrate ad-hoc membership + activity; mark claimed). *As built:
      `provisional.FindUnclaimedByPhone` (cross-community lookup by normalized E.164) +
      `login.(*Service).promoteProvisionalByPhone` (idempotent community join +
      `MergeIntoUser` per match), wired into `PhoneRegister` after the user is persisted.
      `MergeIntoUser` needed no change — RSVP + transfer are the only activity references.*
- [~] Carry the pending response intention through verification — **server side done, client carrier
      deferred to WEB-2/LINK-1.** A provisional RSVP's recorded `intention` already migrates onto the
      real account via `MergeIntoUser`, so a pre-recorded response survives promotion. The "tap
      Yes/Maybe/No on the web view *before* verifying, then auto-fire *after*" carrier
      (`WebEventRsvpHandoffNotifier`, `EventInviteJoinGate`, `?rsvp=…&code=…`) is client-side and
      explicitly pairs with WEB-2; LINK-1 generalizes the carrier.
- [x] Idempotency / re-entry / cross-auth-method handled. *Join guarded by
      `auth.IsMemberOfCommunity`; merge re-checks `claimed_by_user_id`; `PhoneRegister`'s existing
      `GetUserByPhone` AlreadyExists guard means promotion only runs for a genuinely new phone account,
      so there is no cross-auth-method collision to resolve. Best-effort: per-community failures are
      logged loudly and skipped, never blocking registration.*
- [x] Dedup across communities (multiple provisional records for one phone → one real user). *`FindUnclaimedByPhone` gathers every community's match; each is joined + merged into the one account.*
- [x] Tests: merge + idempotency + cross-community dedup. *Intention-replay test belongs to WEB-2 (client auto-fire).* `server/provisional/promote_test.go`, `server/services/login/phone_auth_test.go::TestService_PhoneRegister_PromotesProvisionalUsers`.
**Deferred to WEB-2/LINK-1:** the client pre-verify response-intention carrier (auto-fire RSVP/offer
after verification). ID-2 delivers the server promotion + activity/intention *migration*; the
choose-before-verify *replay* is realized when the web response arm is wired.
**Validates:** the progressive-account promotion + cross-community dedup (server side of the response replay).
**Dependencies:** ID-1. Pairs with WEB-2.
**Key files:** `server/services/login/phone_auth.go`, `server/services/login/registration.go`,
`server/provisional/promote.go`, `server/provisional/merge.go`.

#### WEB-2 · Events end-to-end: link/SMS → web view → verify → RSVP (v1 slice) — *Feature* · was #2504
**Goal:** the first complete phone-invite loop, for events — mostly integration of the built arm
with the new identity/community/link path. **This is the milestone that proves the pivot.**
**Tasks:**
- [ ] Wire the event's audience to its ad-hoc community (COMM-1); verifying invitee becomes a
      promoted provisional user (ID-1/ID-2) + member; link is the ad-hoc open link (LINK-1).
- [ ] `RSVPToExperience` (`server/services/experience/rsvp.go`,
      `RequireMemberOfActiveCommunity`) satisfied by joining the ad-hoc community; auto-fire the
      RSVP post-verify.
- [ ] (Optional, recommended) render read-only event content pre-verify via the SSR landing.
- [ ] Keep anonymous "No" decline working (`web_decline.go`).
- [ ] Update the Playwright spec to cover the ad-hoc + provisional-promotion path.
**Validates:** **the entire pivot, end-to-end, for events, with a host-relayed link and no
platform SMS.**
**Dependencies:** ID-1, ID-2, COMM-1, LINK-1, WEB-1.

#### Phase 1d — Email channel (ships without 10DLC)

#### EMAIL-1 · Email notification channel (Mailgun) + email-provisional loop — *Feature* · new — 🟡 **channel DONE; email-provisional promotion deferred to EMAIL-1b**
**Goal:** mirror the off-app arm for **email** invitees, reusing the existing Mailgun integration
(`server/email/`). **No 10DLC / A2P dependency** — the *email* invite loop ships fully in Phase 1
while SMS waits on registration, and email is the **lower-risk default channel** (CAN-SPAM opt-out
vs. TCPA opt-in — see [Consent precedent & model](#consent-precedent--model)).
**As built (branch `email1-email-channel`, stacked on `id2-promote-on-verify`):** the off-app
**email dispatch channel** is complete and tested — the seam at `service.go:92`, the sender, the
localized template, opt-out + one-click unsubscribe, metrics — but **flag-gated off**
(`--off-app-email-enabled`, default false) so enabling email for all deviceless users is a deliberate
ops decision. The **email-provisional cross-community promotion** (the email analog of ID-2) is split
into a fast-follow (EMAIL-1b) — see *Deferred*.
**Tasks:**
- [x] `EmailSender` abstraction on the notification `service` struct (analogous to `SMSSender`),
      backed by `server/email/` (Mailgun); **introduced the no-device → off-app channel branch** at
      `server/notifications/service.go` (`dispatchOffApp`) for the email path (SMS-3 adds the SMS path).
      *Copy is **not** re-rendered: `Notification.Title/Body` arrive already-localized for the recipient
      (the subscriber renders per-recipient locale), so the email reuses them; the template wraps them.*
- [x] Per-locale notification templates (`server/email/notification_template_{en,es}.{html,txt}`,
      following the existing pattern). *(Invite-specific templates land with CREATE-1's invite send.)*
- [~] **Email-provisional promotion → EMAIL-1b (deferred).** The single-link claim already works for
      any handle type via `completeRegistration`'s existing `MergeIntoUser`; the missing piece is the
      *cross-community* email analog of ID-2 (`provisional.FindUnclaimedByEmail` +
      `promoteProvisionalByEmail` on `EmailRegister`). Small, symmetric with ID-2; tracked separately.
- [x] **CAN-SPAM mechanics:** branded From (existing), one-click unsubscribe (`GET /email/unsubscribe`,
      signed HMAC token via `auth.SignUnsubscribeToken`), durable opt-out (`User.off_app_email_opted_out`,
      field 21). *Physical postal address is a `TODO(#2492)` placeholder in the template — must be filled
      before the flag flips; that's why the channel ships flag-off.*
- [x] **Platform-sent**; `ripls_off_app_notifications_total{channel,outcome}` metric; masked logging
      (never logs the address); unit tests at email/notifications/web layers.
**Deferred to EMAIL-1b:** the email-provisional cross-community promotion (above) and filling the real
CAN-SPAM postal address before flipping `--off-app-email-enabled` on.
**Validates:** a deviceless person with an email on file is reached by email instead of being silently
dropped; (with EMAIL-1b + WEB-2) a non-app person invited by **email** can be notified → respond →
verify → become a real user, no external registration.
**Dependencies:** ID-1 (email-keyed). Independent of SMS-1/2/3. Pairs with WEB-2 for the response
loop.
**Key files:** `server/email/` (mailgun.go, notification_template_*), `server/notifications/`
(service.go, off_app_email.go), `server/services/web/email_unsubscribe.go`, `server/auth/unsubscribe.go`.

### Phase 2 — Turn on platform-originated SMS

#### SMS-2 · Twilio SMS provider + STOP/HELP/opt-out behind the notification abstraction — *Task* · was #2494 — ✅ **DONE (branch `sms2-twilio-provider`)**
**Goal:** "send an SMS to an E.164 number, compliantly." Built against the thin-client design; the
SMS-1 §6 secret/flag scaffold is folded in (the user chose to land both together). **Flag stays
off** — the build is inert until the Twilio secrets exist + counsel signs off.
**As built:** `SMSSender` is a one-method seam in `notifications`; `notifications/sms.TwilioSMSSender`
is the **thin `net/http`** implementation (decision: thin client over `twilio-go` — the SDK adds
deprecated transitive deps + typed clients for products we don't use; **no Twilio type escapes the
`sms` package**). Sends route through a **Messaging Service SID**. Opt-out is a stateful per-E.164
store (`server/smsoptout`, `models.SmsOptOut` + unique index), consulted before every send
(**fail-closed**) and toggled by the webhook. `service.sendOffAppSMS` renders via l10n + suppresses +
sends + records metrics. Doc: [`docs/sms_notifications.md`](../sms_notifications.md).
**Tasks:**
- [x] SMS sender; separate **`SMSSender`** field on the notification `service` struct (`SMSChannel` +
      `WithSMS` in `off_app_sms.go`) — SMS doesn't fit `providers map[DevicePlatform]Provider`.
- [x] **Thin `net/http` REST client chosen over `twilio-go`** (CLAUDE.md minimal-deps; mirrors the
      `server/location` Mapbox/Google clients) — **zero new deps**, so nothing to route through
      Renovate/`versions.env`/`govulncheck`. Rationale recorded in `docs/sms_notifications.md`.
- [x] Render notification + the branded link **via the server l10n catalog** (`sms.notification.body`,
      en+es) — not inline; the two new SMS-rendering files are added to the no-inline-strings guard.
- [x] Webhook for `OptOutType`/keyword (STOP/HELP/START) at **`POST /sms/webhook`**, **X-Twilio-Signature
      verified** before any state change; **persist opt-in/out per E.164** (`models.SmsOptOut`) and
      suppress future sends. **Persist-only**: Twilio's Messaging Service opt-out management owns the
      compliant auto-replies (our webhook acks with an empty `<Response/>` to avoid double-texting).
- [x] Metrics `ripls_off_app_notifications_total{channel=sms,…}` + `ripls_sms_opt_outs_total{keyword}`;
      **PII-masked** logs (`logging.MaskPhone`), body never logged.
- [x] Content rules: branded link + "Reply STOP to opt out, HELP for help." footer; own `ripls.app`
      link domain.
- [x] Unit tests: send form/auth + provider-error PII non-leak, signature (Twilio's worked example),
      opt-out cycle/idempotency, webhook keyword parsing + signature rejection, l10n render.
      `docs/sms_notifications.md`.
- [x] **SMS-1 §6 wiring folded in:** `twilio-*` flags + `resolveSecret` + `-platform-sms-enabled`
      (default off); `entrypoint.sh` optional fetches + env-gated flag + exec args; Terraform
      `PLATFORM_SMS_ENABLED` env (var false) + commented secret stubs in dev+prod.
**Still ops/legal (SMS-1 exit):** create the 3 GSM secrets, uncomment the Terraform grant, counsel
sign-off — then flip the flag. `connecterr.UserVisible` is N/A (the webhook is an HTTP endpoint, not a
Connect RPC; send failures are best-effort + logged, never surfaced to a user).
**Validates:** compliant SMS send + opt-out.
**Dependencies:** SMS-1 secrets (the code is inert without them). Blocks SMS-3 (channel selection).

#### SMS-3 · Off-app channel selection in the dispatcher (no-device fallback) — *Task* · was #2495 — ✅ **DONE (branch `sms2-twilio-provider`)**
**Goal:** the dispatcher picks an **off-app channel** when a recipient has no active app device —
**SMS for a phone handle, email for an email handle** — transparently for all notification paths.
**As built:** `dispatchOffApp` loads the recipient once and **prefers SMS for a phone-bearing recipient**,
falling back to email so the person is still reached. `sendOffAppSMS` now takes the loaded user + returns
whether it sent; `sendOffAppEmail` was extracted from the old email-only `dispatchOffApp`.
**Tasks:**
- [x] **Channel-selection branch** in `dispatchOffApp` (`off_app_email.go`): phone handle → `sendOffAppSMS`,
      else email. Phone resolved from `User.phone_number`. *(All-stale-devices path: deferred — the existing
      `len(devices)==0` seam covers the deviceless case; stale-but-present devices still get push.)*
- [x] One change covers all `NotifyUser` call sites (it's the single no-device branch in `service.go`).
- [x] **Quiet hours for SMS** — `server/quiethours` (22:00–07:00 local via `User.preferred_timezone`),
      extracted to a shared package so notifications + scheduled_notifications share one window (no import
      cycle). Real-time sends can't defer like the scheduled dispatcher, so a quiet-hours text **falls back
      to email** instead. *(Window is 22:00–07:00, the platform-wide quiet window, not the 8am–9pm/9am-TX
      originally sketched — unified with the existing push quiet hours.)*
- [x] Dedup: off-app fires only when `len(devices)==0` (no push+off-app double-send); within off-app a
      single channel is chosen (no email+SMS double-send). Email fallback on SMS opt-out/quiet-hours/failure.
- [x] **Only SMS is flag-gated** (`-platform-sms-enabled`); email needs no flag. Flag off ⇒ `sendOffAppSMS`
      returns false ⇒ every deviceless recipient gets email, exactly as before.
**Validates:** deviceless users get the right off-app channel (SMS or email); device users still get push;
no dupes. Tests: per-channel `sendOffAppSMS` unit cases + end-to-end selection through `NotifyUser`.
**Dependencies:** EMAIL-1 (the seam), SMS-2 + SMS-1 (SMS path). **Deferred follow-ups:** per-user channel
*preference* override (today's policy is phone⇒SMS); provisional-recipient (no `User` row) resolution.

#### NOTIF-1 · Off-app notification copy rework + cold-contact invites — *Feature* (NEW, 2026-06-27)
**Status:** **P1 (server) ✅ DONE & MERGED in #2572** — bundled into the off-app email-backup PR so
messaging went live with the reworked phrasing, not the weak push-derived copy. **P2 (client contacts
picker) is DEFERRED — host-relay already covers cold contacts and a contacts-permission prompt is too
invasive for the marginal value (Bryan, 2026-06-29); P3 (product flip to un-gate cold SMS) remains.**
Design converged through a review discussion
(decisions locked below).

**As built (P1, #2572):** the `server/notifications/notification_content` package (`kinds.go` +
`notification_content.go`) defines `NotifyContent` (built via `FromCommunityEvent`) and the three pure
renderers `RenderPush`/`RenderSMS`/`RenderEmail`, wired into `off_app_sms.go` / `off_app_email.go` /
`community_subscriber/copy.go`. The per-kind `notif.{kind}.message` + `notif.{kind}.cta` catalog,
`Ripls:`/blank-line/opt-out wrappers, the `text/template` plain-text fix in `email/mailgun.go`, and the
render-all utility all landed. **Note:** the utility shipped as **`server/cmd/notification_examples/`**
(not the planned `server/cmd/notif_examples/`); it renders every kind × surface to a reviewable page
(see the `notification-examples.html` staging page from the PR). **Refinements added during review,
beyond the original spec:** STOP/HELP opt-out footer only on the first message + ~monthly rather than
every text (further dropping → **#2573**); stopped notifying on request-fulfilled / offer-selected; and
a copy polish pass (articles, no quotes, push giveaway/loan split, de-jargoned transfers).

**Why.** Reviewing the *rendered* copy from the enabled channels (via a throwaway renderer) showed the
off-app channels reuse the **push** `Title`/`Body` verbatim, which reads wrong off-app: no brand on SMS
(unknown 10DLC), lost context (`Sam is attending your event` — *which event?*), no call-to-action on the
`/go` link, and **cold contacts get generic framing** (the cold-invite email is literally `{HostName}
invited you on Ripls` — `email.invite.{subject,body}` take only `HostName`, no item name, no CTA). Plus:
no surface to review all copy at once, and the plain-text email part is HTML-escaped (`&#39;`).

**Requirements (from review, locked).**
1. `Ripls:` prefix on every platform SMS (unknown 10DLC → brand up front).
2. Cold-contact invites are onboarding-grade: named host + item + clear CTA (*"A. Maintainer invited you
   to Backyard BBQ."*) — the top of the funnel, must be exactly right.
3. Entity context inline in SMS/email (name the item; push relies on title, SMS/email don't have one).
4. A CTA phrase bound to every link — `RSVP at`, `Manage RSVPs at`, `Claim at`, `Offer at`, `View at`.
5. SMS layout: message · blank line · opt-out line · blank line · `{CTA} {link}`.
6. Uniform params + polymorphic per-surface renderers (late-bind copy to *kind × surface*).
7. A render-all-examples utility/test — every kind × surface (push/SMS/email, en+es) — for
   marketing-quality review **and** a golden regression.
8. Device-contacts selection in the invite modal's "Invite people" to feed cold-contact invites (re-adds
   the deferred contact picker).

**Decisions locked (this design discussion, Bryan).**
- **Copy model = message + CTA verb, uniform wrappers.** Per kind the catalog adds exactly two strings:
  a self-contained **`notif.{kind}.message`** (entity inline) + a **`notif.{kind}.cta`** (verb). The
  `Ripls:` prefix, blank-line layout, opt-out line, and email shell are **uniform wrappers**, not per-kind.
  Push keeps its existing title/body.
- **Invitation is a first-class kind**, rendered through the same system (retires the bespoke
  `email.invite.*` template; one consistent voice).
- **Cold vs warm = the path, not a heuristic.** A **provisional** recipient (off-app handle, no account)
  is *cold* — reached only by the invite send (`ShareItem` → `email.InviteSender`, phone host-relayed).
  `NotifyUser` only reaches **real** members (it loads a `User` row), so activity notifications are always
  *warm*. The special path the cold contact needs already exists; we elevate it.
- **Cold SMS invites: design the copy + renderer now, gate the send.** Cold **email** invites are already
  platform-sent (CAN-SPAM) → ship the improved copy now. Cold **SMS** invites stay host-relayed by default;
  the platform-branded cold SMS is built + review-ready but **gated** until a deliberate product flip
  (within the registered A2P invite campaign, but it activates cold-stranger texting — the highest-touch
  compliance surface).
- **Send mechanism is a render param:** `host-relay` (personal, from the host's number, no `Ripls:`/STOP)
  vs `platform` (branded `Ripls:` + STOP/HELP).

**Design.**
- **`NotifyContent`** — a server-side descriptor (not on the wire; all three surfaces render server-side):
  `Kind`, `ActorName`, `Target{Kind, Name, ExperienceID/GearID/RequestID}`, `CommunityName`,
  `RecipientIsOwner`, `Mechanism`. `community_subscriber/copy.go::buildNotification` is refactored to
  **separate DB name-resolution from copy rendering** — it produces `NotifyContent`; the renderers are
  pure (which is what makes the render-all utility + golden test cheap).
- **Three pure renderers, late-bound by `Kind`:** `RenderPush → (title, body)` (≈ today);
  `RenderSMS → "Ripls: {message}\n\nReply STOP to opt out, HELP for help.\n\n{cta} {link}"` (the
  `host-relay` mechanism drops `Ripls:`/STOP); `RenderEmail → (subject, body)` in the shell + CAN-SPAM
  footer. Uniform wrappers: `sms.wrapper.{platform,relay}`, `common.sms_optout`, the email shell.
  Example targets:

  | Kind | message | cta |
  |---|---|---|
  | invitation (event) | `{Actor} invited you to '{Target}'` | `RSVP at` |
  | experience_rsvp_yes | `{Actor} is attending '{Target}'` | `Manage RSVPs at` |
  | request_offer_made | `{Actor} offered to help with '{Target}'` | `View the offer at` |
  | gear_shared | `{Actor} shared {Target}` | `Claim it at` |

- **Cold (invitation) kind** replaces `email.invite.*`: subject = *"{Actor} invited you to {Target}"*,
  body = invitation message + CTA. **Thread the item name** into `InviteEmailInput` (today it carries only
  `HostName` + `ShareURL`). Platform-SMS variant (gated) = the branded form; host-relay variant (client
  `sms:` prefill) = personal phrasing.
- **Render-all-examples utility + golden test** (`cmd/notif_examples` and/or a `_test.go`): enumerate every
  `Kind` with representative `NotifyContent` → render push/SMS/email in en+es → write a golden artifact for
  review + regression.
- **text/plain fix:** `email/mailgun.go::buildLocalizedTemplates` parses `.txt` with `html/template`; switch
  to `text/template` (the `text` map becomes `*texttemplate.Template`; `Execute` is identical). Fixes
  `&#39;`/`&amp;` in the plain-text part of notification + password-reset + waitlist emails.
- **Client contacts selection:** add a "Choose contacts" affordance to `invite_people_sheet.dart` (new dep
  `flutter_contacts`, permission-gated, store only invited contacts). Selected phone contacts → `ShareItem`
  invitees → provisional phone members → cold-contact send (host-relay now; platform when the flag flips).

**Phasing.**
- **P1 — server copy rework — ✅ DONE & MERGED (#2572).** `NotifyContent` + three renderers + per-kind
  message/CTA catalog + `Ripls:`/layout wrappers + text/plain fix + render-all utility. Warm activity
  SMS/email **and** cold **email** invite copy improved immediately; platform cold-SMS built but gated off.
- **P2 — client contacts picker — 🛑 DEFERRED (Bryan, 2026-06-29).** `flutter_contacts` in the invite
  modal feeding provisional invites. **Why deferred:** the cold-contact case is *already covered by
  host-relay* (the host shares the `/go` open link / `sms:` prefill from their own device), and a
  device-contacts-permission prompt is **disproportionately invasive** for the incremental value over
  host-relay. Revisit only if host-relay proves insufficient in practice. No `flutter_contacts` dep is
  added; the invite modal's off-app paths stay on the "coming soon" stub + host-relay link.
- **P3 — product flip (separate, Bryan's call) — ⬜ NOT STARTED.** Un-gate platform-sent cold SMS invites.

**Open questions.** The full CTA-verb table per kind; activity email-subject style (terse headline vs
"{Actor} {action} {Target}"); golden artifact location (`docs/` for review vs `testdata/` for a pure gate)
+ whether it's a CI gate; does chat off-app get the full treatment now or a minimal fallback.

**Key files.** `server/notifications/community_subscriber/copy.go` (buildNotification → NotifyContent),
`server/notifications/{off_app_sms,off_app_email,deeplink}.go`, `server/email/{mailgun,invite}.go`,
`server/services/community/share_item.go` (thread item name), `server/l10n/source/{en,es}.toml`,
`app/lib/presentation/widgets/sharing/invite_people_sheet.dart` (+ `flutter_contacts`), new
`server/cmd/notif_examples/`.
**Dependencies:** EMAIL-1/SMS-2/SMS-3 (the channels), CREATE-1/INVITE-1 (the invite surface), the `/go`
link resolver from #2572.

### Phase 3 — Extend to other item types + host controls

> **WEB-3 + WEB-4 — ✅ DONE together (branch `web34-item-web-response`, stacked on #2552).**
> One **generic item web-response arm** was built for both non-RSVP types, mirroring the event arm.
> The detail under each item below is the original spec; the as-built notes are authoritative.
>
> **As built (the shared mechanics):**
> - **Server SSR (no RPC change).** Each item type gets its own phone-first landing dispatched from
>   `HandleInvitePage`: `gear_page.go`/`gear.html` for `gear_id`+`transfer_id` (transfer resolves to its
>   gear; CTA adapts loan **"Ask to borrow"** vs giveaway **"I want this"** → `/item/{gearId}?intent=interest`),
>   and `request_page.go`/`request.html` for `request_id` (**"Offer to help"** → `/need/{requestId}?intent=offer`).
>   Both reuse the `item_preview.go` resolvers + `event.css`. A pure community invite still renders
>   `invite.html`; legacy `?gear_id=` query-param links keep the old behavior (TODO #2058).
> - **Client.** One auth-exempt `WebItemActionScreen` (`/item/{gearId}`, `/need/{requestId}`, branch on
>   `WebItemKind`) mirrors `WebExperienceScreen`: unauthenticated + intent → `/verify-phone`; post-verify →
>   join the ad-hoc community (reusing the generic `AcceptInvitationLink` join gate) → auto-fire the action.
>   `WebGearInterestHandoffNotifier` (→ `ExpressInterest(gearId)`) + `WebRequestOfferHandoffNotifier`
>   (→ `OfferToFulfill(requestId, communityId)`, community resolved from the share code) share a retry/backoff base.
> - **Phone-first generalized.** `/rsvp-phone` → generic `/verify-phone`; `PhoneAuthScreen._isRsvpFlow` →
>   `_isGuestActionFlow` (item-typed header copy "…to claim {item}" / "…to help with {item}"); the post-auth
>   redirect + `postRegistrationDestination` map `gear_id`→`/item`, `request_id`→`/need`.
> - **No web-response RPC change.** `ExpressInterest`'s `findMatchingCommunityAndType` and
>   `OfferToFulfill`'s `RequireMemberOfActiveCommunity` have no name-presence filter, so joining the item's
>   nameless ad-hoc community satisfies them like any community (the load-bearing recon finding).
> - **Gear creation now provisions its per-item community.** A follow-on found `SaveGear` was the only
>   create RPC that did *not* provision its ad-hoc community (it was lazy via `ShareItem`), so a new gear
>   had no audience community. Fixed: `SaveGear` provisions + shares in (defaulting `FOR_LOAN`) and returns
>   `item_community_id`, via an injected provisioner composed in `main.go` (avoids a gear→community-service
>   dependency). Asserted by `item-creation-adhoc-community.spec.ts` (gear + request: nameless community,
>   invite a member, reuse-not-duplicate).
> - **Hermetic e2e DB.** The harness now provisions a uniquely-named Postgres DB per run
>   (`e2e/lib/ephemeral-db.ts`, dropped in teardown) so reused fixtures (the phone-first specs' fixed phone
>   numbers) never collide across local re-runs; CI already used a throwaway container.
> - **Verified.** Workflow docs `phone_first_gear_interest.md` + `phone_first_request_offer.md`; e2e
>   `phone-gear-interest-full-loop.spec.ts` + `phone-request-offer-full-loop.spec.ts` (real phone OTP via the
>   Auth Emulator) + `item-creation-adhoc-community.spec.ts` **green**, and the event `phone-rsvp-full-loop`
>   spec still green after the `/verify-phone` rename. Server tests in `gear_page_test.go`/`request_page_test.go`,
>   `gear/service_test.go` (provisioner); client `web_action_handoff_view_model_test.dart`.
> - **Deferred:** no anonymous decline path for gear/request (interest is opt-in, by design); an optional
>   read-only pre-verify content view beyond the SSR landing.

#### WEB-3 · Requests: web view + verify + OfferToFulfill — *Feature* · was #2505 — ✅ **DONE**
**Goal:** extend the WEB-2 pattern to requests (no web-response surface exists today).
**Tasks:**
- [x] Web view: SSR request landing (`request_page.go`/`request.html`) + the auth-exempt `/need/{id}`
      web route (`WebItemActionScreen`).
- [x] `OfferToFulfill` (`server/services/request/offers.go`) satisfied by joining the request's ad-hoc
      community; auto-fired post-verify by `WebRequestOfferHandoffNotifier`. *No server change.*
- [x] Response-intention carrier extended to an "offer" intention (`?intent=offer`).
- [x] Playwright e2e (`phone-request-offer-full-loop.spec.ts`). *(Read-only pre-verify view deferred.)*
**Validates:** the loop generalizes beyond events.
**Dependencies:** WEB-2 (pattern), WEB-1, ID-2, COMM-1, LINK-1.

#### WEB-4 · Giveaways & loans: web view + verify + ExpressInterest — *Feature* · was #2506 — ✅ **DONE**
**Goal:** extend the pattern to transfers (gear).
**Tasks:**
- [x] Web view: SSR gear landing (`gear_page.go`/`gear.html`, handles `gear_id`+`transfer_id`) + the
      auth-exempt `/item/{gearId}` web route (`WebItemActionScreen`).
- [x] `ExpressInterest` (`server/services/transfer/interest.go`, `findMatchingCommunityAndType`)
      satisfied by joining the gear's ad-hoc community. *No server change — no name-presence filter.*
- [x] Both `FOR_LOAN` and `FOR_GIVEAWAY` handled (CTA + copy adapt off `CommunityGear.availability`).
- [x] "express interest" intention carrier (`?intent=interest`); Playwright e2e
      (`phone-gear-interest-full-loop.spec.ts`). *(Read-only pre-verify view deferred.)*
**Validates:** all four item types covered.
**Dependencies:** WEB-2 (pattern), WEB-1, ID-2, COMM-1, LINK-1.

#### COMM-2 · Optional host controls for ad-hoc communities — *Feature* · was #2499
**Goal:** opt-in brakes on the open-link model (Partiful-style).
**Tasks:**
- [ ] Require-approval gate: link tap → pending → host approves/denies (statuses
      Approved/Pending/Removed), in `AcceptInvitationLink`.
- [ ] Optional capacity (default none); remove/block a member; guest-list visibility toggle.
- [ ] Tests for approval, capacity, removal.
**Validates:** hosts can tighten an ad-hoc community when they want.
**Dependencies:** COMM-1. Lower priority.

### Phase 4 — Future

#### RCS-1 · RCS Business Messaging — *Task* · was #2507 · **✅ SUBMITTED 2026-07-01, in carrier review**
**Goal:** branded RCS sender with rich cards + auto SMS fallback; a registration + template task,
not a re-architecture (Twilio RCS rides the same API). **Canonical detail: `2492-rcs-business-messaging.md`.**
**Tasks:**
- [x] Register + verify an RCS Agent — Google + US/Aegis registration **submitted together 2026-07-01**
      (in review ~5–9 wk; Aegis + carrier fees at approval). Compliance prep (#2574) merged.
- [x] Branded sender live in the prod Messaging Service (`XE841…`); RCS-first + auto SMS fallback verified
      end-to-end in prod; welcome-on-opt-in send built + deployed; use-case video at `ripls.org/assets/rcs-demo.mp4`.
- [x] No changes at `NotifyUser` call sites — sends already go via the Messaging Service SID, so RCS-or-SMS
      is a pool property (no send-code change). #2609 per-recipient gating CLOSED (cold messaging parked).
**Validates:** richer invites without re-plumbing dispatch. **Remaining: await approval, then it's launched.**
**Dependencies:** SMS-1/2/3. Deferred until platform SMS is proven in production.

---

## Progress / Status

One or more PRs per item. Update Status + PR as work lands.

| # | Item | Phase | Type | Status | PR |
|---|------|-------|------|--------|----|
| SMS-1 | 10DLC registration + Twilio + legal | 0 (parallel) | Task | ✅ **CLEARED + ENABLED** — A2P Brand + Campaign approved; consent + counsel sign-off done; Twilio secrets in GSM + entrypoint. **`platform_sms_enabled` flipped `true` in Terraform (dev+prod), merged #2572** — SMS turns on next deploy. | #2508, #2530 (consent, merged); #2566 (secrets/flag, merged); #2572 (flip, merged) |
| ID-1 | Provisional phone identity | 1a | Feature | **DONE & MERGED** — ProvisionalUser rename + contact handle + DB migration | **#2520 (merged)** |
| COMM-1 | Ad-hoc community model | 1a | Feature | **DONE & MERGED** — ad-hoc model + ProvisionAdHocCommunity + promote-by-naming + **audience dedup** (`member_signature`); cap exemption deferred | **#2520 (merged)** |
| CREATE-1 | Per-item communities at item creation | 1b | Feature | ✅ **DONE (server + client).** Server: create RPCs provision the per-item community (events/requests) / gear lazy via `ShareItem`; `SubmitRequest.community_id` optional; `ShareItem` adds-to. Client: unified create is **Save-only** (button → "Save {type}"), at-creation Share UI reverted (`share_audience_sheet.dart` + `preview_invite_selector.dart` deleted), **auto-opens the Share sheet after Save** (home_screen + blank_create_dispatcher). Full `flutter analyze` clean; create-flow tests updated. | **#2527 (merged)** |
| INVITE-1 | Per-item invite: Share sheet (QR) + Who's In | 1b | Feature | ✅ **DONE — additive-invite redesign (2026-06-22).** Glass share sheet always opens with **Invite people** (chip-complete existing Ripls members across the host's communities) + **Invite community** (add not-yet-shared communities). **"Who's In"** = flat roster + per-row status pills + a Communities section ("N hasn't responded"); directly-invited individuals show as "Invited". **Host management:** tap a roster row → set RSVP / remove (`SetExperienceMemberRSVP`/`RemoveExperienceMember`); re-invite-after-remove restores the membership. **Real-time:** host-set RSVP streams to the member's open screen (`EXPERIENCE_RSVP_*` → content). Invited individuals now appear on the invitee's **home/calendar**. e2e: `event-invite-home`, `event-rsvp-streaming` (both green, CI). Off-app SMS "Invite people" still gated on A2P (re-add device-contacts picker with SMS-2/3). | **#2527 (merged)** |
| DISPLAY-1 | Group-text display | 1b | Feature | **DONE & MERGED** (#2543, 2026-06-25) — group-text **name** for nameless communities everywhere they surface (pickers/roster/switcher, AccessSheet, item header, portfolio) via `communityDisplayName`/`communityDisplayNameFromList`, plus a group-chat **`GroupAvatar`** cluster on those surfaces. Promote nudge reworked: switcher rows **select** and show a **"from {item}"** origin label (server field `origin_item_name`), naming moved to the **overview "Name this group" header pill**; persistent **Invite pill**; **pager re-align** on list re-sort; light-mode phone-RSVP theming fix. Host-only single-member ad-hoc ones suppressed (#2527). Provisional-member visual already shipped (`ProvisionalUserAvatar` + status subtitle). Widget + server tests + two-flow web e2e. Follow-ups: #2547, #2548. | **#2543 (merged)**; #2527 (merged) |
| WEB-1 | Harden web phone auth | 1b | Task | **DONE & MERGED** — kIsWeb-guard getAPNSToken + web-aware router redirect | **#2520 (merged)** |
| LINK-1 | Ad-hoc open links + intention | 1b | Feature | **DONE (server) & MERGED** — nameless-community public landing, **localized** (l10n); client carrier in WEB-2; remaining SSR i18n → #2090 | **#2520 (merged)** |
| ID-2 | Promote-on-verify | 1c | Feature | **DONE (server) & MERGED** — promote-on-verify across communities | **#2520 (merged)** |
| WEB-2 | Events end-to-end (v1 milestone) | 1c | Feature | **DONE & MERGED** — full phone-first loop e2e via Auth Emulator (invite→web→phone OTP→promote→RSVP) running in CI. Plan: [`2492-web2-pivot-e2e.md`](./2492-web2-pivot-e2e.md) | **#2520 (merged)** |
| EMAIL-1 | Email channel (Mailgun) + email-provisional loop | 1d | Feature | **Channel DONE & MERGED** — off-app email + opt-out + unsubscribe (#2520). **EMAIL-1b + enablement DONE & MERGED (#2572):** `FindUnclaimedByEmail` + `promoteProvisionalByEmail` on `EmailRegister`/`OIDCRegister`; CAN-SPAM postal address filled (single `mailgun_postal_address` config; default corrected Denver→Boulder in #2575); `off_app_email_enabled` flipped true in Terraform; off-app SMS/email now carry a `/go` short link (`ShortLinkCodeForEntity`). Verified-email gate → #2571. In-app opt-out toggle → #2524. | **#2520, #2572 (merged)**; #2575 (postal fix) |
| SMS-2 | Twilio provider + opt-out | 2 | Task | ✅ **DONE & MERGED** — thin `net/http` Twilio provider behind a one-method `SMSSender` seam (no SDK; no Twilio type escapes the `sms` pkg); Messaging-Service-SID sends; per-E.164 opt-out store (`models.SmsOptOut`, fail-closed before every send); `POST /sms/webhook` with X-Twilio-Signature verification, **persist-only** (Twilio owns the replies); l10n copy (en/es); `ripls_off_app_notifications_total{channel=sms}` + `ripls_sms_opt_outs_total{keyword}`; masked-phone logs. **SMS-1 §6 secret/flag scaffold folded in** (flag off); secrets + Terraform grants applied in dev+prod. **Outbound delivery + inbound STOP/START/HELP both verified end-to-end on real Twilio.** Doc: `docs/sms_notifications.md`. Follow-up: delivery **status callbacks → #2569**. | **#2566 (merged)** |
| SMS-3 | Off-app channel selection (SMS + email) | 2 | Task | ✅ **DONE & MERGED** — `dispatchOffApp` chooses by handle (phone→SMS, else email); SMS falls back to email on disabled/no-phone/quiet-hours/opt-out/failure. Shared `server/quiethours` window (22:00–07:00 local). Flag-gated; off ⇒ email as before. Unit + end-to-end selection tests. | **#2566 (merged)** |
| WEB-3 | Requests web response | 3 | Feature | ✅ **DONE & MERGED** — SSR request landing (`request_page.go`/`request.html`, "Offer to help" CTA → `/need/{id}?intent=offer`) + `WebItemActionScreen` + `WebRequestOfferHandoffNotifier` (auto-fires `OfferToFulfill`, resolving the community from the share code). No server RPC change. Workflow doc + phone-first e2e green. | **#2560 (merged)** |
| WEB-4 | Giveaways/loans web response | 3 | Feature | ✅ **DONE & MERGED** — SSR gear landing (`gear_page.go`/`gear.html`, handles `gear_id`+`transfer_id`; CTA adapts "Ask to borrow"/"I want this" → `/item/{id}?intent=interest`) + `WebItemActionScreen` + `WebGearInterestHandoffNotifier` (auto-fires `ExpressInterest`). No server RPC change (`findMatchingCommunityAndType` already accepts the ad-hoc community). Workflow doc + phone-first e2e green. | **#2560 (merged)** |
| SHARE-1 | In-app "Shared with" surface (gear/request) + invitee polish | 1b/3 | Feature | 🟢 **DONE on branch `finish_pivot` (NOT yet PR'd).** `SharedWithCard` on gear+request read shells ("Shared with N" + invited faces → `AccessSheet`/`ItemShareSheet`); server `invited_individuals` on `GetGearResponse`/`Request` (shared `FetchInvitedIndividuals`). Drop-into-item after share (all types); `AccessSheet` title "Shared with", lists the item's own ad-hoc community + people **by name**, **updates in place** on add. Fixes: access surfaces trust server-scoped `sharedCommunities` (not client `communitiesProvider`) — kills the blank-panel + community-add bugs; `refreshRequest()` keeps `sharedCommunities` fresh; **returning member logs in** (`PhoneAuthScreen`→`PhoneLogin` on `AlreadyExists`). New e2e: gear/request authoring-share, gear-giveaway phone-first, returning-member login. See [E2E coverage](#e2e-coverage-finish_pivot). | **#2589 (open)** |
| NOTIF-1 | Off-app notification copy rework + cold-contact invites | 2 | Feature | 🟡 **P1 (server) DONE & MERGED (#2572); P2 + P3 remain.** Shipped: `server/notifications/notification_content` (`NotifyContent` + `RenderPush`/`RenderSMS`/`RenderEmail`); per-kind message + CTA-verb catalog; `Ripls:` prefix + entity-inline + CTA-with-`/go`-link SMS layout; `&#39;` text/plain fix; render-all utility (`server/cmd/notification_examples/`). Refinements: opt-out footer first-message+~monthly (→ #2573), stopped notifying request-fulfilled/offer-selected, copy polish. **P2** = device-contacts picker (`flutter_contacts`) — **🛑 DEFERRED** (host-relay already covers cold contacts; contacts-permission prompt too invasive); **P3** = product flip to un-gate cold platform SMS (built, gated). | #2572 (merged, P1) |
| COMM-2 | Host controls | 3 | Feature | **Partial (#2527 merged)** — Who's-In host management landed early: `SetExperienceMemberRSVP` (set/reset a member's RSVP) + `RemoveExperienceMember` (uninvite), owner-only, via the roster manage bottom sheet. Require-approval gate + capacity + guest-list visibility still pending. | #2527 (merged) |
| RCS-1 | RCS Business Messaging | 4 | Task | **✅ SUBMITTED 2026-07-01, in carrier review (~5–9 wk).** Prep (#2574) merged; welcome send built+deployed; sender live in prod pool; RCS-first verified; Google+US/Aegis filed together. Remaining: await approval. #2609 gating CLOSED (cold messaging parked). | #2574 (prep, merged); `2492-rcs-business-messaging.md` |

**Phase exit criteria**
- **Phase 1 done** = WEB-2 green end-to-end (an invite — via host-relayed link **or platform email
  (EMAIL-1)** — reaches a non-app person, who views the event, verifies phone/email, and RSVPs,
  becoming a real user). **No platform SMS required** — the email + relayed-link loop is complete.
- **Phase 2 done** = a deviceless phone user receives a real platform **SMS** for a notification,
  with opt-out + quiet hours honored, behind the flag flipped on after SMS-1.
- **Phase 3 done** = the same loop works for requests, giveaways, and loans; host controls
  available.

---

## Audit notes

`/ripls-audit` ran against this plan. It was already grounded in precise file references; the
audit folded the following documented standards into the
[cross-cutting requirements](#conventions--cross-cutting-requirements-apply-to-every-phase) and the
relevant work items:

- **proto conventions** — `optional` fields, no vendor names in proto comments, no cross-RPC type
  reuse, round-trip tests (`proto_conventions.md` / api_design rubric).
- **SMS render boundary** — SMS is server-rendered; route copy through the go-i18n catalog with
  email-style recipient-locale resolution, and close the provisional-user-has-no-locale gap
  (`server/l10n.md`). *This corrected an original SMS-2 task that rendered copy inline.*
- **PII & observability** — mask phone numbers in logs; add SMS metrics; `CheckInvitation` must not
  leak phone numbers (security rubric / `observability.md` / `metrics.md`).
- **dependency hygiene** — prefer a minimal REST client over the full Twilio SDK; route through
  Renovate + `govulncheck` (`dependency_review_prompt.md`, CLAUDE.md minimal-deps).
- **testing / workflow sync** — update workflow docs + system/e2e tests for the new flows
  (`update_system_tests.md`).
- **accessibility** — new web / creation / display surfaces use a11y primitives + semantics
  identifiers (`accessibility.md`).

**Positive alignment:** the pivot directly advances a stated v1 product constraint — older-adult
accessibility via "receiving an invite, RSVPing … over text, email, or simple web interface,
without ever opening the app" (`docs/users/priorities.md`). The cap exemption is correctly scoped
to ad-hoc communities, preserving the bounded-circle thesis for persistent communities. No
contradiction with a documented decision was found.

**Docs consulted:** `proto_conventions.md`, `users/priorities.md`, `server/l10n.md`,
`reviews/security_review_prompt.md`, `reviews/api_design_review_prompt.md`,
`reviews/dependency_review_prompt.md`, plus the leaf docs loaded during planning
(`push_notifications.md`, `registration_and_login.md`, `invitations.md`, `single_player_mode.md`,
`client/sharing.md`, `realtime_updates.md`, `community_health.md`, `users/competitor_analysis.md`).

## Related Issues

- **#2492** — this epic (the umbrella tracker; remains open).
- **#2493–#2507** — the 15 child issues, **closed** and consolidated into this document (SMS-1,
  SMS-2, SMS-3, ID-1, ID-2, COMM-1, COMM-2, CREATE-1, DISPLAY-1, WEB-1, LINK-1, WEB-2, WEB-3,
  WEB-4, RCS-1 respectively).
- Builds on the invitation/short-link system (`docs/invitations.md`), shadow users
  (`docs/single_player_mode.md`), phone auth (`docs/registration_and_login.md`), and the
  push-notification abstraction (`docs/push_notifications.md`).
