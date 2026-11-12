# Issue #2279: Simplify location & time poll modals (TBD-first, single morphing sheet)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2279
**Filed:** 2026-06-03 | **Last activity:** 2026-06-03
**Labels:** enhancement | **Assignees:** unassigned
**Context docs:** [`docs/client/polls.md`](../client/polls.md), [`docs/cowork/App Design/location-modal-tbd-prototype.html`](../cowork/App%20Design/location-modal-tbd-prototype.html), [`docs/cowork/App Design/time-modal-tbd-prototype-v3.html`](../cowork/App%20Design/time-modal-tbd-prototype-v3.html)

## Problem Statement

The location and time **poll** flows today are each five separate glass-modal
surfaces — Propose / Vote / Manage / Confirm / Finalized — dispatched
state-aware from the Event-tab chip ([`docs/client/polls.md`](../client/polls.md)).
The new prototypes collapse this into **one sheet that morphs across three
user-visible statuses** (`tbd` → `poll` → `set`), making "keep it TBD," "pin
one option," and "float a few" all one-tap paths from a single default surface.

The rework is explicitly framed as **modification + reuse, not a rewrite**:
keep the existing modal bodies, view-models, and service methods, restyle them
to the prototypes, and pull the now-identical chrome into shared
`widgets/poll/` components. Parity between location and time is the design
contract — any divergence not in the "Intentional differences" list is a bug.

Three things in the prototypes are genuinely **new capability**, not restyles,
and the bulk of the risk lives there:

1. **Flexible votes** ("anywhere/anytime works") that fold positively into
   *every* option's tally. There is no proto representation for this today.
2. **Friend-suggested options surfaced with attribution** before the host has
   set anything ("Maya suggested" badge). The data exists (`proposedBy`) but
   isn't surfaced this way.
3. **Reply progress + social proof** ("N of M replied · K flexible", avatar
   stacks, "X, Y chimed in") and an inline **soft-deadline** toggle.

