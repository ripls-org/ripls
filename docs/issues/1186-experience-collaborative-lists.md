# Issue #1186: Experience collaborative lists: Requests, Offers & Chat integration

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1186
**Filed:** 2026-04-18 | **Last activity:** 2026-04-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** [docs/workflows/request.md](../workflows/request.md)

## Problem Statement

Experiences currently have three tabs (Event, Details, Chat), but the Details
tab only shows impact tiles and Quality Time attributes. There is no way for
participants to coordinate *what they're bringing* or *what the group still
needs*. All coordination happens informally in chat, which means needs get
buried in scroll-back and duplicates are common.

The issue asks for two collaborative lists on the Details tab:

1. **Requests ("Still needed")** - things the group needs, proposed by anyone,
   with slot counts so multiple people can claim the same need
2. **Offers ("Who's bringing what")** - things people are bringing/doing,
   grouped by person as tappable chips

Both lists integrate with chat: every add/claim/offer posts a system message
automatically. The model is fully democratic (no host powers, no approval flow).

## Context from Comments

No comments on the issue. The issue was filed with comprehensive specs
including a reference React mock at `docs/cowork/mocks/potluck.jsx`. The
requirements are well-defined; the main planning work is mapping them onto
existing architecture.

## Potential Solutions

### Option A: New standalone proto models (ExperienceRequest + ExperienceOffer)

Create two new proto messages in `proto/ripls/models/` that are purpose-built
for experience coordination, completely separate from the existing `Request`
and `RequestOffer` models.

- **Approach:** New `ExperienceRequest` model (with slots, proposer, note) and
  `ExperienceOffer` model (with title, description, from_request linkage).
  New RPCs on ExperienceService. New storage tables.
- **Pros:** Clean separation; no risk of breaking existing request/offer
  workflows; models can be shaped exactly for the use case; simpler queries
  (no filtering needed to exclude experience-scoped items from feeds).
- **Cons:** Some conceptual overlap with existing Request/RequestOffer models;
  two systems to maintain if we later want to unify; more new code.
- **Effort:** Medium-large.

### Option B: Extend existing Request model with experience scoping

Add `experience_id` to the existing `Request` model and `slots` field.
Extend `RequestOffer` with title/description. Filter experience-scoped
requests out of feeds/discovery.

- **Approach:** Reuse existing Request infrastructure (storage, service,
  notifications, events) but add experience-scoping fields. Existing
  `RequestService` RPCs get experience-aware variants.
- **Pros:** Reuses battle-tested request infrastructure; metrics roll up
  naturally; easier path to unifying experience and standalone request UX.
- **Cons:** Requires careful filtering everywhere requests are listed to
  exclude experience-scoped ones; existing Request model has heavier
  semantics (AI generation, Unsplash, cross-community sharing) that don't
  apply here; `RequestOffer` extension risks breaking existing offer
  workflows; the "democratic" model (anyone proposes/removes) conflicts with
  existing request ownership model (only requester can update/cancel).
- **Effort:** Medium, but high risk of regressions.

### Option C: New models, but on ExperienceService (recommended)

Create new proto models (`ExperienceNeed` + `ExperienceContribution`) in
`proto/ripls/models/experience.proto` and add RPCs to the existing
`ExperienceService`. The models are lightweight and purpose-built, but live
alongside the experience they belong to.

- **Approach:** Add `ExperienceNeed` (name, note, slots, proposer_id,
  experience_id) and `ExperienceContribution` (title, description,
  contributor_id, from_need_id, experience_id) to the experience models
  proto. Add ~6 RPCs to `ExperienceService`. Chat integration uses existing
  `SystemMessageWriter` pattern with new `ChatSystemAction` values.
- **Pros:** Clean models without legacy baggage; lives on ExperienceService
  so all experience coordination is in one place; no risk to existing
  request/offer workflows; models are simple enough to later inform
  improvements to the standalone request system.
- **Cons:** Doesn't reuse existing request infrastructure (but that
  infrastructure doesn't fit well anyway); new storage tables needed.
- **Effort:** Medium.

## Recommendation

**Option C** — new purpose-built models on ExperienceService.

The existing `Request` model is too heavy and too different in semantics.
Requests have AI generation, Unsplash images, cross-community sharing,
owner-only mutations, and a state machine (ACTIVE/OFFERS_RECEIVED/FULFILLED/
CANCELLED). Experience needs are lightweight (name + note + slots), anyone
can propose or remove their own, and they disappear when fully claimed.
Trying to retrofit this onto the existing Request model would require
extensive filtering, permission model changes, and risk regressions in a
core workflow.

