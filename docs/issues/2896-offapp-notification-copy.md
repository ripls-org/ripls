# Issue #2896: SMS notification has confusing wording

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2896
**Filed:** 2026-08-12 | **Last activity:** 2026-08-12
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md`, `proto_conventions.md`, `server/observability.md`,
`testing/architecture.md`; leaves `sms_notifications.md`,
`push_notifications.md`, `server/l10n.md`, `server/scheduled_notifications.md`;
review dimensions `error_handling`, `testing`, `code_quality`, `observability`

> **Status: implemented.** All five steps landed as one PR. Where the work
> diverged from the plan, see *Implementation notes* at the end.

## Problem Statement

A prod user received this platform SMS/RCS instead of an event reminder:

```
Ripls:  posted an update in

View at https://ripls.app/go/Md5UQpRB
```

Two blanks where a name and a community should be, a dangling preposition, and
zero information about what happened. The only context the recipient got came
from the link preview Google Messages rendered underneath (the event's hero
photo and its title) — the message body itself said nothing. The underlying
notification was the 2-hours-before event reminder, whose push copy ("Event
starting soon" / "*Backyard BBQ* starts in 2 hours") is good and was discarded
on the way to SMS.

**Reproduced deterministically.** A throwaway test against
`notification_content` renders exactly the reported string, byte for byte, for
`EXPERIENCE_REMINDER` and seven sibling event types:

```
EXPERIENCE_REMINDER => kind="default"
Ripls:  posted an update in

