# Issue #1011: Single player mode

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1011
**Filed:** 2026-04-03 | **Last activity:** 2026-04-03
**Labels:** none | **Assignees:** tescobar
**Context docs:** none

## Problem Statement

Users want to retroactively log community activities (experiences, giveaways, loans) that already happened, even when participants don't have the app. Today, experience creation assumes a future event: the owner creates it, shares it, people RSVP, then it proceeds through the state machine. There is no way to record something that already occurred, and no way to reference people who aren't registered users.

This is the "manual activity entry" analog to Strava -- users who are engaged with impact metrics will want to track *all* community activity, not just the subset where every participant has the app.

Two core capabilities are needed:
1. **Past-tense creation** -- Create an experience (and eventually giveaways/loans) that already happened, with the owner already RSVPed yes, skipping to the confirmation step.
2. **Shadow accounts** -- Lightweight placeholder accounts for people who don't have the app, so they can be referenced by name in activities. These merge with real accounts if the person later registers.

## Context from Comments

**tescobar (author/maintainer):** Wants to reuse existing UI patterns (creation modal, confirmation modal, @ mentions, share modal) to minimize learning curve. Sees opportunity to compose existing components rather than build net-new flows.

**maintainer (maintainer):** Strongly endorses the feature, framing it as essential for metrics-driven engagement. Key additions:
- Single player mode should cover **all item types** (giveaways, loans, and possibly requests with shadow requesters), not just experiences.
- Highlights shadow users as the trickiest part -- managing complexity without overwhelming users or data models.
- Calls out a **related but distinct need: revision/editing** of existing items (add people after the fact, change location, add helpers). Analogous to how Strava's manual entry and activity editing are closely related.

No disagreements between commenters. Bryan's comment expands scope significantly (all item types + editing).

## Potential Solutions

### Option A: Past-tense AI detection + shadow user model

- **Approach:** Extend the existing AI-powered `GenExperience` flow to detect past-tense language in prompts and set a `is_past` flag. When past-tense is detected, auto-set the owner's RSVP to YES and skip to the confirmation modal. Introduce a `ShadowUser` model for non-registered participants that can be referenced anywhere a regular user can. Shadow users get invite links and merge with real accounts on registration.
- **Pros:** Minimal UI changes -- reuses creation modal, confirmation modal, and share flows. AI already parses natural language prompts. Shadow users are a first-class concept with clean merge semantics.
- **Cons:** Largest scope. Shadow user model touches many parts of the system (RSVPs, attendance, transfers, conversations, member lookup). AI detection of past-tense is another heuristic that could misfire.
- **Effort:** Large.

### Option B: Explicit "Log past activity" mode + simplified participant names

- **Approach:** Add an explicit toggle/button for "Log something that already happened" (no AI detection of tense). This enters a streamlined flow that skips directly to a modified confirmation modal. Instead of full shadow accounts, store non-member participant names as simple strings on the activity record (e.g., `external_participants: ["Mike", "Bhavna"]`). No account creation, no merge logic.
- **Pros:** Much simpler. No shadow user complexity. Can ship quickly as an MVP.
- **Cons:** Doesn't support inviting those people later, doesn't accumulate their history across activities, doesn't support the "reference by name as if they were a member" requirement, doesn't work for giveaways/loans which need a specific recipient.
- **Effort:** Small.

### Option C: Phased hybrid -- MVP with past-tense flow, then shadow users (SELECTED)

- **Approach:** Phase 1 ships past-tense creation for experiences only -- no new mode or toggle, the user simply sets a time in the past via the existing time picker and the server handles it. Existing community members can be tagged as participants. Phase 2 adds the shadow user model, enabling non-member participants, plus a Manage Members screen. Phase 3 extends to giveaways, loans, and the editing/revision flow Bryan requested.
- **Pros:** Delivers value incrementally. Each phase is shippable. Hardest problems (shadow users, multi-item-type support) are deferred but planned for. Lets the team learn from Phase 1 usage before committing to the shadow user data model.
- **Cons:** Users who need shadow users immediately will have to wait. Phase boundaries need to be clean enough that Phase 1 code doesn't need heavy refactoring for Phase 2.
- **Effort:** Large overall, but small-to-medium per phase.

## Recommendation

**Option C (Phased hybrid)** is the right approach. The issue describes a large surface area and Bryan's comments expand it further. Trying to ship everything at once (Option A) risks a long cycle with a big-bang release. Option B is too limited -- it doesn't address the core shadow user requirements. 