The remaining items (TBD-first default, progressive 0/1/2+ CTAs, the "Set the
final" confirm sheet, the finalized card) are largely restyles + rewiring of
what already exists.

## Context from Comments

**The issue has no comments** — it was filed and immediately handed to this
planning pass. That means the requirements are the issue body + the two
prototypes only; there is no maintainer clarification on the open questions
below. The prototypes are detailed and self-consistent, so most decisions can
be made from them, but the **flexible-vote wire model**, the **timezone-aware
rendering scope**, and **where the organizer "Manage" actions live** are not
settled by the issue and need a decision (see Open Questions).

## Codebase Findings (research summary)

**Modal surfaces** (`app/lib/presentation/screens/experience/`): twelve files,
six per kind. Largest are
[`location_poll_vote_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_vote_modal.dart)
(~915 lines — already near the 1,000-line Dart gate),
[`location_poll_finalized_modal.dart`](../../app/lib/presentation/screens/experience/location_poll_finalized_modal.dart)
(~797), `*_confirm_modal.dart` (~680/590), `*_propose_modal.dart` (+ `_atoms`).

**View-models** (`app/lib/presentation/viewmodels/`):
`location_modal_view_model.dart` (~680) + `location_modal_state.dart` (~61);
`time_modal_view_model.dart` (~920) + `time_modal_state.dart` (~95). Location
tracks votes as `Set<String>` of YES proposal ids; time tracks
`Map<String, TimeVoteStatus>`. Status is represented by server flags
(`*PollActive`, `*PollCompleted`, `currentPollId`) plus a `mode` string
('default'/'confirmed' for location; richer for time).

**Service layer** (`app/lib/services/experience/`): all nine parallel RPCs per
kind already exist (`propose / vote / confirm / unlock / cancel / delete /
setDeadline / lockProposals / nudgeVoters`) plus `extract*Candidates` for
bulk-paste.

**Shared widgets** (`app/lib/presentation/widgets/poll/`): only
[`poll_manage_menu_sheet.dart`](../../app/lib/presentation/widgets/poll/poll_manage_menu_sheet.dart)
(`PollManageMenuSheet<T>` + `PollManageMenuItem<T>`) is shared today. The
per-kind `*_poll_manage_menu.dart` wrappers (~69 lines each) define parallel
action enums.

**Reusable pickers**:
[`LocationPickerModal.show(...) → Future<String?>`](../../app/lib/presentation/widgets/location/location_picker_modal.dart)
and
[`DateTimePickerModal.show(...) → Future<DateTimePickerResult?>`](../../app/lib/presentation/widgets/modal/glass/date_time_picker_modal.dart).
Both already match the prototype pickers — reuse as-is.

**Routing**: chip dispatch in
[`experience_content_view.dart`](../../app/lib/presentation/screens/experience/experience_content_view.dart)
(`_showLocationPickerModal`, `_showTimeModal`, `_showTimeModalNonOwner`) and
chip labels in
[`experience_event_pane.dart`](../../app/lib/presentation/screens/experience/widgets/experience_event_pane.dart).

**Proto / vote semantics** (verified in `proto/ripls/api/`):
- `LocationVoteStatus` / `TimeVoteStatus` = `{UNSPECIFIED, YES, NO, NONE_WORK}`.
  **No "flexible / all-works" value exists** — `NONE_WORK` is the negative
  ("none of these"), the opposite of flexible.
- `SpecificTime` already carries `timezone` (IANA), `duration_minutes`,
  `is_all_day` — the data for cross-timezone rendering exists.
- `ExperienceTime.time_type` already includes a `TimeTBD` variant; **location
  has no proto TBD concept** (TBD = empty `location_id` + no active poll).
- `LocationProposal` / `TimeProposal` carry `proposedBy: User` and `pollId`, so
  friend-suggestion attribution is available.

**Tests**: there are currently **no** dedicated tests for these view-models or
modal screens under `app/test/`. The rework should add coverage.

**Theme**: `AppColors.lightAccent` (`0xFF7BA088`) is the canonical sage accent;
`AppTheme.headingFont` is the serif used for titles. (Per memory: reuse these
tokens — do not invent per-section coral/cream/font constants.)

## Potential Solutions

### Option A: Restyle the existing five surfaces in place; keep separate-modal dispatch
- **Approach:** Re-skin each of the existing per-kind modals to the prototype
  visuals and add the new sub-features, but keep the dispatcher pushing a
  distinct modal per state (today's behavior).
- **Pros:** Lowest structural churn; smallest diff per file; easy to review.
- **Cons:** Does **not** deliver the "one sheet that morphs" UX — each
  transition is still a separate sheet push/pop. The TBD-first default and the
  0/1/2+ progressive CTAs don't fit cleanly into the propose-only modal.
  Perpetuates duplication the prototypes are explicitly trying to remove.
- **Effort:** Medium.

### Option B: Full rewrite into one new morphing widget per kind
- **Approach:** Write `LocationPollSheet` / `TimePollSheet` from scratch as a
  single morphing `ConsumerWidget`, deleting the five old files per kind.
- **Pros:** Cleanest end-state; truest to the prototype.
- **Cons:** Throws away working vote/confirm/finalized logic and the only
  test-validated behavior; high regression risk on optimistic voting,
  geocoded-vs-saved materialization, and poll-id scoping; contradicts the
  issue's "reuse, not rewrite" instruction.
- **Effort:** Large.

### Option C (recommended): One morphing container per kind that composes extracted bodies
- **Approach:** Introduce a single entry sheet per kind
  (`LocationPollSheet` / `TimePollSheet`) that reads the view-model status and
  renders a status-specific **body** (`_TbdBody` / `_PollBody` / `_SetBody`).
  Each body is largely the *existing* propose/vote/finalized content, lifted
  out of its old modal file and restyled. The "Set the final" confirm and the
  pickers stay as nested sub-sheets launched from within. Shared chrome
  (progress, social proof, TBD card, flexible row, leading badge, confirm
  sheet) moves to `widgets/poll/`.
- **Pros:** Delivers the morphing UX; maximizes reuse of vetted view-model /
  service logic; naturally fixes the 915-line `vote_modal` size problem by
  splitting bodies + shared widgets; keeps location/time parity enforceable.
- **Cons:** More moving parts than A; requires careful state mapping from
  today's flag soup (`*PollActive` / `*PollCompleted` / `mode`) onto the clean
  `tbd | poll | set` status.
- **Effort:** Large (but incremental — see phases).

## Recommendation

**Option C.** It is the only option that delivers the prototype's core UX (one
morphing sheet) while honoring the issue's "modify and reuse" constraint. The
view-models and RPCs stay; we re-compose the presentation and extract shared
chrome. Sequence the work so each phase ships independently and the riskiest
new capability (flexible votes) is isolated behind a clear wire decision.

The one decision Option C forces up front is **how `tbd | poll | set` maps onto
existing state**. Recommended mapping:
- `set` ⇐ confirmed value with no active poll (`locationId`/`time` set,
  `*PollActive == false`).
- `poll` ⇐ `*PollActive == true`.
- `tbd` ⇐ everything else (no confirmed value, no active poll), with the 0/1/2+
  CTA branch driven by `currentPollProposals.length`.

Add a single computed `pollStatus` getter to each `*ModalData` so the sheet
switches on one value instead of re-deriving from flags.

## Conventions & Compliance

The following are CI-enforced in this codebase. They are not optional polish —
several gate the build. Every phase below folds them in; collected here so the
constraints are explicit.

- **i18n — every user-visible string lives in ARB.** The prototypes are dense
  with copy ("Keep it TBD for now", "Ask the group", "Set the final spot",
  "Leading", "incl. N flexible", "N of M replied", "Send N picks"). All of it
  goes in `app/lib/l10n/app_en.arb` **and** `app_es.arb` (parity gate
  `npm run lint:dart:l10n-parity`) under the `experience*` prefix; a11y labels
  use the `a11y` prefix. Counts use **ICU plural** (`{count, plural, ...}`) —
  "N possible spots/times", "N flexible", "N picks" (polls.md already uses ICU
  plurals for the chip). `npm run lint:dart:i18n` rejects raw string literals.
  **ViewModels never resolve strings** — they expose typed state (enums,
  counts, `UserError?`); widgets call `context.l10n.*` in `build()`.
- **"Event", not "Experience", in user-facing copy.** Per CLAUDE.md, no ARB
  value or a11y label may contain the word "experience" — the user-facing term
  is **Event**. Code identifiers stay `Experience`. The poll copy mostly says
  "spot"/"time"/"the group", but audit every new string.
- **Theme-aware colors — do not paste the prototype hex.** The prototypes
  hardcode `#8FAE7E`, `#0f1210`, etc. Map these to existing tokens
  (`AppColors.lightAccent`, the modal/glass surface + text helpers,
  `AppTheme.headingFont`) — do **not** introduce new `Color(0xFF…)` literals or
  per-section constants (default to reusing existing theme tokens). Architecture doc
  requires `AppColors.xxx(context)` helpers so light/dark both work; the glass
  modals are dark-styled, so reuse the tokens the existing poll modals already
  established rather than inventing new ones.
- **Accessibility primitives are mandatory** (`scripts/check_a11y.js`, empty
  allowlist = every new violation must be fixed in-PR):
  - Vote rows / flexible row are **selectable toggles** → use `Toggle` with
    `selected:` (not `GestureDetector`/`InkWell`, not state baked into the
    label). Non-toggle taps → `Tappable` with a required `semanticsLabel`.
  - Icon-only buttons → `IconAction` (label + tooltip). Sub-sheets (picker,
    confirm) open via `showAccessibleModal`, titles via `ModalHeader`.
  - `PollLeadingBadge` and the reply-progress bar are status indicators — a
    class named `*Badge`/`*Indicator`/`*Dot` **must** contain `Semantics(`/
    `Tooltip(` or `avoid_color_only_status` fails CI. The progress bar needs a
    non-color cue (the "N of M replied" text) and a `Semantics` value.
  - State-change announcements ("YOU'RE IN", "Sent") → `LiveRegion` /
    `SemanticAnnouncer`; sheet animations → `accessibleDuration(context, …)`.
- **State & async safety.** Keep poll/vote state in Freezed `*ModalData`
  (`copyWith`), not manual `Map`/`Set` caches in the notifier beyond what the
  proto drives. The vote toggle is the canonical **optimistic update** (apply,
  then reconcile/rollback on RPC result). Notifiers are async + screen-scoped:
  guard every post-`await` `state =` with `ref.mounted` (or `SafeNotifierMixin`)
  and add a **disposal-safety test** per the architecture doc.
- **Caching.** Data access flows through `ExperienceRepository`; after every
  mutation (propose / vote / flexible-vote / confirm / lock / deadline /
  cancel) the repository invalidates/refreshes the experience cache — no manual
  cache maps in the ViewModel, no direct service calls bypassing the repo.
- **Testing is ViewModel-first.** ViewModel tests (`ProviderContainer` +
  Mockito-mocked repositories) are the primary signal and own the state-machine
  correctness (status transitions, 0/1/2+ branching, flexible-vote folding,
  optimistic rollback). Widget tests are secondary and need l10n delegates
  (`app/test/helpers/l10n_helpers.dart`). Generate mocks with `@GenerateMocks`
  + `build_runner`.

## Resolved Decisions

These were the open design questions; all are now **decided** (recommendations
accepted) and the Implementation Plan below reflects them. Kept here as the
rationale record.

1. **Flexible vote is a new wire value.** Add `*_VOTE_STATUS_FLEXIBLE = 4` to
   both `LocationVoteStatus` and `TimeVoteStatus` (proto3 enum add is
   wire-compatible). The server folds `FLEXIBLE` voters into every option's
   effective tally and the leader calc; the client renders the dashed
   "flexible" avatar and "incl. N flexible" foot. **Rejected:** a purely
   client-side model (auto-YES on all current options) — it breaks the moment a
   new option is added after the flexible vote and can't distinguish "explicitly
   flexible" from "voted for everything." This is the one piece that needs
   server work → Phase 1.
2. **Ship cross-timezone rendering for the time flow.** `SpecificTime` already
   carries the IANA `timezone`; render the viewer's local tz as primary and the
   author tz as the secondary line ("Maya sees this as 4:00 PM PT"). Location
   has no tz dimension — that asymmetry is a documented Intentional Difference,
   not drift. Handled in Phase 4.
3. **Secondary organizer actions stay in `PollManageMenuSheet<T>`.** Keep
   Edit Choices / Lock-Unlock / Nudge / Cancel in the shared Manage sheet,
   surfaced from an overflow/"Manage" affordance on the `poll`-status body
   (owner only). Only *Set the final* and *Change* fold inline (as the
   prototypes draw them). Rationale: those four are low-frequency and would
   clutter the morphing sheet; keeping them shared preserves location/time
   lockstep.
4. **TBD is the absence of a confirmed value, not new persistence.** Treat TBD
   identically on both sides; reuse the existing `TimeTBD` variant only where
   the time flow already depends on it. "Keep it TBD for now" is a no-op that
   dismisses the sheet without creating a poll or value.
5. **Friend-suggest-before-host creates the poll implicitly.** A non-owner
   suggestion calls `Propose*`, which already creates/joins the active poll
   server-side; surface it to the host on the `tbd` body with attribution, with
   "float to the group" / "set one" actions. Phase 1 confirms the server
   permits non-owner `Propose*` with no existing poll and opens that gate if
   not.
6. **Reply-progress / social-proof data: spike first, defer if expensive.**
   The roster (Yes/Maybe RSVP set the `Nudge*` RPC already computes) + who-voted
   + total-invited likely need new **dedicated** fields on the experience/poll
   fetch response (no request/response-type reuse). Phase 1 spikes this; if the
   roster isn't cheaply available, **reply-progress ships as a fast-follow** and
   the modals land without it rather than blocking the restyle.
7. **Route the new vote path through `ExperienceRepository`.** New flexible-vote
   access goes ViewModel → Repository → Service with cache invalidation; nudge
   existing poll calls onto the repository where it's low-cost, but this issue
   does **not** become a full service-access refactor — remaining direct-service
   calls are flagged as tech debt, not fixed here.

## Implementation Plan

### Phase 1: Wire support for flexible votes + reply-progress data (server + proto)
**Goal:** Establish the new persisted semantics and confirm the data the UI
needs exists before any client work depends on it.
**Tasks:**
- [ ] **Spike:** confirm what reply-progress needs (invitee roster, who-voted,
      total-invited) and whether it's already on the experience/poll fetch
      response. If not, add dedicated response fields (own message types per
      RPC — no request/response reuse). Use `optional` for conditionally-present
      fields.
- [ ] Add `*_VOTE_STATUS_FLEXIBLE = 4` to `LocationVoteStatus` and
      `TimeVoteStatus` in `proto/ripls/api/{location,time}.proto` with a
      meaning-only comment (no implementation/vendor detail); run
      `npm run generate`.
- [ ] Update server vote handlers + leader/winner calc to fold `FLEXIBLE`
      voters into every option's effective tally (mirror the prototype's
      `effVoters` math). Keep handler files < 1000 lines; split if needed.
- [ ] Confirm/adjust the server gate so a non-owner can `Propose*` when no poll
      exists yet (friend-suggestion-before-host path).
- [ ] Use `logging.LoggerWithContext(ctx)` with standard fields (`operation`,
      `experience_id`, `user_id`) and `connecterr.Internal(ctx, op, err, …)`
      for internal errors — never bare `connect.NewError(CodeInternal, …)`.
- [ ] Unit tests (preferred) for flexible-vote tallying + non-owner propose;
      integration test for the full vote→confirm flow. Classify any new
      `CommunityEventType` in `server/undo/registry.go` (`TestEveryEventTypeClassified`
      gates the build).
- [ ] `npm run build`, and `go build -tags=integration ./...` since these
      signatures are consumed by experience integration tests.
**Validates:** The wire model for flexibility (and the reply-progress data
shape) holds end-to-end before the client renders it; avoids building UI on a
representation we'd have to redo.

### Phase 2: Extract shared poll chrome into `widgets/poll/`
**Goal:** Create the kind-agnostic components the prototypes make identical.
**Tasks:**
- [ ] `PollReplyProgress` (progress bar + replied/flexible counts + avatar
      stack), `PollSocialProofRow`, `PollTbdCard`, `PollFlexibleVoteRow`
      (a `Toggle`), `PollLeadingBadge`, `PollSuggestedByBadge`, inline
      `PollSoftDeadline` toggle — colors via `AppColors` helpers /
      `AppTheme.headingFont`, no pasted prototype hex.
- [ ] **a11y per component:** `*Badge`/progress widgets carry `Semantics(`/
      `Tooltip(` (else `avoid_color_only_status` fails); progress bar exposes a
      `Semantics` value + text cue; toggles use `selected:`; sub-sheet titles
      use `ModalHeader`.
- [ ] **i18n:** add every label to `app_en.arb` + `app_es.arb` (`experience*` /
      `a11y*` prefixes), ICU plurals for counts; widgets resolve via
      `context.l10n`. Confirm no string contains "experience" (use "Event").
- [ ] Generic `PollConfirmSheet<T>` ("Set the final …": sage final card +
      radio list sorted by tally + "Lock in & notify group"), parameterized on
      a label/format callback so location and time share one widget; presented
      via `showAccessibleModal`.
- [ ] Widget tests (with l10n delegates) for each shared component.
**Validates:** Parity-by-construction — both flows consume one implementation,
so the new chrome can't drift. Supersedes the old "extractions skipped"
parity item in `polls.md`.

### Phase 3: Location morphing sheet
**Goal:** Ship `LocationPollSheet` with `tbd | poll | set` bodies, reusing
existing view-model logic.
**Tasks:**
- [ ] Add `pollStatus` getter to `LocationModalData` (mapping above).
- [ ] Build `LocationPollSheet` composing `_TbdBody` (0/1/2+ CTAs + TBD card),
      `_PollBody` (vote rows + leading badge + progress + flexible row + "Set
      the final" launch), `_SetBody` (finalized card + map + Get directions +
      owner Change). Lift content from the existing propose/vote/finalized
      modals; launch `LocationPickerModal` and `PollConfirmSheet` as sub-sheets.
- [ ] Add flexible-vote casting + friend-suggestion attribution to the
      view-model as an **optimistic update** routed through
      `ExperienceRepository` (cache invalidation on success); guard post-`await`
      `state =` with `ref.mounted`.
- [ ] Re-point `experience_content_view.dart` location dispatch + the chip in
      `experience_event_pane.dart` at the single sheet; sub-sheets via
      `showAccessibleModal`.
- [ ] Delete/retire the superseded per-kind location modal files; keep
      `LocationPollManageAction` + `PollManageMenuSheet`. Confirm
      `npm run lint:dart:size` passes (the old 915-line vote modal should be
      gone, not relocated).
- [ ] **ViewModel tests (primary):** status transitions, 0/1/2+ branching,
      flexible-vote folding, optimistic vote + rollback on RPC error, and a
      **disposal-safety test**. Widget smoke test with l10n delegates.
- [ ] `npm run lint:dart:i18n` / `:a11y` / `:l10n-parity` / `:size` clean.
**Validates:** The full morphing UX works on one kind, including flexible votes
from Phase 1, before duplicating to time.

### Phase 4: Time morphing sheet (parity)
**Goal:** Mirror Phase 3 for time, plus the time-only timezone/duration bits.
**Tasks:**
- [ ] Add `pollStatus` getter to `TimeModalData`; build `TimePollSheet` with
      the same three bodies, reusing `DateTimePickerModal` and the shared
      chrome.
- [ ] Render timezone-aware times (viewer-local primary, author tz secondary)
      using `SpecificTime.timezone`; duration chips; "Add to calendar" on the
      finalized card (no map).
- [ ] Re-point time dispatch (owner + non-owner) and the time chip; sub-sheets
      via `showAccessibleModal`.
- [ ] Retire superseded per-kind time modal files; keep `TimePollManageAction`.
- [ ] Reuse the Phase-2 shared chrome + Phase-1 flexible-vote path; route vote
      through `ExperienceRepository`, `ref.mounted` guards, optimistic + rollback.
- [ ] **ViewModel tests (primary)** mirroring Phase 3 + disposal-safety test;
      add timezone-rendering tests (viewer-local vs author tz). Widget smoke
      test with l10n delegates. `lint:dart:i18n/:a11y/:l10n-parity/:size` clean.
**Validates:** Both flows match their prototypes and each other; the only
differences are the documented intentional ones.

### Phase 5: Docs + cleanup
**Goal:** Make `polls.md` describe the shipped system and close parity debt.
**Tasks:**
- [ ] Rewrite [`docs/client/polls.md`](../client/polls.md): replace the
      five-surface Surfaces table with the morphing single-sheet model
      (tbd/poll/set); document TBD-first default, flexible votes, reply
      progress / social proof, friend-suggestion attribution, inline soft
      deadline; update Routing and the Intentional differences / shared-widgets
      sections (the new shared chrome supersedes closed-parity item #2).
- [ ] Verify no Dart file exceeds the 1,000-line gate
      (`npm run lint:dart:size`); confirm `npm run test:app` passes.
- [ ] Update this plan with decisions made during implementation.
**Validates:** Documentation matches reality and the parity contract is
re-stated for the new structure.

## Related Issues
- #2131 — [`docs/issues/2131-time-poll-migration.md`](2131-time-poll-migration.md):
  the migration that produced the current parallel five-surface structure; this
  issue evolves that structure into the morphing sheet and should keep its
  parity contract intact.
- #1012 — `manage-button-social-proof`: prior work on the Manage button /
  social proof; relevant to where the social-proof row and Manage affordance
  land here.

---

## Audit notes (`/ripls-audit`, 2026-06-03)

Reviewed against `docs/client/{architecture,caching,i18n,accessibility,testing}.md`
and `docs/server/{architecture,observability}.md`. Changes made to bring the
plan into compliance:

- **Added a "Conventions & Compliance" section** capturing CI-gated rules the
  original draft omitted: i18n (ARB + ICU plurals + `es` parity, ViewModels
  never resolve strings), the "Event" not "Experience" copy rule, theme-aware
  colors (map prototype hex to `AppColors`, don't paste), accessibility
  primitives (`Toggle`/`Tappable`/`IconAction`/`showAccessibleModal`/`ModalHeader`
  + the `avoid_color_only_status` trap on `PollLeadingBadge`/progress),
  async-safety + disposal tests, repository-routed cache invalidation, and
  ViewModel-first testing.
- **Added two open questions:** reply-progress data availability (the roster /
  who-voted / total-invited the social-proof UI needs may not be on the fetch
  response today) and the repository-vs-service access path.
- **Hardened Phase 1 (server):** structured logging with standard fields,
  `connecterr.Internal`, unit-preferred + integration tests, `undo/registry.go`
  classification, `optional` proto fields, no request/response-type reuse,
  meaning-only proto comments, and the `-tags=integration` build.
- **Wove i18n / a11y / lint-gate / ViewModel-test tasks into Phases 2–4**, and
  noted the `lint:dart:size` win from retiring the 915-line vote modal.

No architectural contradictions found — Option C (morphing container reusing
existing MVVM view-models + Freezed state + repository data access) aligns with
the documented patterns. The main risk the audit surfaces is scope: the
reply-progress data may require server fetch-response additions, so it's gated
behind the Phase 1 spike and is the most reasonable candidate to defer if the
roster isn't cheaply available.