View at https://ripls.app/go/Md5UQpRB
```

### Root cause — three layers

**1. Eight system-generated event types collapse onto `default`.**
`CommunityEventPayload.event_type` is a free-form `string` documented as "a
`CommunityEventType`'s enum-string form" (`notification_content/kinds.go`).
Eight producers put a *synthetic*, non-enum string in that field instead:

| Event type string | Producer |
|---|---|
| `EXPERIENCE_REMINDER` | `jobs/scheduled_notifications/experience_reminders.go:424` |
| `EXPERIENCE_CLOSE_PROMPT` | `jobs/scheduled_notifications/experience_reminders.go:532` |
| `LOAN_RETURN_REMINDER` | `jobs/scheduled_notifications/loan_return_reminders.go:238` |
| `REQUEST_FOLLOWUP_PROMPT` | `jobs/scheduled_notifications/request_followup_prompts.go:304` |
| `EXPERIENCE_NEEDS_NUDGE` | `services/experience/nudge_needs.go:132` |
| `EXPERIENCE_TIME_NUDGE` | `services/experience/time_poll_manage.go:41` |
| `EXPERIENCE_LOCATION_NUDGE` | `services/experience/location_poll_manage.go:42` |
| `REQUEST_NEEDS_NUDGE` | `services/request/nudge_needs.go:129` |

None appear in `offAppKinds`, so `kindForEventType` returns `"default"` for all
eight.

**2. The `default` copy assumes an actor and a community that these payloads
structurally cannot have.** `notif.offapp.default.message` is
`"{{.ActorName}} posted an update in {{.CommunityName}}"`. These notifications
are *system*-generated — there is no actor, ever. And `CommunityName` is set on
a payload in exactly one place in the whole tree —
`server/cmd/notification_examples/main.go:45`, the review tool. The production
bus path (`community_subscriber/copy.go:207`) sets it; **no synthetic producer
does**. Both template slots resolve to `""` on every one of these sends, in
both locales (`es.toml` has the same shape).

**3. Nothing could have caught it.** The copy-review surface,
`server/cmd/notification_examples`, enumerates `models.CommunityEventType_name`
— the *enum*. The eight synthetic strings are not enum values, so they have
never appeared on the review page. `notification_content_test.go` exercises the
`default` fallback only with `ActorName`/`CommunityName` populated
(`TestRenderSMS_NoLink`, line 190) — precisely the case that cannot occur in
production.

### Scope beyond SMS

`sendOffAppEmail` (`off_app_email.go:157`) runs the same `Content` through
`RenderEmail`, and `EmailParts.Subject = msg`. Every off-app *email* for these
eight types ships with the subject line `"  posted an update in "`. The
reported SMS is the visible half of a bug that also degrades email.

Push is unaffected — `NotifyUser` only reaches off-app when the recipient has
zero `UserDevice` rows, and the push path uses the producer's own good
title/body.

### The copy is duplicated four ways

Investigating the fix surfaced a larger finding. Per-kind notification copy
lives in **four** places today:

1. `community_subscriber/copy.go` — a 27-case switch, the production push copy.
2. `notification_content.RenderPush` — an "illustrative" re-implementation of
   that switch for the review page, which re-encodes the same special cases
   (`experience_started` title-is-the-name, the transfer loan/giveaway split,
   the `experience_updated` variants). Duplicated *logic*, not just strings.
3. `notif.offapp.{kind}.{message,cta}` — the off-app copy.
4. Inline `fmt.Sprintf` English in the eight synthetic producers
   (`renderReminderCopy`, `renderClosePromptCopy`, `renderLoanReturnCopy`,
   `renderFollowupPromptCopy`, the nudge sites) — push copy that is in no
   catalog and is therefore not localized at all.

And (1) and (3) are largely the *same sentence written twice*. Comparing the
`notif.community_event.{kind}.body` and `notif.offapp.{kind}.message` pairs
that exist today:

| kind | push body | off-app message | |
|---|---|---|---|
| `request_offer_made` | `{Actor} offered to help with {Request}` | *identical* | dup |
| `gear_shared_giveaway` | `{Actor} is giving away {GearWithArticle}` | *identical* | dup |
| `gear_shared_loan` | `{Actor} is lending out {GearWithArticle}` | *identical* | dup |
| `experience_created` | `{Actor} created {Experience}` | *identical* | dup |
| `transfer_pickup_proposed` | `{Actor} proposed a pickup time for {Gear}` | *identical* | dup |
| `transfer_interest_expressed` | `{Actor} wants to borrow your {Gear}` | `{Actor} is interested in {Gear}` | drift |
| `invitation_link_used` | `{ActorFirstName} joined {Community}` | `{ActorName} joined {Community}` | drift |
| `experience_rsvp_yes` | `{Actor} is attending your event` | `{Actor} is attending {Experience}` | real |
| `experience_started` | `is starting now` (title = event name) | `{Experience} is starting now` | real |
| `experience_cancelled` | `Event has been cancelled` | `{Experience} was cancelled` | real |
| `planning_need_added` | `{Actor} added a need` (title = event name) | `{Actor} needs {Item} for {Experience}` | real |

Five pairs are verbatim duplicates. Two differ for no defensible reason. The
four that genuinely differ all differ for the *same* structural reason: the
push **title** carries the entity name, so the push **body** is a sentence
fragment. That is the only real surface difference — and in those four cases
the off-app sentence is the better copy, because a lock-screen push truncates
the title first.

## Context from Comments

**The GitHub issue has no comments** — it is a one-shot user-feedback
submission with a screenshot, filed against prod 0.29.2 on a Pixel 9 Pro.
Two clarifications came from the reporter during planning:

- **Why an off-app SMS reached a user who has the app:** it is a dedicated test
  account with a phone number pointed at that handset and no registered device.
  Working as designed — **not a device-token defect, and not in scope.** The
  earlier suspicion that push was silently failing is withdrawn.
- **Direction:** prefer reusing the FCM copy, and prefer deduplication among
  the notification copy generally, "unless it is architecturally unsound."
  Addressed head-on in Options B and C below — reusing the *rendered push
  string* is unsound and there are concrete counterexamples, but the underlying
  instinct is right and Option C is the sound version of it.

## Potential Solutions

### Option A: Write bespoke off-app copy for each of the eight kinds
- **Approach:** Map the eight strings in `offAppKinds`, add
  `notif.offapp.{kind}.{message,cta}` entries to `en.toml` + `es.toml`, and
  populate the payload names each needs.
- **Pros:** Fits the current architecture exactly; no risk to push.
- **Cons:** Adds a *fifth* place per-kind copy lives, and writes new sentences
  that mostly restate the ones the producers already wrote inline. Leaves the
  four-way duplication untouched, and leaves the eight producers' push copy
  unlocalized.
- **Effort:** Medium.

### Option B: Reuse the rendered push title/body off-app
- **Approach:** When `Kind == "default"`, feed `Notification.Body` through the
  legacy `sms.notification.body` wrapper (as the chat path already does).
- **Pros:** One small change; covers all eight and anything future.
- **Cons:** **Architecturally unsound as stated, with concrete
  counterexamples.** Push splits one sentence across title + body because the
  OS renders both; SMS and an email subject have no title. Reusing the rendered
  body verbatim would send:
  - `experience_started` → `"Ripls: is starting now"`
  - `experience_cancelled` → `"Ripls: Event has been cancelled"` (which event?)
  - `planning_need_added` → `"Ripls: Sam added a need"` (which need? which
    event?)
  - `default` → `"Ripls: Something happened in your community"`

  Concatenating title + body instead does not rescue it — that yields
  `"Backyard BBQ is starting now"` for one kind and `"Help Offered Sam offered
  to help with Folding Table"` for the next. Separately, the eight producers'
  bodies are inline English, so a Spanish recipient would get English SMS where
  today they get (bad) Spanish — re-crossing the render boundary
  `docs/sms_notifications.md` §Localization draws.