The phased approach lets us ship past-tense experience creation quickly (high user value, moderate complexity), then layer in shadow users once the UX patterns are validated.

## Open Questions (Resolved)

- **Should AI detect past-tense automatically, or should the user explicitly choose "log past activity"?** -- **Decision:** Neither toggle nor AI detection needed. The user sets a date in the past via the existing time picker during creation preview. If the time is in the past at save time, the server treats it as a past event. Simple, explicit, no new UI.

- **How should shadow users relate to communities?** -- **Decision:** Shadow users are scoped to a community. Visible to all members but not "members" themselves. Avoids polluting the global user space.

- **What happens when a shadow user matches an existing registered user's name?** -- **Decision:** No auto-match on name (names aren't unique). Merge happens only via invite link click or email match during registration. For shadow-to-shadow deduplication within a community, prompt the creator: "Is this the same Mike from [previous activity]?"

- **Should shadow users count toward the 32-member community limit?** -- **Decision:** No. Shadow users are activity participants, not community members.

- **How should shadow users appear in conversations/chat?** -- **Decision:** Not chat participants. Their names appear in attendance records and activity summaries only.

- **Should this extend to requests (per Bryan's comment)?** -- **Decision:** Defer to Phase 3. Experiences, giveaways, and loans are the priority.

- **How does the "revision/editing" capability Bryan mentioned relate to this work?** -- **Decision:** Separate but parallel workstream. Shares some UI components (confirmation modal, participant management) but different entry points and constraints.

## Implementation Plan

### Phase 1: Past-tense experience creation with existing members

**Goal:** Allow users to create experiences that already happened and immediately proceed to confirmation, with existing community members as participants. No new UI mode or toggle -- the user simply sets a time in the past via the existing time picker during creation preview, and the server detects it.

**Tasks:**

*Proto changes (in `proto/ripls/api/experience_service.proto` — API package only):*
- [x] Add `repeated string mentioned_names` to `GenExperienceResponse` — names extracted from the prompt by AI (e.g., "hike with Mike and Bhavna")
- [x] Use existing `ExperienceTime` with `SpecificTime` for the past event time (no new proto field needed — server checks if time is in the past at save time)
- [ ] Run `npm run generate` to regenerate Go and Dart code

*Server changes:*
- [x] Update AI prompt in `server/services/experience/gen_ai.go` to extract participant names from prompts
- [x] Update `SaveExperience` handler in `server/services/experience/save.go`: when experience time is in the past, auto-create owner's RSVP as YES and set state to IN_PROCESS
- [x] Add name-based fuzzy search to community service: new `SearchCommunityUsers` RPC in `proto/ripls/api/community_service.proto` with its own dedicated request/response types, implemented in `server/services/community/membership.go`. Use `ILIKE` for matching per SQL efficiency guidelines
- [x] Add unit tests for SaveExperience with past-time detection (`server/services/experience/save_test.go`)
- [ ] Add unit tests for name extraction in AI response parsing (`server/services/experience/gen_ai_test.go`)
- [ ] Run `npm run build` to verify lint + compilation

*Flutter changes (all UI strings via `context.l10n`, add keys to both `app_en.arb` and `app_es.arb`):*
- [x] Extend the existing `GenExperienceViewModel` to handle AI-extracted `mentioned_names` — resolve them against community members via repository, present as suggested participants in the preview modal
- [x] Add community member search to `CommunityRepository` (or create a dedicated repository method) with CacheService for search results. All data access through repository, not direct service calls
- [x] Update `experience_preview_modal.dart`: show member picker for tagging participants when a community is selected. Member data flows through ViewModel/Repository, not fetched in widget. Widget reads state via `ref.watch()`, triggers actions via `ref.read().notifier`
- [x] After save when time is in the past, navigate to mark-completed modal
- [ ] Update `mark_completed_modal.dart`: pre-populate attendance list with tagged participants
- [x] Add i18n strings for new UI text: member picker labels, participant tagging, etc.
- [x] Add ViewModel unit tests using `ProviderContainer` with mocked repository
- [ ] Add disposal safety tests for any modified async Notifiers (required per architecture docs)

**Validates:** Users can retroactively log experiences with existing community members and get metrics credit. Confirms the UX pattern works before adding shadow user complexity.

### Phase 2: Shadow user accounts

**Goal:** Allow users to reference non-member participants by name, creating lightweight placeholder accounts that can later be claimed.

**Tasks:**

*Proto changes:*
- [x] Design `ShadowUser` proto in `proto/ripls/models/shadow_user.proto` (models package — storage type): `id`, `name`, `community_id`, `created_by_user_id`, `created_at_unix_sec`, `optional string invite_link_id`, `optional string claimed_by_user_id`
- [x] Add separate API representation in `proto/ripls/api/shadow_user.proto` (API package): lightweight `ShadowUser` with `id`, `name`, `optional bool is_claimed`. API and models packages must not import each other
- [x] Add shadow user RPCs to `proto/ripls/api/community_service.proto` (community-scoped): `CreateShadowUser`, `ListShadowUsers`, `SearchShadowUsers` — each with dedicated request/response types (no type reuse across RPCs)
- [x] Update `ExperienceRSVP` and attendance-related API types to accept `optional string shadow_user_id` alongside `user_id`
- [x] Add `optional string shadow_user_id` to `CommunityInvitationLink` in models for per-shadow-user invite links
- [x] Run `npm run generate`

*Server changes:*
- [x] Create `shadow_user` storage table with indexes on `community_id`, `claimed_by_user_id`, and `name` (for `ILIKE` search)
- [x] Implement shadow user RPCs in `server/services/community/shadow_users.go` (new file in community service — shadow users are community-scoped). Keep file focused and < 500 lines
- [x] Implement `SearchShadowUsers` with `ILIKE` fuzzy matching and `LIMIT` (never unbounded queries)
- [x] Create shared library `server/shadow/merge.go` for shadow-to-real user merge logic — this crosses entity boundaries (RSVPs, attendance, transfers) so it must be a standalone library, not embedded in a service. Inject `*storage.ProtoSQLStorage` only.
- [x] Update registration flow in `server/services/login/`: when a user registers via a shadow-user invite link, call the merge library to move activity history to the real account
- [x] Update experience RSVP handler in `server/services/experience/rsvp.go` to handle shadow_user_id
- [x] Add comprehensive unit tests for shadow user CRUD (`server/services/community/shadow_users_test.go`)
- [x] Add unit tests for merge logic (`server/shadow/merge_test.go`) — test both success paths and edge cases (already-claimed shadow, no activities to merge, concurrent merge)
- [x] Run `npm run build`

*Flutter changes:*
- [x] Create `ShadowUserRepository` with CacheService namespace `'shadow_user'`. Cache keys: `'shadow_user:community:{communityId}:list'`. Handle invalidation after create mutations
- [x] Create `ManageMembersViewModel` (Riverpod Notifier with Freezed state) — loads both real and shadow member lists via `CommunityRepository` and `ShadowUserRepository`. Handles shadow user creation and invite link generation
- [x] Update confirmation modal (mark_completed_modal): add "Add someone new" option that creates a shadow user with just a name, adds them to attendance. Uses `context.l10n` for all labels
- [ ] Update share modal: show shadow users with pending invite links
- [ ] Update RSVP/attendance display widgets to distinguish shadow vs. real users (use API `is_claimed` field)
- [x] Implement **Manage Members screen** in community settings: lists real members and shadow members in separate sections. From this screen, users can create new shadow members, send invite links to existing shadow members, and see claimed/unclaimed status. New screen gets `SwipeToCloseMixin` and is pushed via `NavigationHelpers.pushScreen()`
- [x] Add i18n strings for shadow user UI and Manage Members screen: section headers ("Members", "People"), creation labels, invite status, empty states
- [x] Add ViewModel unit tests with mocked `ShadowUserRepository` and `CommunityRepository`
- [x] Add disposal safety tests for `ManageMembersViewModel`

**Validates:** Non-member participants can be tracked by name, invited later, and seamlessly merged when they register. Community owners have a dedicated screen to manage both real and shadow members.

### Phase 2.1: Polish — completion modal search, preview modal cleanup, past-event nav flow

**Goal:** Fix three rough edges discovered after shipping Phases 1 and 2: (1) "Add someone new" in the completion modal can only create shadow users by name, it can't find existing members; (2) the preview modal has an "Add Participants" button that shouldn't exist there; (3) the post-preview "Mark Completed" flow dumps the user on the feed without the completion modal open.

---

#### Problem 1: "Add someone new" in completion modal doesn't search existing users

**Current behavior (`mark_completed_modal.dart` → `_showAddPersonDialog()`):** Free-text name dialog that calls `addShadowAttendee()` directly. No lookup — always creates a new shadow user regardless of whether a matching member or existing shadow user already exists.

**Desired behavior:** Search community members and existing shadow users by name first. Present results. If no match is found, offer "Add [name] as a new person" which creates a shadow user.

**Proposed solution:**

Replace the `AlertDialog` name-only prompt with a search-first bottom sheet (similar to the existing `_ParticipantPickerSheet` in the preview modal). The sheet:
1. Has a live search field that queries both `communityRepository.searchMembers()` and `shadowUserRepository.searchShadowUsers()` in parallel via `Future.wait`
2. Shows results in a single list with a visual distinction between registered members (avatar) and shadow users (placeholder icon + "Not yet on Ripls" label)
3. If the query returns no results, shows a single "Add [query] as a new person" row that calls `addShadowAttendee()`
4. Tapping a result adds them to the attendance list immediately

No new RPC needed — `searchMembers` and `searchShadowUsers` both exist. This is a Flutter-only change to `mark_completed_modal.dart` and `event_modal_view_model.dart`.

---

#### Problem 2: Preview modal has "Add Participants" button + wrong button label for past events

**Current behavior (`experience_preview_modal.dart`):** `_buildParticipantPicker()` renders an "Add participants (optional)" tappable row. The primary action button always says "Create Experience".

**Desired behavior:**
- Remove `_buildParticipantPicker()` entirely from the preview. Participant management happens in the completion modal.
- When `selectedTime` is set to a time in the past (or any time today earlier than now), the primary button label changes from "Create Experience" to "Mark Completed".

**Proposed solution:**

In `experience_preview_modal.dart`:
- Delete the `_buildParticipantPicker()` method and its invocation in the layout
- Add a computed getter on `GenExperienceState` (or derive inline): `bool get isTimeInPast` — checks if `selectedTime` is a `SpecificTime` whose unix sec is ≤ now
- Button label: `isTimeInPast ? context.l10n.experiencePreviewMarkCompleted : context.l10n.experiencePreviewCreate`
- The `_handleCreateExperience()` already branches on past time to show the completion modal, so no handler logic changes are needed for this label swap

---

#### Problem 3: Post-preview "Mark Completed" opens completion modal over experience screen, not feed

**Current behavior (`experience_preview_modal.dart` → `_handleCreateExperience()`):** On past-event save, the code calls `showModalBottomSheet` for `MarkCompletedModal` right after `createAndShareExperience()`, then pops the preview. The completion modal ends up over whatever was behind the preview — usually still the creation modal stack, not the feed.

**Desired behavior:** After creating the experience:
1. Close the creation and preview modals (return to feed)
2. The feed scrolls to / highlights the new experience card
3. The completion modal opens on top of the feed

**Additionally**, the completion modal should be pre-populated with auto-resolved participants from AI-mentioned names: for each name in `mentionedNames`, call `searchMembers(query: name, limit: 3)`. If exactly one result is returned, auto-accept it. If multiple results, prompt the user to pick. If zero results, create a shadow user with that name. All resolved IDs are passed as `preTaggedUserIds` / `preShadowUserIds` when opening the modal.

**Proposed solution:**

The current `_handleCreateExperience()` runs inside the preview modal and drives post-creation navigation. This needs to change for the past-event case:

1. `GenExperienceViewModel.createAndShareExperience()` returns the created `experienceId` (already does this).
2. A new method `GenExperienceViewModel.resolveParticipantsForCompletion()` iterates `mentionedNames`, calls `communityRepository.searchMembers()` and (for zero results) `shadowUserRepository.createShadowUser()`, and returns `ResolvedParticipants(userIds: [...], shadowUserIds: [...])`. This keeps the logic out of the widget.
3. In `_handleCreateExperience()`, for the past-event branch:
   - Call `resolveParticipantsForCompletion()` (showing a brief loading state)
   - Call `Navigator.pop()` on the preview modal (and the creation modal via root navigator if needed) — returning the `experienceId` up the call stack
   - The feed layer catches the returned `experienceId`, scrolls to / highlights the card, and calls `showModalBottomSheet` for `MarkCompletedModal` with the resolved participant IDs

This keeps navigation concerns at the feed level, which already holds a reference to the `experienceId` result from the creation modal.

---

#### Problem 4: Attendee sheet shows RSVP buttons on completed experiences

**Current behavior (`attendee_list_sheet.dart` → `_buildToggleSection()`):** The "Can you make it?" RSVP toggle (Yes/Maybe/No) renders regardless of experience state. On a completed experience, tapping a button calls `experienceProvider.updateRSVP()`, which hits the server and gets a `CodeFailedPrecondition` error. The server already blocks this correctly (`rsvp.go` lines 44-49: only ACTIVE and JOINED states accept RSVPs), but the client shows the controls anyway.

The main experience screen (`experience_content_view.dart`) correctly hides the RSVP button on completed/cancelled experiences (via `isTerminal` check). But the attendee sheet, which is reachable by tapping the attendee list, does not have the same guard.

**Desired behavior:** On a completed (or cancelled/in-process) experience, the attendee sheet should show the list of attendees in read-only mode. No RSVP toggle. Users can see who attended but cannot change status.

**Proposed solution:**

Pass the experience state (or a simple `bool isReadOnly`) into `AttendeeListSheet`. When the experience is not ACTIVE or JOINED, skip rendering `_buildToggleSection()` entirely. The attendee list itself remains visible.

This is a small, self-contained change — one widget file plus its call sites.

---

#### Open Questions

- **"The creation modal should be open"**: You said after tapping "Mark Completed" from the preview "the creation modal should be open." Assuming this means the **completion modal** (`mark_completed_modal`), not the creation modal. Please confirm — if it truly means the creation modal, the flow is different.

- **Multiple name matches during auto-resolution**: If the prompt says "Mike" and the community has "Mike Smith" and "Mike Johnson", should we: (a) show a disambiguation prompt before the completion modal opens, or (b) add both as suggestions inside the completion modal's pre-populated list where the user can uncheck? Recommendation: (b) is less disruptive — add both as pre-tagged attendees and let the user uncheck the wrong one.

- **Search scope in "Add someone new"**: Should the search in the completion modal also include shadow users from other experiences in the community (not just the current experience's RSVPs)? Recommendation: yes — avoid creating a second "Mike" shadow user if one already exists in the community.

---

#### Tasks

*Flutter changes only — no proto or server changes needed:*

- [x] **Completion modal — replace name-only dialog with search sheet**: Delete `_showAddPersonDialog()` in `mark_completed_modal.dart`. Add `_showAddPersonSheet()` that renders a search field querying `searchMembers` + `searchShadowUsers` in parallel. Show results distinguished by member vs. shadow type. "Add [query] as new person" row when empty. Add i18n keys: `markCompletedAddPersonSearch`, `markCompletedAddPersonShadowLabel`, `markCompletedAddPersonNewLabel`
- [x] **`EventModalViewModel`**: Added `addExistingShadowAttendee()` and `addMemberAttendee()` so the search sheet can add either type without duplicating logic
- [x] **Preview modal — remove participant picker**: Deleted `_buildParticipantPicker()` and its call site in `experience_preview_modal.dart`. Deleted `_ParticipantPickerSheet` class.
- [x] **Preview modal — conditional button label**: Computed inline using `_isTimeInPast()`. Bound to `context.l10n.experiencePreviewMarkCompleted` vs `context.l10n.experiencePreviewCreate`. Both i18n keys added.
- [x] **`GenExperienceViewModel.resolveParticipantsForCompletion()`**: Implemented — iterates `mentionedNames`, calls `communityRepository.searchMembers(query: name, limit: 3)`. If ≥1 result: adds all userIds. If zero: calls `shadowUserRepository.createShadowUser(name: name)`. Returns `ResolvedParticipants` value object.
- [x] **Past-event navigation in `_handleCreateExperience()`**: Past-event branch calls `resolveParticipantsForCompletion()`, then pops the preview with `ExperienceCreationResult` (carrying `experienceId`, `isPastEvent`, `preTaggedUserIds`, `preShadowUsers`). Creation modal passes result up to home screen.
- [x] **Feed layer**: `home_screen.dart` catches `ExperienceCreationResult`, calls `MarkCompletedModal.show()` with resolved participant IDs over the feed.
- [x] **Attendee sheet — read-only on completed experiences**: Pass experience state (or `bool isReadOnly`) into `AttendeeListSheet`. When state is not ACTIVE/JOINED, hide `_buildToggleSection()`. Update all call sites to pass the state.
- [ ] Add ViewModel unit tests for `resolveParticipantsForCompletion()` covering: single match, multiple matches, zero matches (shadow creation), mixed
- [ ] Add disposal safety tests for any async paths added to `GenExperienceViewModel`

**Validates:** The completion modal is reachable for any past event without knowing the participants in advance, the preview modal is streamlined, and participant resolution from AI hints is automated.

---

### Phase 3: Extend to giveaways, loans, and all item types

**Goal:** Apply single-player mode patterns to giveaways and loans, enabling retroactive logging of any community activity.

**Tasks:**

*Proto changes:*
- [ ] Add `optional bool is_past_event` to transfer creation RPCs in `proto/ripls/api/transfer_service.proto` (dedicated per-RPC request types, no reuse)
- [ ] Run `npm run generate`

*Server changes:*
- [ ] Update transfer service in `server/services/transfer/`: when `is_past_event` is true, create transfer in COMPLETED state with recipient (real or shadow user) already set
- [ ] For past giveaways: skip the interest/selection phase, go straight to completed
- [ ] For past loans: set start date in the past, allow setting as already returned
- [ ] Add unit tests for past-tense transfer creation
- [ ] Run `npm run build`

*Flutter changes:*
- [ ] Update Flutter gear/transfer creation flow: add past-tense mode similar to experiences. New state managed via ViewModel (Notifier + Freezed), data through repositories
- [ ] Update transfer flows: allow selecting shadow users as recipients (reuse `ShadowUserRepository`)
- [ ] Add i18n strings for past-tense giveaway/loan UI
- [ ] Consider past-tense requests (lower priority per Bryan's comment)
- [ ] Add ViewModel and repository tests for past-tense giveaways and loans

**Validates:** All core item types support single-player retroactive logging, completing the Strava "manual entry" analog.

## Related Issues

- No directly related issues referenced, but Bryan's comment about **revision/editing** of existing items is a natural companion feature that should be tracked as a separate issue.

---

## Revision History

**v4 (2026-04-05):** Added Problem 4 to Phase 2.1 — attendee sheet shows RSVP toggle on completed experiences. Server blocks it correctly but client renders the controls. Fix: pass experience state into `AttendeeListSheet` and hide the toggle section for non-ACTIVE/JOINED states.

**v3 (2026-04-04):** Added Phase 2.1 to fix three polish issues found after Phases 1+2 shipped: (1) completion modal "Add someone new" searches members + shadow users before creating new; (2) preview modal loses "Add Participants" button and gets conditional "Mark Completed" label; (3) past-event flow navigates to feed first, then opens completion modal with AI-mentioned names auto-resolved.

**v2 (2026-04-03):** Resolved all open questions per owner feedback. Key simplification: no AI past-tense detection or toggle needed -- user sets time in the past via existing time picker, server detects it. Added Manage Members screen to Phase 2 for managing real + shadow members. Removed `is_past_event` proto fields.

## Audit Notes

The following changes were made during architectural compliance review:

1. **Proto package separation enforced.** Original plan didn't distinguish API vs. models packages. Fixed: `ShadowUser` storage model goes in `proto/ripls/models/`, API representation in `proto/ripls/api/`. Client must never reference models package protos. Each new RPC gets dedicated request/response types (no reuse).

2. **Service independence for merge logic.** Original plan put shadow-to-real merge logic implicitly in the registration service, but it crosses entity boundaries (RSVPs, attendance, transfers). Fixed: factored into a standalone shared library `server/shadow/merge.go` per the "Shared Functionality in Libraries" pattern. Uses transactions for atomicity.

3. **Client MVVM compliance.** Original plan described UI changes without specifying ViewModels or repositories. Fixed: added `ExperiencePastEntryViewModel` and `ShadowUserViewModel` (Riverpod Notifiers with Freezed state), `ShadowUserRepository` with CacheService namespace. All data access goes through repositories, widgets interact only with ViewModels via `ref.watch()`/`ref.read()`.

4. **Cache invalidation specified.** Original plan omitted client caching concerns. Fixed: added CacheService namespaces (`'experience_past'`, `'shadow_user'`), cache key patterns, and invalidation after mutations.

5. **i18n compliance.** Original plan had no mention of localization. Fixed: every phase now includes a task to add i18n strings to both `app_en.arb` and `app_es.arb`, with `context.l10n` for all user-visible text.

6. **SQL efficiency.** Added `ILIKE` for fuzzy name matching (not `LOWER(col) LIKE`), `LIMIT` on search queries, index on `name` column for shadow users, and transaction use for merge operations.

7. **Navigation compliance.** Specified `NavigationHelpers` for screen transitions (not raw `Navigator.push`).

8. **Testing requirements.** Added disposal safety tests for async Notifiers (required per architecture docs), ViewModel unit tests with `ProviderContainer`, and explicit test file locations for server tests.

9. **Build verification.** Added `npm run generate` after proto changes and `npm run build` after server changes per CLAUDE.md guidelines.
