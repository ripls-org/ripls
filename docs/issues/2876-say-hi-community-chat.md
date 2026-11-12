# Issue #2876: The "say hi" shortlink when a new member joins does not lead to the community discussion

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2876
**Filed:** 2026-08-09 | **Last activity:** 2026-08-09
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/invitations.md`, `docs/push_notifications.md`,
`docs/sms_notifications.md`, `docs/client/deep_linking.md`,
`docs/client/chat.md`, `docs/server/l10n.md`,
`docs/workflows/phone_first_community_join.md`, `docs/ad_hoc_communities.md`,
anchors (`docs/client/architecture.md`, `docs/client/conventions.md`,
`docs/server/architecture.md`, `docs/server/conventions.md`,
`docs/proto_conventions.md`, `docs/users/priorities.md`), review prompts
(security, accessibility, error_handling, observability, testing, api_design)
**Sibling:** #2875 (merged as PR #2878) changed the page this issue was filed
against — see *What #2875 already fixed*.

## Problem Statement

A member-joined notification says *"Aisha joined Ferndale Tool Library"* with a
**"Say hi at {link}"** CTA. The promise is the community conversation. The link
delivers something else.

Mechanically: `COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED` produces a
notification with a `community_id` and **no item entity**. For off-app channels
(email, SMS), `notificationLink` (`server/notifications/deeplink.go`) calls
`ShortLinkCodeForEntity`, whose no-entity branch mints a **community-invite**
`ShareLink` — so "say hi" is literally an *invitation* link. For push, the
client's route chokepoint sends community-scoped-with-no-entity notifications to
`/` (home).

So the CTA that says "say hi" resolves, depending on channel, to an invitation
landing or to the home screen. Neither is a conversation.

The reporter also asks two follow-ons in the body: whether SMS has the same
problem (**it does** — same `notificationLink`, same code), and that FCM should
deep-link to the chat area too (**it doesn't** — see below).

### What #2875 already fixed, and what it didn't

The issue was filed against the install-only card, one minute after #2875. That
card is gone. I instrumented the current behavior rather than reasoning about it
(throwaway Playwright probe, an authenticated **member** following a real
community-invite `/go/{code}`):

1. The landing renders, and its kicker reads **"INVITATION"** with a
   **"Join the group"** CTA — shown to someone who has been a member for weeks.
2. Tapping it goes to `/group/{id}?intent=join&code=…`, the join is idempotent,
   and the **community profile renders correctly** ("Aisha and Sam · All
   members · 2"). So the link is no longer a dead end.
3. But the destination is the **profile**, not the discussion. The conversation
   is one further tap — the green Message action, which calls
   `_expandConversation` to morph-reveal the chat panel.

So what remains is narrower and more precise than "opens the old-fashioned
community join flow":

- **A — Framing.** A public, pre-auth page cannot know the viewer is already a
  member, so it addresses everyone as an invitee. For a "say hi" recipient,
  *every* recipient is a member by construction.
- **B — Destination (web/email/SMS).** Lands on the community profile; the
  issue asks for the discussion.
- **C — Destination (push).** `routeNotificationPayload`
  (`app/lib/services/fcm_service.dart`) maps community-scoped-with-no-entity to
  `route = '/'`. A push tap doesn't reach the community at all, let alone chat.

### Scope boundary worth stating up front

The community `/go/` fallback is **not** exclusive to member-joined. Four event
types carry a community and no item entity:

| Event type | Right destination |
|---|---|
| `INVITATION_LINK_USED` ("say hi") | the **discussion** |
| `COMMUNITY_DELETED` | the community / restore surface |
| `COMMUNITY_RESTORED` | the community |
| `OWNERSHIP_TRANSFERRED` | the community |

Only the first wants chat. So the destination must be chosen **per event type**,
not applied to the community fallback wholesale — a blanket "community links go
to chat" change would drop a deletion notice into a chat pane.

## Context from Comments

**No comments.** The body is one screenshot plus two explicit follow-ons (SMS,
FCM), both of which I've confirmed against the code and folded into the problem
statement above. What the issue does *not* say is which of the four
community-fallback event types it means — it names only "say hi", and the table
above is my scoping, not the reporter's. That's the main thing worth confirming.

## Potential Solutions

### Option A: A typed destination hint on the notification's `/go/` URL

- **Approach:** The member-joined sender appends a small, typed hint —
  `/go/{code}?to=discuss`. The SSR community landing reads it and (i) renders
  *member* copy instead of invitation copy, and (ii) forwards `tab=discuss` into
  its CTA (`/group/{id}?…&tab=discuss`). `WebCommunityScreen` opens the
  conversation panel on arrival. The client `/go/:code` redirect forwards the
  hint so the installed-app path keeps it too.
- **Pros:** Keeps `/go/` as the link — which matters, because `/go/*` is the
  **only** universal-linked path, so any other URL shape opens the browser
  instead of the app on mobile. One mechanism fixes A, B, and (with the FCM arm)
  C. The hint is per-event-type by construction, which respects the scope table.
  No proto change, no new share-link rows.
- **Cons:** Reintroduces a query parameter on `/go/`, which #2562 deliberately
  pruned. That removal was about *item-id* params that duplicated (and could
  contradict) the row's target; a destination hint is a different kind of thing
  — it says where to go *within* the resolved target, not what the target is.
  Still, it's a boundary worth defending explicitly rather than crossing quietly.
- **Effort:** Medium.

### Option B: A new `ShareLink` target variant for the community conversation

- **Approach:** Add a `community_conversation_id` (or similar) variant to the
  `oneof target`. The member-joined sender mints that flavor; the SSR dispatches
  on it like any other typed target.
- **Pros:** The destination lives on the row, exactly as #2562 intended —
  bare URLs, no params, no ambiguity. Fits the existing dispatch shape.
- **Cons:** A proto change and a new persisted row flavor to express "the same
  community, but land on a different tab." It also multiplies rows: every
  community would now accumulate a second share link whose only difference is
  the landing tab. The `oneof target` answers *what is this link about*; the tab
  is *how to present it*, and conflating them makes the model worse to reason
  about. Heavier than the problem.
- **Effort:** Large.

### Option C: Point notifications at `/group/{id}?tab=discuss` directly

- **Approach:** Skip share links for community-scoped notifications; build the
  destination URL directly.
- **Pros:** Simplest to write. No hint, no new row, no SSR change.
- **Cons:** **Breaks the installed-app path.** Only `/go/*` is in
  `apple-app-site-association`; a `/group/` URL in an email opens the mobile
  browser instead of the app — a regression for exactly the recipients most
  likely to act. It also loses the SSR preview card (`ShortLinkCodeForEntity`
  exists so off-app links unfurl), and a bare `/group/{id}` has no auth context,
  so an unauthenticated tap dead-ends at login with no invite to accept.
- **Effort:** Small, and wrong.

## Recommendation

**Option A**, scoped to `INVITATION_LINK_USED` only.

It's the one that respects the constraint doing the most work here: `/go/*` is
the only universal-linked path, so the link *must* stay `/go/` or mobile
recipients lose the app. Given that, the destination has to be expressed either
on the row (Option B) or on the URL (Option A) — and a landing-tab hint is
presentation, not identity, so the URL is where it belongs. Option B's proto
change buys purity at the cost of a second share-link row per community that
differs only in which tab opens.

The tradeoff I'm accepting: one query parameter comes back to `/go/`. I'd
constrain it hard — a closed vocabulary (`to=discuss` and nothing else),
ignored when unrecognized, never carrying an entity id — and document at the
mint site why this doesn't reopen what #2562 closed. If a reviewer would rather
not reintroduce params on `/go/` at all, Option B is the principled fallback and
the plan below survives it with Phase 1 swapped out.

**Deliberately not in scope:** changing where the other three community-fallback
event types land. They're correct today.

## Open Questions

- **Is "say hi" the only notification that should land in chat, or should the
  Workshop "say hi to your crew" prompt and any future community-chat nudge
  share the mechanism?** — Recommendation: build the hint generically
  (`to=discuss` is a destination, not an event type) but *set* it only for
  `INVITATION_LINK_USED` in this PR. That gets the seam without speculatively
  re-pointing notifications nobody complained about.
- **On the landing, does `to=discuss` change the copy, or only the CTA target?**
  — Recommendation: **both**. The framing defect (A) is half the report ("even
  for a user who is already signed in"), and the hint is exactly the signal that
  the recipient is a member. Concretely: drop "{Inviter} invited you", swap the
  kicker off "INVITATION", and make the CTA "Open the discussion".
- **Does `/group/{id}` become the cross-platform community route, or does mobile
  get its own?** — Recommendation: **generalize `/group/{id}`**. It already
  behaves correctly on mobile for an authenticated visitor with no share code
  (skips the join, renders `CommunityPublicScreen`), so FCM can route to it as
  is. The work is honesty about it: the screen and its README currently say
  "web-only", and that comment would become wrong. Renaming
  `WebCommunityScreen` → `CommunityEntryScreen` (or similar) is the clean-up.
- **Should the FCM payload carry the destination too, or should the client infer
  it from `event_type`?** — Recommendation: **infer from `event_type`**, which
  is already in the payload and already switched on
  (`REQUEST_FOLLOWUP_PROMPT` precedent). Adding a payload field for something
  derivable is churn, and the client is where the route vocabulary lives.
- **What should an *unauthenticated* visitor who opens a `to=discuss` link see?**
  — Recommendation: treat it as a normal community invite (the existing
  phone-first join), then land in the discussion after joining. The hint says
  "you're probably a member"; it must not become an authorization assumption.

## Implementation Plan

**One PR.** Phases are build order, not release increments. Two standing
constraints, both of which bite this change specifically:

- **No phase markers in code comments** — `check_ephemeral_comments.js` fails the
  build on "Phase 2" and friends in `*.go`/`*.dart`. Reference `#2876`.
- **Phase 3 cannot be validated by CI or by me.**
  `docs/push_notifications.md` §"Deep-Link Recovery Release Gate (#2636)"
  requires a **physical-device matrix** — {Android, iOS} × {foreground,
  background, terminated} — before shipping *any* change to the notification
  tap→navigate pipeline, and `fcm_service.dart` is named in it explicitly.
  Emulators do not reproduce the OEM process-kill quirks that caused the
  original field drop. That gate is a human task on real hardware; the PR is not
  shippable without it, and I can't run it. Plan for it rather than discovering
  it at review.

### Phase 1: Carry a destination hint from the notification to the landing

**Goal:** A member-joined off-app notification's link says "open the
discussion", and the SSR landing renders member framing rather than an
invitation.

**Tasks:**
- [ ] Add a closed destination vocabulary to the notification link builder
      (`server/notifications/deeplink.go`): `notificationLink` appends
      `?to=discuss` only for `INVITATION_LINK_USED`. Unrecognized values are
      never emitted and are ignored on read.
- [ ] `handleCommunityLanding` reads `to`, and for `discuss`: renders member
      copy (kicker + headline without "invited you"), a CTA labelled "Open the
      discussion", and forwards `tab=discuss` into the `/group/{id}` CTA href.
- [ ] New `web.community.*` l10n keys for the member-framed variant, `en` + `es`
      (the `l10n-parity` and `no-inline-user-strings` gates both bite here).
- [ ] Comment at the mint site explaining why a destination hint is not the
      thing #2562 removed — the next reader will otherwise assume a regression.
- [ ] Go tests: hint present → member copy + `tab=discuss` in the CTA; hint
      absent → today's invitation copy unchanged; unrecognized `to` → treated as
      absent; the other three community-fallback event types → no hint.
- [ ] Confirm the 30s SSR cache stays correct — `?to=` is part of the cache key,
      so the member and invite variants can't alias.

**Validates:** That the destination survives the server→link→landing hop, and
that adding it can't change what a plain invite renders.

### Phase 2: Land in the discussion

**Goal:** `/group/{id}?tab=discuss` opens the conversation, on web and mobile.

**Tasks:**
- [ ] `WebCommunityScreen` reads `tab`; when `discuss`, opens the community
      conversation on arrival once the join has settled.
- [ ] **Resolve the morph-origin problem.** The existing entry point is
      `_expandConversation(Rect)` → `ProfileConversationPanel(communityId:)`,
      and the `Rect` is the *tap origin* the morph-reveal animates from. An
      arrival has no tap, so this needs either a post-layout rect read off the
      Message action or a non-morph entry into the same panel. Don't fake a
      rect — a panel that flies in from `Rect.zero` reads as a glitch.
- [ ] Forward the hint through the client `/go/:code` redirect, which currently
      drops query params on its way to `/invite`.
- [ ] Decide and apply the naming per the open question — if `/group/{id}` is
      now cross-platform, rename the screen and fix the "web-only" claims in its
      doc comment and `screens/web/README.md`.
- [ ] **Accessibility of an auto-opened panel** (`client/accessibility.md` + the
      a11y lint): opening a conversation without a user gesture must move focus
      deliberately and announce itself — a screen-reader user who followed a
      "say hi" link should be told they're in the discussion, not silently
      re-parented. Treat it as a `liveRegion` + focus-management case, not just
      a navigation.
- [ ] Flutter tests: `tab=discuss` opens the conversation; absent leaves the
      profile as-is; the panel doesn't open before the join succeeds.

**Validates:** The user-visible promise of the issue — "say hi" lands in the
conversation.

### Phase 3: Push

**Goal:** A member-joined push tap reaches the discussion too.

**Tasks:**
- [ ] In `routeNotificationPayload`, replace the community-no-entity `'/'` arm
      with `/group/{communityId}`, adding `?tab=discuss` when
      `event_type == 'INVITATION_LINK_USED'`. Keep the other three
      community-fallback types on the community itself.
- [ ] Verify the `PendingNotificationRoute` holder + replayer carry the new
      route through the cold-start and unwired-callback paths they exist for.
- [ ] **Keep the deep-link telemetry honest.** `routeNotificationPayload` is the
      single choke point that emits `NotificationDeepLinkEvent` (`phase: route`,
      then `nav_result`), and #2636's prod queries key on `entity` / `route` /
      `noEntityId`. The community arm keeps `entity: 'community'` but its
      `route` value changes from `/` to `/group/{id}` — say so in the PR, since
      anyone querying historical deep-link data across this change will
      otherwise compare two different routes under one label.
- [ ] Flutter tests on the route chokepoint: member-joined → discussion;
      community-deleted → community, not chat; no regression for the
      gear/experience/request arms; the emitted analytics event carries the new
      route.
- [ ] **Run the #2636 physical-device matrix** ({Android, iOS} × {foreground,
      background, terminated}) with a member-joined payload. Human task, real
      hardware, blocking — see the preamble.

**Validates:** The reporter's second follow-on, and closes the last channel.

### Phase 4: e2e + docs

**Goal:** The loop is covered and the docs describe it.

**Tasks:**
- [ ] Playwright: a member follows a `to=discuss` link and lands in the
      conversation — asserted on the chat surface, not on having navigated.
      Extend `invite-link-deep-link-paths.spec.ts` or add a sibling.
- [ ] Re-run the phone-first suite: Phase 1 touches the shared community
      landing, so all four landings and the five phone-first loops are the
      regression surface.
- [ ] Docs: `invitations.md` (the `/go/` destination-hint vocabulary),
      `push_notifications.md` + `sms_notifications.md` (where member-joined
      lands), `client/deep_linking.md` (the route table),
      `workflows/phone_first_community_join.md` (the member path). Regenerate
      the context map, advance freshness stamps.
- [ ] **Correct `push_notifications.md` §Membership Events**, which currently
      claims `INVITATION_LINK_USED` "deep-links to the community detail card via
      `community_id`". The code routes to `/` with the community selected; after
      Phase 3 it routes to the discussion. The line is stale either way.
- [ ] **Adjacent, not this PR:** `client/chat.md` §conversation model says the
      community conversation is `Community.conversation_id`, but that field is
      `reserved` in `proto/ripls/models/community.proto` and
      `ProfileConversationPanel` resolves the conversation from the community id
      instead. File it for the doc-freshness ratchet rather than widening scope
      here.

**Validates:** That the three channels agree, and that the next person to touch
`/go/` learns the param rule from the docs rather than from a surprise.

## Related Issues

- **#2875** — merged as PR #2878. Built the community landing and
  `/group/{communityId}` this issue extends; it also removed the install-only
  card the screenshot in this issue shows. Read its plan first.
- **#2562** — removed query params from `/go/` URLs. The constraint Option A
  negotiates with; its removal targeted item-id params, not destination hints.
- **#2492** — the phone-first pivot; established that `/go/*` is the only
  universal-linked path, which is what rules out Option C.
- **#2636** — notification deep-link recovery (`PendingNotificationRoute`), the
  machinery Phase 3 routes through, and the source of the physical-device
  release gate that blocks shipping it.

## Audit Notes

Audited with `ripls-audit` after drafting. Scope: server
(`server/notifications/**`, `server/services/web/community_page.go`,
`server/l10n/source/**`) + client (`app/lib/services/fcm_service.dart`,
`app/lib/presentation/screens/web/**`, `app/lib/core/router/**`) + e2e + docs.
Dimensions: security, accessibility, error_handling, observability, testing,
code_quality, api_design, tech_debt.

Findings folded in:

1. **The #2636 physical-device release gate is blocking and I can't run it.**
   `push_notifications.md` names `fcm_service.dart` explicitly; Phase 3 edits it.
   Promoted to the Implementation Plan preamble so it's a known cost, not a
   review surprise. → preamble + Phase 3 task.
2. **`_expandConversation` takes a morph-origin `Rect`.** An auto-open on arrival
   has no tap origin; faking one animates the panel in from nowhere. → Phase 2.
3. **Auto-opening a chat panel is an a11y event, not just navigation** — focus
   must move deliberately and the arrival must be announced. → Phase 2.
4. **`NotificationDeepLinkEvent`'s `route` value changes** for the community arm
   (`/` → `/group/{id}`), and #2636's prod queries key on it. → Phase 3.
5. **`push_notifications.md` §Membership Events is already stale** and will be
   stale differently after this. → Phase 4.
6. **`client/chat.md` contradicts the proto** on `Community.conversation_id`
   (the field is `reserved`). Logged as adjacent, deliberately not scoped in.
7. **Phase-marker comment ban** — a plan organized in phases that ships as one
   PR trips `check_ephemeral_comments.js` if the numbering leaks into code.
   → preamble.

Clean against: proto conventions (no proto change — the hint is a URL param, not
a schema change, which is precisely the Option A/B tradeoff argued above); SQL
safety and N+1 (no new queries); `connecterr.Internal` (no new RPC handlers);
`no-inline-user-strings` + `l10n-parity` (new copy goes through the catalog in
`en` + `es`, already a Phase 1 task); file-size gates.

One security note carried from the plan body rather than added: the `to=discuss`
hint is a **presentation** signal and must never become an authorization one. It
says "this recipient is probably a member"; membership is still established by
`AcceptInvitationLink` server-side. An unauthenticated visitor who forges
`?to=discuss` gets the ordinary phone-first join, not a shortcut into a private
conversation — Phase 1's tests should assert that directly.

Docs consulted: `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md`, `client/architecture.md`, `client/conventions.md`,
`proto_conventions.md`, `server/observability.md`, `testing/architecture.md`;
review prompts `security`, `accessibility`, `error_handling`, `observability`,
`testing`, `api_design`; leaf docs `push_notifications.md`,
`sms_notifications.md`, `invitations.md`, `client/chat.md`,
`client/deep_linking.md`, `client/accessibility.md`, `server/l10n.md`,
`workflows/phone_first_community_join.md`; plus `CLAUDE.md` and the #2875 plan.