- **Effort:** Small, and wrong.

### Option C: One sentence per kind, plus per-surface adornments
- **Approach:** Collapse `notif.community_event.{kind}.body` and
  `notif.offapp.{kind}.message` into a **single** `message` key per kind — the
  self-contained, entity-inline sentence. Each surface then adds only what is
  actually surface-specific:
  - **push** = short category/entity `title` + `message`
  - **SMS/email** = `message` + `cta` verb on the `/go` link

  The five verbatim-duplicate pairs merge for free. The two drifted pairs pick
  one wording. The four fragment bodies get promoted to full sentences, which
  improves push as a side effect. The eight synthetic producers move their
  existing inline sentence into the catalog as that kind's `message` — so
  #2896's fix is *moving* eight sentences, not *writing* eight new ones, and it
  localizes their push copy in the same move. `RenderPush` stops being a second
  implementation of the `copy.go` switch and becomes the only one.
- **Pros:** This is the deduplication the reporter asked for, in the form that
  survives the counterexamples. It removes two of the four copy locations
  outright (the `RenderPush` re-implementation and the inline `fmt.Sprintf`
  copy) and merges a third into the catalog. It fixes #2896, the email-subject
  half of it, and the unlocalized-reminder-push bug together.
- **Cons:** Touches every push notification in production. The four fragment
  cases need a copy *decision*, not a mechanical merge. Larger in scope than the
  reported bug strictly requires.
- **Effort:** Medium–large.

### Option D: Promote the eight strings into the `CommunityEventType` enum
- **Approach:** Make `event_type`'s vocabulary singular and typed.
- **Cons:** Wrong semantics and real cost. These are never persisted as
  `CommunityEvent` rows, never published to the bus, never in the Feed. Per
  `docs/server/conventions.md` §"Adding New CommunityEventType Values", every
  new enum value must be classified in **two** registries
  (`server/undo/registry.go`, `server/activity_digest/registry.go`) or the
  build fails — so eight reminders would need undo semantics and ops-digest
  categories they have no meaning for.
- **Effort:** Large, and it buys a worse model.

## Recommendation

**Ship a guard first, then Option C. Reject B and D; A is what C replaces.**

