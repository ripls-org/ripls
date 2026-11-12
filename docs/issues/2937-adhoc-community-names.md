# Issue #2937: Settings for ad hoc communities show blank community names

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2937
**Filed:** 2026-08-16 | **Last activity:** 2026-08-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** `docs/llms.txt`; anchors `client/architecture.md`,
`client/conventions.md`, `proto_conventions.md`, `users/priorities.md`; leaves
`ad_hoc_communities.md`, `client/i18n.md`, `client/sharing.md`,
`client/accessibility.md`, `client/testing.md`, `community_delete_and_leave.md`;
review dimensions accessibility, code_quality/tech_debt, architecture.

## Problem Statement

Settings → Communities renders every community the viewer belongs to as an
avatar + name + description row. It reads `CommunityItem.name` raw:

```dart
// app/lib/presentation/screens/settings/settings_hub_screen.dart:166-180
leading: CommunityAvatar(community: community, radius: 20),
title: community.name,
subtitle: community.description.isNotEmpty ? community.description : null,
```

Ad-hoc communities have **no name** — name-presence *is* the ad-hoc discriminator
(`docs/ad_hoc_communities.md`, `community.IsAdHoc`). So every ad-hoc row renders:

- a **blank title** (`community.name` is `""`),
- a **blank subtitle** (ad-hoc communities have no description either),
- a generic **"C" avatar** — `CommunityAvatar._getInitials` returns the literal
  `'C'` for an empty name (`community_avatar.dart:130`),
- and a **blank accessibility label** — the row's `Tappable(semanticsLabel: title)`
  (`settings_hub_screen.dart:190`) announces nothing to TalkBack/VoiceOver.

That is exactly the screenshot: five named communities, then seven identical,
unlabelled, indistinguishable "C" rows.

