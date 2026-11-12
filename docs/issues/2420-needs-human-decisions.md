# #2420 needs-human decisions — agent execution briefs

Per-doc decisions for the 15 needs-human docs from the Phase-3 sweep
(`docs/issues/2420-sweep-report.md`). Each entry is the brief a per-doc agent
executes. The agent: reads the `ripls-doc-freshness` SKILL rubric + precision
rules, **verifies against current code** (don't trust these pointers blindly —
they're a head start), applies the fix, then runs
`node scripts/doc_freshness.js stamp <doc> --commit <head>`. New docs get a
breadcrumb comment + `context:` block + a `freshness:` block. Agents do **NOT**
run git or `npm run lint` — the orchestrator commits and regenerates the map.

Decisions: #1 salvage+delete · #6 retire+new doc · #13/#14 remove dangling ref ·
all others "rewrite to match current code."

---

## 1. `docs/daily.md` → **B: salvage server assembly → new `docs/server/portfolio.md`, delete daily.md**

**Why:** documents a dead client surface (the rich week-grid "daily" screen),
replaced by the 4-pill portfolio inbox (`docs/client/inbox.md`, which says it
replaced "the old week-grid view"); `widgets/daily/weekly_*` are orphaned. But the
**server assembly is live** — `GetPortfolioInboxView` still populates all fields.

**Brief:**
1. Create **`docs/server/portfolio.md`** — `lens: [server, domain]`,
   `globs: [server/services/portfolio/**]`, `domain: server`. Server-assembly
   content only.
2. Salvage from `daily.md`, corrected:
   - RPC `GetDailyView` → `GetPortfolioInboxView` (`proto/ripls/api/portfolio.proto:17`);
     request carries only `timezone`; **cross-community** (all the user's
     communities), not per-community — rewrite every "this community"
     (`computeWeeklyMetrics(ctx, userID, d.communityIDs, …)`).
   - Files: `server/services/portfolio/{inbox_view.go,metrics.go,actions.go,assembly.go}`
     (replace the `server/services/daily/*` table). `buildCommunityPeople`,
     `buildComingUpGroups` in inbox_view.go; `isoWeekBounds` Mon–Sun in tz.
   - **Quality minutes source field CHANGED:** was `SocialFootprint.SfPoints.Mean`,
     now **`ImpactEstimate.QualityTime.QualityTimeMinutes.Mean`** (metrics.go:190).
     Others verified: `MoneySaved.ValueUsd.Mean`, `TimeSaved.Minutes.Mean`,
     `EmissionsPrevented.{ManufactureAvoidedCarbon,WasteReducedCarbon}.Co2EGrams.Mean`.
   - Actions categories A/A2/B/C verified intact (actions.go); `IsItemDone` at
     `server/conversation/state.go`. Item types keep `Daily*` names
     (`proto/ripls/api/daily.proto`).
3. **DROP** all client-UI sections (Screen Layout, Header tap, Breakdown Panel,
   People tap-to-expand, the whole client "Data Freshness" / `DailyNotifier`
   section). Add a one-line pointer to `docs/client/inbox.md`.
4. **Delete `docs/daily.md`.**

## 2. `docs/impact_metrics.md` → rewrite to match code (+ investigate client table)

- Proto field numbers: Transfer.ImpactEstimate field 13 ✓; **Request field 15**
  (doc said 14); **Experience field 16** (doc said 15).
- Client renames: `impact_metrics_service.dart`→`impact_service.dart`;
  `impact_metrics_repository.dart`→`impact_repository.dart`.
- Remove the `GetMethodologyDoc` RPC + Go-template-rendering claims — **no such
  RPC**; methodology drill-down is client-side now (consistent with the
  `provenance.md` fix). `server/impact_metrics/estimator/docs/` doesn't exist;
  per-dimension specs live under `docs/impact_metrics/`.
- Service file `metrics.go`→`service.go` (RPC dispatch service.go:41-130).
- **Investigate** the "Impact 2.0" client component table — it cites 6 files that
  don't exist (`user_metrics_screen.dart`, `user_impact_view_model.dart`, …).
  Find the current client impact surfaces (`app/lib/presentation/screens/impact_metrics/**`,
  and personal impact on `screens/users/user_screen.dart` per the portfolio
  README) and rewrite the table to real files.

## 3. `docs/stories.md` → rewrite to async event-bus model (#510)

- Story generation migrated from **synchronous direct-injection** to an **async
  event-bus subscriber** model. `EXPERIENCE_CONCLUDED` stories come from the
  event-bus subscriber (`server/story/subscriber.go`), not a background job
  `server/jobs/experience_stories.go` (doesn't exist).
- Interface `story.StoryCreator`→**`story.Creator`** (`server/story/types.go:12`);
  generator `StoryGenerator`→**`Generator`** (`generator.go:18`).
- Transfer constructor: `lifecycle.go` publishes `CommunityEvents` via
  `s.bus.Publish`; `transfer.New` signature now takes a `cebus.Publisher` (verify
  `server/services/transfer/service.go:70`).
- Rewrite the "SYNCHRONOUS via direct StoryCreator-interface injection" framing.

## 4. `docs/workshop.md` → rewrite to current widgets/brief/pipeline

- `BriefPayload` gained fields (`proto/ripls/api/workshop_service.proto:207,214,220`):
  `unique_people_count`(12), `specialties`(13), availability — doc omits them.
- Removed client widgets: `WorkshopToday*` etc. don't exist; `widgets/workshop/`
  now has `recent_items_list.dart`, `workshop_community_carousel.dart`, …
  `WorkshopActsDetailScreen`/`workshop_acts_detail_screen.dart` don't exist.
- CTA contract changed (#2020): `workshop_service.proto:294,301` add `item`
  (Item, 12) + `action_name`(13) — the unified entity/action.
- `FloatingHeader` config (workshop_screen.dart:121-133) and the
  Generation/Home-payload pipeline diagrams omit steps `brief.go:114-150` adds
  (`countUniquePeople`, `loadSuppressedKeysForScope`, `known_for.Derive`).
- Rewrite "What's on the screen" / "Client rendering" / brief-payload / pipeline.

## 5. `docs/client/chat.md` → rewrite to match (event-bus, SystemPill, inbox)

- `server/services/chat/notifications.go` + `notifyParticipants` gone — SendMessage
  now publishes to the chat event-bus (`server/chat_events*`). Verify the path.
- System-message enum: doc says `RSVP_GOING`/`RSVP_NOT_GOING`/`TIME_CONFIRMED`;
  `proto/ripls/models/chat.proto` `ChatSystemAction` has **`RSVP_YES`(57),
  `RSVP_NO`(59), no `TIME_CONFIRMED`** (has `TIME_PROPOSED`(14), etc.).
- Removed client surfaces: `screens/inbox/inbox_screen.dart` +
  `conversation_screen.dart` gone (now `PortfolioInbox`); widget cards
  `time_voting_card.dart`/`request_info_card.dart`/`workflow_widget_factory.dart`
  don't exist — `WorkflowWidgetFactory`→`SystemPill` + `system_pill_content_factory`.
- `WorkflowActionButtons` has `buildGiveaway`+`buildExperience` only (no
  `buildRequest`); `ConversationScreen` gone.

## 6. `docs/client/workflow.md` → **RETIRE; create `docs/client/manage_menus.md`**

The "four workflows share one manage-menu pattern" thesis is dead (gear-only now;
gear is slated to migrate to the flat-sheet pattern too — write to current reality).

- **Delete `docs/client/workflow.md`.**
- **Create `docs/client/manage_menus.md`** — `lens: [client, workflow]`,
  `globs: [app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart,
  app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart,
  app/lib/presentation/screens/gear/widgets/gear_menu_items.dart]`. Describe:
  - The shared flat-sheet primitive **`PollManageMenuSheet<T>`** and its per-type
    wrappers `ExperienceManageMenuSheet` / `RequestManageMenuSheet`.
  - Gear still uses the **legacy status-icon checklist** manage menu
    (`gear_menu_items.dart`, which inserts a "past transfers" item before Close in
    both giveaway and loan menus) — note it's **slated to migrate** to the
    flat-sheet pattern (forward-looking, label it as such).
  - Post-completion "View Impact" renders inline via `ContentImpactRow` (there is
    no `ExperienceMetricsScreen`).

## 7. `docs/client/time.md` → rewrite to current time sheets

- `UnifiedTimeModal`/`unified_time_modal.dart` gone — time-poll UI is now
  `time_poll_sheet.dart` (single morphing sheet) + per-mode modals. Rebuild the
  architecture section around it.
- `needs_section.dart` moved to `app/lib/presentation/widgets/needs/needs_section.dart`.
- `_PollBannerOrLabel` → public `PollBannerOrLabel` at
  `app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart`.

## 8. `docs/workflows/experience.md` → rewrite conversation-model sections

(12 path/symbol edits already applied in the sweep.) Remaining:
- **Conversation Model / Multi-Community Behavior / Cross-Community Sharing**
  sections (lines ~173-207) describe per-community isolated conversations —
  **reversed**: there is now a **single experience-scoped conversation reused
  across all communities**, stored on `Experience.conversation_id`
  (`server/services/experience/share.go:76-104`). Rewrite to single-conversation.

## 9. `docs/workflows/delta.md` → rewrite to match

- Conversation storage row: `conversation_id` migrated **off junction tables onto
  the primary entities** — `Request.conversation_id` (`models/request.proto:88`),
  `Gear.conversation_id`, `Experience.conversation_id`.
- Experience row "each community gets its own isolated conversation" → **reversed**
  to a single experience-scoped conversation reused across communities (see #8).

## 10. `docs/release/google_sign_in.md` → fix to match (3 edits already applied)

- `google-services.json` lives **per build type**:
  `app/android/app/src/{debug,profile,release}/google-services.json` (not
  `app/android/app/google-services.json`).
- Debug/dev builds use the **`.dev`** applicationId suffix
  (`app/android/app/build.gradle.kts:126` `applicationIdSuffix='.dev'`) — fix the
  OAuth-client table + troubleshooting step.

## 11. `docs/release/google_checklist.md` → fix doc link; leave checklist status

- `docs/release/build_automation.md` → **`docs/release/automation.md`** (already
  fixed in sweep — verify).
- The missing `feature_graphic.png` is an **unchecked checklist item** in a status
  log, **not drift** — leave it. (Don't manufacture the asset or remove the item.)

## 12. `docs/release/notes_prompt.md` → trivial fix

- Step 6: `npm run generate` → **`npm run generate:release-notes`** (the targeted
  command both consumers use — `release_cut.yaml:166`).

## 13. `docs/users/rallies.md` → fix + remove dangling ref (3 edits already applied)

- Sibling filename `latent-host-problems-and-solutions.md` → `latent-host.md`
  (already fixed — verify).
- **Remove** the dangling `impact-screen-proactive-host-brief.md` reference
  (line ~46) — that spec doc was never written; point at the real source if one
  exists, else drop the sentence.

## 14. `docs/users/latent-host.md` → remove dangling ref

- **Remove** the `impact-screen-proactive-host-brief.md` citations (rows 8, R1) —
  no such doc; drop or repoint to the real source.

## 15. `docs/users/measurement.md` → find real widget / genericize

- `ForestComparisonCard` doesn't exist, but the **functionality does** (landmark
  carbon comparisons). Find the current widget name and use it, or genericize the
  reference to the feature (don't cite a non-existent class). This is a
  product-strategy doc — keep the strategy prose; only fix the code reference.