To answer the direct question: reusing the FCM copy is the right instinct but
the wrong mechanism. Reusing the *rendered* push string is unsound — the four
counterexamples above are real keys in the catalog today, and `"Ripls: is
starting now"` is a worse message than the one that prompted this issue.
Reusing the *source sentence* is sound, and the catalog already shows why: five
of the eleven comparable pairs are verbatim duplicates, and two more have
drifted apart with no one deciding they should ("wants to borrow your X" vs "is
interested in X"). That drift is the cost of keeping two catalogs, and it will
keep accruing.

So the target is one `message` per kind, with `title` (push) and `cta`
(off-app) as the only per-surface keys. Push and off-app do differ — but they
differ in *adornment*, not in the sentence.

**Delivered as a single PR** (reporter's call; SMS/email volume is low enough
that incremental exposure buys nothing). One caveat to hold onto: the low-volume
argument covers the off-app surfaces, but step 4 rewrites production **push**
copy for every kind, and push is not low-volume. That risk is not managed by
splitting the PR — it is a catalog diff, so splitting it just spreads the same
diff over more reviews. It is managed by the two things this repo already has:
the `notification_examples` render-all page, which exists precisely to review
the whole copy corpus at once, and the `copy_test.go` / `copy_locale_test.go`
assertions. Regenerating and reading that page is therefore a **PR-level gate**
below, not an optional nicety — and reviewing all copy in one page is easier
than reviewing it across five PRs, so one PR is the better shape here anyway.

The steps below are a work order inside that one PR, not separate deliverables.
Step order still matters: steps 2–3 prove the "one sentence, adornments per
surface" shape on the eight kinds that have no existing catalog entry (so
nothing can regress) before step 4 applies it to kinds that already have two.

## Open Questions

- **How should the three reminder offsets be discriminated?** The payload has
  no offset field, and the copy differs per slot ("tomorrow" / "in 2 hours" /
  "starting now"). — *Recommendation:* distinct event-type strings
  (`EXPERIENCE_REMINDER_DAY_BEFORE` / `_TWO_HOUR` / `_STARTING`), not a new
  proto field. `event_type` is already `string`, the offset is known at
  `experience_reminders.go:417`, and no proto change means no generate or
  breaking-change step. Keep bare `EXPERIENCE_REMINDER` mapped too, so an
  in-flight row written by an older binary still renders sane copy.
- **Is renaming any existing synthetic string safe?** — *Recommendation:* no
  for `REQUEST_FOLLOWUP_PROMPT` — `app/lib/services/fcm_service.dart:618`
  matches it verbatim to route to `/request/{id}`, so an older installed client
  would lose that routing. Add new strings freely; rename none. The other seven
  have no client-side matcher (routing is entity-id-driven).
