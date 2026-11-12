# #2420 doc-freshness sweep — report

One-time semantic sweep of all durable docs (Phase 3). Run as a 27-agent
resumable workflow at commit `9fdddc4d0`; every doc verified against the code
it governs and stamped with a `freshness:` block.

## Coverage

- **112 docs verified** (27/27 batches OK — full coverage, none skipped or capped)
- **63 edited**, **34 confirmed fresh**, **15 flagged for a human**
- **279 edits applied**, **134 high-severity findings**

## Per-domain

| Domain | docs | edited | fresh | needs-human | edits |
|---|--:|--:|--:|--:|--:|
| root | 27 | 15 | 8 | 4 | 79 |
| client | 26 | 17 | 6 | 3 | 76 |
| design | 1 | 1 | 0 | 0 | 2 |
| impact_metrics | 5 | 5 | 0 | 0 | 10 |
| release | 7 | 2 | 2 | 3 | 6 |
| reviews | 12 | 8 | 4 | 0 | 11 |
| server | 13 | 7 | 6 | 0 | 34 |
| testing | 3 | 2 | 1 | 0 | 4 |
| users | 13 | 3 | 7 | 3 | 10 |
| workflows | 5 | 3 | 0 | 2 | 47 |

## Needs human (15) — architectural rewrites the skill would not auto-guess

These docs describe subsystems that were renamed or restructured wholesale; a mechanical path/name swap would leave them internally inconsistent, so the sweep recorded findings but applied only the unambiguous partial fixes. Each needs a human-authored rewrite.

### `docs/daily.md` _(partial edits: 0)_