This is a **client render bug only**. The server already sends everything needed.
`ListCommunities` (`server/services/community/membership.go:239-270`) suppresses
host-only ad-hoc communities, and for every *remaining* nameless community fills
`member_count`, `member_preview_first_names` (up to 3 other members' first names),
and `origin_item_name` — precisely so the client can render group-chat style
without a name lookup. Nothing is missing on the wire.

The app **already has the canonical fix**, in two shared helpers that the Settings
hub simply doesn't call:

- `communityDisplayName(community, l10n)` (`app/lib/core/utils/community_display.dart`)
  — returns the name when present, otherwise a localized Oxford-comma people list
  ("You and Alex", "You, Alex, Sam, and 5 others"), falling back to
  `"N members"` when no preview names resolve. It is **never empty**. Covered by
  `app/test/core/utils/community_display_test.dart`.
- `GroupAvatar` + `communityGroupAvatarMembers` (`app/lib/presentation/widgets/group_avatar.dart`)
  — a 2–4 face cluster standing in for the missing community photo.

The Workshop switcher already pairs both (`workshop_switcher_dropdown.dart:143-210`),
and the sharing community picker uses them too (`sharing/community_list_item.dart:40-46`).
Settings and the whole governance chain behind it were missed.

**Ambiguity in the report.** The title says "blank community names"; the body says
*"The should be styled like to the community list on the people tab"* (sic — the
subject is elided, and the sentence has a typo). Read together with the
screenshot, the ask is: **make the Settings community rows render ad-hoc groups
the way the People tab does** — a real label and a group avatar instead of a blank
row and a "C". The People tab is the Directory (`DirectoryScreen`,
home tab index 3, `home_screen.dart:559-566`).

**One complication worth surfacing before implementing:** the People tab is not
actually a clean reference. `DirectoryScreen` does *not* use the shared helpers —
`directory_view_model.dart:70-76` has its own private `_derivedGroupName`, which
(a) prefers `originItemName` over the member rollup, the **opposite** priority
from `communityDisplayName`, (b) can still return `''` when there are no preview
names, and (c) emits **hardcoded English** — `'${...} & ${n} others'` — a live
i18n violation on the very surface the issue names as the model. So "style it like
the People tab" means adopting the People tab's *row shape*, while deliberately
**not** adopting its name derivation. See Open Questions.

### UI investigation

`ripls-ui-investigate` was **not run**. The skill exists to bisect an ambiguous
failure to a layer; here the layer is not in question. The defect is a single
unambiguous code read — `settings_hub_screen.dart:170` passes `community.name`
straight to `Text`, and `community_avatar.dart:130` returns `'C'` for an empty
name — and the server side is confirmed to populate the substitute fields. A
hermetic stack boot would reproduce a blank row the screenshot already proves,
against seed data rather than the reporter's. The one thing it genuinely can't
answer (do prod ad-hoc rows actually carry preview names, or will they fall back
to "N members"?) is a **data** question; see Open Questions for the prod query
that does answer it.

## Context from Comments

**There are no comments.** This is an auto-filed `user-feedback` issue submitted
through the in-app reporter by user2@example.org (Ripls 0.30.0, Android 17, Pixel 9
Pro, PROD, build 9709). The screenshot carries most of the signal; the one-line
body is terse and partially garbled.

The absence of discussion is not blocking here — the report is reproducible by
inspection and the intended rendering is already established by
`docs/ad_hoc_communities.md` § Surfacing:

> The client renders the empty name group-text style ("Name, Name & N others")
> with an "add a name" nudge to promote it; that rendering is DISPLAY-1's job.

DISPLAY-1 landed the helpers and wired the Workshop switcher, sharing picker, and
Directory. Settings was never wired. This issue is the tail of that epic item, not
new design.

The one thing no existing doc settles is the title-vs-subtitle priority question
(members-first vs origin-item-first), because the two shipped surfaces disagree.
That needs a call from Bryan.

## Blast radius: the blank name doesn't stop at the row

Tapping a Settings community row opens `GovernanceScreen`, and the empty name
propagates through the entire governance chain. Every one of these is user-visible
today for an ad-hoc community:

| Site | Renders |
|---|---|
| `governance_screen.dart:80` | App-bar title. `_community?.name ?? 'Community Settings'` — the name is `""`, not `null`, so `??` never fires → **blank app bar**. (The fallback is also a hardcoded English string.) |
| `governance_screen.dart:244` | `showDangerZoneChoiceModal(communityName: '')` |
| `leave_community_screen.dart:56,109` | `communityLeaveBody('')` / `communityLeaveSuccess('')` — "Leave ?" |
| `owner_leave_confirm_screen.dart:68,123` | Leave-with-handoff confirm copy |
| `sole_member_leave_explainer_screen.dart:48` | Sole-member explainer copy |
| `delete_community_screen.dart:64,116` | `communityDeleteBody('', n)` / `communityDeleteSuccess('')` |
| `manage_members_screen.dart:272,294` | `InviteSheet(communityName: '')` — blank name in **invite** copy |
| `community_helper.dart:34` | `communitiesTitle` returns `communities.first.name` → blank Workshop switcher pill when the viewer's only community is ad-hoc |

`community_edit_screen.dart:653` (`initialName: community.name`) is **correct** as
is — that's the promote-by-naming field, and an empty text field is what it should
show.

Fixing only the list row leaves a user who taps it staring at a blank app bar and,
one step further, a "Leave ?" dialog. Scoping the fix to the row alone would be
narrowing the report.

## Potential Solutions

### Option A: Point-fix the Settings row
- **Approach:** In `settings_hub_screen.dart`, swap `community.name` →
  `communityDisplayName(community, context.l10n)` and, when nameless, swap
  `CommunityAvatar` → `GroupAvatar(members: communityGroupAvatarMembers(...))`.
  Feed the derived label into `semanticsLabel` too. Nothing else changes.
- **Pros:** Smallest possible diff, ~20 lines in one file. Zero new abstractions.
  Reuses helpers that are already tested and localized. Closes the literal report.
- **Cons:** Leaves the whole governance chain blank — the row now has a name but
  tapping it still lands on a blank app bar and blank leave/delete/invite copy.
  Leaves a fourth divergent nameless-community rendering in the tree (Settings,
  Workshop, Directory, sharing picker) with nothing preventing a fifth.
- **Effort:** Small.

### Option B: Fix the Settings row + the governance chain behind it
- **Approach:** Option A, plus resolve the display name **once** at the boundary
  and thread it through: give `GovernanceScreen` and its leave/delete/invite
  children a resolved display name rather than `community.name`. Concretely,
  extract a small `CommunitySettingsRow` widget (row shape + derived label +
  avatar + semantics) and have `governance_screen.dart:80` and the copy call sites
  use `communityDisplayName` instead of the raw field.
- **Pros:** The tap target actually works end to end. Kills eight blank-copy sites
  in one pass, including invite copy that goes to *other people*. Still bounded —
  no proto change, no server change, and exactly **one** new ARB key
  (`communityGenericGroupLabel`, for the outbound-invite case); the display keys
  `workshopGroupFromItem`, `workshopChooserMembersCount`, and `directoryMembers`
  all already exist.
- **Cons:** Touches ~8 files instead of 1. Slightly more review surface. Doesn't
  yet stop a ninth site from being written.
- **Effort:** Medium.

### Option C: Option B + converge the divergent renderings and guard the class
- **Approach:** Option B, plus (1) make `_derivedGroupName` in
  `directory_view_model.dart` delegate to `communityLabel`, deleting the
  hardcoded-English `& N others`; (2) have `CommunityAvatar` itself fall back to
  the group cluster for a nameless community, so the `'C'` default becomes
  unreachable; (3) add a regression test asserting no nameless community renders
  an empty label on any of these surfaces.
- **Pros:** Removes the bug *class*, not the instance. Fixes a real unlocalized
  string on the People tab. Makes `CommunityAvatar` correct by construction, so
  future call sites can't reintroduce "C". Directly serves the product constraint
  that a user's community count now grows with items touched, not the "1-5" the
  pre-pivot code assumed (`ad_hoc_communities.md` § Lifecycle) — these rows will
  only get more numerous.
- **Cons:** Changes the People tab's rendering, which the reporter named as the
  *reference* — the derived labels there will shift from "Dinner at Este" to
  "You, Alex, and Sam" unless we keep origin-first (see Open Questions). Widest
  diff; a `CommunityAvatar` change touches every community avatar in the app and
  needs its own visual check.
- **Effort:** Medium–large.

## Recommendation

**Option C, staged so that Option A ships first and independently.**

The reasoning:

1. The issue as reported is Option A, and Option A is a genuinely small, low-risk
   diff against helpers that are already correct, localized, and tested. It should
   not wait on anything.
2. Option B is not gold-plating — it's the rest of the same defect. A row you can
   now read that opens a screen you still can't is a worse experience than before,
   because it invites the tap. The invite-copy site (`InviteSheet(communityName: '')`)
   in particular sends blank text to *other people*, which is a bigger user-facing
   failure than the Settings list itself.
3. Option C's step (2) — making `CommunityAvatar` cluster-fallback for a nameless
   community — is what actually prevents recurrence. Right now every new call site
   that writes `CommunityAvatar(community: c)` silently gets a "C" for ad-hoc
   groups, and no lint catches it. The `'C'` literal in `_getInitials` is the root
   cause of the avatar half of this bug at every surface simultaneously.
4. Option C's step (1) is small and fixes a shipped i18n violation
   (`directory_view_model.dart:75`) that would otherwise be enshrined by this
   issue naming that surface as the model to copy.

What I'd **not** do: build a generic "community row" component shared across
Settings, Workshop, Directory, and the sharing picker. Those four rows have
genuinely different affordances (chevron / checkmark / toggle / name-chip + time-ago)
and different visual densities. Converging the *derivation* (name, avatar members)
is the win; converging the *chrome* would be over-engineering against
`CLAUDE.md` § "Minimal Dependencies" and would balloon the diff.

## Open Questions

- **Title = member rollup or origin item?** The two shipped surfaces disagree:
  the Workshop switcher titles the row "You, Alex, and Sam" with
  "Group from Dinner at Este" as subtitle; the Directory titles it
  "Dinner at Este" and drops the people entirely.
  — **Recommendation: adopt the Workshop order** (people as the title, origin as
  the subtitle) in Settings, and change the Directory to match. Rationale:
  `communityDisplayName` is the localized, tested helper and is *structurally
  incapable* of returning an empty string, whereas `_derivedGroupName` returns
  `''` when there are no preview names — i.e. the Directory can reproduce this
  exact bug today for a group whose other members have no name set. Origin-item
  names are also frequently long and truncate badly in a one-line row, and a
  Settings row needs two identifying signals anyway (who / what it came from),
  which the two-line row already affords. **This contradicts a literal reading of
  "styled like the people tab" — flag for Bryan before implementing.**

- **Do prod ad-hoc rows actually carry preview names?** If the other members are
  provisional users with no `name` set, `firstFewMemberFirstNames` returns empty
  and every row falls back to "3 members" — better than blank, but still not
  distinguishing. — **Recommendation:** answer it with data before Phase 1 lands,
  not by guessing: query prod for nameless communities with `member_count > 1`
  and count how many have ≥1 other member with a non-empty name. If the ratio is
  poor, the origin-item subtitle (Phase 1, task 3) carries the identification
  load and its priority rises.

- **Should ad-hoc groups get their own section in Settings?** Today they're
  interleaved into one "COMMUNITIES" card, and with community count now growing
  per item touched that card grows unbounded. **The list is fully eager**, and
  the outer `ListView` is not the reason: `buildSettingsCard` wraps its children
  in a plain `Column` (`settings_widgets.dart:32-34`), so every community row is
  built regardless of the sliver's laziness. Switching the outer widget to
  `ListView.builder` alone would change nothing — the card itself has to become
  builder-backed.
  — **Recommendation: split the sections now, defer virtualization.** Keep
  "COMMUNITIES" for named ones and add a "GROUPS" section below, mirroring
  `DirectoryKind.community` vs `DirectoryKind.group`. It keeps the communities
  with real governance actions at the top where they were, and it makes the
  eager-build cost visible. File the virtualization follow-up with the `Column`
  mechanism named, so whoever picks it up doesn't "fix" the `ListView` and
  measure no change.

- **Does the group-cluster avatar multiply media fetches?** Asked and answered —
  **no**, and it should not be re-raised. `groupAvatarMembers`
  (`group_avatar.dart:103-112`) synthesizes preview-only members as
  `User(name: ...)` with **no `mediaId`**, and `UserAvatar` only watches
  `mediaUrlProvider` when `user.mediaId.isNotEmpty` (`user_avatar.dart:55`).
  Only the viewer's own circle resolves media, and it's the same media id on
  every row → one cached provider entry for the whole list.

- **Does the ad-hoc row need the "Name this group" affordance in Settings?**
  The Directory has a `directory-name-group-chip`; the Workshop switcher shows a
  `workshopNameGroupPrompt` subtitle. — **Recommendation: no chip in Phase 1.**
  Settings → row → Governance → "Update Profile" *is* the promote path, and it's
  one tap away. Adding a fourth promote entry point is scope the issue didn't ask
  for. Revisit if the Groups section proves to be where people go to name things.

- **Is `governance_screen.dart`'s hardcoded English in scope?** That file has
  `'Update Profile'`, `'Manage Region'`, `'View & Invite Members'`,
  `'Manage Privacy'`, `'Leave Community'`, `'Delete Community'`, and the
  `'Community Settings'` fallback — none localized. `scripts/check_i18n.js` only
  greps a narrow literal allowlist, so nothing catches them.
  — **Recommendation: out of scope; file a separate issue.** Phase 2 should
  replace the `'Community Settings'` fallback *only* because it is on the blank-name
  path. Migrating the other six is an `#2827`-class copy migration and shouldn't
  ride along.

## Implementation Plan

### Phase 1: Settings hub renders ad-hoc groups
**Goal:** The reported screen stops showing blank rows. Shippable alone.
**Tasks:**
- [ ] Extract `_buildCommunityItem` into a `CommunitySettingsRow` widget under
      `app/lib/presentation/widgets/settings/`, so the row is testable without
      booting the whole hub (`client/conventions.md` § Flutter Screens).
- [ ] Title from `communityDisplayName(community, context.l10n)`; pass the same
      derived string to `Tappable(semanticsLabel:)` so the row is announced
      (`client/accessibility.md`).
- [ ] Add a pure `communitySubtitle({name, description, originItemName,
      memberCount, l10n})` next to `communityDisplayName` in
      `core/utils/community_display.dart`, returning: description when present,
      else `l10n.workshopGroupFromItem(originItemName)` when origin resolves,
      else `l10n.workshopChooserMembersCount(memberCount)`. Put the branching
      here, not in the widget — it is the testable half
      (see the test task below).
- [ ] **Title needs `maxLines: 1` + `TextOverflow.ellipsis`.** The current row
      title (`settings_hub_screen.dart:203-209`) has neither — only the subtitle
      does. A derived label like "You, Alex, Sam, Bo, and 5 others" at 200% text
      scale overflows the row (accessibility review § Visual Accessibility →
      Text Scaling).
- [ ] Avatar: `GroupAvatar(members: communityGroupAvatarMembers(community:, viewer:),
      diameter: 40, ringColor: AppColors.cardBackground(context))` when the member
      list is non-empty, else keep `CommunityAvatar`.
- [ ] Split the section into "COMMUNITIES" (named) and "GROUPS" (nameless), each
      suppressed when empty.
- [ ] Add `semanticsIdentifier: 'settings-nameless-row'` on ad-hoc rows, matching
      the `workshop-nameless-row` precedent, so e2e can target them
      (`client/testing/semantics_identifiers.md`).
- [ ] Update `app/lib/presentation/widgets/settings/README.md` for the new file
      (CLAUDE.md § "Every multi-file directory … must have a README.md").
- [ ] **Tests, unit-first.** Extend `app/test/core/utils/community_display_test.dart`
      with `communitySubtitle` cases (description / origin-item / member-count
      fallback) — it already runs against `AppLocalizationsEn()` with no widget
      tree. Add **one** widget smoke test for `CommunitySettingsRow` asserting a
      nameless community renders a non-empty title and a cluster avatar.
      `client/testing.md:24` is explicit that widget tests are "slower, more
      brittle, and often test framework behavior rather than app logic" — the
      derivation is where the defect lives, so that is where the coverage goes.

**On reusing `workshop*` keys in a Settings screen.** `client/i18n.md` § Key
Naming assigns the `settings*` prefix to this screen, so `workshopGroupFromItem`
and `workshopChooserMembersCount` are off-convention here. **Reuse them anyway,
and don't mint `settings*` twins.** Minting a second key with identical copy is
precisely the failure #2925 diagnosed: `workshopChooserMembersCount` and
`directoryMembers` are *already* two keys holding the same ICU string, and a
third would guarantee they drift. If the prefix bothers a reviewer, the correct
move is a rename to a neutral `community*` prefix (`communityGroupFromItem`,
`communityMembersCount`) across all three call sites in one pass — a mechanical
change with an ARB-parity update in both locales — not a duplicate.

**Validates:** That the shared helpers alone close the reported defect, and that
`member_preview_first_names` is populated well enough in practice for the labels
to be distinguishing (answering Open Question 2 empirically on a real device).

### Phase 2: The governance chain behind the row
**Goal:** Tapping an ad-hoc row leads somewhere legible, and no blank name escapes
into copy sent to other people.
**Tasks:**
- [ ] `governance_screen.dart:80` — resolve via `communityDisplayName`; replace the
      hardcoded `'Community Settings'` fallback with an ARB key (only reachable when
      `_community == null`).
- [ ] Thread the resolved name into `showDangerZoneChoiceModal`,
      `LeaveCommunityScreen`, `OwnerLeaveConfirmScreen`,
      `SoleMemberLeaveExplainerScreen`, `DeleteCommunityScreen`.
- [ ] **`InviteSheet` gets a generic public label, NOT the member rollup.**
      `manage_members_screen.dart:272,294` feeds `InviteSheet.communityName`, and
      `invite_sheet.dart:125-130` builds **outbound share text** —
      `'Join $_selectedCommunityName! $_inviteUrl'` and the share `subject` —
      delivered by SMS/share-sheet to people who are **not members**. Pushing
      `communityDisplayName` here would put members' first names in front of
      non-members. `docs/ad_hoc_communities.md` § Surfacing already sets this
      rule for the public `/go/` SSR landing, where
      `communityDisplayName` (`server/services/web/service.go:61-66`)
      substitutes a generic **"a Ripls group"** for exactly this reason. Mirror
      it: add one new ARB key (`communityGenericGroupLabel`, both locales — the
      **only** new key this plan needs) and use it whenever the community is
      nameless. Do not reuse the people rollup on any outbound surface.
- [ ] `community_helper.dart:34` — `communitiesTitle` resolves the single-community
      case through `communityDisplayName` instead of `.name`. This one is
      viewer-facing only (the Workshop switcher pill), so the rollup is correct
      here.
- [ ] Audit each remaining site for viewer-facing vs outbound before substituting:
      the app-bar title, leave/delete confirms, and the danger-zone modal are all
      viewer-facing (rollup is fine); anything that produces text the viewer
      sends onward takes the generic label.
- [ ] Leave `community_edit_screen.dart:653` (`initialName`) alone — an empty field
      is correct on the promote path. Add a one-line comment saying so, so the next
      sweep doesn't "fix" it.
- [ ] Tests: extend `community_helper` tests for the nameless single-community case;
      widget-test that the governance app bar renders a non-empty title for a
      nameless community.
**Validates:** That resolving at the boundary (rather than at each `Text`) is
sufficient, and surfaces any call site that holds only an id + name and can't
resolve — those would need `communityDisplayNameFromList`.

### Phase 3: Close the class
**Goal:** A future call site cannot reintroduce a blank name or a "C" avatar.
**Tasks:**
- [ ] `directory_view_model.dart:70-76` — delete `_derivedGroupName` and its
      hardcoded-English `' & '` / `' others'`, delegating to `communityLabel`.
      **This is not a one-line swap, and the plan's first draft understated it.**
      Two constraints bind:
      1. `client/i18n.md` § "Viewmodels never resolve strings" rules out plumbing
         `AppLocalizations` into `directoryEntriesProvider` — that option is off
         the table, not a tradeoff. (`DirectoryEntry.displayName` is itself a
         viewmodel-resolved string today, i.e. a standing violation this fixes.)
      2. `DirectoryView.arrange` **sorts, filters, and A–Z-buckets on
         `displayName`** (`directory_entry.dart:110,115-116,124-130,167-174`), so
         the label cannot simply move into the row widget or ordering breaks.

      The shape that satisfies both: have `directoryEntriesProvider` carry the
      *typed* fields (`name`, `memberCount`, `memberPreviewFirstNames`,
      `originItemName`) on `DirectoryEntry`, and have `DirectoryScreen.build()`
      — which has `context.l10n` — resolve each entry's `displayName` before
      handing the list to `DirectoryView.arrange`. `arrange` stays pure and
      unchanged.
- [ ] Update `directory_view_model_test.dart` and `directory_screen_test.dart` for
      the changed labels; `_cluster()` in `directory_screen.dart:405-432` currently
      re-splits the derived label on `[,&]` to recover names — replace that with
      `groupAvatarMembers`, which has the names directly rather than reverse-parsing
      a display string (and whose `'#'` fallback, like `CommunityAvatar`'s `'C'`,
      is the magic-string form of this same bug).
- [ ] `community_avatar.dart` — for a nameless `CommunityItem`, render the
      `GroupAvatar` cluster instead of `_getInitials` → `'C'`. Keep the literal
      `'C'` only for the `community == null && name == null` case. It is already a
      `ConsumerWidget`, so it can resolve the viewer itself via
      `ref.watch(authStateProvider.select((s) => s.user))` — no call-site
      signature change. Visually check every `CommunityAvatar` call site (~10)
      for size/ring regressions, including the `showBadge: true` Stack overlay.
- [ ] Add a test asserting `communityDisplayName` is non-empty for every shape of
      `CommunityItem` including all-empty preview names, as the invariant the other
      surfaces now rely on.
- [ ] **Refresh the docs this changes.** `docs/ad_hoc_communities.md` § Surfacing
      still describes the client rendering as `"Name, Name & N others"` — already
      stale against the shipped `communityDisplayName` ("You, Alex, and Sam"), and
      Phase 3 changes the Directory on top of it. `docs/workshop.md:34` references
      the Directory's nameless-group chip. Update both and advance their
      `freshness:` stamps, per `docs/context_map.md` § Prose freshness — otherwise
      the nightly `claude_refresh_docs` ratchet opens a PR doing it for us.
**Validates:** Whether "correct by construction" holds at the avatar layer, or
whether enough call sites want the flat circle that the fallback has to stay
opt-in.

### Phase 4 (follow-up issues, not this PR)
- Localize the remaining six hardcoded `GovernanceScreen` action titles
  (`#2827`-class copy migration).
- Virtualize the Settings communities list. **File this with the mechanism
  named:** the blocker is `buildSettingsCard` → `Column(children:)`
  (`settings_widgets.dart:32-34`), not the enclosing `ListView`. Swapping the
  outer widget to `ListView.builder` measures no improvement; the card has to
  become builder-backed.
- Collapse the duplicate member-count keys (`workshopChooserMembersCount` /
  `directoryMembers`, identical ICU strings) onto one neutral `community*` key.

## Related Issues
- **#2492** (epic, phone-first pivot) — COMM-1 introduced ad-hoc communities;
  DISPLAY-1 owns exactly this rendering and shipped the helpers Settings never
  called. This issue is DISPLAY-1's tail.
- **#2568** (Directory) — built the People-tab rows the reporter points at, with
  its own private derivation rather than the shared helper. Phase 3 reconciles them.
- **#2867** (per-community → per-user realtime) — the first thing broken by
  "community count now grows with items touched." The unbounded Settings list is
  the same pressure on a different surface.
- **#2827 / #2835** (in-app copy migration) — the home for the hardcoded
  `GovernanceScreen` strings; deliberately not merged into this issue.

---

## Audit notes

Audited against the documentation context map. Findings applied in place:

1. **PII on outbound invite copy — the one finding that changes the design.**
   The first draft told Phase 2 to substitute `communityDisplayName` at
   `manage_members_screen.dart:272,294`. That feeds `InviteSheet`, whose
   `invite_sheet.dart:125-130` composes **share text sent to non-members**
   (`'Join $_selectedCommunityName! $_inviteUrl'`). The member rollup would have
   leaked members' first names outside the group — the exact rule
   `ad_hoc_communities.md` § Surfacing states for the public `/go/` landing,
   where the server substitutes "a Ripls group". Phase 2 now splits
   viewer-facing sites (rollup) from outbound sites (generic label) and adds the
   single new ARB key that requires.

2. **`client/i18n.md` § "Viewmodels never resolve strings".** Phase 3 offered
   "plumb `AppLocalizations` into the provider" as an option; that is a documented
   violation, now removed. The draft also missed that `DirectoryView.arrange`
   sorts/filters/A–Z-buckets on `displayName`, so the derivation cannot simply
   move into the row widget. Phase 3 now specifies the shape that satisfies both.

3. **Performance § "List Performance".** The draft blamed the outer
   `ListView(children:)`. The real blocker is `buildSettingsCard` →
   `Column(children:)` (`settings_widgets.dart:32-34`), which defeats sliver
   laziness outright. Corrected, and the follow-up now names the mechanism so it
   isn't "fixed" at the wrong layer.

4. **Accessibility § "Text Scaling".** Added the missing `maxLines`/`ellipsis` on
   the row title (`settings_hub_screen.dart:203-209` has neither today) — a
   derived people-list label overflows at large text scale.
   `require_l10n_for_semantic_labels` is satisfied: the plan passes a derived
   variable, and the rule only rejects string *literals*
   (`check_a11y.js:274-275`). No gate catches the *runtime*-empty label this
   issue is about, which is why it survived to production.

5. **`client/testing.md:24` / conventions § Client Testing.** The draft was
   widget-test-first. Restructured to unit-first: the subtitle branching becomes
   a pure `communitySubtitle` in `community_display.dart`, tested where
   `communityDisplayName` already is, with one widget smoke test remaining.

6. **`client/i18n.md` § Key Naming.** Reusing `workshop*` keys on a Settings
   screen is off-prefix; the plan now states why reuse still beats minting
   `settings*` twins (the `workshopChooserMembersCount` / `directoryMembers`
   duplicate pair is the drift already in the tree) and names the correct
   alternative if a reviewer objects.

7. **Tech-debt § Documentation Debt + the doc-freshness ratchet.** The draft had
   no doc task. `ad_hoc_communities.md` § Surfacing and `workshop.md:34` both
   describe rendering this change alters; Phase 3 now updates them and advances
   their `freshness:` stamps.

8. **CLAUDE.md § READMEs.** Added the `app/lib/presentation/widgets/settings/README.md`
   update for the new `community_settings_row.dart`.

9. **Pre-empted a false alarm.** Recorded in Open Questions that the group-cluster
   avatar does *not* multiply media fetches — preview-only members carry no
   `mediaId` and `UserAvatar` only watches `mediaUrlProvider` when one is present.

**Verified compliant, no change needed:** colors route through `AppColors.*(context)`;
no new navigation (so `lint:nav` and `SwipeToCloseMixin` rules are untouched);
no proto or RPC change, so `proto_conventions.md` is not engaged; no new
dependency; no server change, so the Go anchors are not engaged; file sizes stay
far under the 1,000-line gate; no TODOs introduced, so `lint:todo-refs` is unaffected.

**Docs consulted:** `docs/llms.txt`; `docs/client/architecture.md` ⚓;
`docs/client/conventions.md` ⚓; `docs/testing/architecture.md` ⚓;
`docs/client/i18n.md`; `docs/client/accessibility.md`; `docs/client/testing.md`;
`docs/client/testing/semantics_identifiers.md`; `docs/client/sharing.md`;
`docs/ad_hoc_communities.md`; `docs/community_delete_and_leave.md`;
`docs/workshop.md`; `docs/users/priorities.md`;
`docs/reviews/accessibility_review_prompt.md`,
`docs/reviews/code_quality_review_prompt.md`,
`docs/reviews/tech_debt_review_prompt.md`,
`docs/reviews/testing_review_prompt.md`,
`docs/reviews/performance_review_prompt.md` (Areas to Review);
`docs/reviews/architecture_review_prompt.md`. Considered and not engaged:
`docs/proto_conventions.md` ⚓ (no proto change), the server anchors,
`docs/reviews/security_review_prompt.md` (no auth/secret surface — the PII
finding above came from the domain doc, not this rubric),
`docs/reviews/api_design_review_prompt.md`, `docs/reviews/dependency_review_prompt.md`.