- **For the four genuinely-divergent pairs, which wording wins?** —
  *Recommendation:* the off-app sentence, in all four. It is the self-contained
  one, and promoting it to the push body improves push (lock screens truncate
  the title first). The push title then becomes a pure category label ("New
  Event", "RSVP"), which is what it already is for most kinds.
- **Should the `default` copy change for the genuinely unmapped *enum* types?**
  (`GEAR_UNSHARED`, `MEMBER_LEFT`, `TRANSFER_COMPLETED`, the `*_UNDONE`
  family…) — *Recommendation:* no. They carry a real actor and community, and
  step 1's variant selection is param-driven, so a populated payload keeps
  today's sentence unchanged. Whether those types should reach off-app at all
  is a separate question.
- **Should the SMS keep relative time phrasing given SMS can be delayed?** —
  *Recommendation:* yes. The 2h and starting-now slots use
  `QuietHoursPolicySkip` (`experience_reminders.go:356-367`), so they are
  dropped rather than fired late; only the 24h slot defers, and "is tomorrow"
  survives a defer to 07:00 by design.

## Implementation Plan

**One PR.** The steps below are the work order within it, sequenced so that the
zero-regression-risk work lands before the work that touches existing push
copy. Each step's **Checkpoint** is what to verify before starting the next —
a commit boundary, not a merge boundary.

### PR-level acceptance

The PR is not done until all of these hold:

- [ ] `go test ./server/notifications/... ./server/jobs/scheduled_notifications/...`
      green, including the new empty-`Content` guard and per-kind render tests.
- [ ] `npm run lint:go` green — notably `lint:go:l10n-parity`
      (`check_l10n_toml_parity.js`) and `lint:go:no-inline-user-strings` with
      the newly enrolled files.
- [ ] `go run ./server/cmd/notification_examples > /tmp/notifications.html` —
      regenerated, opened, and **read end to end in both locales**. This is the
      review surface for the push-copy changes in step 4; the PR description
      should say it was reviewed. It must now include the eight synthetic kinds
      (step 5), which it never has before.
- [ ] No `notif.*` key is left with two spellings of the same sentence, and no
      kind renders an empty template slot on an empty payload.
- [ ] `REQUEST_FOLLOWUP_PROMPT` still matches verbatim in
      `app/lib/services/fcm_service.dart:618` — no synthetic event-type string
      was renamed.
- [ ] **Every new key exists in `es.toml` with a real Spanish translation**, not
      an English placeholder. `check_l10n_toml_parity.js` proves the key exists;
      it cannot prove it was translated. This PR adds ~12 kinds × 2–3 subkeys.
- [ ] New catalog copy says **"event", never "experience"** (CLAUDE.md
      terminology rule; `scripts/check_event_terminology_go.sh` gates the Go
      side but **nothing gates the TOML catalog** — this is a manual check).
- [ ] **No new per-send storage reads.** See the N+1 note in step 2.
- [ ] `proto/ripls/models/notification.proto` — `event_type`'s comment ("The
      type of event") is updated to document that the field carries **two**
      vocabularies (`CommunityEventType` enum names for bus events, synthetic
      strings for system-generated notifications) and that both must have copy.
      This is the contract `kinds.go` currently mis-states.

**Failure mode to design against:** `Localizer.T` falls back to **the message ID
itself** on a miss (`server/l10n/localizer.go:86-90`) — deliberately, so
operators notice. With ~12 new keys, a typo ships
`notif.offapp.experience_reminder.message` as the literal SMS body. Parity lint
plus the step 5 gate are what stand between this PR and that outcome; treat them
as load-bearing, not cleanup.

### Step 1: Make `default` degrade honestly
**Goal:** No off-app message can render an empty template slot, whatever the
event type — including ones nobody has enumerated yet.
**Tasks:**
- [ ] Make `notif.offapp.default.message` param-conditional in `en.toml` +
      `es.toml`: both names present (today's sentence, unchanged), community
      only, neither. Note the catalog **already has this pattern** —
      `notif.offapp.planning_need_added.message` uses
      `{{if .PlanningItemName}}…{{else}}something{{end}}` — so this is applying
      an existing idiom, not inventing one.
- [ ] Add a `notification_content` test that renders **every** kind in
      `offAppKinds` plus `default` against a **fully empty** `Content` and
      asserts no doubled space, no leading/trailing space on any line, and no
      empty template residue.
- [ ] Pin current `kindForEventType` behavior for the eight synthetic strings
      in `notification_content_test.go` before it changes.
- [ ] **Log a rate-limited WARN when `kindForEventType` falls through to
      `default`**, with the `event_type` as a structured field. This defect was
      invisible for months because falling back is silent; the l10n layer
      already sets this precedent (`logMissing`,
      `server/l10n/localizer.go:85`). Per `docs/server/observability.md` the
      line is then countable by the existing log-based-metric stack, so "an
      event type is reaching users with no copy" becomes alertable rather than
      waiting on a screenshot. WARN, not ERROR: the send still succeeds.
- [ ] `npm run lint:go:l10n-parity` — new keys must land in both catalogs.
**Checkpoint:** That the empty-param render is the whole mechanism behind the
screenshot, that the guard holds for unenumerated kinds, and that a fallthrough
now shows up in logs.

### Step 2: Move the four scheduled reminders' copy into the catalog
**Goal:** The reported case reads correctly on SMS and email — and its push
copy becomes localized in the same change.
**Tasks:**
- [ ] Split `EXPERIENCE_REMINDER` into per-offset event-type strings at
      `experience_reminders.go:417-428`; keep the bare string mapped as a
      fallback.
- [ ] For each of the four scheduled types, move the producer's existing
      sentence out of `fmt.Sprintf` into `notif.{kind}.message` and add a `cta`
      + a push `title`. Render push from the catalog at the producer; render
      off-app via `offAppKinds`. One sentence, three surfaces.
- [ ] Populate the payload names the sentence needs: `ExperienceName` on
      `EXPERIENCE_REMINDER*` and `EXPERIENCE_CLOSE_PROMPT` (the `Experience` is
      already loaded at `experience_reminders.go:385` and `:497`).
      `LOAN_RETURN_REMINDER` already carries `GearName`;
      `REQUEST_FOLLOWUP_PROMPT` already carries `RequestTitle`.
- [ ] Keep the producers' existing empty-name fallbacks ("your event", "the
      item", "the owner") as catalog conditionals — do not lose them in the
      move. These are the graceful-degradation path for a failed name lookup
      and must survive, per the error-handling rubric.
- [ ] **Do not add a community-name lookup.** An earlier draft of this plan said
      to populate `CommunityName` "wherever the lookup is cheap" — that is an
      N+1 waiting to happen: `Dispatch` renders inside a per-row loop
      (`dispatch.go:46`), so a community read in `Render` is one query per
      notification per tick, on top of the reads `buildEmailExtras` already
      does per send. It is also unnecessary — none of these four sentences
      names a community ("*Backyard BBQ* starts in 2 hours"). Populate only
      names already in hand from an entity the dispatcher has **already
      loaded**; add no new query.
- [ ] Extend the existing tests rather than adding parallel ones:
      `notification_content_test.go` (per-kind SMS/email render),
      `experience_reminders_dispatcher_test.go` and the sibling dispatcher
      tests (payload fields now populated), `off_app_sms_test.go` (the rendered
      body end to end through `renderSMSBody`), `off_app_email_test.go`
      (subject is no longer blank).
- [ ] Run the suites locally, not just in CI:
      `go test ./server/... -count=1` and the integration build tag. A
      copy-shaped change is exactly the kind that hollows out an assertion
      without failing it.
**Checkpoint:** The reported notification now carries its own context, and the
"one sentence, adornments per surface" shape works end to end on four real
kinds before it is applied to the ones that already have two catalog entries.

### Step 3: Same treatment for the four nudges
**Goal:** Close the remaining half of the eight.
**Tasks:**
- [ ] `EXPERIENCE_NEEDS_NUDGE`, `EXPERIENCE_TIME_NUDGE`,
      `EXPERIENCE_LOCATION_NUDGE`, `REQUEST_NEEDS_NUDGE`: move the inline
      sentence to the catalog, add `cta`, populate `ExperienceName` /
      `RequestTitle` (both in scope at the send sites — `expStored.Name`,
      `requestStored.Title`).
- [ ] The two poll nudges are the weakest case: their body ("The organizer is
      waiting on your pick.") names no entity at all, and the event name lives
      only in the title. Promote to a full sentence.
- [ ] Note the payload gap: the three experience nudges set no `CommunityId`
      (`poll_manage.go:276`, `nudge_needs.go:132`), so `notificationLink`
      cannot resolve a community-scoped `/go` short link and falls back to the
      bare app URL. **Resolve it by writing copy that names no community** —
      same reasoning as step 2, no new lookup — and leave the link gap alone
      here; it is a real but separate defect. If it is worth fixing, file an
      issue and reference it as `TODO(#NNNN)` per the CLAUDE.md TODO policy;
      do not leave a bare `TODO:` or a template slot that can render empty.
- [ ] With the inline English gone, pre-flight and then enroll
      `server/jobs/scheduled_notifications/*.go` and the nudge sites in
      `scripts/check_no_inline_user_strings.js` `SCANNED_FILES`.
**Checkpoint:** Every event-type string reaching `NotifyUser` has catalog copy —
the precondition for the step 5 gate.

### Step 4: Merge the duplicated pairs *(the rest of Option C)*
**Goal:** One `message` per kind across push and off-app; delete the second
implementation.
**Tasks:**
- [ ] Merge the five verbatim-duplicate pairs (`request_offer_made`,
      `gear_shared_giveaway`, `gear_shared_loan`, `experience_created`,
      `transfer_pickup_proposed`) — mechanical, no copy decision.
- [ ] Resolve the two drifted pairs (`transfer_interest_expressed`,
      `invitation_link_used`) to one wording each.
- [ ] Promote the four fragment bodies (`experience_rsvp_yes`,
      `experience_started`, `experience_cancelled`, `planning_need_added` and
      its `planning_*` siblings) to the self-contained off-app sentence; reduce
      the push title to a category label.
- [ ] Delete `notification_content.RenderPush`'s re-implementation of the
      `copy.go` switch — with one `message` per kind, both surfaces render from
      the same key and the review page shows production copy rather than an
      illustrative approximation.
- [ ] Work kind-by-kind in separate commits, each keeping the existing
      `copy_test.go` / `copy_locale_test.go` assertions green, so a reviewer can
      read the push-copy change one kind at a time and `git bisect` has
      something to land on. One PR, not one commit.
**Checkpoint:** The duplication is removable at the level the comparison table
claims, and the regenerated `notification_examples` page shows no push-copy
regression in either locale.

### Step 5: Make the gap impossible to reintroduce
**Goal:** A new scheduled notification or nudge cannot ship without copy.
**Tasks:**
- [ ] One canonical list of every event-type string that can reach `NotifyUser`
      (enum names + synthetic), plus a test in the shape of the existing
      `TestEveryEventTypeClassified` asserting each has copy or an explicit,
      commented exclusion.
- [ ] Point `server/cmd/notification_examples` at that list instead of
      `models.CommunityEventType_name`, so the review page shows the synthetic
      kinds it has never shown.
- [ ] Update `docs/sms_notifications.md` §Localization (the "unknown →
      `default`" sentence understates the rule),
      `docs/server/scheduled_notifications.md` (off-app copy is part of adding
      a kind), and `docs/push_notifications.md` / the `notification_content`
      package doc (whose "reusing the push title/body off-app reads wrong"
      rationale is superseded by the one-message-plus-adornments model).
      Advance the `freshness:` stamps.
**Checkpoint:** That the third root-cause layer is closed, not just the first
two.

## Related Issues
- #2274 — established the `REQUEST_FOLLOWUP_PROMPT` synthetic-event-type
  pattern (`docs/issues/2274-request-followup-prompt.md`) that the other seven
  producers copied; also the source of the client-side verbatim match in
  `fcm_service.dart`.
- #2573 — RCS. The screenshot is an RCS thread; RCS drops the STOP/HELP footer
  entirely, which makes the message body the *only* copy the recipient sees.
- #2827 / #2835 — the in-app copy migration that established
  `check_no_inline_user_strings.js` and its `SCANNED_FILES` / `NO_COPY_FILES`
  enrollment discipline, which step 3 follows.
- #1659 — closed; its "localisation is a follow-up" comments are still cited in
  `renderReminderCopy` and `renderFollowupPromptCopy`, and steps 2–3 retire
  them.

## Implementation notes

What the build found that the plan did not anticipate:

1. **The stand-in had to be general, not default-only.** The plan scoped the
   empty-slot guard to the `default` line. But every kind interpolates names
   from lookups that can miss, so `experience_rsvp_yes` could render "  is
   attending " just as easily. The fix moved up a level: `Content.params`
   substitutes a localized stand-in (`notif.standin.*`) for any empty name, and
   the `default` line's four wordings sit on top of that, reading the raw
   Content fields to tell "the lookup missed" from "there was never an actor".
2. **Two bus events had no copy either.** `REQUEST_OFFER_SELECTED` and
   `REQUEST_FULFILLED` were never in the kind map — same defect class as the
   eight system types, in the enum vocabulary. The registry gate found them the
   moment push started routing through the same map.
3. **The transfer merge needed two payload fields.** Push split
   `TRANSFER_ACTIVE`/`TRANSFER_CANCELLED` three ways (loan / giveaway / covered
   offer) from a `Transfer` read the off-app senders don't do, so off-app had
   one coarse sentence for all three. Merging to one sentence per kind required
   `transfer_type` and `transfer_offer_covered` on `CommunityEventPayload` —
   both additive, both mirroring `gear_availability`'s existing rationale.
4. **`community_deleted_fallback` disappeared.** It existed because a failed
   community read left the name blank. The stand-in covers that, and the
   fallback's copy was strictly worse (it dropped the actor), so the kind and
   its branch are gone.
5. **The follow-up prompt lost its day count** ("You asked 3 days ago") as
   planned — no payload field carries it — and `request_followup_prompt` kept
   its event-type string, as the plan required for `fcm_service.dart`.
6. **Push locale is unchanged and now tracked.** `SystemNotification` renders
   push with the default English localizer: the dispatcher holds only a user id,
   and resolving `preferred_language` would be one read per due row inside the
   per-row loop. Off-app *is* localized. That copy was hardcoded English before,
   so this is a carried-forward gap, filed as **#2897** with a `TODO(#2897)` at
   the call site.
7. **A new gate came out of reviewing the rendered page.** The merge spliced
   `GearWithArticle`'s English article into Spanish ("está regalando **a** Power
   Drill"). `TestNonEnglishCatalogsAvoidEnglishOnlyParams` now fails the build on
   an English-composed param appearing in a translated catalog. This is exactly
   what the "read the review page end to end in both locales" gate was for.

Not done: nothing from the plan was dropped.

## Audit notes

Audited against the documentation context map. Scope: server-only; subsystems
notifications (push + off-app SMS/email), scheduled notifications, server l10n.
Engaged dimensions: `error_handling`, `observability`, `performance`, `testing`,
`code_quality`. Not engaged: `accessibility` (no UI), `dependency` (no new
deps), `security` (no auth/secret/PII surface — the SMS path's PII handling,
`logging.MaskPhone` and never logging the body, is untouched by this plan),
`api_design` (no proto schema change; the one proto edit is a comment).

Changes the audit made to the plan:

1. **Performance — removed an N+1.** The draft said to populate `CommunityName`
   "wherever a lookup is cheap". `Dispatch` renders inside a per-row loop, so
   that is one query per notification per tick. Replaced with an explicit
   no-new-reads rule; none of the affected sentences needs a community name.
2. **Observability — added the missing signal.** The draft fixed the copy but
   left the fallthrough silent, which is *why this was invisible for months*.
   Added a rate-limited WARN on `kindForEventType` fallthrough, following the
   existing `logMissing` precedent, so it is countable by the log-based-metric
   stack.
3. **Error handling — named the runtime failure mode.** `Localizer.T` returns
   the message ID on a miss, so a typo in one of ~12 new keys ships the raw key
   as an SMS body. Made parity lint + the step 5 gate explicitly load-bearing.
4. **Testing — replaced "unit-test it" with the actual files.** Named
   `notification_content_test.go`, `experience_reminders_dispatcher_test.go`,
   `off_app_sms_test.go`, `off_app_email_test.go`, and required running the
   suites locally with `-count=1` plus the integration tag rather than relying
   on CI.
5. **Conventions — added three acceptance gates the draft missed:** real
   Spanish translations (parity lint proves existence, not translation);
   "event" not "experience" in the new catalog copy (the Go gate does not cover
   TOML); and updating the `event_type` proto comment, which does not document
   the two-vocabulary reality the whole bug rests on.
6. **TODO policy — closed an open fork.** Step 3's "either plumb the community
   or write copy that does not name one" would have produced an untracked TODO.
   Resolved to the no-lookup option, with the link-resolution gap called out as
   a separately-filed issue if pursued.

**Docs consulted:** `docs/llms.txt`; anchors `server/architecture.md`,
`server/conventions.md` (§Adding New CommunityEventType Values, §Go Code
Comments), `proto_conventions.md`, `server/observability.md`,
`testing/architecture.md`; leaves `sms_notifications.md` (§Localization,
§Per-message STOP/HELP disclosure cadence), `push_notifications.md`,
`server/l10n.md` (§The render boundary), `server/scheduled_notifications.md`;
review prompts `error_handling`, `testing`, `code_quality`, `performance`,
`observability` (Areas to Review); plus `CLAUDE.md` (TODO policy, file-size
gates, Edit-tool rule, terminology rule).
