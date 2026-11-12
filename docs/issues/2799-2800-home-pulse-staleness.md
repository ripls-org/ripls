# Issues #2799 + #2800: Home pulse shows stale items, and cards ignore the viewer's own participation

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2799 ·
https://github.com/Type2Fun-Foundation/ripls/issues/2800
**Filed:** 2026-07-29 (11:34Z and 11:36Z — two minutes apart, same session) |
**Last activity:** 2026-07-29
**Labels:** `user-feedback` (both) | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `proto_conventions.md`, `server/architecture.md`,
`server/conventions.md`, `users/priorities.md`; leaves `client/inbox.md`,
`client/the_feed.md`, `client/design.md`, `client/i18n.md`,
`client/accessibility.md`, `client/testing.md`, `workflows/experience.md`,
`workflows/request.md`; review prompts `api_design`, `security`,
`performance`, `accessibility`, `testing`, `code_quality`, `error_handling`,
`tech_debt`. Full list and what each caught: [Audit notes](#audit-notes).

> **Combined plan.** The user asked for #2799 and #2800 in one plan and one PR,
> and then for the request card's **"I have one"** CTA — the exact same defect
> as #2800 on the neighbouring payload — to be fixed in the same PR. All three
> live on the **same widget fed by the same RPC** (`CommunityPulseSection` ←
> `GetFeed`), the first two were reported from the same scroll of the same
> screen, and all three are "the card doesn't reflect reality." The prior plan
> for #2796 already anticipated the batching
> (`docs/issues/2796-nested-zero-state-nudge.md` → Related Issues: *"if a
> Home-screen batch is assembled they belong together"*). They are sequenced as
> separate phases below and can be reviewed independently inside the PR.

## Problem Statement

Both reports are about the Home tab's long tail — the **"IN YOUR COMMUNITIES"**
section, which is [`CommunityPulseSection`](../../app/lib/presentation/widgets/home/community_pulse_section.dart)
rendering `feedProvider` items as full-width snapshot posts (see
[`client/inbox.md`](../client/inbox.md) → *Community pulse*). Neither is about
the "Needs you" decision queue.

**#2799 — "Old events and items in home screen are awkward and confusing."**
The screenshot shows the reporter scrolled into story cards dated **24d, 25d,
and 26d ago** ("Leslie, Bryan, Susan Hosted Talent Show", "Doug Left Amazon")
followed by more gear. There is **no absolute time horizon anywhere in the feed
pipeline.** An item is dropped only if the viewer has *seen* it and 24 h have
passed ([`isItemFresh`](../../server/services/feed/freshness.go)); an item with
no view record is "always fresh," forever. Compounding it, ephemeral items
(experiences, requests, giveaways, open transfers) are exempt from expiry
*unconditionally* — an event whose date passed weeks ago but that was never
marked completed stays in the feed indefinitely.

**#2800 — "Home screen event prompts to join but I'm already going."**
The "Love Your Neighbor 5K" card reads **"3 going"** and still shows a ghost
**"Join"** pill. `ExperienceCreatedPayload`
([`proto/ripls/api/feed_service.proto:282`](../../proto/ripls/api/feed_service.proto))
carries `yes_count`, `maybe_count`, `creator`, `can_edit`, `state` — but
**nothing about the viewer's own RSVP.** The client therefore *cannot* know,
and hardcodes the label:
`ctaLabel: l10n.homePulseEventCta` (= `"Join"`) at
[`community_pulse_section.dart:176`](../../app/lib/presentation/widgets/home/community_pulse_section.dart).
`yes_count` already includes the viewer, so the card contradicts itself in one
line: it counts you among the 3 going and then asks you to join.

**Unfiled sibling — the request card's "I have one" CTA.** Folded in at the
user's request. `RequestCreatedPayload` carries `offer_count` but no
viewer-offer state, and
[`community_pulse_section.dart:158`](../../app/lib/presentation/widgets/home/community_pulse_section.dart)
hardcodes `ctaLabel: l10n.homePulseAskCta` (= `"I have one"`) exactly as the
event card hardcodes `"Join"`. Someone who already offered is asked to offer
again; the requester is asked to answer their own request. Same defect class,
same payload shape, same fix — and, as with RSVPs, the offer rows the answer
needs are **already batch-fetched** at
[`service.go:388`](../../server/services/feed/service.go).

Found while scoping that change: `createRequestCreatedItem`
([`items.go:110`](../../server/services/feed/items.go)) is the one feed-item
builder that never receives `viewerUserID`, and hardcodes **`CanEdit: false`**
— so `RequestCreatedPayload.can_edit` is always false, including for the
requester. No client reads it today (grep: only `communityCreated.canEdit`, in
the orphaned `FeedScreen`), so it is latent rather than user-visible, but the
viewer-state work has to thread `viewerUserID` into that function anyway and
leaving a knowingly-wrong field behind it would be worse than fixing it.

No contradictions between the reports; there are no comments on either.

### Investigation note — why `ripls-ui-investigate` was not run

The skill exists to bisect a UI report to a layer when the layer is a guess.
Here both layers are established analytically and are not in doubt:

- **#2800** is a missing field on the wire. `ExperienceCreatedPayload` has no
  viewer-RSVP member; a client cannot render state it was never sent. Verified
  by reading the proto and the single unconditional `ctaLabel` assignment.
- **#2799** is an absent filter. `isItemFresh` returns `true` whenever
  `view == nil`, and `generateFeedItems` applies no age bound of any kind.
  Verified by reading `freshness.go` and `service.go:578–628`.

A hermetic repro would re-observe what the code already states. What a repro
*could* add — confirmation of the view-record claim below — is instead covered
by Phase 0's read-only production check, which uses real data rather than a
seeded world.

### The finding that explains the severity (verify in Phase 0)

The seen-based expiry is **inert on the Home tab**, so the horizon's absence is
not a slow leak — it is unbounded growth.

`MarkFeedItemsViewed` is driven by `FeedNotifier.setPageIndex()`, and grep
finds exactly one caller: [`feed_screen.dart:65`](../../app/lib/presentation/screens/feed/feed_screen.dart)
— the swipe `PageView`. That screen sits at `IndexedStack` index 0, and
`RiplsNavDestination` ([`nav_destination.dart`](../../app/lib/presentation/widgets/navigation/nav_destination.dart))
maps its four destinations to stack indices **1, 4, 2, 3** — index 0 belongs to
no destination and the enum's own doc comment calls it *"the orphaned Feed."*
`navDockEnabled` defaults to `true`
([`environment.dart:116`](../../app/lib/core/config/environment.dart)), so in a
shipped build **`setPageIndex` is never called.**

`CommunityPulseSection` is a plain `Column` of cards; it never marks anything
viewed. The only surviving call is `markCurrentItemViewed()` inside
`FeedNotifier.initialize()`, which marks **item index 0 and nothing else**.

Net effect in production today:

| Item | View record | Outcome |
|---|---|---|
| The single newest feed item | created on every load | expires 24 h later — **the newest card is the only one that ever disappears** |
| Every other item, ever | never created | `isItemFresh` → `true` forever |

That inversion — newest silently vanishes, 26-day-old stories persist — is a
precise fit for "awkward and confusing."

## Context from Comments

**Neither issue has any comments.** Both are raw in-app feedback submissions
(the pipeline in [`user_feedback.md`](../user_feedback.md)) from
user2@example.org on Android 0.28.0 build 9326 against PROD, filed two minutes
apart. There is no maintainer direction and no discussion to weigh.

The reporter is the product owner, and the reports are terse but unambiguous
about the desired end state: #2799 explicitly names the remedy ("Should be a
time horizon for these"), and #2800 names the expected render ("Should show
that I have rsvp'd yes"). The absent discussion is a gap only for the
*tuning* questions — how long a horizon, and what a responded card should say
— which are carried as Open Questions rather than assumed away.

## Potential Solutions

### #2799 — a time horizon for the pulse

#### Option A: Client-side filter in `CommunityPulseSection`

- **Approach:** Drop posts whose `occurredAtUnixSec` is older than N days in
  `_postData`.
- **Pros:** One file, no proto, no server deploy.
- **Cons:** The wrong layer. The server still assembles, sorts, paginates, and
  ships items nobody renders — a 20-item page can arrive entirely filtered,
  producing a blank stretch of scroll and a pagination loop that fetches until
  the feed is exhausted. `GetFeedStatus`, the sidebar badge, and any future
  feed consumer keep the old behavior. Directly contradicts the project's
  fix-the-root-cause preference over client-side workarounds.
- **Effort:** Small.

#### Option B: Absolute age cap in `generateFeedItems`

- **Approach:** Drop any item whose `last_activity_at` is older than a horizon
  constant, regardless of view state, before the sort.
- **Pros:** Root cause, one place, every consumer inherits it.
- **Cons:** Naïvely applied it also drops **live opportunities** — an event
  created 40 days ago that happens tomorrow, an open giveaway, a transfer
  mid-negotiation. Those are exactly the items the ephemeral exemption exists
  to protect, and dropping them would be a worse regression than the bug.
- **Effort:** Small.

#### Option C: Horizon + a liveness test that replaces the blanket exemption *(recommended)*

- **Approach:** Same horizon as B, but an item also survives when it is still
  *live*: it has a **future anchor date** (event start, request needed-by) or
  an **in-flight transfer** (interest expressed / recipient selected / active).
  The current `isEphemeralEvent` "exempt forever" rule narrows to "exempt while
  the opportunity is open."
- **Pros:** Fixes the reported staleness *and* the unreported twin — a past
  event that was never wrapped up stops advertising itself. Reuses the existing
  `available_now.ExperienceStartUnixSec` helper. Keeps giveaways and open loans
  visible, which is what the exemption was for.
- **Cons:** More branches to test; needs a decision on undated open requests
  (see Open Questions).
- **Effort:** Medium.

### #2800 and the request CTA — reflect the viewer's own participation

#### Option D: Infer client-side

- **Approach:** Cross-reference RSVP / offer lists the client already holds.
- **Pros:** No proto change.
- **Cons:** It does not hold them. The pulse would have to fetch per-experience
  RSVPs and per-request offers — an N+1 across every event and request card on
  Home — to render a label. The feed contract is explicitly server-computed
  (`the_feed.md`: *"clients never evaluate unread rules themselves"*); this
  would be the first exception.
- **Effort:** Medium, and wrong.

#### Option E: Carry viewer state on both payloads *(recommended)*

- **Approach:** `RSVPIntention viewer_rsvp` on `ExperienceCreatedPayload` and
  `bool viewer_has_offered` on `RequestCreatedPayload`. The server **already
  batch-fetches every RSVP row and every offer row** for these items to compute
  `yes_count` / `maybe_count` ([`service.go:425`](../../server/services/feed/service.go))
  and `offer_count` ([`service.go:388`](../../server/services/feed/service.go));
  the viewer's own state is one extra condition inside loops that already run.
  **Zero additional queries for either.**
- **Pros:** Follows the established viewer-scoped-field precedent (`can_edit`,
  "*Whether the current viewer can edit…*"). Server owns the semantics, client
  renders. Free at the data layer. One symmetric change across the two card
  types rather than two differently-shaped ones.
- **Cons:** Proto change → regenerate; the client must handle four event states
  plus the host case, and two request states plus the requester case.
  `createRequestCreatedItem` needs `viewerUserID` threaded in (two call sites).
- **Effort:** Small.

The asymmetry in the two field shapes is deliberate: an RSVP is a three-way
intention the viewer can hold, so the enum carries real information; an offer
is binary (a `RequestOffer` row exists and is not `withdrawn`). Mirroring the
enum onto requests would invent states the domain does not have, and exposing
the offer row itself would leak storage shape onto the wire for no client gain.

## Recommendation

**Option C for #2799, Option E for #2800 and the request CTA**, shipped as one
PR in the phases below.

The trade being made on #2799 is deliberate: a horizon is a blunt instrument,
and Option C spends its extra complexity buying back precision exactly where
bluntness would hurt — live opportunities. Without the liveness test the fix
would delete upcoming events from the Home feed, which is a strictly worse
product than the bug being fixed. With it, the rule reads as one sentence: *an
item stays if something happened recently, or if it hasn't happened yet, or if
it is mid-handoff.*

On the viewer-state defects, Option E is the only option that respects the
server-computes-feed-semantics contract, and it happens to be nearly free
because both the RSVP rows and the offer rows are already in memory. The
alternative trades two proto fields for an N+1 on the Home screen's hot path.

Fixing the event and request CTAs together is also the cheaper order: they
share one resolver shape, one chip widget, one set of accessibility decisions,
and one test file. Doing them in separate PRs would mean designing the status
chip twice.

**Explicitly out of scope**, and filed rather than folded in:

1. **Making the pulse mark items viewed.** The seen-based expiry is inert
   (above), and restoring it means inventing "viewed" semantics for a passive
   scroll list — visibility detection, a new dependency or fiddly `RenderBox`
   math, and a judgement call about whether scrolling past a card counts as
   reading it. That is its own design question. The horizon does not depend on
   it; after this PR the horizon is the operative bound, which is why the
   horizon value must be chosen as a *sole* bound rather than a backstop.
2. **Bounding the event query at the storage layer.** `generateFeedItems` calls
   `QueryByField(community_id)` and loads **every `CommunityEvent` a community
   has ever produced** into memory on every Home load, then filters in Go. This
   violates two explicit conventions at once — *"always LIMIT unbounded
   queries"* and *"push filtering to SQL"* — and a real fix needs new storage
   plumbing, not just a flag: `ProtoSQLStorage.QueryByField`
   ([`protosql.go:236`](../../server/storage/protosql.go)) emits **no `ORDER
   BY`**, and `QueryOptions.Limit` is honoured only by `ListAll`
   ([`protosql.go:161`](../../server/storage/protosql.go)), so capping this
   query today would return an arbitrary subset. The horizon shrinks what is
   *returned*, not what is *fetched*, so this PR neither worsens nor fixes it —
   but the cost grows with community age forever, and it deserves its own issue
   with the ordering and bounding designed together.

## Open Questions

- **How long is the horizon?** — Recommendation: **14 days.** The reporter
  flagged 24–26-day items as stale; an 8-day story appeared in the #2800
  screenshot and drew no complaint, so the line sits between. 14 days is two
  weeks of "what's been happening" and exactly double the existing 7-day
  `unreadWindowSeconds`, which keeps the two windows legible against each
  other (nothing can be unread and beyond the horizon).
- **Constant or per-community config?** — Recommendation: **constant.**
  `Community` already carries `feed_expiry_hours` / `feed_expiry_views`, so a
  `feed_horizon_days` sibling would be symmetric — but neither existing knob has
  ever been set away from its default, and CLAUDE.md's minimal-dependencies rule
  argues against shipping a second unused dial. A `const feedHorizonDays = 14`
  next to `defaultExpiryHours` is one line to promote later if a community
  actually needs it.
- **Should a quiet community keep a floor of recent items rather than an empty
  pulse?** — Recommendation: **no floor.** A floor re-admits precisely the
  26-day-old cards the report objected to, and the section already self-hides
  when empty (`posts.isEmpty → SizedBox.shrink()`), with Home's Needs-you
  section and zero-state prompts still above it. If empty-pulse feedback comes
  back, a post-merge floor of ~3 is the cheap follow-up — but shipping it
  pre-emptively would blunt the fix the user asked for.
- **Do open requests with no needed-by date count as live?** —
  Recommendation: **no** — they age out on the horizon. A request nobody
  answered in two weeks is not news, and it stays reachable from the
  requester's own surfaces and the Library. A request with a *future*
  needed-by stays live until that date passes.
- **What does an event card say once you've responded?** — Recommendation:
  **"Going" / "Maybe" / "Not going" / "Hosting"** (host wins over RSVP), and
  crucially rendered as a **non-tappable status chip**, not a button. A chip
  labelled "Going" that is wrapped in a `Tappable` announces to a screen reader
  as an action named "Going", which promises something it does not do. The card
  body stays tappable, so changing an RSVP is still one tap away.
- **Should a `NO` RSVP hide the card entirely?** — Recommendation: **no.**
  Hiding removes the only affordance for changing your mind, and the event may
  still concern you. "Not going" is self-explanatory and honest.
- **What does a request card say once you've offered, or if it's yours?** —
  Recommendation: **"Offered"** and **"Your request"**, as the same non-tappable
  status chip. Note the noun rule (`feedback_never_ask_as_noun`): the verb form
  in the existing header string `"{name} asks"` is fine, but the chip must not
  say "Your ask". The `homePulseAskCta` *key* keeps its name — key names are not
  user-visible and renaming it would churn every reference for nothing.

## Implementation notes (as built)

Shipped on `home_improvements`. Three decisions changed during implementation:

1. **Both viewer fields are `optional`.** The plan argued for a bare enum, on
   the grounds that `RSVP_INTENTION_UNSPECIFIED = 0` already encodes "no
   response" — matching the existing `state` / `attended` fields. That was
   wrong for these two, and the audit's fail-fast finding is why: the batch
   fetch they ride on logs-and-continues, so a failure would have produced
   `UNSPECIFIED` / `false` — indistinguishable from "you have not responded",
   which is precisely the reported bug arriving silently. With `optional`,
   failure produces *absent*, and both the server (nil maps) and the client
   (fall back to the invite) treat that as "unknown".

2. **`viewer_rsvp` uses a feed-scoped `FeedRSVPIntention`, not the shared
   `RSVPIntention`.** The shared enum cannot be referenced from
   `feed_service.proto`: `experience_service.proto` (where it lives) imports
   `portfolio.proto`, which imports `feed_service.proto` — an import cycle.
   Moving it to `experience.proto` (same package, so the fully-qualified name
   and the Go type are unchanged) compiles and passes the Go build, but breaks
   **33 Dart files**: generated Dart re-exports per file, so consumers reach
   `RSVPIntention` through `experience_service.pb.dart`. `import public` fixes
   that in one line but then leaks `ExperienceState` unprefixed, colliding with
   the client's own Freezed `ExperienceState` class. A four-value mirror enum,
   documented with the cycle, costs less than either. Nothing converts between
   the two — feed cards only render the value.

3. **Phase 0 was not run.** It was a read-only prod check of the view-record
   premise, sized to inform the horizon value. The user approved 14 days
   directly, so the number it would have informed was already settled, and the
   premise it would have confirmed is provable from the code (`setPageIndex`
   has exactly one caller, on a screen no dock destination points at). Worth
   running if the horizon is ever retuned.

`createRequestCreatedItem`'s hardcoded `CanEdit: false` is fixed as planned.
Test fixtures `setupTestExperience` / `setupTestRequest` now default to a
future date, so the items they build read as live opportunities — a test
experience with no scheduled time at all was always a degenerate fixture.

## Implementation Plan

Ordered so each phase is independently reviewable and the riskiest change
(the horizon) lands only after its premise is confirmed against real data.

**Conventions that apply throughout.** No `TODO` without a filed issue and
inline context (`TODO(#NNNN):`). **No phase markers in code comments** —
"Phase 2", "new in this PR" and the like fail the build via
`npm run lint:ephemeral-comments`; the phase numbering here lives in this
document only. Go log fields use `target_request_id` for `models.Request` IDs,
never `request_id` (reserved for the HTTP correlation ID). Both size gates
(`lint:go:size`, `lint:dart:size`, 1,000 lines) apply —
`community_pulse_section.dart` is at 813 lines, so the CTA resolvers and the
status chip must be added with the file's headroom in mind; if it crosses,
split rather than adding the `// dart-line-count-allow` hatch.

### Phase 0: Confirm the view-record premise in production

**Goal:** Verify the claim that drives the severity of #2799 — that Home users
accumulate essentially no `FeedItemView` records — before changing behavior on
the strength of it.

**Tasks:**
- [ ] Query prod for `FeedItemView` rows for the reporter's user ID: count
      rows, and compare `MAX(last_seen_at_unix_sec)` against the age of the
      items visible in the screenshot.
- [ ] Confirm the expectation: a small number of records (roughly one per feed
      load, all pointing at whichever item was index 0), not one per item
      scrolled past.
- [ ] Sample the age distribution of items `GetFeed` currently returns for that
      user — this is the "before" number the horizon is sized against.
- [ ] Read-only. No writes, no migration, no backfill.

**Validates:** That the seen-expiry path is inert on the dock Home, so the
horizon is a sole bound rather than a backstop — which is the assumption behind
choosing 14 days and behind declining a floor. If the query instead shows
plentiful view records, the horizon can be longer and Phase 1's sizing should
be revisited before it lands.

### Phase 1: Server — the horizon and the liveness test (#2799)

**Goal:** Stale items stop being assembled; live opportunities survive
untouched.

**Tasks:**
- [ ] Add `const feedHorizonDays = 14` alongside `defaultExpiryHours` in
      [`service.go`](../../server/services/feed/service.go), with a comment
      stating what it bounds and why the value.
- [ ] Add `isWithinHorizon(lastActivityAt, now)` to
      [`freshness.go`](../../server/services/feed/freshness.go).
- [ ] Add `isLiveOpportunity(event, fc, now)` to the same file, replacing
      `isEphemeralEvent`'s unconditional exemption:
      - experience — live while `available_now.ExperienceStartUnixSec(exp)` is
        at or after the start of today; past-dated events are no longer exempt;
      - request — live while non-terminal **and** `needed_by` is unset-with-
        recent-activity or in the future (per the Open Question, an undated
        request falls through to the horizon);
      - gear — live while an open transfer exists (interest expressed /
        recipient selected / active) or the listing is `FOR_GIVEAWAY`.
- [ ] Apply in the event loop (`service.go:582–611`) and the story loop
      (`service.go:613–628`): keep when `isLiveOpportunity(...) ||
      isWithinHorizon(...)`, and keep the existing `isItemFresh` seen-expiry on
      top for items that survive. Stories are never live opportunities.
- [ ] Confirm `GetFeedStatus` needs no change — `communityHasNewFeedItems` runs
      `isItemUnread`, already bounded at 7 days, so no badge can outlive the
      14-day horizon. Add a test that pins this relationship so a future
      horizon change below 7 days cannot silently desync the badge.
- [ ] Unit tests in
      [`service_freshness_test.go`](../../server/services/feed/service_freshness_test.go):
      table-driven `isWithinHorizon`; per-kind `isLiveOpportunity`; and the
      regression that names the bug — an unseen 26-day-old story is dropped
      while an unseen 40-day-old event scheduled for tomorrow is kept.
- [ ] Update the existing ephemeral-exemption tests (`service_freshness_test.go:214`
      onward) — they currently assert "survives expiry" for past-dated
      experiences, which is the behavior being deliberately removed. Change the
      assertions rather than deleting the cases, so the new contract is pinned
      where the old one was.

**Validates:** That the horizon can be expressed without collateral damage to
live items — the single largest risk in this plan, and the reason Option B was
rejected.

### Phase 2: Server + proto — viewer state on both payloads (#2800 + request CTA)

**Goal:** The wire carries what the cards need.

**Tasks:**
- [ ] Add to `ExperienceCreatedPayload` in
      [`feed_service.proto`](../../proto/ripls/api/feed_service.proto), placed
      next to `yes_count`/`maybe_count` per the field-order convention (declare
      near semantically related fields, not appended at the end) while taking
      the next free tag:

      ```proto
      // The viewer's own response to this event. Unspecified when they
      // have not responded.
      RSVPIntention viewer_rsvp = 14;
      ```

      Non-`optional`: `RSVP_INTENTION_UNSPECIFIED = 0` already carries "no
      response", matching the existing `state` and `attended` enum fields.
- [ ] Add to `RequestCreatedPayload`, next to `offer_count`:

      ```proto
      // Whether the viewer has an active offer on this request.
      bool viewer_has_offered = 12;
      ```

      The name has no model counterpart (storage keeps a `RequestOffer` row
      with a `withdrawn` flag), so per the naming-parity rule this is an
      aggregate whose derivation is documented rather than mirrored. Comments
      on both fields stay free of server mechanics per the proto-comments rule.
      **Tag 12, not 9 or 10:** `RequestCreatedPayload` runs 1–8 then jumps to
      `can_edit = 11`, and 9/10 are an unreserved gap. Do not backfill them —
      an unreserved gap is presumed to have held a removed field, and reusing
      those tags would silently mis-decode any stored or in-flight message that
      still carries one.
- [ ] Add `viewerRSVPMap map[string]api.RSVPIntention` and
      `viewerOfferedSet map[string]bool` to `feedLookups`
      ([`freshness.go`](../../server/services/feed/freshness.go)); populate them
      inside the RSVP loop (`service.go:425`) and the offer loop
      (`service.go:388`) that already run, recording only rows whose `UserId`
      is the viewer — and, for offers, only those not `Withdrawn`, matching the
      condition `offer_count` already applies. **No new queries.**
- [ ] **Decide and document the community scoping.** `ExperienceRSVP` and
      `RequestOffer` both carry a `community_id`, and the RSVP model's own
      comment says RSVPs are community-scoped *"to prevent cross-community data
      leakage."* The batch fetches key on `experience_id` / `request_id`, so
      they return the viewer's rows from **every** community, while
      `generateFeedItems` runs per community. Recommendation: **key the viewer
      maps on the viewer alone and ignore `community_id`** — this is the
      viewer's own data being shown back to them, and the alternative is
      re-introducing the reported bug ("Join" on an event they are attending)
      whenever the RSVP was made from a different shared community. Note in the
      code comment that `yes_count` / `maybe_count` / `offer_count` already
      aggregate across communities the same way, so this is consistent with
      shipped behavior rather than a new exception. The invariant that must
      hold is per-**user**, not per-community — see the leakage test below.
- [ ] **Resolve the fail-fast tension.** Both batch fetches currently log the
      error and continue with an empty map, which is a "warn and continue" the
      Go conventions permit only with a documented reason. Once viewer state
      rides on those same maps, a failed fetch renders "Join" to someone who is
      already going — the exact defect being fixed, arriving silently. Either
      return the error (preferred by the convention) or keep the soft failure
      with an explicit comment stating that a degraded CTA is preferable to a
      failed Home load. Do not leave it undocumented.
- [ ] Populate `ViewerRsvp` in `createExperienceCreatedItem`
      ([`items.go:255`](../../server/services/feed/items.go)).
- [ ] Thread `viewerUserID` into `createRequestCreatedItem`
      ([`items.go:110`](../../server/services/feed/items.go)) — it is the only
      builder that does not take it — updating both call sites (`items.go:19`
      and `:33`). Populate `ViewerHasOffered`, and replace the hardcoded
      `CanEdit: false` with `request.RequesterId == viewerUserID`, matching how
      the experience builder computes it. Currently unread by any client, so
      this is a correctness fix with no behavior change of its own.
- [ ] Map the stored `"YES"`/`"MAYBE"`/`"NO"` strings to `RSVPIntention` in one
      unexported helper in the service package, per the conversion-lives-in-the-
      service-layer rule.
- [ ] Tests: a viewer with each intention sees it echoed and a viewer with no
      RSVP gets `UNSPECIFIED`; a viewer with an active offer gets `true`, one
      with only a **withdrawn** offer gets `false`, one with none gets `false`;
      `can_edit` is true for the requester and false otherwise.
- [ ] **Cross-user leakage test — the load-bearing one.** Every case above runs
      on a **two-member fixture** where the other member has the *opposite*
      state, asserting the viewer sees their own and never the other's. A
      single-user fixture passes even if the map is keyed wrong, which is the
      whole failure mode: these are the first per-viewer fields on a payload
      whose neighbours are all community aggregates, so a mis-keyed map hands
      one member another member's participation. This is the
      authorization-bypass class from the security rubric ("can users access
      others' resources"), scoped to a field rather than an endpoint.
- [ ] **Prove the no-new-queries claim mechanically**, don't assert it in
      prose: wrap the `GetFeed` call in `storage.WithQueryStats(ctx)` and
      `storage.AssertMaxQueries(t, ctx, N, ...)` with `N` pinned to the
      pre-change count. This is the documented guard against N+1 regressions
      and it converts the central performance claim of Option E into a test
      that fails if someone later adds a per-item lookup.
- [ ] `npm run generate`, then `npm run build` (golangci-lint + `go build`) per
      the Go build-verification convention. `go build -tags=integration ./...`
      is not required here — `createRequestCreatedItem` is unexported and
      package-local, so no integration-tagged file sees the signature change.
- [ ] No new `CommunityEventType` values, so neither the undo registry nor the
      digest registry gate applies. Stated so a reviewer does not have to check.

**Validates:** That viewer-scoped state is expressible for free on payloads
already carrying the aggregate counts — and that it is correctly scoped to the
viewer on both.

### Phase 3: Client — state-aware CTAs (#2800 + request CTA)

**Goal:** The cards stop contradicting themselves.

**Tasks:**
- [ ] In [`community_pulse_section.dart`](../../app/lib/presentation/widgets/home/community_pulse_section.dart),
      replace the unconditional `ctaLabel: l10n.homePulseEventCta` with a
      resolver over `(p.canEdit, p.viewerRsvp)`:
      `canEdit → "Hosting"`, `YES → "Going"`, `MAYBE → "Maybe"`,
      `NO → "Not going"`, `UNSPECIFIED → "Join"`.
- [ ] Replace the unconditional `ctaLabel: l10n.homePulseAskCta` with a
      resolver over `(p.canEdit, p.viewerHasOffered)`:
      `canEdit → "Your request"`, `viewerHasOffered → "Offered"`,
      otherwise `"I have one"`. Both resolvers are small private methods on the
      state class, next to the existing `_eventStatusLine` / `_eventMeta`
      helpers, keeping `_postData` a flat mapping.
- [ ] Add `bool ctaIsStatus` to `_PostData`. When true, `_header` renders
      `_CtaButton` **outside** any `Tappable` — a status chip announces as text,
      not as a button that lies about its action. Only the two "not yet
      participating" cases remain tap targets. The existing sibling-not-nested
      `Tappable` discipline documented in `_PostCard.build` is preserved.
- [ ] Give the status chip its own visual treatment (leading check for
      `Going` / `Hosting` / `Offered`) so it is not read as a dimmed button;
      verify contrast in both themes per `client/design.md`'s `AppColors`
      routing. Note the solid/ghost split still means what its doc comment says
      — solid for give/ask verbs, ghost for join — so the status variant is a
      third treatment, not a reuse of either; update that comment.
- [ ] **Never signal the state by colour alone** — the glyph and the label text
      carry it, colour only reinforces. This is both the accessibility rubric's
      colour-independence rule and a CI gate: `ruleAvoidColorOnlyStatus` in
      `scripts/check_a11y.js`.
- [ ] **Naming trap in that same gate.** The rule fires on any class named
      `*Dot` / `*Indicator` / `*Badge` in a file containing no `Semantics(` or
      `Tooltip(` call — and `community_pulse_section.dart` has neither (it goes
      through `Tappable(semanticsLabel:)` and `CachedMediaImage`). So naming
      the chip `_StatusBadge` fails the build. Either name it something else
      (`_StatusChip`) or wrap its output in `Semantics(label: …)`. The
      allowlist cannot absorb it: CI enforces a ratchet where it may shrink but
      never grow.
- [ ] Labels come from `context.l10n`, never string literals —
      `ruleRequireL10nForSemanticLabels` in the same gate.
- [ ] Add ARB keys `homePulseEventCtaGoing` / `Maybe` / `NotGoing` / `Hosting`
      and `homePulseAskCtaOffered` / `homePulseAskCtaOwn`, with descriptions.
      No string may say "experience" (`client/conventions.md` → *UI
      Terminology: "Event", not "Experience"*) and none may use "ask" as a noun
      — the chosen labels comply with both.
- [ ] Update
      [`community_pulse_section_test.dart`](../../app/test/presentation/widgets/home/community_pulse_section_test.dart),
      whose line 136 asserts `find.text('Join')` and `find.text('I have one')`
      for items with no viewer state — both still correct; add cases for each
      new state on both card types, including that the status chip is **not**
      wrapped in a `Tappable` and that tapping the card body still opens the
      item.

**Validates:** The reported render, end to end — a card reading "3 going" now
says "Going", not "Join"; and a request the viewer already answered says
"Offered", not "I have one".

### Phase 4: Vestigial item-0 view marking

**Goal:** Remove the asymmetry where the newest post is the only one that ever
expires.

**Tasks:**
- [ ] Move the `markCurrentItemViewed()` call out of
      `FeedNotifier.initialize()`
      ([`feed_view_model.dart:104`](../../app/lib/presentation/viewmodels/feed_view_model.dart))
      and into `FeedScreen`, after its first-page settle. The existing comment
      explains it exists for the `PageView`'s "`setPageIndex(0)` is a no-op"
      contract — that contract belongs to the `PageView`, and the pulse never
      wanted it.
- [ ] Confirm `FeedScreen` still marks item 0 on open (it remains in the
      `IndexedStack` and must keep working if ever re-linked).
- [ ] ViewModel test: `initialize()` no longer marks anything viewed;
      `setPageIndex` still does.

**Validates:** That the pulse no longer silently deletes its own newest card a
day later — closing the second half of "awkward and confusing."

### Phase 5: Docs

**Goal:** The two docs that describe this behavior stop being wrong.

**Tasks:**
- [ ] [`client/the_feed.md`](../client/the_feed.md) — rewrite *Ephemeral Items*
      (exemption is now conditional on liveness) and *Filtered-Out Items* (add
      the horizon as a third exclusion); note `viewer_rsvp` and
      `viewer_has_offered` on the two payloads.
- [ ] [`client/inbox.md`](../client/inbox.md) — Community pulse section: note
      the horizon and the viewer-state CTAs. Its post anatomy currently
      describes the CTA as "solid = give/ask verbs, ghost = join"; that is now
      one of three treatments.
- [ ] [`workflows/request.md`](../workflows/request.md) — check whether its
      offer-state description needs the new viewer-facing surface; update only
      if it does.
- [ ] Advance both `freshness:` stamps; run `npm run generate:context-map` if
      any `context:` block changed.
- [ ] Verify `npm run lint` (includes `lint:context-map`, `lint:dart:size`,
      `lint:go:size`) and the Go + Flutter suites.

**Validates:** That the next reader of the feed docs gets the shipped rules.

## Testing

Per [`feedback_run_the_suite_that_covers_your_change`](../../CLAUDE.md) and
`testing/architecture.md`:

- `go test ./server/services/feed/...` and with `-tags=integration` — the
  horizon and liveness branches, and the viewer-scoping tests from Phase 2
  (both payloads, two-member fixtures).
- `flutter test --reporter expanded app/test/presentation/widgets/home/` and
  `.../viewmodels/` — the CTA states on both card types and the Phase 4
  initialize change.
- `npm run lint`.
- No Playwright walkthrough asserts on the pulse CTAs (grep: the only `'Join'`
  assertions are the widget test above and an unrelated
  `open_now_list_test.dart`; `'I have one'` appears only in that same widget
  test), so no reel re-render is expected — confirm at implementation time
  rather than assuming, since a copy rename can hollow out a
  `toHaveCount(0)`-style assertion without failing.

## Related Issues

- **#2796** — *Nested zero-state nudge.* Same Home screen, same reporting
  session, and its plan already names #2799/#2800 as batch-mates. Touches
  `HomeZeroStateCard`; no file overlap with this plan.
- **#2801** — *Planning nudge text cut off on calendar screen.* Same reporter,
  same morning. Different surface (Plans), unrelated cause.
- **#2634** — *Converged nav dock.* Introduced `CommunityPulseSection` and
  orphaned `FeedScreen` at stack index 0 — the change that made the
  seen-expiry path inert. Both defects are consequences.
- **#2435** — *Home tab v4.1.* The redesign the pulse lives inside.
- **To file before implementation starts:**
  - Pulse view-marking semantics (scope exclusion 1) — whether scrolling past a
    post should mark it viewed, and how.
  - Unbounded `CommunityEvent` fetch in `generateFeedItems` (scope exclusion 2)
    — needs a time-range `QueryOptions`.

  The request-card CTA is **no longer a follow-up** — it was folded into this
  plan (Phases 2–3) at the user's request, so no issue is needed for it. The
  PR description should say so explicitly, since the change is not traceable to
  either #2799 or #2800.

---

## Audit notes

Audited against the documentation context map (`docs/llms.txt`). **Engaged
dimensions:** `api_design` (two new proto fields), `security` (per-viewer
fields on a shared payload), `performance` (feed hot path, batch fetches, an
unbounded query), `accessibility` (a status chip replacing a button),
`testing`, `code_quality`, `error_handling`, `tech_debt`. **Considered and not
engaged:** `dependency` (no new package — the deliberate reason the pulse
view-marking work is excluded rather than solved with a visibility-detector
dependency), `observability` (no new RPC, no new failure mode beyond the
fail-fast item now called out in Phase 2).

Fixes applied by the audit:

1. **Cross-community scoping made an explicit decision** (Phase 2). Both
   `ExperienceRSVP` and `RequestOffer` carry a `community_id` and the RSVP
   model comment names cross-community leakage as the reason — but the batch
   fetches key on the item, not the community, while `generateFeedItems` runs
   per community. The plan now states the chosen scoping (per-viewer, ignoring
   community), why it is right for the viewer's own data, and that the existing
   counts already behave this way. It was previously silent, which would have
   left the implementer to discover the invariant mid-change.
2. **Cross-user leakage promoted from a test bullet to the load-bearing
   control** (Phase 2), with two-member fixtures required on every case and the
   reasoning spelled out: a single-user fixture passes even when the map is
   mis-keyed. Mapped to the security rubric's authorization-bypass area.
3. **The "zero additional queries" claim is now a test**, not prose —
   `storage.WithQueryStats` + `storage.AssertMaxQueries` pinned to the
   pre-change count, per the documented N+1 regression guard.
4. **Fail-fast tension surfaced** (Phase 2). Both batch fetches log-and-continue
   with an empty map; once viewer state rides on them, a fetch failure silently
   renders the exact bug being fixed. The Go conventions allow a soft failure
   only with a documented reason, so the plan now requires the decision be made
   and written down.
5. **Two accessibility CI gates named** (Phase 3). `ruleAvoidColorOnlyStatus`
   fires on any class named `*Dot`/`*Indicator`/`*Badge` in a file with no
   `Semantics(`/`Tooltip(` — and `community_pulse_section.dart` has neither, so
   `_StatusBadge` would fail the build and the allowlist ratchet forbids
   absorbing it. Plus `ruleRequireL10nForSemanticLabels`.
6. **Proto field numbering hazard** (Phase 2). `RequestCreatedPayload` has an
   *unreserved* 9/10 gap; the plan now says explicitly to take 12 and not
   backfill, since an unreserved gap likely held a removed field.
7. **Unbounded-query deferral sharpened** (scope exclusion 2). The original
   text said `QueryOptions` lacks a time range; verification showed the problem
   is worse — `QueryByField` emits no `ORDER BY` at all and ignores
   `QueryOptions.Limit` entirely (only `ListAll` honours it), so ordering and
   bounding must be designed together. The deferral stands, better justified.
8. **Cross-cutting conventions added** before Phase 0: the ephemeral-comment
   gate (this doc's "Phase N" numbering must not reach code comments), the
   `target_request_id` log-field rule, the `TODO(#NNNN)` rule, and the
   1,000-line size gates — `community_pulse_section.dart` sits at 813 lines,
   which is the file this PR grows most.
9. **Build verification** (Phase 2): `npm run build` required after Go changes;
   `-tags=integration` explicitly reasoned as not required, so nobody has to
   re-derive it.

**Not changed, and why:** round-trip tests
(`server/services/*/roundtrip_test.go`) do **not** apply — both new fields are
server-derived read-only state, not values a client sets on a save-style RPC
and reads back, so there is no round-trip contract to assert. Adding new API
fields is wire-compatible, so no `buf breaking` concern. No new
`CommunityEventType` values, so the undo and digest registry gates are inert.

**Docs consulted.** Anchors: `client/architecture.md`, `client/conventions.md`,
`proto_conventions.md`, `server/architecture.md`, `server/conventions.md`,
`testing/architecture.md`, `users/priorities.md`. Leaves: `client/inbox.md`,
`client/the_feed.md`, `client/design.md`, `client/i18n.md`,
`client/accessibility.md`, `client/testing.md`, `workflows/experience.md`,
`workflows/request.md`. Review prompts: `api_design`, `accessibility`,
`security`, `performance`, `testing`, `code_quality`, `error_handling`,
`tech_debt`. Source verified directly where a claim was load-bearing:
`scripts/check_a11y.js`, `server/storage/protosql.go`,
`server/storage/protosql_generic.go`, `server/storage/query_assert.go`,
`server/services/feed/{service,items,freshness}.go`,
`proto/ripls/{api/feed_service,models/experience,models/request}.proto`.