- **[high/comment]** Server code lives in server/services/daily/ — cited ~13 times across the body and the 'Key Server Files' table (daily_view.go, assembly.go, metrics.go, actions.go).
  - → There is no server/services/daily/ directory. The code is in server/services/portfolio/ (which matches the doc's own context: globs of server/services/portfolio/**). Filenames assembly.go/metrics.go/actions.go exist there, but daily_view.go does not — the handler+people assembly now live in server/services/portfolio/inbox_view.go. A blind daily/->portfolio/ path swap would still leave a broken portfolio/daily_view.go link, so this needs a coordinated remap, not a find-replace.
- **[high/comment]** The RPC and response type are GetDailyView / GetDailyViewResponse (e.g. 'GetDailyView handler', 'GetDailyViewResponse.people').
  - → The RPC is GetPortfolioInboxView and the response type is GetPortfolioInboxViewResponse (server/services/portfolio/inbox_view.go:474, :85; client app/lib/services/portfolio_service.dart:82 getPortfolioInboxView; gen server/gen/ripls/api/portfolio.pb.go). 'GetDailyView' no longer exists.
- **[high/comment]** buildCommunityPeople lives in daily/daily_view.go.
  - → buildCommunityPeople is in server/services/portfolio/inbox_view.go:730. (assembleSharedInventory/assembleActionsToday are in portfolio/actions.go:423,:35; computeWeeklyMetrics is in portfolio/metrics.go:20 — function names survived the rename, but their file/dir citations are wrong.)
- **[high/comment]** Client files: app/lib/presentation/screens/daily/daily_screen.dart, app/lib/presentation/viewmodels/daily_view_model.dart, app/lib/data/repositories/daily_repository.dart, app/lib/services/daily_service.dart.
  - → None of these exist. Current files: app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart, app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart, app/lib/data/repositories/portfolio_repository.dart, app/lib/services/portfolio_service.dart. screens/daily/ now holds only metric_detail_screen.dart.
- **[high/comment]** DailyNotifier listens to dailyCacheInvalidationProvider and checks homeProvider.selectedIndex == 1; repositories call dailyCacheInvalidationProvider.notify().
  - → The viewmodel class is PortfolioInboxNotifier (app/lib/presentation/viewmodels/portfolio_inbox_view_model.dart:74) and it listens to portfolioCacheInvalidationProvider (line 94). 'dailyCacheInvalidationProvider' / 'DailyNotifier' do not exist (grep of providers.dart shows portfolioCacheInvalidationProvider, no 'daily' provider).
- **[medium/comment]** Opening sentence: 'The daily screen is the personal daily rhythm view, replacing the inbox tab.'
  - → The subsystem was renamed wholesale from 'daily' to 'portfolio inbox'; the surface is now literally the inbox (PortfolioInboxScreen / GetPortfolioInboxView), inverting the 'replacing the inbox tab' framing. The whole doc's naming and framing need a human-authored re-title/rewrite rather than a mechanical path swap, because behavioral claims (layout, ordering, inclusion rules) must be re-verified against the current ~1500-line portfolio inbox implementation. Recorded as needs-human; no partial edits applied to avoid leaving the doc internally inconsistent (e.g. corrected dirs next to a still-broken daily_view.go link).

### `docs/impact_metrics.md` _(partial edits: 6)_

- **[high/fixed]** ImpactEstimate stored on Transfer (field 13), Request (field 14), Experience (field 15)
  - → transfer.proto:75 field 13 (correct), but request.proto:64 is field 15 (doc said 14) and experience.proto:55 is field 16 (doc said 15).
- **[high/fixed]** Client service impact_metrics_service.dart and repository impact_metrics_repository.dart
  - → Real files are app/lib/services/impact_service.dart and app/lib/data/repositories/impact_repository.dart; the *_metrics_* names do not exist.
- **[high/fixed]** Methodology docs rendered from Go templates with config values and served via GetMethodologyDoc RPC; service table and client service list GetMethodologyDoc as a live RPC
  - → No GetMethodologyDoc RPC exists in proto/ripls/api/impact_service.proto (RPC list ends at DraftImpactEstimateWithOverrides). No RenderMethodologyDoc/methodology*.go on the server. The in-app MethodologyDocScreen (methodology_doc_screen.dart:60) is a placeholder ('Methodology documentation coming soon.'). Corrected the design-decision bullet, the service-table row, the client-service row, the methodology-screen purpose, the estimator-test list, and the backend status line.
- **[medium/fixed]** Methodology docs component row points at server/impact_metrics/estimator/docs/
  - → That directory does not exist; the only subdir is server/impact_metrics/estimator/. The per-dimension methodology specs live under docs/impact_metrics/. Repointed the row.
- **[medium/fixed]** Impact metrics service file is metrics.go exposing GetCommunityImpactMetrics/GetCommunityMetricDetail/GetMethodologyDoc
  - → RPC dispatch is in service.go:41-130 (GetCommunityImpactMetrics, GetUserImpactMetrics, GetCommunityMetricDetail, …); metrics.go only holds an unexported getCommunityImpactMetrics helper; GetCommunityMetricDetail lives in metric_detail.go. Repointed the row to service.go.
- **[high/comment]** Client component tables (Impact 2.0) cite user_metrics_screen.dart, user_impact_view_model.dart (+ its test), comparison_card.dart, confidence_badge.dart, suggest_correction_form.dart, audit_trail_data.dart
  - → None of these six source files (nor app/test/.../user_impact_view_model_test.dart) exist anywhere under app/lib or app/test. The user-impact/audit-trail/suggest-correction client surface was refactored: a profile_metrics_view_model.dart / profile_metrics_data.dart + user_community_impact_screen.dart now exist, and many new impact widgets (formula_visualization, money_saved_detail_view, etc.) appear. There is no clean 1:1 mapping for the deleted widgets/models, so the right rewrite of these Impact 2.0 tables depends on intent I can't infer — left unedited and flagged for a human.

### `docs/stories.md` _(partial edits: 4)_

- **[high/fixed]** STORY_TYPE_EXPERIENCE_CONCLUDED is 'Generated by background job when experience end time passes' via server/jobs/experience_stories.go (Story Types section and Triggers table)
  - → No server/jobs/experience_stories.go exists. EXPERIENCE_CONCLUDED stories are generated by the story event-bus subscriber server/story/subscriber/experience.go on the COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED event (experience.go:25, subscriber.go:71-72)
- **[high/fixed]** Story generation interface is `story.StoryCreator`; generator type is `StoryGenerator`
  - → Interface is `story.Creator` (server/story/types.go:12); struct is `Generator` (generator.go:18). Fixed the two unambiguous occurrences (creation-flow diagram, generation-layer bullet); left the two occurrences embedded in the stale synchronous-injection prose for the architecture rewrite
- **[high/comment]** Story generation is SYNCHRONOUS via direct StoryCreator-interface injection: RecordCommunityEventAndNotify() triggers generation inline, and 'Services inject the story.StoryCreator interface' (lines 26 vs 53/104 also internally contradict each other)
  - → Generation migrated to an async event-bus subscriber model (#510). lifecycle.go publishes CommunityEvents via s.bus.Publish (transfer/lifecycle.go:506,603; community/invitations.go:241); community_event_bus/inprocess.go:20-21 inserts the event row synchronously then 'fans out asynchronously to subscribers'; server/story/subscriber/Subscriber.Handle dispatches by event type. The whole Creation-Flow diagram, 'Key Architectural Decision', and Three-Layer 'Integration' block describe the superseded direct-injection model and need a coherent rewrite
- **[high/comment]** transfer service constructor signature is `func New(storage *storage.ProtoSQLStorage, storyCreator story.StoryCreator) *Service` and 'Services inject optional story.StoryCreator interface via constructor'
  - → server/services/transfer/service.go:70 is `func New(storage *storage.ProtoSQLStorage, notificationService notifications.Service, bus cebus.Publisher, systemMessageWriter *chat.SystemMessageWriter) *Service` — no story.Creator parameter; the Creator is now injected into story_subscriber.New (subscriber.go:28). Left for the same architecture rewrite to avoid a half-corrected block

### `docs/workshop.md` _(partial edits: 1)_

- **[medium/fixed]** The documented `BriefPayload` shape (lines ~184-204) lists only since_unix_sec, hours_together, acts_count, events_count, replaced_cost_usd, co2_avoided_pounds, cta_rows, plus the deprecated fields.
  - → proto/ripls/api/workshop_service.proto:207,214,220 add three live fields the doc omitted: unique_people_count (12), specialties (13), available_now_items (14). All three are populated in server/services/workshop/brief.go:444-451,150 (briefToPayload) and rendered client-side in workshop_screen.dart:229,270.
- **[high/comment]** The 'What's on the screen' ASCII and the 'Client rendering' section describe widgets WorkshopTopStrip, WorkshopHero, WorkshopTicker (3-column acts/saved/CO2), WorkshopSectionHeader 'Three ways to keep it going', WorkshopPostcard, and WorkshopShowMoreLink as the live layout.
  - → None of those widgets exist anymore. app/lib/presentation/widgets/workshop/ contains only recent_items_list.dart, workshop_community_carousel.dart, workshop_community_identity.dart, workshop_lever_dispatcher.dart. workshop_screen.dart:214-283 renders WorkshopCommunityCarousel, a divider, WorkshopCommunityIdentity, KnownForChips, ImpactRows (time/money/co2 shared widget), AvailableNowRail, and LeanCards(rows: ctaRows). The UI was rewritten; the section needs a human to re-author the ASCII mockup and widget responsibilities. lean_cards.dart:113 confirms WorkshopShowMoreLink is now only a historical color reference.
- **[high/comment]** The doc cites WorkshopActsDetailScreen at app/lib/presentation/screens/workshop/workshop_acts_detail_screen.dart in five places (What's-on-the-screen, the ticker-tap narration, and the Client-rendering detail-screens list), describing an 'acts breakdown' detail screen.
  - → workshop_acts_detail_screen.dart does not exist and no WorkshopActsDetailScreen symbol is referenced anywhere in app/lib (grep clean). The acts cell was removed; ImpactRows now exposes only onTapTime/onTapMoney/onTapCo2 (workshop_screen.dart:266-268) and the PEOPLE cell opens WorkshopRosterSheet (workshop_screen.dart:187). Fix is entangled with the full client-section rewrite, so left for a human.
- **[medium/comment]** Per the doc's BriefCTARow notes and the 'action-vocabulary switch ... collapsed' paragraph, cta_action / context_id / thumbnail_media_id are legacy fields the client ignores, and BriefCTARow has no entity carrier.
  - → Under #2020 the contract changed: workshop_service.proto:294,301 add item (Item, field 12) and action_name (field 13) as the unified entity/verb carriers, and briefToPayload (brief.go:404-421) now actively splits cta_action into item vs action_name via itemkind.FromCTAActionString. The doc's BriefCTARow shape omits kicker, item, and action_name, and its 'client ignores cta_action' framing is no longer the whole story. Needs a human to reconcile with the #2020 migration state.
- **[low/comment]** Floating header 'Reuses the shared FloatingHeader widget, configured exactly like the Feed tab's: FloatingHeader(showMenuIcon: true, title: CommunityHelper.selectedCommunitiesTitle(...))'.
  - → workshop_screen.dart:121-133 configures FloatingHeader with leading: IconAction(group_add, invite), title: CommunityHelper.communitiesTitle(...), trailing: ProfileMenuAvatar — no showMenuIcon arg and selectedCommunitiesTitle is now communitiesTitle. Part of the same client rewrite; left for a human.
- **[low/comment]** The Generation/Home-payload pipeline diagrams (lines ~143-172 and ~494-523) and the 'No AI prose brief / structured numerals + postcards' overview describe assembly ending at briefToPayload(brief, thumbs, signal, sinceUnixSec).
  - → brief.go:114-150 adds steps the diagrams omit: countUniquePeople (PEOPLE cell, #1898), loadSuppressedKeysForScope + known_for.Derive (specialties chips), and available_now.Gather (Available-Now rail); briefToPayload now takes specialties and availableNowItems args. The brief is no longer purely 'numerals + postcards'. Recorded for the human pass alongside the client-section rewrite.

### `docs/client/chat.md` _(partial edits: 6)_

- **[high/fixed]** Server component 6 `Notifications ([notifications.go](../../server/services/chat/notifications.go))` with a `notifyParticipants` method (lines 293-298, 411); Server Resources lists notifications.go (line 597).
  - → server/services/chat/notifications.go does not exist and there is no notifyParticipants method. SendMessage now publishes to server/chat_event_bus (messages.go:140-145); push routing lives in server/notifications/chat_subscriber, wired in main.go:997-999 via HasActiveStream/IsUserInForeground hooks (service.go:54-64). Rewrote component 6 to Presence + bus/subscriber, fixed Pattern 4, the Message Broadcasting section, Pattern 2 server text, and the Resources list.
- **[medium/fixed]** Experience system-message types include RSVP_GOING, RSVP_NOT_GOING, TIME_CONFIRMED (line 304).
  - → proto/ripls/models/chat.proto enum ChatSystemAction uses RSVP_YES (57), RSVP_NO (59), and has no TIME_CONFIRMED (it has TIME_PROPOSED 14, TIME_POLL_CANCELLED 17, DETAIL_CHANGED 15, EXPERIENCE_CREATED 19). Fixed the Experience row.
- **[high/comment]** Client Implementation describes standalone `InboxScreen` at screens/inbox/inbox_screen.dart and `ConversationScreen` at screens/inbox/conversation_screen.dart (lines 87, 124, and throughout Resources).
  - → There is no screens/inbox/ directory; inbox_screen.dart and conversation_screen.dart do not exist. The standalone inbox is now PortfolioInboxScreen (screens/portfolio/portfolio_inbox_screen.dart:46) and chat is embedded inline via inline_conversation_view.dart. The entire standalone-screen client section is structurally stale. Too large to safely rewrite — flagged for human.
- **[high/comment]** Widget cards reference time_voting_card.dart, request_info_card.dart, workflow_widget_factory.dart (WorkflowWidgetFactory), and bottom-bar widgets message_input_bar.dart, conversation_top_bar.dart, giveaway_chat_header_card.dart, conversation_compact_header.dart (Workflow Integration section + Resources).
  - → None of these files exist in app/lib/presentation/widgets/chat/. WorkflowWidgetFactory was replaced by SystemPill + system_pill_content_factory.dart + a pill_content/ subdir (widgets/chat/README.md lines 27-37). New cards loan_active_card.dart / loan_completion_card.dart exist but aren't documented. The bottom-bar widgets were removed with the standalone ConversationScreen. Wholesale card-inventory drift — flagged for human.
- **[medium/comment]** workflow_action_buttons.dart static methods are `buildGiveaway`, `buildRequest`, `buildExperience`, used by the ConversationScreen bottom bar (line 228).
  - → WorkflowActionButtons has only buildGiveaway (line 27) and buildExperience (line 90) — no buildRequest. ConversationScreen no longer exists. Left unedited because the clause sits inside the structurally-stale bottom-bar section flagged for human rewrite (avoided a half-fix next to the dead ConversationScreen reference).

### `docs/client/time.md` _(partial edits: 2)_

- **[high/comment]** The doc's architecture is built around `UnifiedTimeModal` ([unified_time_modal.dart]) as the central read-only time card + per-poll history surface, framed as one of 'four standalone modals' (table line 27, the entire 'UnifiedTimeModal Layout' section lines 74-145, bottom-bar table, and cross-references in TimePollProposeModal/PollBanner/FAB sections).
  - → `app/lib/presentation/screens/experience/unified_time_modal.dart` does not exist and there is no `UnifiedTimeModal` reference anywhere in app/lib (grep -rln returns nothing). The replacement surfaces are `TimePollSheet` (time_poll_sheet.dart:24), `TimePollFinalizedModal` (time_poll_finalized_modal.dart:29), and `TimePollManageMenuSheet` (time_poll_manage_menu.dart:15). The other three modals (TimePollProposeModal/VoteModal/ConfirmModal), the RPCs, poll_id/current_poll_id mechanics, and the TimeModalNotifier AsyncNotifierProvider.family still verify. Re-mapping the doc's structure to the new surfaces (which is read-only, which shows finalized state, the new layout/bottom-bar) depends on design intent I cannot safely infer without risking a wrong multi-section rewrite. Left the UnifiedTimeModal prose unchanged for a human.
- **[medium/fixed]** Plan-Tab Banners section cites the Plan tab as rendered by `needs_section.dart` at `app/lib/presentation/widgets/shared/needs_section.dart` (line 222).
  - → File moved to `app/lib/presentation/widgets/needs/needs_section.dart` (find confirms only this path exists). Corrected the link path.
- **[medium/fixed]** Inline Chat Banner section says TIME_PROPOSED system messages are rendered via `_PollBannerOrLabel` in message_list.dart (line 242).
  - → The class is public `PollBannerOrLabel`, extracted to `app/lib/presentation/widgets/chat/message_list/poll_banner_or_label.dart` and used at message_list.dart:288 (not a private symbol inside message_list.dart). Corrected name and added the actual file path; kept message_list.dart as the user.

### `docs/client/workflow.md` _(partial edits: 0)_

- **[high/comment]** Central thesis (description line 5; Overview lines 15-25; 'Four workflow types share a common checklist infrastructure'; 'Shared Checklist Helpers ... shared across all four content views'): every content type drives its lifecycle through an owner-facing ActionDropdownMenu on the MorphingActionButton, rendered as a status-icon checklist using checklist_status_icons.dart.
  - → This pattern now holds ONLY for gear (giveaway + loan), via app/lib/presentation/screens/gear/widgets/gear_menu_items.dart (imports action_dropdown_menu.dart + checklist_status_icons.dart). Experience and Request were refactored to a pencil-icon PollManageMenuSheet bottom sheet with a flat action list and no per-step status icons. app/lib/presentation/screens/experience/experience_content_view.dart imports none of MorphingActionButton / ActionDropdownMenu / checklist_status_icons. The 'four content views' framing is stale; only two use the checklist dropdown.
- **[high/comment]** Experience Workflow > Owner Menu — Active Event (lines 85-94) lists a 6-item status-icon checklist: Set Details, RSVP, Confirm Location, Confirm Time, Mark Completed, Close Event, each with status-icon logic (RSVP detail lines 102-120, Time detail 113-120).
  - → app/lib/presentation/screens/experience/widgets/experience_manage_menu_sheet.dart:6,18 — ExperienceManageMenuSheet exposes only three flat actions (editDetails, markCompleted, closeEvent) via PollManageMenuSheet, opened from a pencil icon. No RSVP / Confirm Location / Confirm Time checklist items and no status icons exist on the owner menu. Location/time/RSVP/impact now render as inline ContentInfoRow/ContentImpactRow in experience_event_pane.dart (e.g. lines 113-122), not menu checklist items.
- **[high/comment]** Request Workflow > Owner Menu — Active Request (lines 136-143) lists Set Details, Confirm Location (with status-icon logic), Mark Fulfilled, Close Request as a status-icon checklist.
  - → app/lib/presentation/screens/request/widgets/request_manage_menu_sheet.dart:11,27 — RequestManageMenuSheet exposes a flat PollManageMenuSheet list: editDetails, markFulfilled, closeRequest, plus a conditional viewImpact row when fulfilled. No 'Confirm Location' item and no status icons. File comment explicitly calls the prior surface 'the legacy overflow menu'.
- **[high/comment]** Chat Integration (line 323) and Key Source Files (line 396): 'Widget cards are rendered by WorkflowWidgetFactory' / 'workflow_widget_factory.dart — Card dispatch factory'.
  - → No WorkflowWidgetFactory class or workflow_widget_factory.dart file exists (grep across app/lib finds only a stale comment in message_list.dart:71). Card/pill content is now dispatched by SystemPillContentFactory in app/lib/presentation/widgets/chat/system_pill_content_factory.dart (maps ChatSystemAction types) plus pill_content/ bodies, per app/lib/presentation/widgets/chat/README.md:29. Replacement is determinable but the references sit in chat-owned territory that docs/client/chat.md (not in this batch) governs and is itself stale on the same names.
- **[high/comment]** Key Source Files (line 398): 'message_input_bar.dart — Chat input with phase progress'.
  - → No message_input_bar.dart exists anywhere in app/lib. The chat input field lives in app/lib/presentation/widgets/content/inline_conversation_view.dart (the only file with TextField/compose/sendMessage in the chat-input area). docs/client/chat.md (out of batch) carries the same dead reference, so the canonical replacement is a chat-doc decision.
- **[medium/comment]** Post-Completion Transformation table (lines 72-77): Experience View Impact destination is ExperienceMetricsScreen (linked to app/lib/presentation/screens/experience/).
  - → No ExperienceMetricsScreen class or experience metrics screen file exists. Completed-experience impact renders inline via ContentImpactRow at app/lib/presentation/screens/experience/widgets/experience_event_pane.dart:117-122 (reading state.experienceStats.impact); there is no push-to-screen View Impact navigation for experiences. Gear and Request rows (ItemMetricsScreen via NavigationHelpers.pushWithSlide) are correct (gear_transfer_handlers_mixin.dart:389; request_content_view.dart:565-567). The correct fix depends on the Experience-section rewrite.
- **[low/comment]** Giveaway Pre-Recipient-Selection menu (lines 165-172) and Loan menus (lines 210-243) are listed as Set Details / Confirm Location / Select Recipient (or borrower sections) / Close.
  - → gear_menu_items.dart also inserts a 'past transfers' (Icons.history) item before Close in both the giveaway (lines 160-164) and loan (lines 381-385) owner menus. Minor omission consistent with intentional summarization, not actively misleading; left as-is.

### `docs/release/google_checklist.md` _(partial edits: 1)_

- **[high/fixed]** "Future Automation Improvements": "See `docs/release/build_automation.md` for detailed implementation instructions."
  - → docs/release/build_automation.md does not exist. Realized automation now lives in docs/release/automation.md (App Store Connect/Play service-account setup, GitHub Actions on git tags, Fastlane upload).
- **[medium/comment]** Line 85 (status log, checked): "Create feature graphic (1024x500 pixels) - `app/assets/feature_graphic.png`"
  - → No feature_graphic.png exists anywhere in the repo (searched app/assets and whole tree). The companion line above (icon_android_512.png) resolves correctly. The graphic was likely uploaded to Play Console without being committed at this path; correct location is undeterminable, so left for a human rather than guessing.

### `docs/release/google_sign_in.md` _(partial edits: 3)_

- **[high/fixed]** google-services.json location is `app/android/app/google-services.json` (stated at lines 135, 141, 282; re-download step at 206).
  - → The file lives per build type at app/android/app/src/{debug,profile,release}/google-services.json (find confirms all three; debug registers package org.ripls.app.dev, release registers org.ripls.app with the release SHA-1). The Gradle google-services plugin selects by build type. Confirmed by the canonical reference in docs/registration_and_login.md:340. Fixed all three location/re-download references.
- **[medium/comment]** OAuth client table (lines 67-69) and troubleshooting step (line 205) state the Android debug package and iOS bundle ID as `org.ripls.app`.
  - → Debug/dev builds use the .dev suffix: app/android/app/build.gradle.kts:126 applies applicationIdSuffix='.dev' to debug, the debug google-services.json registers org.ripls.app.dev, and app/ios/Runner/GoogleService-Info.plist:16 has BUNDLE_ID=org.ripls.app.dev. Only release builds are org.ripls.app. Left as a comment: the table mixes debug+release and the doc may intentionally use the base id; a blanket rewrite risks distorting the OAuth-client narrative.

### `docs/release/notes_prompt.md` _(partial edits: 0)_

- **[low/comment]** Step 6 (line 40): "Rerun `npm run generate` to convert the release notes to HTML in website directory for publication." The preamble (lines 14-16) states this prompt is kept in sync with release_cut.yaml and ripls-release/SKILL.md step 6.
  - → Both consumers it claims to mirror use the narrower targeted command `npm run generate:release-notes`: .github/workflows/release_cut.yaml:166 ("Run `npm run generate:release-notes` to produce the HTML under website/content/release-notes/") and .claude/skills/ripls-release/SKILL.md:63 ("running `npm run generate:release-notes` to produce the website HTML"). `npm run generate` is broader (package.json:8 = generate:proto && generate:flutter && generate:release-notes) but DOES include generate:release-notes, so it is not strictly false. The sibling docs/release/cheat_sheet.md:131 (outside this batch, which notes_prompt.md is meant to mirror) ALSO says `npm run generate`. Left as a comment, not edited: the command is not wrong, and editing only this doc would split it from cheat_sheet.md; a human who can touch both docs should decide whether to narrow both to generate:release-notes.

### `docs/users/latent-host.md` _(partial edits: 0)_

- **[low/comment]** Rows 8 and R1 cite `impact-screen-proactive-host-brief.md` as the authoritative source for the catalyst-pull receiving-end spec and the originating-action eligibility rule; the Source line cites "the impact-screen reframe series in this folder."
  - → No file named `impact-screen-proactive-host-brief.md` exists in `docs/users/` (ls docs/users/ lists only the 14 sibling docs, not this one). The path other in-repo docs use for it — `docs/cowork/App UXR/impact-screen-proactive-host-brief.md` (cited in docs/workshop.md:792, docs/ai/workshop.md:10, docs/issues/1579-workshop-tab.md:7, docs/issues/1580-esm-cards.md:8) — also does not resolve; the `docs/cowork/` directory does not exist in the repo. The brief is an external/uncommitted UXR artifact referenced across multiple docs, not a code path (check-paths did not flag it because it is a bare filename in prose). No determinable in-repo fix: there is no committed target to repoint to, and the same dangling reference is a repo-wide pattern, not drift introduced into this doc by a code change. Left as a comment for a human to decide whether to commit the brief or rewrite the references.

### `docs/users/measurement.md` _(partial edits: 0)_

- **[low/comment]** "Aggregate carbon impact translated into landmark comparisons (seedling, grove, rainforest) as the existing ForestComparisonCard already does." (Framing 2, Under Stewardship → Forest Equivalent)
  - → No widget class named `ForestComparisonCard` exists in app/lib (grep for the symbol returns nothing). The described functionality genuinely exists but as an inline card within app/lib/presentation/screens/impact_metrics/trees_detail_screen.dart — its doc comment (line 17) describes a "'Your forest is...' comparison card (landmark lookup table)" with the seedling/grove/rainforest-style landmark comparisons exactly as the doc claims. Ambiguous whether `ForestComparisonCard` was intended as a precise API symbol (now stale) or a descriptive name for an existing card; the underlying claim (a forest/landmark comparison card already exists) is true, and there is no clean standalone-widget rename target. Left as a comment rather than risk a wrong edit. Note: the doc's current-state claims that I could verify are all accurate — existing tab labels are SAVED/QUALITY TIME/CO₂ AVOIDED (app/lib/l10n/app_localizations_en.dart:4697,4703,7794), `impact_metrics.md` (the superseded doc) exists at docs/impact_metrics.md, and the COMMON WEALTH/PRESENCE/STEWARDSHIP renames are correctly framed as forward-looking "Implementation-level changes required" (not present in code, as expected).

### `docs/users/rallies.md` _(partial edits: 3)_

- **[medium/fixed]** Cites the fabric-tier sibling doc as 'latent-host-problems-and-solutions.md' in three places (lines 14, 53, 196).
  - → The actual file is docs/users/latent-host.md (titled 'Latent Host — Problems and Where They're Solved'), which at line 43 confirms the bidirectional sibling relationship ('Their problems and solutions belong in a sibling doc'). Renamed-but-findable.
- **[low/comment]** Line 46: 'See impact-screen-proactive-host-brief.md for the full state spec.'
  - → No file named impact-screen-proactive-host-brief.md exists anywhere under docs/ (find returns nothing). The identical reference also appears in the human-verified, already-stamped sibling docs/users/latent-host.md (lines 88, 113), so there is no determinable replacement target — likely a not-yet-written brief. Left as-is per the no-guess rule.

### `docs/workflows/delta.md` _(partial edits: 1)_

- **[high/fixed]** Conversation storage row: Requests use `ChatConversation.request_id`; Loans/Giveaways use `CommunityGear.conversation_id`; Experiences use `CommunityExperience.conversation_id` (line 21).
  - → conversation_id migrated off the junction tables onto the primary entity models: `Request.conversation_id` (models/request.proto:88), `Gear.conversation_id` (models/gear.proto:79), `Experience.conversation_id` (models/experience.proto:38). The junction-table fields are deprecated (community.proto:130,155,187). The `ChatConversation` inline topic fields are reserved/removed — topic now lives in `ChatConversation.topic` (ConversationTopic oneof) (models/chat.proto:35, models/conversation_topic.proto:9).
- **[high/comment]** Experiences: Multi-community isolation — 'Each community gets its own isolated conversation (stored in CommunityExperience.conversation_id)'; 'Unlike loans/giveaways which share one conversation per item across communities' (lines 40-42).
  - → The experience conversation model reversed: there is now a SINGLE experience-scoped conversation reused across all communities, stored on `Experience.conversation_id`, not a per-community isolated conversation. share.go:76-104 reads `expStored.ConversationId` and logs 'reusing existing conversation for experience' on subsequent shares; CommunityExperience.conversation_id is deprecated (community.proto:185-187). RSVPs remain community-specific (models/experience.proto:155), but the conversation does not. Left as comment because correcting this is woven through many sections/examples and needs an editorial rewrite, not a surgical edit.

### `docs/workflows/experience.md` _(partial edits: 12)_

- **[high/fixed]** `proto/ripls/api/common_types.proto`: Shared types (Experience, ExperienceState, ExperienceTime) (line 24); `experience.proto`: API types for Experience, RSVP, and time representation (line 23).
  - → common_types.proto does not exist. Experience+ExperienceState are in api/experience.proto:14,53; ExperienceTime+TimeConfidence are in api/time.proto:12,20; RSVP/RSVPIntention/AttendedStatus are in api/experience_service.proto:29,37,45 (not experience.proto).
- **[high/fixed]** `queries.go`: Listing and retrieval operations (line 35); Query tests (`queries_test.go`) (line 779).
  - → experience/queries.go does not exist. GetExperience/ListExperiences/ListMyExperiences live in get.go:25,261,331; tests in get_test.go.
- **[medium/fixed]** `time_modal.dart`: Dedicated time proposal/voting/locking modal (line 44); `community_selection_modal.dart` (line 50).
  - → time_modal.dart no longer exists; the time-poll UI is now time_poll_sheet.dart (a single morphing sheet, per its doc comment) plus per-mode modals. community_selection_modal.dart is now community_selection_sheet.dart.
- **[medium/fixed]** Generate AI-Powered Experience: `ExperienceService.GenExperience` (server), `ExperienceRepository.genExperience` (app) (line 221).
  - → The app has no genExperience method; generation is driven via streamGenExperience (experience_repository.dart:459) → server StreamGenExperience (experience_service.proto:841). The non-streaming GenExperience RPC still exists server-side but is not the app path.
- **[low/fixed]** Implementation line citations: MarkInProcess lifecycle.go:15-68; Complete lifecycle.go:70-122; Cancel lifecycle.go:124-180; SaveExperience save.go:15-141/78-141; ShareExperience share.go:16-104; RSVPToExperience rsvp.go:17-250; RecordAttendance rsvp.go:252-312.
  - → Actual ranges: MarkExperienceInProcess lifecycle.go:25-115; CompleteExperience 120-402; CancelExperience 479-591; SaveExperience save.go:28-491; ShareExperience share.go:20-203; RSVPToExperience rsvp.go:22-329; RecordAttendance rsvp.go:333-481.
- **[high/comment]** Conversation Model section (173-207), Multi-Community Behavior, and Cross-Community Sharing example #5 assert each community gets its own isolated conversation (stored in CommunityExperience.conversation_id); '3 communities = 3 separate conversations'; 'Two separate conversations exist (one per community)'.
  - → Conversation is now single/experience-scoped (one conversation reused across all communities, stored on Experience.conversation_id) — share.go:76-104 reuses expStored.ConversationId; rsvp.go:170-176 adds participants to that single conversation; CommunityExperience.conversation_id deprecated (community.proto:185-187). This per-community-isolation framing is pervasive across the doc; left as comment to avoid an inconsistent partial rewrite.

