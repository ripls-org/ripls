---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: "Name-this-group promote nudge (epic #2492, DISPLAY-1) — a host turns a nameless per-item ad-hoc community into a real, named community through the community create/AI modal running in promote mode (final submit → UpdateCommunity), reachable from two owner-facing surfaces: the Directory (People tab) nameless-group row's 'Name' chip (#2568 Decision 5, superseding the retired Workshop community-switcher row) and the event roster's 'Name this group' link."
  globs: [app/lib/presentation/screens/directory/directory_screen.dart, app/lib/presentation/widgets/workshop/workshop_switcher_dropdown.dart, app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart, app/lib/presentation/screens/communities/community_creation_modal.dart, app/lib/presentation/screens/communities/community_preview_modal.dart, app/lib/presentation/viewmodels/gen_community_view_model.dart, server/services/community/membership.go, server/services/community/lifecycle.go, e2e/tests/workflows/community-promote-nudge.spec.ts]
  triggers: [name-this-group, promote-community, ad-hoc-community, nameless-community, workshop-switcher, whos-in, display-1, community-promotion]
  lens: [workflow, client, server]
freshness:
  verified_commit: "69c4ab218"
  verified_on: "2026-07-19"
---
# Name-This-Group: Promoting an Ad-Hoc Community

The phone-first pivot (epic #2492; canonical plan `docs/issues/2492-phone-first-pivot.md`) gives
every item its own **nameless, per-item "ad-hoc" community** — the item's audience. A nameless
community is intentionally second-class: it renders group-chat style (by its members' first names)
and carries no standing identity. **DISPLAY-1** lets the owner *promote* such a community into a real,
persistent one by giving it a **name, a photo, and a description** — and it does so by reusing the
full community **create / AI-assisted modal** rather than a bare rename, so the promoted community
gets the same rich identity a directly-created one does.

## The promote mechanism (one branch in the create flow)

Promotion reuses the entire community-creation flow — text/image input → AI generation → editable
preview — and changes exactly one thing: the final submit. `CommunityCreationModal.show(context,
promoteCommunityId: …)` runs the modal in **promote mode**; the only behavioural branch is
`GenCommunityNotifier.createCommunity()`
(`app/lib/presentation/viewmodels/gen_community_view_model.dart`), which calls **`UpdateCommunity`**
(name + description + media on the existing id) instead of `CreateCommunity` when seeded with an
`existingCommunityId`, and skips the create-only landing side effects (feed invalidation, tab nav).

Server-side, the first unnamed→named transition in `UpdateCommunity`
(`server/services/community/lifecycle.go`) emits **`COMMUNITY_NAMED`** and the community becomes a
real, persistent one (subject to the 32-member cap going forward, grandfathered per COMM-1).
`UpdateCommunity` is **member-gated**, but the UI only surfaces the nudge to the **owner**.

The deterministic AI provider (`--mock-ai-provider`,
`server/ai/provider_e2e.go` → `server/ai/provider_mock.go` `defaultGenerateCommunity`) supplies a
non-empty title **and** description, so the modal reaches a confirmable preview without a live LLM —
which is what makes the flow e2e-testable.

## The two owner-facing surfaces

1. **Directory (People tab) nameless-group row "Name" chip.** The People tab's one-list-of-everyone
   (`DirectoryScreen`) sources its group rows from `communitiesProvider` — the same `ListCommunities`
   (`server/services/community/membership.go`) call the legacy Workshop switcher used, which sorts
   **nameless communities after named ones** and **suppresses host-only single-member** ones (an
   audience of one has nothing to name yet). A nameless community the viewer **owns** (and that has a
   real audience — ≥2 members) renders group-text style (by member names) with a sibling **"Name"**
   chip (`app/lib/presentation/screens/directory/directory_screen.dart`, semantics id
   `directory-name-group-chip`, #2568 Decision 5) that opens the promote modal; tapping the rest of the
   row still just opens the community. Non-owners get no chip on their nameless rows.
   **This supersedes the Workshop community-switcher row**
   (`app/lib/presentation/widgets/workshop/workshop_switcher_dropdown.dart`, semantics id
   `workshop-name-group-row`, accessible name "Name the group {preview}"): the Workshop tab is retired
   from the default bottom nav behind `directoryEnabledProvider` (on by default — see
   [docs/workshop.md](../workshop.md)). The switcher code and its promote-modal wiring still exist and
   still work when that flag is off, but a default install and the e2e both reach promotion through the
   Directory chip now.

2. **Event roster ("Who's In") link.** When an event's origin per-item community is still nameless, has
   **at least one member besides the host (≥2 members)**, and the viewer is the **host**, a
   **"Name this group…"** link renders between the attendee list and the communities section
   (`app/lib/presentation/screens/experience/widgets/experience_pitching_in_screen.dart`, semantics id
   `roster-name-group-link`; the member count comes from `SharedCommunity.member_count` on the
   `GetExperience` response). Tapping it promotes that origin community; after success the roster's
   group-text label flips to the new name and the link disappears.

Once named, the community's own page (`CommunityPublicScreen`, pushed from the Directory row) carries an
**Invite** action as the fourth icon in its header row — Message / Plans / Library / Invite — opening the
group's [`InviteSheet`](../../app/lib/presentation/screens/communities/invite_sheet.dart) (link + QR +
remaining-spots count). This is the surface for growing the *group* rather than any one item; before it
the only routes to that sheet hung off something else (the feed's community card, the home header,
Settings → Manage members), which put "add someone to this group" behind a search for where it lived.

**Both promote surfaces gate on a real audience (≥2 members)** — naming an audience of one is premature, so the
nudge only appears once a second person is invited (the Directory gate is server-side suppression; the
roster gate is the `member_count` check). **Inviting a person refreshes the community list + the open
roster live** (`invite_members_sheet.dart` calls `reloadCommunities()` + `scheduleRefresh()`), so a real
user never has to reload to see the nudge appear. Nameless communities render group-text style from the
**viewer's** perspective — "**You and** Ada, Sam …" (`community_display.dart` `communityLabel`) — so even
a two-person group reads as a group.

Both open the same modal; both end by `UpdateCommunity`-ing the **per-item community of the item the
host just made**.

## Workflow Examples

Exercised by the Playwright e2e (`e2e/tests/workflows/community-promote-nudge.spec.ts`) against the
deterministic AI provider, driving the **real Flutter Web UI** for create + promote. Both flows assert
success by reading the community back via `GetCommunity` (its `name` is now non-empty).

### Example A — promote from the event roster ("Name this group")

**Preconditions:** a registered host who already belongs to a **named community** ("Trail Crew") with
two other members; no live LLM (mock AI provider). (The named community gives the roster a populated
*Communities* section, so the still-nameless per-item community reads as the odd one out.)

**Steps:**
1. **Host** (web UI): creates an event via unified create (prompt → Generate → **Save Event**); the
   server provisions its host-only per-item community. The spec reads the event + its per-item
   community id back via `ListMyExperiences` (`experiences[0].sharedCommunityIds[0]`) before sharing
   anything else.
2. **Host**: a second person joins the **per-item** community (so it has ≥2 members → the nudge unlocks),
   and the event is shared with **Trail Crew** (**Invite community**) so its members roll up under a named
   community in the roster.
3. **Host**: opens the event (`/event/:id`) → **View attendees** → the Who's-In roster, which now shows
   **Trail Crew** in the Communities section *and* the **"Name this group…"** link for the per-item one.
4. **Host**: taps the **"Name this group…"** link → the create/AI modal opens in promote mode.
5. **Host**: Text → prompt → **Generate** → the preview fills in → confirms (**"Name this group"**).
6. **Server**: `UpdateCommunity` names the origin community and emits `COMMUNITY_NAMED`.

**Postconditions:**
- The event's per-item community now has a non-empty name (and description).
- The roster's "Name this group" link is gone; the audience reads under the real name. (The per-item
  community stays *out* of the Communities section by design — its members are the listed attendees —
  so the section continues to show the separately-shared Trail Crew.)

### Example B — promote from the Directory (People) tab

**Preconditions:** a registered host; no live LLM (mock AI provider).

**Steps:**
1. **Host** (web UI): creates an event (as in A); its per-item community starts **host-only** and is
   therefore **suppressed** from the directory list.
2. **Host**: invites a person via the share sheet's **"Invite people"** (in-app), so the per-item
   community gains a second member and stops being suppressed — the invite refreshes the community list
   live, so the directory reflects it without a reload.
3. **Host**: reloads home (`/`) and taps the **People** tab; the now multi-member community renders as
   a nameless group row (group-text style, e.g. "You and {name}") with the sibling **"Name"** chip.
4. **Host**: taps the **"Name"** chip → the create/AI modal opens in promote mode.
5. **Host**: Text → prompt → **Generate** → confirms.
6. **Server**: `UpdateCommunity` names the community and emits `COMMUNITY_NAMED`.

**Postconditions:**
- The community now has a non-empty name; the directory row shows that name and the **"Name"** chip
  (`directory-name-group-chip`) is gone.

## Notes & gotchas

- **Directory visibility requires a real audience.** A per-item community with only the host is
  suppressed by `ListCommunities`; the e2e must add a second *real* member (a provisional/phone invitee
  does **not** create a `CommunityUser` row, so it does not lift suppression).
- **Label collision.** The roster link and the modal's confirm button both read "Name this group"; the
  confirm button carries a stable `community-preview-confirm` semantics id so the e2e targets it
  unambiguously.
- The Directory chip and roster nudges are gated to the **owner**; non-owners see the nameless
  community render normally with no promote affordance.
- **Headless-harness navigation.** The roster e2e is **fully in-app** (no `page.goto` reload): create →
  Close the share sheet → the app pushes the event detail → open the roster from it. The in-app detail
  *is* tappable in the Playwright web harness — but only on a **clean** flow: opening the share sheet's
  "Invite people" / "Invite community" sub-modals leaves the app on a stale pushed route that mis-renders
  the detail afterward, so the roster's second member + the Trail Crew share are set up via RPC
  (`registerUserViaInvite` + `CommunityService.shareItem`), and the detail needs ~2.5s to settle before
  the first tap. (This was originally mis-diagnosed as a platform-view *iframe* intercepting taps — a DOM
  probe disproved it: the only iframe is a harmless 1×1 Firebase-auth iframe and there is no `<video>`.)
  The Directory (People tab) e2e keeps the in-app "Invite people" (to exercise the live community-list
  refresh) and reaches the People tab via a single home (`/`) reload.
</content>
