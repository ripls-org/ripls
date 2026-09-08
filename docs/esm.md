---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: ESM — a one-question post-experience pulse-check rendered inline on recap stories; its aggregated "do it again" signal feeds the Workshop Repeat Signal detector, with prompt/response storage and read-time enrichment.
  globs: [server/esm/**, server/services/esm/**, server/services/feed/**, proto/ripls/models/esm.proto, app/lib/presentation/widgets/story/**]
  triggers: [esm, experience-sampling, pulse-check, recap-story, repeat-signal, do-again, social-proof]
  lens: [domain, server, client]
  domain: experience
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# ESM (Experience Sampling Methodology)

## Overview

ESM is a lightweight pulse-check that fires after a community experience
concludes. One question, three quick-tap options, no free text. The signal it
collects ("would members do this again?") feeds two surfaces:

1. **The recap story.** Every `STORY_TYPE_EXPERIENCE_CONCLUDED` story renders
   the prompt inline at the bottom of the card so the answer happens *inside*
   the moment the member is already engaging with — not as a separate
   notification or feed item.
2. **The Workshop home.** The Workshop's *Repeat Signal* detector reads the
   aggregated responses and emits a `StoredNudge` row that the
   [Workshop home](workshop.md) renders as an action postcard ("Schedule
   {Name} again" with `Alice and Bob want another` as the atmosphere
   subtitle) once the signal crosses a confidence threshold.

The codename "ESM" comes from the academic experience-sampling literature, but
in product copy this is just the question rendered on the recap story. There
is no separate "ESM" tab, screen, or notification.

## The canonical question

v1 ships a single hand-tuned question with three stable option keys:

> **Want to join if they do something like this again?**
> - `do_again` → "Yes"
> - `maybe` → "Maybe"
> - `not_for_me` → "Not for me"

The question text and options live in
[server/story/subscriber/esm.go](../server/story/subscriber/esm.go).
The `do_again` key is load-bearing: the Workshop detector looks it up by
literal string, and the client maps it to the social-proof "good company"
voted state. v2 will let hosts pick from a small bank of templates; until then
this single question carries every concluded story.

## Data model

Two storage messages, both in
[proto/ripls/models/esm.proto](../proto/ripls/models/esm.proto):

- **`StoredESMPrompt`** — one row per `(experience, community)` pair. Carries
  the question, the option set, the recipient list, and a `closes_at_unix_sec`
  expiration (48 hours from creation; `DefaultPromptWindowSeconds` constant in
  [server/esm/authoring.go](../server/esm/authoring.go)).
- **`StoredESMResponse`** — one row per `(prompt, user)` pair. Tracks the
  user's `consumed_action` (`UNRESOLVED` → `RESPONDED` or `DISMISSED`),
  their selected option key, and a `respondent_was_attendee` flag that
  distinguishes pre-seeded attendee rows from rows lazily created when a
  non-attendee story viewer votes (see *Authorization* below).

The API contract for inline rendering — `EmbeddedEsmPrompt` and the
social-proof fields embedded on `StoryPayload` — lives in
[proto/ripls/api/feed_service.proto](../proto/ripls/api/feed_service.proto).

## Lifecycle

### 1. Prompt materialization (at experience completion)

When an experience transitions to `EXPERIENCE_STATE_COMPLETED`, the
`CompleteExperience` RPC ([server/services/experience/lifecycle.go](../server/services/experience/lifecycle.go))
emits an `EXPERIENCE_COMPLETED` community event. That event flows through
the community-event bus to the story subscriber
([server/story/subscriber/experience.go](../server/story/subscriber/experience.go)),
which creates the recap story and then materializes the ESM prompt. The
materialization is idempotent — re-running on an already-prompted
`(experience, community)` pair is a no-op. Pre-seeded attendee rows are
inserted at the same time, all with `respondent_was_attendee = true`, so
the Workshop detector can later filter to attendees only without a join.

The materialization helper (`materializeESMPromptForStory`, with the
canonical question) is
[server/story/subscriber/esm.go](../server/story/subscriber/esm.go);
the underlying prompt-building helper that both the recap path and the
manual-authoring path share is `BuildPromptForExperience` in
[server/esm/authoring.go](../server/esm/authoring.go).

ESM materialization runs **inline** with story generation — there is
no background job. If materialization fails, the failure is logged but does
not roll back the story. The story degrades gracefully (no embedded prompt,
the bottom of the card is empty) rather than disappearing.

### 2. Read-time enrichment

When a client requests stories or the feed, the feed service joins each
`STORY_TYPE_EXPERIENCE_CONCLUDED` story with its prompt and the viewer's
own response (if any) so the recap story can render its voting block on
first paint without a second round-trip. This enrichment is in
[server/services/feed/stories_esm.go](../server/services/feed/stories_esm.go),
which delegates to [server/esm/story_embed.go](../server/esm/story_embed.go)
for the per-story lookup and social-proof slicing (max 3 inline avatars
plus a `+N` remainder).

### 3. Response submission

When the viewer taps a vote button, the client calls `RespondToESMPrompt`
([server/services/esm/responses.go](../server/services/esm/responses.go)).
The handler validates that the prompt is still open, the option key is
known, and the caller is authorized (see next section), then writes
`consumed_action = RESPONDED` and the chosen option key to the response
row. **Vote changes are allowed** — repeat submissions overwrite the prior
choice — but **dismiss-after-vote is rejected** with `FailedPrecondition`,
since once you've contributed signal you can't withdraw it by dismissing.

## Authorization

The gate is `requireESMRespondentOrStoryViewer` in
[server/services/esm/authorization.go](../server/services/esm/authorization.go).
It reflects the design choice that any community member who can see the
story should be able to vote, not just the people who attended:

- **Existing response row.** If the caller already has a row (i.e. they
  were pre-seeded as an attendee, or they voted before), the gate just
  confirms the row belongs to them. This is the common path.
- **No row yet.** If the caller has no row, the gate requires them to be
  an active member of the prompt's community (via
  `auth.RequireMemberOfActiveCommunity`) and lazily inserts an `UNRESOLVED`
  row with `respondent_was_attendee = false` before persisting their vote.

This lazy-insert path is why the `respondent_was_attendee` flag exists:
the Workshop detector needs to count attendee opinions only, even though
the response table contains rows from non-attendees who saw the story and
decided to weigh in.

## Aggregation and the Workshop signal

Aggregation is a pure function in
[server/esm/aggregation.go](../server/esm/aggregation.go) that takes a
slice of response rows and returns the per-option distribution plus the
respondent count. Two entry points:

- `Aggregate(rows, completedAt)` — counts everyone with a response.
- `AggregateAttendeesOnly(rows, completedAt)` — pre-filters to
  `respondent_was_attendee == true` before delegating. This is the variant
  the Workshop detector uses.

The detector itself is
[server/services/workshop/momentum/esm_repeat_signal_detector.go](../server/services/workshop/momentum/esm_repeat_signal_detector.go)
(the standalone variant) plus
[server/services/workshop/momentum/per_event.go](../server/services/workshop/momentum/per_event.go)
(the per-event variant invoked from `EnsureHeroCard`). Both fire when
the experience has:

- ≥2 attendee respondents (`ESMRepeatSignalMinRespondents`), and
- ≥60% of those attendees selected `do_again`
  (`ESMRepeatSignalThreshold`).

Both thresholds are necessary — the floor keeps a single "Yes" out of
two from firing while the ratio keeps two "Yes" votes among many "No"s
from firing. Non-attendee votes are visible in the recap story's
social-proof avatar row but never count toward the threshold.

**Atmosphere line — names instead of counts.** When the detector
fires, it batch-loads the `User` rows for the attendees who selected
`do_again` and emits a names-formatted atmosphere line:

- 1 name → `"Alice wants another"`
- 2 names → `"Alice and Bob want another"`
- 3+ names → `"Alice, Bob, and 2 others want another"`

The detector falls back to the count-shaped string
(`"X of Y said yes — pulse came back"`, or
`"X of Y said yes, window closed"` after the prompt's 48-hour window
has expired) when the user lookup fails or returns no usable display
names. The per-event variant uses the count-shaped string directly;
adding the names lookup there is a follow-up.

**Two variants of the headline.** The detector picks one based on
whether the prompt's response window has closed:

- Open window (still accepting responses) → headline is
  `"Schedule {Name} again"`.
- Closed window (≥48 hours after creation) → headline shifts to
  `"Make {Name} a weekly thing"` so the host's next move is to
  propose a cadence rather than a single instance.

**Recap-fallback upgrade.** The per-event flow guarantees every
recently-completed experience has *some* postcard — when no specific
signal has fired yet, a recap-fallback nudge is materialized
("Schedule {Name} again / Just wrapped {weekday}"). When the ESM
detector subsequently fires (e.g., the second attendee's `do_again`
arrives the next day), `EnsureHeroCard` re-runs `DetectForEvent`,
recognizes the prior nudge as a fallback by its kicker
(`"Just wrapped"`), soft-deletes it, and inserts the upgraded
ESM-signal nudge in its place. See
[docs/workshop.md](workshop.md#per-event-detection--recap-fallback)
for the full upgrade flow.

## Client rendering

The voting block is [app/lib/presentation/widgets/story/story_embedded_esm.dart](../app/lib/presentation/widgets/story/story_embedded_esm.dart),
mounted by [app/lib/presentation/screens/story/story_content_view.dart](../app/lib/presentation/screens/story/story_content_view.dart)
when `StoryPayload.embedded_esm_prompt` is present. Two states:

- **Voting state** (no current vote): the question, then a row of three pill
  buttons. The `do_again` button is the accent-color primary; the other two
  are muted. Tapping submits the vote optimistically; on RPC failure the
  optimistic update rolls back.
- **Voted state** (current vote present): a small status label
  ("You'd be in good company" for `do_again`; soft acknowledgements for the
  other two), the social-proof avatar row (only meaningful for `do_again`,
  where co-respondents make sense), and a low-opacity *change* link that
  flips back to the voting state.

Vote submission flows through
[app/lib/data/repositories/esm_repository.dart](../app/lib/data/repositories/esm_repository.dart),
which calls the underlying service in
[app/lib/services/esm_service.dart](../app/lib/services/esm_service.dart)
and bumps two cache notifiers on success: the feed-listing one and the
ESM-specific one (`esmCacheInvalidationProvider` in
[app/lib/services/providers/cache_providers.dart](../app/lib/services/providers/cache_providers.dart)).
Any screen watching either notifier refreshes when the count increments.

## Tests

- Aggregation: [server/esm/aggregation_test.go](../server/esm/aggregation_test.go)
- Story-embed enrichment: [server/esm/story_embed_test.go](../server/esm/story_embed_test.go)
- Response RPC contract (auth, vote-changes, dismiss-after-vote, expiry,
  unknown options): [server/services/esm/responses_test.go](../server/services/esm/responses_test.go)
- Workshop detector (≥2 / ≥60% threshold; names atmosphere line for
  1/2/3+ respondents): the relevant tests in
  [server/services/workshop/momentum/detectors_test.go](../server/services/workshop/momentum/detectors_test.go)
- Voting widget (voting state, voted state, optimistic rollback,
  change affordance): [app/test/presentation/widgets/story/story_embedded_esm_test.dart](../app/test/presentation/widgets/story/story_embedded_esm_test.dart)
- Repository (notify-on-success, no-notify-on-failure):
  [app/test/data/repositories/esm_repository_test.dart](../app/test/data/repositories/esm_repository_test.dart)
