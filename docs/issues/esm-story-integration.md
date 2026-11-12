# Plan: Integrate ESM voting into the post-experience Story surface

**Status:** draft (not yet filed). Will be renamed `<number>-esm-story-integration.md` once a GitHub issue number is assigned.
**Companion issues:** #1579 (Workshop tab), #1618 (Workshop Daily Brief), #1580 (ESM v1 — feed card path)
**Reference design:** [docs/cowork/App Design/story-recap-v1-voted.jsx](../cowork/App%20Design/story-recap-v1-voted.jsx)

## Summary

Merge the post-experience Story (`STORY_TYPE_EXPERIENCE_CONCLUDED`) and the ESM
feed-card prompt (#1580) into a single full-screen editorial surface. The Story
stops being a passive recap and becomes the place where attendees answer
"want to do something like this again?" — with the response feeding the
existing `StoredESMResponse` table that drives the Workshop's ESM Repeat
Signal detector ([server/momentum/esm_repeat_signal_detector.go](../../server/momentum/esm_repeat_signal_detector.go)).

The reference design is [story-recap-v1-voted.jsx](../cowork/App%20Design/story-recap-v1-voted.jsx):
hero photo + editorial title + impact stats + a bottom voting block
(Yes / Maybe / Not for me) that resolves into a "you'd be in good company"
social-proof state showing the avatars of others who said yes.

## Why

Today the two surfaces are decoupled:

- `STORY_TYPE_EXPERIENCE_CONCLUDED` Stories ([server/jobs/experience_stories.go](../../server/jobs/experience_stories.go))
  recap the event but ask nothing.
- ESM prompts (#1580) are separate feed cards that render generically and
  compete with the rest of the feed.

The verdict-after-pulse lever in the Workshop Daily Brief
([docs/workshop.md](../workshop.md) §ESM Repeat Signal) needs ≥3 respondents at
≥60% `do_again` to fire. Splitting the recap from the prompt costs response
rate: the Story surface is emotionally hot (you just lived the thing), the
feed card is cold. The voted-state design also delivers latent social proof —
"Carmen, Derek, Emma + 2 others are also in" — which a standalone ESM card
can't earn because it has no recap context to tie it to.

## Scope (v1)

### Proto contract

- [ ] Add an `optional EsmPromptInline embedded_esm_prompt` field to
  `StoryPayload` in [proto/ripls/api/feed_service.proto](../../proto/ripls/api/feed_service.proto).
  Use the proto3 `optional` keyword per the field-conventions section in
  [CLAUDE.md](../../CLAUDE.md) — the field is conditionally present (only on
  Stories the recipient is eligible to vote on) and the client needs explicit
  `hasEmbeddedEsmPrompt()` presence checks.
- [ ] Define `EsmPromptInline` in `proto/ripls/api/esm_service.proto`
  (companion to #1580) with inline scalars: `prompt_id`, `question`,
  `repeated EsmResponseOption response_options`, `optional string
  current_response_option_key` (set if the user already voted),
  `int64 closes_at_unix_sec`. **Do not** reuse the request/response messages
  defined for `RespondToESMPrompt` or `GetESMSignal` — per
  [CLAUDE.md](../../CLAUDE.md), each RPC owns its own request/response, and
  payload messages embedded in other surfaces are also distinct.
- [ ] Proto comments on the new field describe what it means to a client,
  not how the server populates it. No "the feed service joins…" mechanics, no
  vendor names, no "issue #1580 contract" references — those go in the
  internal docs.
- [ ] **Models stay in `proto/ripls/models/esm.proto`**, untouched. The new
  inline payload lives in the API package only; conversion from
  `StoredESMPrompt` + the recipient's `StoredESMResponse` row to
  `EsmPromptInline` happens in the shared library (next section). API and
  Models do not import each other.

### Server: `server/esm/` shared library extension

**Audience model change (this issue extends #1580).** #1580 pre-creates
`StoredESMResponse` rows only for confirmed attendees, with row existence
acting as the eligibility check. The Story surface opens voting to **everyone
who can see the Story** (the community-scoped Story audience), which is
unbounded at prompt-creation time. The shared library therefore moves to a
**lazy-on-submit** ingest model for Story-driven votes: the
`StoredESMPrompt` is created when the Story is created, but
`StoredESMResponse` rows are written only when a user actually submits.
Pre-created `UNRESOLVED` rows are still produced for the attendee set so
the existing #1580 feed-card path keeps its dedup behavior; non-attendee
votes are inserted on submit.

- [ ] Add `StoredESMResponse.respondent_was_attendee` (proto3 `optional bool`
  per [CLAUDE.md](../../CLAUDE.md) field conventions) on the
  `proto/ripls/models/esm.proto` model. Set at insert time by checking
  `ExperienceRSVP.attended == YES` for the (user, experience) pair. The
  field is load-bearing for the Workshop's ESM Repeat Signal detector
  (see Workshop integration section below).
- [ ] Add a helper to [server/esm/](../../server/esm/) (introduced in #1580) —
  `esm.EmbeddedPromptForStory(ctx, store, userID, experienceID) → (*api.EsmPromptInline, error)`.
  Reads the `StoredESMPrompt` for the experience and the user's existing
  `StoredESMResponse` row in two batched queries (no N+1 per the SQL
  Efficiency rules in [CLAUDE.md](../../CLAUDE.md)). For attendees with
  pre-created rows, returns the prompt with `current_response_option_key`
  populated if they've already voted. For non-attendees with no row,
  returns the prompt in unvoted form. Returns nil only when no
  `StoredESMPrompt` exists or the prompt has expired.
- [ ] Add a helper `esm.SocialProofForStory(ctx, store, viewerUserID,
  experienceID, responseOptionKey) → ([]models.User, int)` returning up
  to N (default 3) user records that voted the same option, plus a
  remainder count. **Authorization rule for v1 (per the
  Story-audience-visibility decision):** the helper enforces
  "viewer can see this Story" via the existing community-membership /
  Story-visibility check, **not** the previous attendee-only restriction.
  The host-facing `GetESMSignal` aggregation already separates raw
  responses (host-only) from social-proof avatars (Story-audience). The
  per-respondent privacy contract from #1580 still holds: the helper
  surfaces names/avatars for vote-on-the-same-option only — it never
  reveals an opposing vote.
- [ ] Add a `RespondToESMPromptRequest` server-side path that inserts a
  fresh `StoredESMResponse` row for first-time non-attendee responders.
  The existing #1580 handler's "the row's existence is what makes the
  prompt visible to that recipient" precondition relaxes to "the prompt
  exists, hasn't expired, and the user can see the Story." Authorization
  helper updates accordingly: `requireESMRespondent` becomes
  `requireESMRespondentOrStoryViewer` (or a renamed equivalent). Mark the
  newly-created row's `respondent_was_attendee` field at insert time.
- [ ] Both read helpers are pure library functions (Pattern 1 — Standalone
  Libraries per [docs/server/architecture.md](../server/architecture.md)).
  The feed service and the experience-stories job both depend on them; they
  do **not** import the ESM service. This preserves Service Independence.

### Server: feed assembly + experience-stories job

- [ ] In [server/jobs/experience_stories.go](../../server/jobs/experience_stories.go),
  when the job creates a `STORY_TYPE_EXPERIENCE_CONCLUDED` row, **also** call
  into `server/esm/` to (a) create the `StoredESMPrompt` for the experience
  with the v1 canonical question and (b) `InsertBatch` the
  `StoredESMResponse` rows (one per attendee, `consumed_action = UNRESOLVED`)
  via the existing #1580 ingest path. **Extract the ESM materialization into a
  separate file** under the job package (e.g.
  `experience_stories_esm.go`) — do not let `experience_stories.go` grow past
  the 500-line target / 1000-line hard cap from
  [docs/server/architecture.md](../server/architecture.md).
- [ ] In the feed service's Story payload assembly path, call
  `esm.EmbeddedPromptForStory(...)` (and ignore the result if nil — Stories
  for non-attendees ship without the embedded prompt). The feed service must
  not import the ESM service; only the shared library.
- [ ] Errors from the materialization step return via
  `connecterr.Internal(ctx, "ExperienceStoriesJob.MaterializeESM", err,
  "experience_id", expID, "story_id", storyID)` — never bare
  `connect.NewError(connect.CodeInternal, err)` (gated by
  `scripts/check_codeinternal.js`).
- [ ] If any goroutine fans out the per-recipient row insertion, wrap with
  `logging.GoSafe(ctx, "ExperienceStoriesJob.MaterializeESM", fn)` so panics
  are captured.
- [ ] Logging in the new path uses
  `logging.LoggerWithContext(ctx).With("operation",
  "ExperienceStoriesJob.MaterializeESM", "experience_id", expID, "story_id",
  storyID, "esm_prompt_id", promptID)`. Do **not** name a field
  `request_id` — that key is reserved for the X-Request-ID HTTP correlation
  injected by `LoggerWithContext` (see
  [docs/server/observability.md](../server/observability.md)).
- [ ] Successful materialization → `INFO`. Per-recipient skip (already had a
  row) → `DEBUG`. Storage write failure → `ERROR`. Validation failures (e.g.
  experience has no attendees) → `WARN`, not `ERROR`.

### Server: undo registry

- [ ] No new `CommunityEventType` is introduced — the existing #1580 RPC
  (`RespondToESMPrompt`) handles vote events and its classification entry in
  [server/undo/registry.go](../../server/undo/registry.go) is shared. Verify
  the existing entry still applies; update its rationale if the Story
  surface changes the natural inverse path.

### Server: Workshop ESM Repeat Signal — attendee-only threshold

The decision to let everyone-who-can-see-the-Story vote means the
`StoredESMResponse` table now mixes attendee and non-attendee responses.
The Workshop's ESM Repeat Signal detector
([server/momentum/esm_repeat_signal_detector.go](../../server/momentum/esm_repeat_signal_detector.go))
fires on ≥3 respondents at ≥60% `do_again` — a non-attendee saying "yes,
I'd want them to do this again" carries different signal weight than an
attendee saying "yes, I'd do this again." Conflating them risks the Daily
Brief's verdict-after-pulse lever firing on enthusiasm-from-the-sidelines
rather than actual post-event satisfaction.

- [ ] **The detector filters to `respondent_was_attendee == true` for the
  ≥3 respondents / ≥60% threshold.** Non-attendee votes still persist and
  still count toward Story social proof (the "+N also in" tally), but they
  do not feed the verdict-after-pulse lever. The constants
  `ESMRepeatSignalMinRespondents = 3` and `ESMRepeatSignalThreshold = 0.60`
  remain unchanged; only the source set changes.
- [ ] Add an attendee-only filter in `esm.Aggregate(...)` so
  `GetESMSignal` returns both the full distribution and an
  `attendee_only_distribution` slice. The Workshop detector consumes the
  attendee-only slice; the host-facing aggregation surfaces the full
  distribution (with respondent counts split by attendee status) so the
  host can read the room *and* see the wider sentiment.
- [ ] Update the Workshop docs prose in [docs/workshop.md](../workshop.md)
  §ESM Repeat Signal once this lands so the threshold definition stays in
  sync.

### Client: data layer

- [ ] Repository: extend the existing `ESMRepository` (created in #1580)
  with `getEmbeddedPromptCacheKey(experienceID)` and a
  `getSocialProof(experienceID, optionKey)` method. Cache namespace `'esm'`,
  keys `'esm:embedded:<experienceId>'` and
  `'esm:social_proof:<experienceId>:<optionKey>'` per the
  [client caching conventions](../client/caching.md). Do **not** add manual
  cache `Map<String, T>` fields to the StoryViewModel — repository owns
  caching.
- [ ] After a successful `respondToPrompt(...)` call, the repository
  invalidates `'esm:embedded:<experienceId>'` and
  `'esm:social_proof:<experienceId>:*'` *and* notifies an
  `esmCacheInvalidationProvider` (NotifierProvider<int> counter, mirroring
  `searchCacheInvalidationProvider`) so the standalone ESM feed card and
  any other surfaces refresh.

### Client: Story viewer rendering

- [ ] Port the [story-recap-v1-voted.jsx](../cowork/App%20Design/story-recap-v1-voted.jsx)
  voting + voted layouts into the existing Story viewer screen. Keep the
  build method short by extracting `_VotingBlock`, `_VotedBlock`, and
  `_SocialProofRow` private widgets per
  [docs/client/architecture.md](../client/architecture.md) §Flutter Screens.
- [ ] Vote buttons (Yes / Maybe / Not for me) must use the `Toggle`
  accessibility primitive (state-selectable, exposes `selected:`) — never
  raw `GestureDetector`/`InkWell`/`IconButton`. Hit-target ≥48px. See
  [docs/client/accessibility.md](../client/accessibility.md).
- [ ] Avatar row uses `CachedMediaImage(semanticsLabel: ...)` with a
  per-user label derived from `context.l10n.a11yPhotoOfUser(name)`. Never
  `Image.network` for user media.
- [ ] The "+N also in" remainder pill and the italic descriptor wrap in a
  `LiveRegion` so screen readers announce the social-proof update when it
  appears post-vote.
- [ ] Use `accessibleDuration(context, dur)` for the vote → voted state
  transition animation. Do not pass raw `Duration` to `AnimatedSwitcher`.
- [ ] State-bearing widgets use `selected: vote == option.key` rather than
  baking "selected" into the label string — `avoid_color_only_status` and
  the selectable-state lints will fail otherwise.

### Client: ViewModel + state

- [ ] `StoryViewModel` (the existing Story screen viewmodel) gains a typed
  `EsmVoteState` (enum: `unresolved`, `voting`, `submitting`, `voted`,
  `errored`), an optional `EsmEmbeddedPrompt` and `EsmSocialProof` slice,
  and methods `submitVote(optionKey)` and `clearVote()`. The VM never
  resolves user-visible strings; it exposes typed enums + counts.
- [ ] `submitVote` calls `esmRepository.respondToPrompt(...)` and, on
  success, fetches fresh social proof via the same repo (cache miss path).
  Optimistic-update the social-proof avatars: append the user's own avatar
  to the local list before the await, replace with the server response on
  success, roll back on error. Pattern from
  [docs/client/architecture.md](../client/architecture.md) §Optimistic
  Updates.
- [ ] All async paths add `if (!ref.mounted) return;` between every `await`
  and any subsequent `state =` assignment. Disposal-safe per the Async
  Safety section.

### Client: i18n

- [ ] All voting-block strings live in both `app/lib/l10n/app_en.arb` and
  `app/lib/l10n/app_es.arb`. Keys under a stable `esmStory*` scope:
  `esmStoryQuestionDoAgain`, `esmStoryOptionYes`, `esmStoryOptionMaybe`,
  `esmStoryOptionNo`, `esmStoryVotedYesGoodCompany`, `esmStoryVotedMaybe`,
  `esmStoryVotedNo`, `esmStoryChange`. Per
  [docs/client/i18n.md](../client/i18n.md).
- [ ] The "+N also in" remainder uses ICU plural in
  `esmStoryRemainderCount`: `{count, plural, =0{} =1{+1 other} other{+{count}
  others}}`.
- [ ] All user-visible copy says **"Event"** when referring to the underlying
  `Experience` (per [CLAUDE.md](../../CLAUDE.md) UI Terminology rule). The
  question copy "Want to join if they do something like this again?" already
  avoids the term, but any descriptor that references the underlying object
  must say "Event".
- [ ] Accessibility labels (semantic labels for icons, avatars, voting
  buttons) come from the same ARB file under an `a11y*` prefix —
  `a11yEsmVoteYes`, `a11yEsmVoteMaybe`, `a11yEsmVoteNo`,
  `a11yEsmPhotoOfUser`. Raw string literals in `Semantics(label: ...)` will
  fail the `require_l10n_for_semantic_labels` custom lint.

### Tests (required, not "if time permits")

**Server (Go):**

- [ ] Unit tests for the new helpers in `server/esm/` —
  `EmbeddedPromptForStory_test.go` covering: no prompt, expired prompt,
  unresolved row, already-responded row, non-attendee. Use
  `storage.SetupTestStorage(t)` per [CLAUDE.md](../../CLAUDE.md).
- [ ] Integration test in `server/jobs/experience_stories_integration_test.go`
  (file already gated by `//go:build integration`) verifying that a
  completed experience produces both the Story row AND the StoredESMPrompt
  + StoredESMResponse rows for each attendee.
- [ ] N+1 guard via `storage.AssertMaxQueries(t, ctx, max, fn)` on the
  embedded-prompt assembly path — three batched queries max.
- [ ] After signature changes, run `go build -tags=integration ./...` per
  [CLAUDE.md](../../CLAUDE.md) §Go Server Build Verification — `npm run
  build` does not compile integration files.

**Client (Dart):**

- [ ] ViewModel tests in `app/test/presentation/viewmodels/story_view_model_test.dart`
  using `ProviderContainer` with `MockESMRepository`. Cover loading →
  voting → submitting → voted, error path, and disposal-during-submit. Use
  `--reporter expanded`.
- [ ] Repository tests in `app/test/data/repositories/esm_repository_test.dart`
  asserting cache hit (no service call), cache miss-then-store, and
  invalidate-then-refetch after `respondToPrompt`.
- [ ] Widget tests for `_VotingBlock` and `_VotedBlock` covering:
  `Toggle.selected` reflects state; `Semantics` labels match `context.l10n`;
  hit-target ≥48px; `LiveRegion` wraps social-proof updates.
- [ ] Coverage targets per [docs/client/testing.md](../client/testing.md):
  90% for the ViewModel, 80% for the repository helpers.

## Out of scope (defer)

- Multi-prompt Stories — v1 ships one embedded prompt per Story.
- Free-text follow-up in the Story surface — the #1580 standalone card
  remains the free-text capture path.
- Replacing the standalone ESM feed card — both surfaces stay operational;
  the standalone card is the catch-up for users who skip the Story.
- Cross-experience or community-wide prompts — bound to a single
  `experience_id`.

## Resolved decisions

- **(2026-05-02) Story-audience visibility for the prompt.** Everyone who
  can see the Story sees the prompt and can vote — not just attendees. This
  diverges from #1580's "attendees-only" eligibility model and forces the
  ingest changes documented in the *Server: `server/esm/` shared library
  extension* section: `StoredESMResponse` rows are pre-created for
  attendees but inserted lazily on submit for non-attendees, and a new
  `respondent_was_attendee` field is added to the model so the Workshop
  detector can keep the verdict-after-pulse threshold attendee-only while
  the social-proof tally counts everyone.

## Open questions

- **Vote change semantics.** Recommendation: update the existing
  `StoredESMResponse` row in place, with a new `response_changed_at_unix_sec`
  field on the model so aggregations can optionally exclude flip-floppers.
  Adds a one-line schema addition to the #1580 model; needs explicit sign-off
  before we ship since #1580 didn't anticipate it.
- **Question selection.** v1 hardcodes the canonical "want to do something
  like this again?" Question. v2 lets the host swap from a small bank at
  story-author time, mirroring the host-side authoring flow planned in
  [docs/runbooks/esm-manual-authoring.md](../runbooks/esm-manual-authoring.md).

## Test plan (end-to-end acceptance)

- [ ] Story renders with embedded ESM prompt for an attendee with
  `consumed_action = UNRESOLVED`.
- [ ] Tapping a vote option calls `RespondToESMPrompt` and flips the
  surface to the voted state without a feed-level reload.
- [ ] "You'd be in good company" social proof shows the right avatars and
  `+N` count when ≥1 other attendee voted the same option, with proper
  ICU pluralization in en and es locales.
- [ ] "Change" returns to the voting state and updates the response
  server-side (vote-change semantics — pending the open question above).
- [ ] Workshop Daily Brief's verdict-after-pulse lever fires after 3
  **attendees** (not non-attendees) vote `do_again` on the embedded prompt
  — no separate ESM card path required. Non-attendee yeses do not move the
  threshold.
- [ ] Non-attendee community member who can see the Story can vote, sees
  their vote reflected in social proof, and sees other Story-audience
  members' avatars in the "+N also in" tally — but their response does
  not change whether the Workshop verdict-after-pulse lever fires.
- [ ] Privacy review: only people who can see the Story can see the
  social-proof avatars (Story-audience visibility, not attendee-only); the
  helper never reveals an opposing vote; never names another participant in
  the prompt itself.
- [ ] Accessibility audit: VoiceOver and TalkBack walk the voting block,
  the social-proof row, and the "change" affordance with correct
  announcements; vote-state changes announced via `LiveRegion`.

## References

- Companion: #1580 (ESM v1 — feed card path, contract this builds on)
- Companion: #1579 (Workshop tab) and #1618 (Workshop Daily Brief — the
  consumer of the resulting signal)
- Reference design: [docs/cowork/App Design/story-recap-v1-voted.jsx](../cowork/App%20Design/story-recap-v1-voted.jsx)
- Workshop integration: [docs/workshop.md](../workshop.md) §ESM Repeat
  Signal
- Story creation path: [server/jobs/experience_stories.go](../../server/jobs/experience_stories.go)

---

## Decision log

- **2026-05-02 — Story-audience visibility for the embedded prompt.**
  Everyone who can see the Story can see the question and submit a vote,
  not only confirmed attendees. Forces three downstream changes from the
  initial draft:
  1. `StoredESMResponse` ingest model splits — pre-created `UNRESOLVED`
     rows for attendees (preserves #1580 dedup); lazy on-submit insert for
     non-attendee responders.
  2. New `StoredESMResponse.respondent_was_attendee` field, set at insert
     time by checking `ExperienceRSVP.attended == YES`.
  3. Workshop ESM Repeat Signal detector filters to
     `respondent_was_attendee == true` for its ≥3 / ≥60% threshold —
     non-attendee enthusiasm doesn't fire the verdict-after-pulse lever
     while still feeding Story social proof.

## Audit notes (added 2026-05-02)

What changed from the initial draft and why, with reference docs cited:

**Proto layer**

- **Added explicit `optional` instruction.** `embedded_esm_prompt` was
  described as "optional" in prose but didn't call out the proto3 `optional`
  keyword that gives `hasEmbeddedEsmPrompt()` in Dart and `*EsmPromptInline`
  in Go. Required by the Proto Field Conventions in [CLAUDE.md](../../CLAUDE.md).
- **Forbade reuse of `RespondToESMPromptRequest` etc. for the inline
  payload.** Made it explicit that `EsmPromptInline` is a fresh message,
  per the no-RPC-type-reuse rule in [CLAUDE.md](../../CLAUDE.md).
- **Tightened proto-comment guidance.** The original draft hinted at
  describing the populating mechanism in proto comments. Replaced with the
  rule from [CLAUDE.md](../../CLAUDE.md) §Proto Comments: comments describe
  what the field means to a client, not how the server populates it; no
  vendor names; no transport plumbing.
- **Surfaced API/Models separation.** Added an explicit "models stay in
  `proto/ripls/models/esm.proto`" note so the conversion path is visible —
  the original draft glossed over this and could have been read as putting
  the inline payload in the models package.

**Server layer**

- **Service Independence enforced.** Original draft said "the feed
  assembler emits the Story payload, populate `embedded_esm_prompt` by
  joining against `StoredESMResponse`." That reads as the feed service
  reaching into ESM internals. Rewrote to route through `server/esm/`
  shared-library helpers (`EmbeddedPromptForStory`, `SocialProofForStory`)
  per Pattern 1 in [docs/server/architecture.md](../server/architecture.md).
  Both the feed service and the `ExperienceStoriesJob` consume the library;
  neither imports the other service.
- **File size guard.** Original draft folded the ESM materialization
  directly into `experience_stories.go`. Added an explicit "extract to
  `experience_stories_esm.go`" requirement to keep both files under the
  500-line target / 1000-line hard cap from
  [docs/server/architecture.md](../server/architecture.md).
- **`connecterr.Internal` requirement.** Added the canonical error
  pattern; bare `connect.NewError(connect.CodeInternal, err)` would fail
  `scripts/check_codeinternal.js` per [CLAUDE.md](../../CLAUDE.md).
- **Goroutine safety.** Added `logging.GoSafe` requirement for any
  background fan-out — `experience_stories.go` already runs in a job
  context where stray panics are easy to lose without it.
- **Structured logging fields.** Original draft said nothing about
  observability. Added the explicit `operation`, `experience_id`,
  `story_id`, `esm_prompt_id` field set; called out the `request_id` /
  `target_request_id` collision per
  [docs/server/observability.md](../server/observability.md).
- **Log level discipline.** Added the level mapping (validation = WARN,
  storage failure = ERROR, per-recipient skip = DEBUG) so we don't
  flood ERROR with "expected" failures and lose alerting signal.
- **Undo registry verification.** Added a verification step — no new
  `CommunityEventType` should be needed since #1580's classification covers
  the response-creation flow, but the audit forced us to confirm rather
  than assume. `TestEveryEventTypeClassified` would catch a regression.
- **Integration build flag.** Added the `go build -tags=integration ./...`
  step explicitly. `experience_stories.go` has integration tests; signature
  changes there will only be caught by the tagged build, per
  [CLAUDE.md](../../CLAUDE.md).

**Client layer**

- **Caching architecture.** Original draft never mentioned the repository
  layer. Added the `ESMRepository` extension with namespace `'esm'`, keys
  `'esm:embedded:<experienceId>'` and
  `'esm:social_proof:<experienceId>:<optionKey>'`, plus invalidation-after-
  mutation and the `esmCacheInvalidationProvider` cross-screen pattern, per
  [docs/client/caching.md](../client/caching.md). The original draft would
  have invited stashing voting state on the StoryViewModel as a
  `Map<String, EsmEmbeddedPrompt>` — explicitly forbidden.
- **Accessibility primitives.** Original draft showed three voting buttons
  without naming the primitive. Added the `Toggle` requirement (selectable
  state, hit-target ≥48px), `CachedMediaImage(semanticsLabel:)` for
  avatars, `LiveRegion` for social-proof updates, and
  `accessibleDuration(context, dur)` for the state-flip animation, per
  [docs/client/accessibility.md](../client/accessibility.md). Without this,
  the new code would fail `avoid_raw_gesture_detector`,
  `require_semantic_label_on_icon_button`, and the selectable-state lints.
- **i18n discipline.** Original draft had user-visible strings inline in
  prose. Added explicit ARB-key naming under the `esmStory*` and `a11y*`
  scopes, the requirement to update both `app_en.arb` and `app_es.arb`, ICU
  plural for the "+N others" remainder, and the **Event vs Experience**
  rule (user-visible copy says "Event" even though code identifiers stay
  `Experience`), per [docs/client/i18n.md](../client/i18n.md) and
  [CLAUDE.md](../../CLAUDE.md).
- **ViewModel separation.** Added a typed `EsmVoteState` enum + structured
  `EsmEmbeddedPrompt` / `EsmSocialProof` payloads on the VM. The original
  draft would have invited storing localized strings or
  `String currentVote` on the VM — both forbidden by
  [docs/client/architecture.md](../client/architecture.md) and
  [docs/client/i18n.md](../client/i18n.md).
- **Disposal safety.** Added the `if (!ref.mounted) return;` requirement
  between every `await` and `state =` assignment — the original draft
  didn't mention it and the Story viewer is a likely autoDispose surface.
- **Optimistic-update pattern.** Added the optimistic-then-rollback pattern
  for the social-proof avatar list rather than a "wait for server then
  show" path — closer to the design's intent (instant flip to the voted
  state).

**Tests**

- **Promoted from "test plan" to required scope items.** Original draft had
  end-to-end acceptance criteria but no breakdown of unit / VM / repo /
  widget layers. Added explicit ViewModel + repository + widget test
  requirements with coverage targets (90% VM, 80% repo) per
  [docs/client/testing.md](../client/testing.md), and explicit
  `storage.SetupTestStorage` + `storage.AssertMaxQueries` requirements on
  the server side per [CLAUDE.md](../../CLAUDE.md).
- **Disposal test required.** Added explicit disposal-during-submit
  ViewModel test per the disposal-testing section in
  [docs/client/architecture.md](../client/architecture.md).
- **N+1 guard.** Added the `AssertMaxQueries` requirement on the
  embedded-prompt assembly path — without it, an innocent-looking helper
  refactor could turn three batched queries into one-per-attendee.
