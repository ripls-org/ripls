# Issue #1324: Refactor Dart files longer than 1000 lines to be shorter

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1324
**Filed:** 2026-04-25 | **Last activity:** 2026-04-25
**Labels:** none | **Assignees:** unassigned
**Context docs:** none (pulled in #1329, #1358, #1293 directly)
**Branch:** `shorten_long_files` (already checked out)

## Problem Statement

The repository's CLAUDE.md states that Dart files should preferably stay under
1,000 lines, but presentation-layer files have crept well past that ceiling:

| File | Lines |
|---|---|
| `app/lib/presentation/viewmodels/conversation_view_model.dart` | 3,092 |
| `app/lib/presentation/screens/gear/gear_content_view.dart` | 2,465 |
| `app/lib/presentation/screens/experience/experience_content_view.dart` | 2,100 |
| `app/lib/presentation/widgets/experience/experience_needs_sheets.dart` | 2,032 |
| `app/lib/presentation/screens/impact_metrics/community_metrics_screen.dart` | 1,642 |
| `app/lib/presentation/screens/request/request_content_view.dart` | 1,343 |
| `app/lib/presentation/widgets/gear/gear_metadata_sheet.dart` | 1,342 |
| `app/lib/presentation/widgets/media/media_carousel.dart` | 1,325 |
| `app/lib/presentation/viewmodels/gear_sharing_view_model.dart` | 1,274 |
| `app/lib/presentation/widgets/shared/needs_section.dart` | 1,261 |
| `app/lib/presentation/screens/experience/experience_preview_modal.dart` | 1,245 |
| `app/lib/presentation/widgets/chat/message_list.dart` | 1,191 |
| `app/lib/services/providers.dart` | 1,150 |
| `app/lib/presentation/viewmodels/experience_view_model.dart` | 1,137 |
| `app/lib/core/utils/drill_down_builder.dart` | 1,078 |
| `app/lib/presentation/viewmodels/request_view_model.dart` | 1,041 |
| `app/lib/presentation/screens/impact_metrics/metric_detail_screen.dart` | 1,036 |
| `app/lib/presentation/viewmodels/time_modal_view_model.dart` | 1,012 |
| `app/lib/services/experience_service.dart` | 1,007 |

(l10n files excluded — they're generated from `.arb` by `gen_l10n`.)

19 non-generated Dart files exceed 1,000 lines; four exceed 2,000. Reviews,
merge conflicts, test isolation, and incremental compilation all degrade as
files grow. Issue #1293 proposed a CI gate failing on files >1,500 lines and
was closed as completed, but **the gate is not actually wired into any
workflow** (`.github/workflows/test_flutter.yaml` has no line-count step) —
which is why the files have continued to grow.

## Context from Comments

Issue 1324 itself only contains a one-liner ("@claude please fix") referencing
the underlying review (#1293). The substantive context is in the deduped
issues:

- **#1329 (architecture review)** — Targets `conversation_view_model.dart`.
  Notes growth 1,263 → 2,624 → 3,092 over three review cycles "despite
  explicit split recommendations." Recommends:
  `conversation_messages_notifier.dart`, `conversation_input_notifier.dart`,
  `conversation_presence_notifier.dart`, plus shared
  `conversation_state.dart`. Target: <600 lines per file.
- **#1358 (tech_debt review)** — Lists the four files >2,000 lines, says two
  of them roughly doubled since the last review. Recommends a soft CI check
  warning at ~1,500 lines and prioritizing the conversation viewmodel first.
- **#1293 (closed completed)** — Specified a CI gate (find-based shell
  script, `// dart-line-count-allow` escape hatch, fail at 1,500 lines).
  Closed but **not implemented** — verified by greppping `.github/` and
  `package.json`. The "completed" close is misleading; the work didn't
  happen.

No maintainer disagreements; the direction is clear.

## Codebase observations

- `conversation_view_model.dart` is a single `ConversationNotifier` class
  with **~50 methods** spanning: messages stream + send/retry/reactions
  (~1,000 lines), transfer actions (~600), request actions (~250),
  experience actions (~600), entity caching + parent refresh (~500),
  pending-attachments upload UI (~150). The split lines suggested in #1329
  map cleanly onto these groupings. State is already a single
  `ConversationState` Freezed class.
- `gear_content_view.dart` and `experience_content_view.dart` are each a
  single `ConsumerStatefulWidget` with one giant `_State` class — split
  is "extract widgets," not "extract notifiers".
- `experience_needs_sheets.dart` already defines several public classes
  (`AddContributionSheet`, `BatchAddNeedResult`, …) plus private helpers —
  splits cleanly by sheet kind.
- The repo has no current CI line-count gate. CLAUDE.md says "preferably
  <1,000 lines" — that's the target threshold for the gate.

## Decision: big-bang refactor (Option A)

User chose to do all 19 files in a single branch (`shorten_long_files`,
already checked out) rather than phased PRs. Trade-offs accepted: large
review surface and merge-conflict risk against in-flight branches in
exchange for done-in-one and a single green CI run at the final threshold.

This dedupes #1329 and #1358 cleanly: both close as duplicates of #1324
when this branch lands.

## Open Questions

- **CI gate threshold — 1,000 (CLAUDE.md) or 1,500 (#1293)?**
  Recommendation: **1,000**, since we're shrinking everything to fit. The
  whole point of the big-bang is to avoid leaving long-tail files just
  under 1,500. Keep the `// dart-line-count-allow` escape hatch from
  #1293 as a pressure release for the rare legitimate exception.
- **Should l10n be excluded explicitly from the CI gate?** Yes — exclude
  `app/lib/l10n/app_localizations*.dart` along with `gen/`, `*.g.dart`,
  `*.freezed.dart`. They are regenerated from `.arb` files.
- **For `conversation_view_model.dart`, separate `Notifier`s or a
  coordinator delegating to plain Dart classes?** Recommendation: separate
  notifiers + a small coordinator. Riverpod families compose well; the
  messages stream already owns its `StreamSubscription` and benefits from
  living alone. The `ConversationState` Freezed class can stay shared.
- **Re-export shims for the public providers?** Yes — keep
  `conversationProvider`, `experienceProvider`, `requestProvider`,
  `gearSharingProvider`, etc. exported from a re-export file at the old
  path so callers don't churn. Splits are internal.
- **Do the screen content views (`gear_content_view`,
  `experience_content_view`, `request_content_view`,
  `metric_detail_screen`, `community_metrics_screen`,
  `experience_preview_modal`) need viewmodel splits or just widget
  extraction?** Widget extraction first; revisit viewmodels only if their
  associated viewmodel files are also too long (which several are — Phase
  4 of the original plan rolled in).
- **Should we close #1293 again or reopen and let this work close it?**
  Recommendation: **reopen #1293**, attach this branch to it, and close
  it when the gate lands. The current "completed" state is misleading.

## Audit findings (2026-04-25)

Reviewed against `docs/client/architecture.md`, `docs/client/caching.md`,
`docs/client/i18n.md`, `docs/server/architecture.md`. The plan is
**architecturally compliant** but several patterns from the docs need to
be made explicit so per-file sub-agents don't accidentally regress them.

### Compliance — confirmed

- **MVVM layering preserved.** The split keeps repositories →
  ViewModels → Screens unchanged; nothing crosses layer boundaries. No
  `Controller` class is introduced (architecture.md §"No Controllers").
- **Riverpod `Notifier` (not `AsyncNotifier`) is correct for the
  conversation viewmodel.** architecture.md §"Manual Stream Management"
  uses `ConversationNotifier` itself as the canonical example for keeping
  regular `Notifier` when there are multiple streams + timers.
- **Server architecture is unaffected.** This is a Dart-only refactor;
  no proto, no service, no storage changes.
- **i18n exclusion is correct.** `app/lib/l10n/app_localizations*.dart`
  is generated by `flutter gen-l10n` (per `app/l10n.yaml`); the gate
  excludes it correctly. Note: i18n.md says these files live in
  `.dart_tool/flutter_gen/` (synthetic package), but in this repo they
  are in-tree at `app/lib/l10n/` — the exclusion pattern matches the
  actual location, which is what matters.
- **Caching unchanged.** No ViewModel currently has manual cache maps;
  splits must not introduce any (caching.md §"Repositories own
  caching").

### Required additions (folded into the plan below)

The following **must be explicit instructions to every sub-agent** —
otherwise a parallel split will silently regress one of these patterns:

1. **Preserve `ref.onDispose` cleanup.** The conversation viewmodel
   owns a `StreamSubscription` and a `Map<String, Timer>`. After the
   split, each owning notifier must register its own `ref.onDispose` in
   `build()` (architecture.md §"Resource Cleanup"). Specifically:
   `_messageStreamSubscription` + `_pendingMessageTimers` move with
   `conversation_messages_notifier.dart`.
2. **Preserve `SafeNotifierMixin` and `ref.mounted` checks.** Any
   notifier currently using `SafeNotifierMixin` or manual `ref.mounted`
   guards keeps them in every split (architecture.md §"Async Safety
   in Notifiers"). Sub-agents must not "clean up" these checks.
3. **Preserve disposal tests.** Per architecture.md §"Disposal
   Testing" ("All async Notifiers **must** have disposal tests"), every
   new notifier needs its own disposal test. If the original test file
   covers disposal, split it alongside the notifier.
4. **Family parameterization preserved.**
   `conversationProvider` is a family parameterized by `conversationId`.
   The split notifiers must each take the same `conversationId` (or
   read it from a shared parent), and `conversation_view_model.dart`
   must continue to expose `conversationProvider.family(...)` so all
   call sites keep working unchanged.
5. **`ConversationState` stays a single Freezed class.** The split is
   a coordinator + action-notifier pattern, not a state-slicing pattern.
   Action notifiers update the parent's state via the coordinator
   (`ref.read(conversationProvider.notifier).state = ...` with
   `copyWith`) rather than maintaining their own state slices. This
   avoids cross-notifier state-sync bugs.
6. **Extracted widgets keep `ConsumerWidget` and theme-aware colors.**
   When agents extract section widgets from `gear_content_view.dart`,
   `experience_content_view.dart`, etc., the new widgets:
   - extend `ConsumerWidget` / `ConsumerStatefulWidget` (not plain
     `StatelessWidget`) when they need `ref`;
   - use `AppColors.xxx(context)` and `Theme.of(context)` — never
     hardcoded `Colors.white` / `Color(0xFF...)` (architecture.md
     §"Colors and Theming");
   - resolve user-visible strings via `context.l10n.*` — never
     hardcoded strings (i18n.md §"Best Practices"). Strings already
     resolved in the original file pass through unchanged; sub-agents
     do **not** introduce new strings.
7. **Navigation helpers preserved.** Any screen split must not change
   `NavigationHelpers.pushScreen` / `SwipeToCloseMixin` usage
   (architecture.md §"Navigation & Gestures"). Sub-agents must not
   introduce raw `Navigator.push(MaterialPageRoute(...))`.
8. **`services/providers.dart` split — re-export discipline.** This is
   the highest-risk file because of how many files import from it.
   Hard requirements for that sub-agent:
   - Every previously-public symbol must remain importable from
     `package:ripls/services/providers.dart` via `export`.
   - After the split, `grep -r "from 'package:ripls/services/providers.dart'"`
     should return the same set of files as before — no caller edits.
   - Run `flutter analyze` on the entire `app/lib/` tree (not just the
     services directory) before reporting done.

### Plan adjustments

- Phase 1 / Phase 2 task lists now reference these requirements via
  the **"Sub-agent contract"** subsection added below.
- Phase 3 (CI gate) is unchanged.
- Phase 4 (close out) gains an explicit theme/i18n grep step to
  catch regressions before the PR is opened.

### Sub-agent contract (applied to every dispatched agent)

Every Sonnet sub-agent dispatched in Phase 1 or Phase 2 receives, in
its prompt, a hard checklist derived from the audit findings above:

1. Preserve all existing imports' public surface — leave a re-export
   shim at the original file path so callers don't need edits.
2. Preserve `ref.onDispose` cleanup for streams/timers in the notifier
   that owns them.
3. Preserve `SafeNotifierMixin` and `ref.mounted` checks — do not
   remove them as "dead code".
4. Preserve family parameters (e.g. `conversationId`) in every split
   notifier.
5. Do not introduce new `ConversationState`-style state classes; share
   the parent's Freezed state via the coordinator notifier.
6. Extracted widgets use `ConsumerWidget` (when they need `ref`),
   `AppColors.xxx(context)` for colors, `context.l10n.*` for strings.
7. Preserve `NavigationHelpers` / `SwipeToCloseMixin` usage; never
   introduce raw `Navigator.push(MaterialPageRoute(...))`.
8. Migrate corresponding disposal tests alongside any notifier split.
9. Run `flutter analyze app/lib/<area>/` on the touched directory
   before reporting done; for the `services/providers.dart` agent,
   run `flutter analyze` on the whole `app/lib/` tree.
10. Commit as a single commit on `shorten_long_files` (or in an
    isolated worktree for the parent to merge in) with a descriptive
    message; **no Claude branding or co-authorship lines** per the
    user's global preference.

## Implementation Plan

Single branch, single (big) PR. The 19 file splits are largely independent
(each file split is self-contained; only `services/providers.dart` and the
re-export shims have cross-file coupling), so most of the work runs in
parallel: dispatch one **Sonnet sub-agent per file** and join at the end of
each phase before running `flutter analyze` / `flutter test`.

**Parallel-agent ground rules (apply to every dispatch):**
- Each agent owns exactly one source file and creates its split files
  next to it (or under a sibling `widgets/` directory for screen splits).
- Each agent must preserve the original file's public symbols by leaving
  a re-export shim at the original path, so other files don't need
  import edits. (Exception: `services/providers.dart` — shim must
  re-export from each domain split.)
- Each agent runs `flutter analyze app/lib/<area>/` on the directory it
  touched before reporting done. It does **not** run the full test
  suite — that happens once per phase, after the join.
- Each agent commits its work as a single commit on the
  `shorten_long_files` branch via the project's "commit after each
  self-contained change" convention. Agents must coordinate on commits
  by working in worktrees or by being scheduled sequentially per phase
  (see Coordination below).
- No agent edits `CLAUDE.md`, `.github/workflows/`, or
  `scripts/check_dart_file_size.sh` — those land in Phase 3 by the
  parent agent.
- Phase boundaries are synchronization points: parent runs
  `flutter analyze` + `flutter test --reporter expanded` after all
  agents in the phase report back, before kicking off the next phase.

**Coordination:** Because all agents commit to the same branch, dispatch
each phase's agents using `isolation: "worktree"` so they work on
independent copies, then the parent merges/cherry-picks each agent's
commit onto `shorten_long_files` in order. This avoids the
write-conflict risk of N concurrent agents touching the same git index.

Phases below are commit-cluster boundaries — each phase's commits should
leave the tree green. The CI gate goes in **last** so intermediate
commits aren't held to a threshold the codebase doesn't yet meet.

### Phase 1: Split the four 2k+ files
**Dispatch:** 4 parallel Sonnet agents in worktrees, one per file. Parent
joins commits onto `shorten_long_files` in any order, then runs analyze
+ tests.

**Files & target structure:**

- [ ] `conversation_view_model.dart` (3,092) →
  - [ ] `conversation_state.dart` — Freezed state + part file.
  - [ ] `conversation_messages_notifier.dart` — `loadMessages`,
    `startMessageStream`, `_handleStreamedMessage`, `sendMessage`,
    `retryMessage`, `_onMessageTimeout`, `addReaction`, `removeReaction`,
    `_handleSystemMessageUpdate`, `_handleReactionUpdate`,
    `_refreshMessagesAfterMutation`, message-stream subscriptions/timers.
  - [ ] `conversation_transfer_actions.dart` — `cancelTransfer`,
    `startLoan`, `completeTransfer`, `expressInterest`,
    `withdrawInterest`, `approveTransferRequest`,
    `declineTransferRequest`, `completeLoan`, `cancelLoan`,
    `updateLoanPickupDetails`, `expressInterestInLoan`,
    `withdrawLoanInterest`, transfer-status helpers.
  - [ ] `conversation_request_actions.dart` — `fetchRequestDetails`,
    `_fetchRequestImpact`, `fetchRequestMedia`, `offerToFulfill`,
    `markRequestFulfilled`, `cancelRequest`, `withdrawOffer`,
    `_invalidateAndRefreshRequest`,
    `_invalidateRequestViewModelIfActive`.
  - [ ] `conversation_experience_actions.dart` —
    `fetchExperienceDetails`, `_resolveExperienceLocation`,
    `_fetchImpactEstimate`, `fetchExperienceMedia`,
    `_resolveBackgroundImage`, `rsvpToExperience`, `voteOnTime`,
    `lockTime`, `recordSingleAttendance`, `recordBulkAttendance`,
    `completeExperienceWithAttendance`, `markExperienceInProcess`,
    `completeExperience`, `cancelExperience`,
    `updateExperienceLocation`, `_invalidateAndRefreshExperience`,
    `_invalidateExperienceViewModelIfActive`.
  - [ ] `conversation_attachments_notifier.dart` — pending-attachments
    + upload progress.
  - [ ] `conversation_view_model.dart` — thin coordinator: `initialize`,
    `refresh`, `selectRecipient`, `fetchOtherParticipantName`,
    `fetchTransferDetails`, `fetchGearThumbnail`,
    `fetchConversationContext`, `_buildTransferStatusMap`,
    `_invalidateParentEntityCacheIfNeeded`, `_updateParticipants`,
    `_refreshParticipantsFromSystemMessage`, `getOtherParticipantName`,
    `getOtherParticipantId`, `refreshAfterModalAction`. Re-exports
    `conversationProvider`.
- [ ] `gear_content_view.dart` (2,465) → extract section widgets
  (`_GearHeader`, `_GearImpactSection`, `_GearActionsSection`,
  `_GearTransferList`, etc.) under
  `app/lib/presentation/screens/gear/widgets/`. Keep the
  `ConsumerStatefulWidget` as a composer.
- [ ] `experience_content_view.dart` (2,100) → extract
  `_ExpandableDescription`, `_RsvpButton`, `_CarouselMediaPickerSheet`,
  plus larger inline section widgets, under
  `app/lib/presentation/screens/experience/widgets/`.
- [ ] `experience_needs_sheets.dart` (2,032) → split by sheet kind
  (`add_contribution_sheet.dart`, `add_need_sheet.dart`,
  `batch_add_need_sheet.dart`, etc.) + `needs_sheet_models.dart` for
  the result/draft data classes.

**Validates:** Each Phase 1 file <1,000 lines. `flutter analyze` clean,
`flutter test --reporter expanded` green. Manual smoke: conversation
screen (messages/transfers/requests/experiences), gear detail,
experience detail, experience needs sheets.

### Phase 2: Split the 1k+ long tail
**Dispatch:** 15 parallel Sonnet agents in worktrees, one per file.
**Sequencing constraint:** `services/providers.dart` is touched
indirectly by re-export shims in many of the viewmodel splits — schedule
its agent **last in this phase** (or run it in a second wave) so the
other agents have already settled their re-exports. Parent joins commits
in waves: viewmodels + widgets first, then `services/providers.dart`,
then analyze + tests.

**Files (target <1,000 lines each):**

- [ ] `community_metrics_screen.dart` (1,642) — extract per-metric tile
  widgets.
- [ ] `request_content_view.dart` (1,343) — same playbook as gear /
  experience content views; widgets under
  `app/lib/presentation/screens/request/widgets/`.
- [ ] `gear_metadata_sheet.dart` (1,342) — extract per-section editors.
- [ ] `media_carousel.dart` (1,325) — extract per-media-kind handlers.
- [ ] `gear_sharing_view_model.dart` (1,274) — split notifier by concern;
  re-export the public provider from the original path.
- [ ] `needs_section.dart` (1,261) — extract per-need-type widgets.
- [ ] `experience_preview_modal.dart` (1,245) — extract preview sections.
- [ ] `message_list.dart` (1,191) — extract message-row widgets +
  scroll/pagination logic.
- [ ] `services/providers.dart` (1,150) — split by domain
  (`auth_providers.dart`, `chat_providers.dart`, `gear_providers.dart`,
  `experience_providers.dart`, `media_providers.dart`, …) and re-export
  from `providers.dart`. **Touchy** — many import paths reference this
  file. Re-exports must preserve every existing public symbol.
- [ ] `experience_view_model.dart` (1,137) — split notifier (mirror the
  conversation viewmodel split: messages/state/actions).
- [ ] `drill_down_builder.dart` (1,078) — split by metric kind.
- [ ] `request_view_model.dart` (1,041) — split notifier.
- [ ] `metric_detail_screen.dart` (1,036) — extract section widgets.
- [ ] `time_modal_view_model.dart` (1,012) — split notifier.
- [ ] `experience_service.dart` (1,007) — split by RPC group; keep one
  `ExperienceService` facade so callers don't churn.

**Validates:** All 19 files <1,000 lines. `flutter analyze` clean,
`flutter test --reporter expanded` green. Spot-check imports: no callers
need to be updated (re-exports preserve the public API).

### Phase 3: Land the CI line-count gate
**Goal:** Make this self-enforcing so the regression cycle that produced
this issue doesn't repeat.
**Tasks:**
- [ ] Reopen #1293 (its "completed" close was misleading — gate never
  shipped). Reference #1324 as superseder.
- [ ] Add `scripts/check_dart_file_size.sh`:
  - Scan `app/lib/**/*.dart`.
  - Exclude `gen/`, `*.g.dart`, `*.freezed.dart`,
    `app/lib/l10n/app_localizations*.dart`.
  - Fail at >1,000 lines unless the file contains
    `// dart-line-count-allow`.
- [ ] Add `lint:dart-size` npm script.
- [ ] Wire `lint:dart-size` into `.github/workflows/test_flutter.yaml`
  between analyze and test.
- [ ] Update CLAUDE.md: gate enforced at 1,000 lines; document the
  `// dart-line-count-allow` escape hatch and that any use must include
  a TODO with a tracking issue.
- [ ] Verify no file in the repo currently needs the escape hatch
  (Phase 2 should have eliminated all offenders). If any file
  legitimately can't be split, add the hatch with a documented reason.
**Validates:** Gate fires on a deliberately overlong file in a test
commit; passes on the final tree.

### Phase 4: Close out
- [ ] Run `npm run build` (Go side, sanity — should be unaffected).
- [ ] Run `flutter analyze` and `flutter test --reporter expanded`.
- [ ] Run `npm run lint:dart:i18n` (per i18n.md) to confirm no
  hardcoded strings were introduced during widget extraction.
- [ ] Run `npm run lint:nav` to confirm no raw
  `Navigator.push(MaterialPageRoute(...))` was introduced
  (per architecture.md §"Navigation & Gestures").
- [ ] Grep for hardcoded color regressions:
  `grep -rE "Colors\.(white|grey\[|black87)|Color\(0x" app/lib/presentation/`
  — diff against `main` to confirm sub-agents didn't introduce any.
- [ ] Manual smoke: conversation, gear detail, experience detail,
  experience needs sheets, request content, community metrics,
  media carousel, message list.
- [ ] Open the PR titled e.g. "refactor: shorten Dart files >1000 lines
  + add CI gate (#1324)". Body lists every file split and links #1329,
  #1358, #1293.
- [ ] Close #1329, #1358 as duplicates / fixed-by once merged.
- [ ] Close #1293 once the gate lands.

## Related Issues

- #1329 — Split `conversation_view_model.dart`. Fully covered by Phase 1;
  close when this branch merges.
- #1358 — Presentation files >2,000 lines unchecked. Covered by Phase 1
  + Phase 3 (the CI gate it asked for, set tighter than its 1,500
  recommendation). Close when this branch merges.
- #1293 — CI gate proposal. Marked closed-completed but not actually
  implemented. Reopen and let Phase 3 close it.