The models should be named `ExperienceNeed` and `ExperienceContribution`
(not "Request" and "Offer") to avoid confusion with the existing standalone
Request/Offer concepts in the codebase. The issue uses "request" and "offer"
as user-facing terms, but the proto models should be distinct.

## Open Questions (Resolved)

- **Should experience needs have their own conversation, or share the
  experience conversation?** — **Decision: Share the experience conversation.**
  The issue spec shows system messages posting to the experience chat, and
  having separate conversations would fragment discussion. The experience
  already has a `conversation_id`.

- **Should the slot count be capped?** — **Decision: Yes, cap at 20.**
  Validate server-side.

- **How should "edits within ~5 minutes update the original system message"
  work?** — **Decision: Out of scope for this issue.** This is a generic
  problem that also affects RSVP system messages. Filed as #1188 to solve
  it once for all system action types in the `SystemMessageWriter`. This
  issue (#1186) will benefit from that solution once it lands.

- **Should experience needs count toward the user's standalone request
  metrics?** — **Decision: Metrics roll up to the experience, but community
  events should reflect requests and offers.** Users should be recognized
  for their collaboration (e.g., "Gary offered help at Spring Potluck")
  via community events, even though the impact metrics aggregate under the
  parent experience rather than inflating standalone request counts.

- **What happens to needs/contributions when an experience is cancelled or
  completed?** — **Decision: Mark completed and follow existing patterns.**
  They become read-only (no new adds, edits, or claims) using the existing
  `isTerminal` pattern. Existing data remains visible for reference.

- **Should we support quick-pick suggestions for needs (like "Dessert",
  "Wine" in the mock)?** — **Decision: Defer to a later phase.** Start
  with free-form input only.

## Implementation Plan

### Phase 1: Proto models + server storage + basic RPCs

**Goal:** Establish the data model and server API so the client can build against it.

**Tasks:**
- [ ] Add `ExperienceNeed` message to `proto/ripls/models/experience.proto`:
  `id`, `experience_id`, `name`, `optional string note`, `int32 slots` (default 1),
  `proposer_id`, `int64 created_at_unix_sec`, `optional DeletedMetadata deleted`.
  Use the `optional` keyword for all conditionally-present fields (`note`,
  `deleted`) per proto field conventions.
- [ ] Add `ExperienceContribution` message to `proto/ripls/models/experience.proto`:
  `id`, `experience_id`, `contributor_id`, `title`, `optional string description`,
  `optional string from_need_id`, `optional string original_need_name`,
  `optional string original_need_note`, `int64 created_at_unix_sec`,
  `optional int64 updated_at_unix_sec`, `optional DeletedMetadata deleted`.
- [ ] Add API-level response types to `proto/ripls/api/experience.proto`:
  `ExperienceNeedResponse` (enriched with proposer User object, remaining
  slots count) and `ExperienceContributionResponse` (enriched with
  contributor User object, grouped by contributor).
- [ ] Add RPCs to `proto/ripls/api/experience_service.proto` — each RPC must
  have its own dedicated request and response message types with inline
  fields (never reuse request/response types across RPCs):
  - `AddExperienceNeed` — create a need (any participant)
  - `RemoveExperienceNeed` — remove a need (proposer only)
  - `ClaimExperienceNeed` — claim a slot, creates a contribution linked to need
  - `UnclaimExperienceNeed` — return a claimed contribution, restores slot
  - `AddExperienceContribution` — free-form offer (any participant)
  - `EditExperienceContribution` — update title/description (contributor only)
  - `RemoveExperienceContribution` — delete own contribution (contributor only)
  - `ListExperienceNeedsAndContributions` — returns all needs + contributions
    for an experience (renamed from `GetExperienceDetails` to avoid confusion
    with the existing `GetExperience` RPC)
- [ ] Run `npm run generate` to produce Go + Dart code
- [ ] Create SQL migration for `experience_needs` and `experience_contributions` tables
- [ ] Implement server handlers in `server/services/experience/`:
  - `needs.go` — AddExperienceNeed, RemoveExperienceNeed, ClaimExperienceNeed, UnclaimExperienceNeed
  - `contributions.go` — AddExperienceContribution, EditExperienceContribution, RemoveExperienceContribution
  - `needs_and_contributions.go` — ListExperienceNeedsAndContributions
    (batch-fetch needs + contributions + users via `services.FetchAPIUsersBatch`
    to avoid N+1 queries — this is a Phase 1 requirement, not polish)
- [ ] Add validation: participant-only access, proposer/contributor ownership
  checks, slot count bounds (max 20), terminal state guard
- [ ] Atomic slot operations: add a `DecrementField` or `ConditionalUpdate`
  helper to `storage.ProtoSQLStorage` for atomic `slots_remaining - 1
  WHERE slots_remaining > 0` semantics. Do NOT write raw SQL in the
  service layer — all database access goes through the storage abstraction.
- [ ] Unit tests for all new RPCs with `storage.SetupTestStorage(t)`
- [ ] Run `npm run build` to verify lint + compile

**Validates:** Data model supports all operations from the issue spec; slot
mechanics work correctly under concurrent claims.

### Phase 2: Chat system message integration

**Goal:** Every need/contribution/claim action posts a system message to the
experience conversation.

**Tasks:**
- [ ] Add new `ChatSystemAction` values to `proto/ripls/models/chat.proto`:
  - `CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_ADDED = 22`
  - `CHAT_SYSTEM_ACTION_EXPERIENCE_CONTRIBUTION_ADDED = 23`
  - `CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_CLAIMED = 24`
  - `CHAT_SYSTEM_ACTION_EXPERIENCE_CONTRIBUTION_REMOVED = 25`
  - `CHAT_SYSTEM_ACTION_EXPERIENCE_NEED_REMOVED = 26`
- [ ] Post system messages from each handler using existing
  `SystemMessageWriter` pattern (used by RSVP, time proposals, etc.)
- [ ] System message description format: include item name + optional note
  snippet (e.g., "Offered: 2 bottles red — A 2019 Tempranillo...")
- [ ] Unit tests verifying system messages are posted for each action
- [ ] Run `npm run build`

**Validates:** Chat shows a natural interleaved timeline of human messages
and coordination actions.

### Phase 3: Flutter client — repository + viewmodel layer

**Goal:** Client data layer ready for UI work.

**Tasks:**
- [ ] Run `npm run generate` to get Dart generated code
- [ ] Add methods to `ExperienceRepository` using existing `CacheService`
  with the `'experience'` namespace:
  - `listNeedsAndContributions(experienceId)` — fetch needs + contributions
  - `addNeed(experienceId, name, note, slots)`
  - `removeNeed(needId, experienceId)`
  - `claimNeed(needId, experienceId, note)`
  - `unclaimNeed(contributionId, experienceId)`
  - `addContribution(experienceId, title, description)`
  - `editContribution(contributionId, experienceId, title, description)`
  - `removeContribution(contributionId, experienceId)`
- [ ] Cache key: `needs_and_contributions:$experienceId` within the existing
  `'experience'` CacheService namespace. Invalidate on all mutation methods.
- [ ] All mutation methods must call
  `dailyCacheInvalidationProvider.notify()` after success so that the Daily
  screen and other cross-screen listeners pick up changes automatically
  (follows the event-driven cache invalidation pattern from `caching.md`).
- [ ] Add/extend experience viewmodel (Notifier with Freezed state) with
  needs + contributions state. ViewModels call repositories only — never
  services directly. Use `ref.mounted` checks after async gaps if using
  `autoDispose`. Viewmodels must NOT resolve localized strings — expose
  typed state (enums, data objects) and let widgets resolve via
  `context.l10n` in `build()`.
- [ ] Unit tests for repository (mock service with Mockito) and viewmodel
  (use `ProviderContainer`). Include disposal safety tests for async
  notifiers.

**Validates:** Data flows correctly from server through cache to viewmodel.

### Phase 4: Flutter client — Details tab UI

**Goal:** Ship the collaborative lists UI.

**Tasks:**
- [ ] Add all user-visible strings to ARB files (`app_en.arb` and `app_es.arb`)
  before building UI. Key prefix: `experienceNeeds*` and
  `experienceContributions*`. Use `context.l10n` for all text — no
  hardcoded strings in Dart code. Run `npm run generate` after adding ARB
  entries. Run `npm run lint:dart:i18n` to verify no violations.
- [ ] Add "Still needed" section to Details tab (`_buildNeedsSection`):
  - Use `AppColors` for all colors — the "coral accent" must be a theme-aware
    semantic color (e.g., `AppColors.warning(context)` or a new named helper
    in `AppColors` if needed). Never use hardcoded `Color(0xFFE07B54)`.
  - Slot count badge when slots > 1
  - Expandable cards showing note + "Suggested by [name]" + Remove link
  - "I got this" button on each card
- [ ] Add "Who's bringing what" section (`_buildContributionsSection`):
  - Grouped by person, current user's row floats to top with "You" badge
  - Chip pills: use `AppColors.surface(context)` for neutral chips and a
    theme-aware tint for need-linked chips. Never use hardcoded colors.
  - Dot indicator `•` for chips with notes
  - Tap own chip → edit sheet; tap other's chip → read-only sheet
- [ ] Add action buttons at bottom: `+ Offer something`, `+ Request something`
- [ ] Bottom sheets (use standard `showModalBottomSheet` pattern):
  - Request sheet: name input, optional note, slot stepper
  - Claim sheet: pre-filled name, note input
  - Offer sheet: title input, optional description
  - Edit chip sheet: title + note, Save + Delete/Return row
  - View chip sheet: read-only display
- [ ] Chat tab badge: increment unread count when system messages are posted
  while user is on a different tab
- [ ] Terminal state handling: hide action buttons and disable mutations when
  experience is completed/cancelled
- [ ] Keep `_buildNeedsSection` and `_buildContributionsSection` as private
  methods within the existing `experience_content_view.dart` to reduce
  nesting. If the file grows beyond ~1000 lines, extract the sections into
  separate widget files.
- [ ] Widget tests for key interactions

**Validates:** End-to-end user flow matches the reference mock behavior.

### Phase 5: Polish and edge cases

**Goal:** Handle remaining edge cases and UX refinements.

**Tasks:**
- [ ] Empty states: localized strings via `context.l10n` (e.g.,
  `experienceNeedsEmptyState`, `experienceContributionsEmptyState`)
- [ ] Metrics rollup: experience needs/contributions factor into experience
  impact metrics at completion. Community events should reflect individual
  requests and offers so users are recognized for their collaboration.
- [ ] Ensure experience-scoped needs do NOT appear in standalone request
  feeds, discover screen, or search results (verify with existing query
  paths — these are separate models so this should be automatic)
- [ ] End-to-end integration test covering full workflow (propose need →
  claim → offer free-form → complete experience → verify read-only)
- [ ] System message edit coalescing is handled separately in #1188

**Validates:** Production-ready quality with no regressions.

### Phase 6: Details tab layout rework — metrics, editable quality time modal, and request/offer polish

**Goal:** Clean up the Details tab layout: keep the top-line metrics visible,
move the editable social context fields into the quality time modal (which
already shows the calculation breakdown), and bring the request/offer sections
closer to the reference design at `docs/cowork/mocks/experience-help.png`.

#### Problem

The Details tab currently stacks three concerns vertically:

1. **Impact tiles** (money saved, CO₂, quality time) — small horizontal row
2. **QtAttributesCard** — a full card with five editable picker rows (tie
   strength, reciprocity, novelty, vulnerability, modality) that takes
   significant vertical space and feels disconnected from the metrics it
   influences
3. **Needs & contributions** — the collaborative lists added in Phase 4

Meanwhile, `ContentMetricSheet` (the bottom sheet opened by tapping the
quality time tile) already displays the exact same five dimensions with their
multiplier weights — but read-only. Users can see the calculation *or* edit
the inputs, but never both at once, making it hard to understand how changes
affect the score.

#### Design changes

**A. Remove QtAttributesCard from the Details tab.** The five editable fields
(tie strength, reciprocity, novelty, vulnerability, modality) should no longer
appear inline on the Details pane. This reclaims vertical space for the
collaborative lists, which are the primary content of the tab.

**B. Make the quality time modal (`ContentMetricSheet`) editable.** When
opened for the `qt` metric on an experience the owner can edit, each
multiplier row in the CALCULATION card should become a tappable picker row
(label + current value + chevron) instead of a static label + "× 1.0" string.
Tapping a row opens the same enum picker sheet used by `QtAttributesCard`.
When a value changes:
- Recalculate the quality time score client-side using the hardcoded weight
  functions already in `content_metric_sheet.dart`
- Update the displayed multiplier values and final score in real time
- On dismiss (or on each change), call `saveSocialContext()` to persist and
  trigger a server-side recalculation

Non-owners and non-QT metrics remain read-only. The existing `isCompleted`
flag gates editability for terminal experiences.

**C. Polish request/offer sections to match reference mock.** The reference
design (`docs/cowork/mocks/experience-help.png`) shows:
- A section header count: "Still needed  3 open" / "Who's bringing what
  7 contributing"
- Need cards with a warm-tinted left border accent, darker card backgrounds,
  and the "I GOT THIS" button right-aligned in a prominent green pill
- Expandable cards: the expand chevron `▾` sits inline with the need name;
  expanded state shows note + "Suggested by" with avatar
- Contributor rows with circular avatar initials (colored by person), "YOU"
  badge is a green pill next to the name, and contribution chips have a dot
  `•` indicator for notes
- Overall more generous spacing and bolder typography for section headers

#### Tasks

- [ ] **Remove QtAttributesCard from `_buildDetailsPane`** in
  `experience_content_view.dart`. The impact tiles remain; the attributes card
  does not. Verify that `request_content_view.dart` (standalone requests) is
  NOT affected — it should keep its own QtAttributesCard since requests don't
  have the quality time modal flow.
- [ ] **Convert `ContentMetricSheet` to `StatefulWidget`** (currently
  `StatelessWidget`). Add `isEditable` and `onSocialContextChanged` parameters.
  When `isEditable` is true and `metricId == 'qt'`, render the five dimension
  rows as tappable pickers instead of static text. Reuse the picker sheet
  pattern from `QtAttributesCard._showPickerSheet`.
- [ ] **Add client-side live recalculation.** When a dimension value changes in
  the modal, recompute `qualityTime = duration × modality × groupSize ×
  tieStrength × reciprocity × novelty × vulnerability` using the weight
  functions already defined in the file. Update the displayed multiplier
  values and the final "Quality Time" total immediately.
- [ ] **Persist on change.** After each picker selection, call the
  `onSocialContextChanged` callback. The caller (`experience_content_view.dart`)
  passes `saveSocialContext()` through this callback. The modal stays open so
  the user can adjust multiple dimensions in one session.
- [ ] **Add i18n strings** for any new labels (picker sheet titles, editable
  hints) to `app_en.arb` and `app_es.arb`. Run `npm run generate` and
  `npm run lint:dart:i18n`.
- [ ] **Polish need cards to match reference mock:**
  - Add warm-tinted left border accent to need cards (use
    `AppColors.transferCoral` with low opacity for the left `Border` side)
  - Section header count badge: "Still needed  {n} open" / "Who's bringing
    what  {n} contributing" — add count as a secondary `TextSpan` with muted
    color
  - "I GOT THIS" button: right-aligned, green pill (`AppColors.experienceSageGreen`),
    uppercase bold text, larger tap target matching the reference
  - Expand chevron `▾` inline with need name (not at card trailing edge)
- [ ] **Polish contribution rows to match reference mock:**
  - Add circular avatar initials (colored) to the left of each contributor
    name, using `ContentAvatar` or `UserAvatar`
  - "YOU" badge as a green pill (`AppColors.experienceSageGreen` background)
    next to the name label
  - Contribution chips: slightly larger with more padding, dot `•` visible
    when description is present
- [ ] **Pass `isEditable` through the metric sheet call chain.** In
  `experience_content_view.dart`, pass `isEditable: state.isOwner && !isTerminal`
  when opening `ContentMetricSheet.show()` for experiences. Existing callers
  (requests, gear) pass `isEditable: false` or omit it (defaults to false).
- [ ] **Unit tests:**
  - Test that `ContentMetricSheet` in editable mode calls
    `onSocialContextChanged` when a picker value changes
  - Test client-side QT recalculation produces expected values for known inputs
  - Test that non-owner / terminal state renders read-only rows
- [ ] **Widget tests** for the polished need cards and contribution rows

**Validates:** Quality time editing is discoverable and contextual — users
see the score update as they adjust each dimension. The Details tab is
cleaner with collaborative lists as the primary content. Request/offer
sections match the reference design.

### Phase 7: LLM-powered suggestion chips + post-completion Results tab

**Goal:** Two parts:

1. **Suggestions:** When the Plan tab is empty (no needs or contributions yet),
   show an interactive "starter menu" of category-specific suggestion chips that
   let users add requests or offers with a single tap. When the tab is populated,
   show a collapsible "Suggestions" row with remaining unused chips. The LLM
   generates suggestions tailored to the experience's name, description, and
   category at creation time.

2. **Results tab:** After an experience is completed, rename the second tab from
   "Plan" to "Results". The Results tab shows impact metrics (QT, money, CO2),
   an attendee tally (who attended), and a summary of what everyone brought.
   Impact tiles are *not* shown during the active phase — only after completion.

**Reference:** `docs/cowork/mocks/requests/suggestions.jsx` (React mock) and
`docs/cowork/mocks/requests/suggestions.png` (screenshot).

**Key decisions:**
- Suggestions are generated at experience creation time (same flow as other
  Gen RPCs), not on-demand when the Plan tab opens. This means no loading
  spinner on the Plan tab.
- Suggestions are category-specific — the LLM tailors them to the activity
  type (potluck → food items; hike → gear/snacks; game night → games/drinks).
- A generic fallback chip set is included client-side for when the AI provider
  is unavailable. These are broad enough to apply to many experience types
  (e.g. "Snacks", "Drinks", "Plates & cutlery", "Setup help", "Cleanup help",
  "Music", "Extra chairs", "Ice").
- The client-side toggle ("I'll bring" / "We need") determines whether tapping
  a chip creates an offer or a request. The LLM does not assign modes.

#### Server

- [ ] **Add `GenerateExperienceSuggestions` to `ai.Provider` interface**
  (`server/ai/provider.go`). Input: experience name, description, category.
  Output: `ExperienceSuggestionResult` — a flat list of 8–12 suggestion strings
  plus a `category_hint` string the LLM infers from context (e.g. "potluck",
  "hike", "game night") for the client to display contextual helper text.
- [ ] **Add prompt in `server/ai/prompts.go`**. The prompt should:
  - Receive experience name, description, and category
  - Generate 8–12 short suggestion labels (2–4 words each) appropriate for the
    activity type — category-specific, not generic
  - Use the shared `toneAndStyle` constant for consistency with other prompts
  - Mix both "things to bring" and "things we need" — the client decides which
    mode each chip lands in via a toggle
  - Return JSON: `{ "suggestions": ["..."], "category_hint": "potluck" }`
- [ ] **Implement in all three providers** (Gemini, OpenAI, Anthropic) following
  existing patterns (structured output, shared prompt, same schema). Use value
  types (not pointers) for the output struct to comply with OpenAI Strict mode
  requirements (see `docs/server/llm.md` Schema Definition Guidelines).
- [ ] **Call `GenerateExperienceSuggestions` during experience creation**. In the
  `CreateExperience` RPC handler, after inserting the experience, call the AI
  provider to generate suggestions and persist them via `s.storage.Update()`.
  This follows the eager generation pattern — suggestions are ready before anyone
  opens the Plan tab. If the AI provider is nil or the call fails, the experience
  is still created successfully (suggestions are optional; log the error at
  `Warn` level, do not return it to the caller).
- [ ] **Add `suggestions` and `category_hint` fields to the Experience model**
  (`proto/ripls/models/experience.proto`): `repeated string suggestions` and
  `optional string category_hint`. These are populated at creation time and
  returned in `GetExperience` responses.
- [ ] **Surface suggestions in the API** — add `repeated string suggestions`
  and `optional string category_hint` to the `Experience` API message
  (`proto/ripls/api/experience.proto`). The service copies them from the model
  when building the API response. **Do not reuse the model message directly** —
  API and model protos must remain independent per CLAUDE.md conventions.
- [ ] **Unit tests** with `MockProvider` — verify suggestions are generated
  and stored during creation, verify experience creation succeeds when AI
  provider is nil (graceful degradation), verify structured logging uses
  `logging.LoggerWithContext(ctx)` with `experience_id` field.

#### Client

- [ ] **Read suggestions from `ExperienceDetails` response** — no separate RPC
  needed since suggestions are stored on the experience model and returned
  with `GetExperience`. The client accesses them via `api.Experience` proto
  fields (never `models.Experience` — per CLAUDE.md "Flutter & Protos" rule).
- [ ] **Add suggestions state to `ExperienceNeedsState`**: `suggestions`
  (List<String>), `categoryHint` (String?). Populated from the experience
  details passed down from the parent view model. These are Freezed fields
  with `@Default` values per architecture.md Riverpod conventions.
- [ ] **Generic fallback chips** — define a static `_fallbackSuggestions` list
  in the needs section widget: `["Snacks", "Drinks", "Plates & cutlery",
  "Setup help", "Cleanup help", "Music", "Extra chairs", "Ice"]`. Used when
  the server returns no suggestions (AI provider unavailable at creation time).
  All chip labels must use i18n keys (not hardcoded English strings) per
  `docs/client/i18n.md` — define `experienceSuggestionsFallback*` keys.
- [ ] **Empty state widget** (`_EmptyStateSuggestions`): Shown when both needs
  and contributions lists are empty. Contains:
  - An "I'll bring" / "We need" toggle (segmented control). Toggle state is
    local widget state (`StatefulWidget` with `setState`), not viewmodel
    state — this is a UI-only concern per architecture.md rules.
  - Helper text that adapts to mode and `categoryHint`
    (e.g. "Tap something you'll bring to the potluck") — use `context.l10n`
    with ICU message format for the placeholder.
  - Suggestion chips in a `Wrap` — tapping a chip opens the corresponding
    sheet (AddContributionSheet or AddNeedSheet) with the name pre-filled.
    Use `AppColors` helpers for all chip colors — no hardcoded hex values
    per architecture.md theming rules.
  - Fallback text: "Or use the buttons below to add something custom"
- [ ] **Populated suggestions row** (`_SuggestionChipsRow`): Shown below the
  contributions section when the tab is populated and unused suggestions remain.
  Collapsible via a "Suggestions · N more" toggle. Contains:
  - Mini "I'll bring" / "We need" toggle
  - Remaining suggestion chips (excluding names already used in needs or
    contributions — case-insensitive comparison)
- [ ] **Pre-fill sheet flow**: When a suggestion chip is tapped, open the
  relevant sheet with `initialName` set to the chip label. The sheet's name
  field is pre-filled and focus moves to the note/details field.
  `AddNeedSheet` and `AddContributionSheet` already accept content via their
  constructors; add an `initialName` parameter if not present.
- [ ] **Post-completion "Results" tab**: When the experience state is
  `EXPERIENCE_STATE_COMPLETED`, change the second tab label from "Plan" to
  "Results" (new i18n key `experienceTabResults`). The Results tab content
  replaces the Plan tab content and shows:
  - Impact tiles (QT, money saved, CO2 avoided) — reuse `ContentImpactTiles`
    and `ContentMetricSheet` (read-only, `isEditable: false`)
  - Attendee tally — who marked as attended (from RSVPs with `attended: YES`)
  - Contributions summary — the "Who's bringing what" section rendered
    read-only (no action buttons, no claim/unclaim, no suggestions)
  - All data comes from the existing `ExperienceState` and
    `ExperienceNeedsState` — no new repository calls needed.
- [ ] **i18n strings** (add to both `app_en.arb` and `app_es.arb`, run
  `npm run lint:dart:i18n` to verify no hardcoded strings remain):
  - `experienceTabResults` ("Results")
  - `experienceSuggestionsIllBring` / `experienceSuggestionsWeNeed` (toggle)
  - `experienceSuggestionsOfferHelper` / `experienceSuggestionsRequestHelper`
    (with `{categoryHint}` placeholder, ICU format)
  - `experienceSuggestionsToggle` ("Suggestions")
  - `experienceSuggestionsMore` ("{count} more", with `{count}` placeholder)
  - `experienceSuggestionsFallback` ("Or use the buttons below...")
  - Fallback chip labels: `experienceSuggestionChipSnacks`, etc.
- [ ] **File size check**: If `experience_needs_section.dart` exceeds ~1000
  lines after adding empty state and suggestion widgets, extract suggestion
  widgets into a new `experience_suggestions.dart` file.

**Validates:** The Plan tab is immediately actionable — new users see
category-specific suggestions instead of a blank page. One-tap flow
reduces friction from "open sheet → think of item → type name" to just
"tap chip → optionally add details → submit." After completion, the tab
transforms into a meaningful retrospective showing impact and contributions.

## Related Issues

- #1188 — Coalesce rapid system messages in chat (spun out from this issue's edit coalescing requirement; applies to RSVPs too)
- #967 — Experiences beta catchall (may have related polish items)
- #1160 — Experience RSVP bugs (recently fixed; similar experience UI work)
- #965 — Request beta polish (existing request system improvements)

---

## Audit Log

**Audited:** 2026-04-18 against `docs/client/architecture.md`,
`docs/client/caching.md`, `docs/client/i18n.md`, `docs/server/architecture.md`.

**Changes made:**

1. **Raw SQL removed from service layer** (server/architecture.md violation) —
   Phase 1 originally proposed raw SQL for atomic slot decrements. Changed to
   require a new `storage.ProtoSQLStorage` helper method. Services must not
   contain raw SQL.

2. **Dedicated request/response types per RPC** (CLAUDE.md proto convention) —
   Added explicit note that each RPC needs its own message types with inline
   fields. This was implicit but needed to be called out.

3. **`optional` keyword for proto fields** (CLAUDE.md proto field conventions) —
   Added explicit `optional` annotations for all conditionally-present fields
   in `ExperienceNeed` and `ExperienceContribution` models.

4. **RPC renamed: `GetExperienceDetails` → `ListExperienceNeedsAndContributions`**
   — Avoids confusion with the existing `GetExperience` RPC.

5. **Batch user fetches moved from Phase 5 to Phase 1** (CLAUDE.md SQL
   efficiency: "Never query in a loop") — N+1 prevention is a correctness
   requirement, not polish. Added explicit `services.FetchAPIUsersBatch` call.

6. **i18n compliance added to Phase 4** (i18n.md) — All user-visible strings
   must go through ARB files with `context.l10n`. Added task for
   `experienceNeeds*` and `experienceContributions*` key prefixes in both
   `app_en.arb` and `app_es.arb`, plus `npm run lint:dart:i18n` verification.

7. **Theme-aware colors enforced** (architecture.md colors section) — Replaced
   references to "coral-accented" with `AppColors` helpers. Hardcoded hex
   colors are prohibited; all UI colors must be theme-aware.

8. **Cache invalidation pattern** (caching.md Pattern 7) — Added
   `dailyCacheInvalidationProvider.notify()` requirement to repository
   mutation methods for cross-screen reactivity.

9. **ViewModel conventions** (architecture.md) — Added Freezed state
   requirement, `ref.mounted` checks, no-string-resolution-in-viewmodels
   rule, and disposal safety test requirement.

10. **File size guard** (architecture.md / server architecture.md) — Added
    note to extract sections into separate widget files if
    `experience_content_view.dart` exceeds ~1000 lines.

**Phase 7 audit (2026-04-18):**

11. **OpenAI Strict mode compliance** (llm.md) — Added note that the
    `ExperienceSuggestionResult` output struct must use value types (not
    pointers) and no `omitempty` tags to comply with OpenAI Structured
    Outputs strict schema requirements.

12. **Shared prompt constant** (llm.md) — Added requirement to use the
    `toneAndStyle` constant in the suggestions prompt, consistent with all
    other content generation prompts.

13. **API/model proto separation** (CLAUDE.md) — Added explicit note that
    `suggestions` and `category_hint` fields must be defined independently
    in both `proto/ripls/api/experience.proto` and
    `proto/ripls/models/experience.proto`. The service handles conversion.

14. **Client proto access rule** (CLAUDE.md "Flutter & Protos") — Added
    note that the client reads suggestions from `api.Experience` fields,
    never from `models.Experience`. The client should never reference
    storage/model protos.

15. **Fallback chip labels must be i18n** (i18n.md) — The original plan
    hardcoded English strings for generic fallback suggestions. Changed to
    require `experienceSuggestionChip*` i18n keys in both ARB files.

16. **Toggle state is local, not viewmodel** (architecture.md) — The
    "I'll bring" / "We need" toggle is a UI-only concern. Clarified that
    it should be `StatefulWidget` local state, not Freezed viewmodel state,
    per the rule that viewmodels only hold business state.

17. **Suggestion generation failure handling** (CLAUDE.md error handling /
    server architecture.md) — Clarified that suggestion generation failures
    during experience creation should log at `Warn` level and not propagate
    to the caller. The experience creation response is not degraded —
    suggestions are purely additive.

18. **Freezed defaults for new state fields** (architecture.md) — Added
    note that `suggestions` and `categoryHint` in `ExperienceNeedsState`
    must use `@Default([])` and `@Default(null)` respectively, consistent
    with existing Freezed state patterns.

19. **File size guard for suggestions widgets** (architecture.md) — Added
    task to extract suggestion widgets into `experience_suggestions.dart`
    if `experience_needs_section.dart` exceeds ~1000 lines after Phase 7.
