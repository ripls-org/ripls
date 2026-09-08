---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Retroactively logging past activities — past-event creation that skips the invitation state machine (experiences, giveaways, loans) plus provisional users (formerly "shadow users"), lightweight placeholders for unregistered people — optionally keyed by a phone/email contact handle — that are mergeable when they join.
  globs: [server/services/experience/save.go, server/services/transfer/create.go, server/services/community/provisional_users.go, server/provisional/**, server/contact/**, app/lib/presentation/screens/gear/past_transfer_modal.dart, app/lib/data/repositories/provisional_user_repository.dart]
  triggers: [single-player, past-event, log-past, provisional-user, retroactive, manual-entry, merge-provisional]
  lens: [domain, client, server, workflow]
  domain: experience
freshness:
  verified_commit: "ecb19a3d8"
  verified_on: "2026-06-30"
---
# Single Player Mode

Single player mode lets community members retroactively log activities that already happened — experiences, giveaways, loans — even when some participants don't have the app. It is the "manual entry" analog to Strava's activity logging.

Two capabilities underpin it:

1. **Past-event creation** — create any item with a time in the past; the server detects it and skips the invitation state machine.
2. **Provisional users** — lightweight placeholder accounts for people who aren't registered, referenced by name across activities and mergeable when they eventually join.

---

## How past-event creation works

### Experiences

**User flow:**

1. Open the experience creation modal and describe what happened: *"morning run with Sarah and Tom"*.
2. In the time picker, select a date and time in the past (past dates are allowed for the "Set time" tab; they are intentionally blocked for time polls, which only make sense for future events).
3. Tap **Mark Completed** (the primary button label switches from "Create Experience" when the selected time is in the past).
4. The server saves the experience and immediately sets its state to `IN_PROCESS`, provisions the event's per-item community (`community.ProvisionPerItemCommunity`), and shares the event into it via the internal `shareExperienceToCommunity`, which auto-creates the owner's attendance RSVP as YES.
5. The creation modal returns an `ExperienceCreationResult` with `isPastEvent: true` to the home screen, which opens the completion modal directly over the feed.
6. The completion modal is pre-populated with any participants the AI extracted from the prompt (see *Participant resolution* below).
7. The owner confirms or adjusts the attendee list and taps **Complete**. The server records attendance, generates the completion story synchronously (LLM call, ~5–8 s), and returns. The client immediately refreshes the feed, which now contains the new story.

**Server-side detection:**

In `server/services/experience/save.go`, the `isPastTime()` helper checks whether the experience's `SpecificTime.unix_timestamp_sec` is strictly before the current server clock at save time. If so:

- Initial state is set to `ExperienceState_EXPERIENCE_STATE_IN_PROCESS` (not `ACTIVE`).
- `started_at_unix_sec` is recorded immediately.
- No invitation notifications are sent.

**AI time extraction:**

When the prompt contains temporal language (*"last Friday"*, *"two weeks ago"*, *"yesterday evening"*), `GenExperience` extracts a structured date, time, and confidence level inline in the same LLM call. The server returns `extracted_time_unix_sec` in `GenExperienceResponse` and pre-populates `suggested_time` from it when confidence is EXPLICIT or INFERRED. The Flutter preview modal sets the time picker to `suggested_time`, showing a subtle "Suggested from your description" indicator so the user can confirm or adjust.

**Retrospective descriptions:**

When the resolved time is in the past, `GenExperience` selects `fromTextPromptPast` (defined in `server/services/experience/prompts.go`) instead of the default invitation-framing prompt. The past-tense prompt instructs the LLM to write a record of what happened (*"We hiked the ridge trail together"*) rather than an invitation (*"Join us for a hike"*).

### Giveaways and loans

Past-tense giveaways and loans are logged from the gear content view via **Log Past Giveaway** and **Log Past Loan** menu items in the gear action menu.

**User flow:**

1. From the gear detail view (in the pre-selection state), open the menu and tap **Log Past Giveaway** or **Log Past Loan**.
2. `PastTransferModal` opens (`app/lib/presentation/screens/gear/past_transfer_modal.dart`). The modal has:
   - A recipient/borrower picker that searches registered members and existing provisional users, with a *"Create [name] as a new person"* fallback that creates a provisional user on the spot.
   - A date picker (up to 5 years in the past) for when the giveaway or loan happened.
   - For loans only: an *"Already returned"* toggle and a return date picker, allowing a completed loan to be logged in a single step.
3. On submit, the modal calls `TransferRepository.createTransfer()` which issues the `CreateTransfer` RPC.
4. The server creates the transfer in COMPLETED state immediately (bypassing the interest and selection state machine) and generates a completion story synchronously.
5. The modal dismisses, the client calls `handlePostCreation()`, the feed refreshes, and the new story appears at the top.

**Proto additions** (`proto/ripls/api/transfer_service.proto`):

- `optional int64 completed_at_unix_sec` — set for past-tense creation; the server detects this and skips the state machine.
- `optional string provisional_user_id` — alternative to `recipient_user_id`; accepts a provisional user as the giveaway recipient or loan borrower.
- `optional int64 returned_at_unix_sec` — for loans; when set alongside `completed_at_unix_sec`, the loan is immediately marked RETURNED.

**Server-side** (`server/services/transfer/create.go`):

When `completed_at_unix_sec` is set and in the past, the handler creates the transfer in COMPLETED state, sets the recipient (registered user or provisional user), and calls `generateStoryForCompletedTransfer` synchronously before returning. Provisional user recipients are handled explicitly: the story generation looks up the provisional user name in `models.ProvisionalUser` rather than the registered user table.

---

## Provisional users

### What they are

A provisional user is a named placeholder for a person who doesn't have the app. They are community-scoped: each community has its own set of provisional users. Provisional users:

- Appear in attendance records and completion stories exactly like real users.
- Count toward impact metrics (CO₂ saved, cost saved, time saved) in exactly the same way.
- Do **not** count toward the community member limit (currently 32).
- Are **not** chat participants; their names appear in activity summaries only.
- May carry an optional **contact handle** — a phone number (E.164) or email — so a
  deviceless person can be invited and notified off-app before they register. The
  handle is the dedup key (one provisional user per handle per community) and is
  normalized server-side (`server/contact.NormalizePhoneE164` / lowercased email).
  It is treated as PII: masked in logs and never exposed on the API.

### Data model

```
proto/ripls/models/provisional_user.proto  — storage model
proto/ripls/api/provisional_user.proto     — API representation (id, name, is_claimed)
```

The models record `community_id`, `created_by_user_id`, `invite_link_id`, and `claimed_by_user_id`. The API surface exposes only `id`, `name`, and whether the provisional user has been claimed (the contact handle is never exposed).

### Visual distinction

Provisional users render with a `ProvisionalUserAvatar` widget (`app/lib/presentation/widgets/provisional_user_avatar.dart`) — a greyed-out circle with initials. Search results and attendee tiles show a *"Not yet on Ripls"* subtitle. This makes it immediately clear to the owner who has the app and who doesn't, creating a natural prompt to send invite links.

### Creating provisional users

Provisional users can be created in three places:

1. **Completion and search modals** — any `DarkPersonSearch` widget queries both registered members and existing provisional users. If the search returns no results, a *"Add [name] as a new person"* row appears at the bottom; tapping it creates a provisional user immediately.
2. **Past transfer modal** — the recipient picker in `PastTransferModal` follows the same search-first pattern.
3. **Manage Members screen** — community owners can create provisional users proactively (Settings → Governance → Manage Members → FAB).

### Profile sheet

Tapping a provisional user's avatar or name throughout the app opens `ProvisionalUserProfileSheet` (`app/lib/presentation/widgets/provisional_user_profile_sheet.dart`). The sheet shows:

- The provisional user's name and initials avatar.
- All experiences they've participated in, with dates and impact.
- Cumulative impact metrics across all activities.
- An *"Invite to Ripls"* button that generates or resends their personal invite link.

Provisional users with 3 or more activities get a visual highlight in the profile sheet indicating that their history makes them a strong invite candidate.

### Claiming (merge on registration)

Each provisional user has a personal invite link. When an invited person registers via that link, `server/provisional/merge.go:MergeIntoUser()` migrates all activity history — RSVPs, attendance records, transfer records (as giver and as recipient/borrower) — to the real account. The provisional user record is then marked `claimed`.

**Promote-on-verify (across communities).** Beyond the per-link claim, verifying a phone number promotes the person in every community at once. On phone registration (`PhoneRegister`), the shared `provisional.PromoteByPhone` helper gathers every unclaimed placeholder seeded for that E.164 number — in any community, via `provisional.FindUnclaimedByPhone` — joins the new account to each of those communities (skipping any it already belongs to), and runs `MergeIntoUser` for each. This deduplicates a contact across communities (several placeholders for one phone collapse into one real user) and works whether or not the invite link the person registered through was tied to a specific placeholder. Both the join and the merge are idempotent, so it composes safely with the per-link claim above and is best-effort — a failure on one community is logged and never blocks registration. The same promotion runs on email and OIDC registration: `EmailRegister`/`OIDCRegister` call `promoteProvisionalByEmail`, which uses `provisional.FindUnclaimedByEmail` to match placeholders by email handle across communities and merges each via the same idempotent path. Email promotion is **gated on proven ownership** (#2571): `promoteProvisionalByEmail` returns early unless the account carries `email_verified_at_unix_sec`, which only a completed email one-time code or an identity provider's `email_verified` claim can set. A legacy password registration proves nothing about the address and therefore promotes nothing — closing the former "email shadowing" hole, where anyone could register an address they did not own and absorb the placeholders keyed to it.

**Third trigger — adding a phone to an existing account.** `provisional.PromoteByPhone` is shared by a third caller: `UserService.AddPhoneNumber` (#2596), which lets an already-registered email/OIDC user attach a verified phone. The same cross-community reconciliation runs, so a phone-keyed placeholder seeded before that person had the app is claimed the moment they add their number — not just at first registration. See [registration_and_login.md](registration_and_login.md) → *Adding a Phone Number to an Existing Account*.

### Where provisional users appear in settings

**Settings → Governance → Manage Members** shows two sections: *Members* (registered users) and *People* (provisional users). From this screen, owners can:

- See which provisional users are claimed vs. unclaimed.
- Send a personal invite link to any unclaimed provisional user.
- Create new provisional users directly.

### In the share modal

Provisional user attendees of an experience appear in the share modal with a *"Send invite link"* action instead of the standard RSVP invite. This gives the owner a contextual prompt to invite them at the moment they're thinking about that activity.

---

## Participant resolution (completion modal)

When the AI extracts names from an experience prompt (e.g. *"Sarah and Tom"* → `mentioned_names: ["Sarah", "Tom"]`), `GenExperienceViewModel.resolveParticipantsForCompletion()` resolves each name against the community in order:

1. Search registered members by name via `communityRepo.searchMembers()`. If matches, add as pre-tagged attendees.
2. If no member match, search existing provisional users via `provisionalRepo.searchProvisionalUsers()`. If one matches, add as pre-tagged provisional attendee. If multiple match, add all as suggestions (user unchecks wrong ones in the modal).
3. Only if zero provisional matches exist, create a new provisional user.

This prevents duplicate provisional users accumulating for the same person across multiple activities.

Resolved participants arrive as `preTaggedUsers` and `preProvisionalUsers` on the completion modal.

---

## Completion and fulfillment modals

### Experience completion

`MarkCompletedModal` (`app/lib/presentation/screens/experience/mark_completed_modal.dart`) is a dark glassmorphic bottom sheet that handles the experience wrap-up:

- A live **impact bar** (money saved, quality time, CO₂) that scales with the number of confirmed attendees. `baseCount` recomputes dynamically as attendees are added or removed; when `baseCount == 0`, scale defaults to `1.0` so a solo activity still shows full impact.
- A **TAP TO CONFIRM** attendee list showing all YES RSVPs, extra members added via search, and provisional attendees — each toggleable.
- A **community quick-add grid** showing up to 5 community members not yet in the attendee list. Hidden while the search field is active.
- An **inline search field** that queries both registered members and provisional users. When no results match, a *"Add [name]"* row creates a new provisional user.
- On **Complete**, the server records attendance and generates the story synchronously. The client refreshes the feed and scrolls to the new story.

### Request fulfillment

`MarkFulfilledModal` (`app/lib/presentation/screens/request/mark_fulfilled_modal.dart`) replaces the old single-tap fulfillment flow with a confirmation modal that lets the owner identify who helped:

- Opens when the owner taps **Mark Fulfilled** on an active request.
- Pre-populated with the request's existing offerers (people who offered help), minus the owner themselves.
- Each helper is shown as a `DarkPersonTile` — tap to toggle confirmed/not confirmed.
- A `DarkPersonSearch` widget at the bottom allows adding anyone else from the community (or creating a provisional user), with the owner always excluded from suggestions.
- On **Mark Fulfilled**, the server stores `confirmed_helper_ids` on the request, uses those exact IDs to generate the story participants, and returns an updated `Request` with `offerers` populated from the confirmed helpers (not all who ever offered).
- After dismiss, `handlePostCreation()` refreshes the feed so the new story appears immediately.

---

## Shared completion widget library

The dark glassmorphic UI used across completion and fulfillment modals is extracted into reusable widgets at `app/lib/presentation/widgets/completion/`:

| Widget | Purpose |
|---|---|
| `DarkCheckCircle` | Animated check circle; also exports shared color constants (`kCompletionDark`, `kCompletionGreen`, etc.) |
| `DarkPlaceholderAvatar` | Gradient circle with initials for people not yet in the system |
| `DarkPersonTile` | Glassmorphic attendee/helper row with avatar, name, optional subtitle, and toggle |
| `DarkSearchResultTile` | Search result row with avatar, name, subtitle, and trailing action label |
| `DarkPersonSearch` | `ConsumerStatefulWidget` combining the community quick-add grid and inline search field with results |
| `completion_widgets.dart` | Barrel export for all of the above |

`DarkPersonSearch` owns all search state internally: the `TextEditingController`, debounce, community grid cache, and search results. Parent widgets only provide callbacks and exclusion sets.

---

## Feed refresh after completion

All completion paths — experience, giveaway, loan, request — call `PostCreationService.handlePostCreation()` after the modal dismisses. This:

1. Refreshes the feed in-place (invalidates cache, fetches fresh stories).
2. Navigates to tab 0 (the feed tab) if not already there.
3. Scrolls to the top so the new story is immediately visible.

Story generation is synchronous on the server for all completion types: the LLM call completes before the RPC returns, so the story exists in the database by the time the feed refresh runs.

---

## What is complete

| Capability | Status |
|---|---|
| Past-time detection on save (server) | Done |
| Owner auto-RSVP'd YES on past experience | Done |
| Past dates allowed in time picker | Done |
| Conditional "Mark Completed" button label | Done |
| AI participant name extraction from prompt | Done |
| LLM time extraction from natural language prompts | Done |
| Retrospective AI descriptions for past events | Done |
| Community member search by name (server + client) | Done |
| AI name extraction pre-populates attendees on completion modal | Done |
| Participant resolution: search existing provisional users before creating new | Done |
| Provisional user data model + server RPCs | Done |
| Provisional user creation in completion, search, and transfer modals | Done |
| Provisional user create/list/search via Manage Members | Done |
| Provisional-to-real merge on registration via invite link (all activity types) | Done |
| Manage Members screen in community settings | Done |
| Provisional user visual distinction (ProvisionalUserAvatar, "Not yet on Ripls" labels) | Done |
| Provisional user profile sheet (activities, impact, invite button) | Done |
| Provisional users in share modal with invite links | Done |
| Completion modal: attendee toggle, search, provisional create | Done |
| Impact metrics live-recalculation as attendees change | Done |
| Impact metrics division-by-zero guard | Done |
| Completion story generated synchronously (no polling) | Done |
| Post-completion feed refresh shows new story immediately | Done |
| Past-event navigation: feed first, then completion modal | Done |
| Attendee sheet read-only on completed experiences | Done |
| Single player mode: past giveaways (provisional recipient) | Done |
| Single player mode: past loans (provisional borrower, optional return) | Done |
| Request fulfillment confirmation modal with helper selection | Done |
| Confirmed helpers stored on request, used in story and display | Done |
| Shared completion widget library (DarkPersonSearch, DarkPersonTile, etc.) | Done |

---

## What is not complete

### Retroactive request logging

The lowest-priority item from the original plan. There is currently no way to back-fill a request that was fulfilled entirely offline (e.g., someone asked for something outside the app and it was delivered). This would require a provisional requester concept and a separate creation flow. Deferred until giveaway/loan retroactive logging is validated in production.

### Editing existing activities

A closely related capability is editing an activity after the fact: add a participant who was missed, correct the location, update the description. This is a separate workstream with its own permissions model (can the owner edit? attendees?) and notification requirements (do changes notify participants?). It builds on the same completion modal and provisional user infrastructure but should be tracked as a dedicated issue.
