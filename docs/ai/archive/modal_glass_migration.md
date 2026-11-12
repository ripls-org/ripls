# Modal Glass Migration — Triage and Inventory (ARCHIVED)

**Status:** Archived 2026-05-10 — all phases (0–7) complete.
**Plan:** [docs/issues/1802-modals-glass-sweep.md](../../issues/1802-modals-glass-sweep.md)
**Parent:** [docs/issues/1797-glass-modal-revamp.md](../../issues/1797-glass-modal-revamp.md)

The canonical glass-modal conventions now live in
[docs/client/modals.md](../../client/modals.md). This file is kept for
historical reference — the per-phase tables below show what changed,
why, and which files were touched. Token deletes deferred to a parallel
effort (creation/preview redesign, impact-detail celebration aesthetic)
are noted in the Phase 7 closing summary.

## Phase 0 dead-code findings

The Phase 0 sweep surfaced **12 modal files with zero callers**
(zero filename imports in `app/lib` or `app/test` outside the file
itself, excluding generated and ARB-derived files). All 12 were
deleted in this Phase 0 PR. Far more than the 1 file
(`weekly_goal_editor.dart`) the issue body anticipated.

`flutter analyze` is clean and `flutter test` passes 4,199 tests
after deletion. If anything breaks downstream, the files are
recoverable from git history.

### Deleted: unambiguous (4 files + 1 test)

These had no entry in the issue body and were unambiguously dead.

| File | Lines | Why dead |
|------|------:|----------|
| `app/lib/presentation/widgets/portfolio/weekly_goal_editor.dart` | 248 | Already migrated to glass in #1797. `WeeklyGoalEditor` referenced only by its own file and `app/test/presentation/widgets/portfolio/weekly_goal_editor_test.dart` (also deleted). `WeeklyGoal*` matches in `metric_detail_screen.dart` are the proto type. |
| `app/lib/presentation/widgets/experience/experience_metric_sheet.dart` | 8 | Backwards-compat re-export shim (`typedef ExperienceMetricSheet = ContentMetricSheet`). Zero callers of the typedef or filename. |
| `app/lib/presentation/widgets/transfer/pickup_scheduling_sheet.dart` | 186 | `PickupSchedulingSheet` referenced only inside its own file. Not enumerated in #1802 issue body. |
| `app/lib/presentation/screens/request/request_management_modal.dart` | 139 | Explicitly `@deprecated` ("deprecated in favor of [RequestOwnerModal]"). `RequestOwnerModal` does not exist in the codebase. Zero callers. |

### Deleted: confirmed by user (8 files)

These were explicitly listed in the #1802 issue body as migration
targets, but Phase 0 grep proved them zero-caller. The user
authorized deletion — "if anything breaks we can restore them
later." Issue body wave assignments are dropped accordingly.

| File | Lines | Evidence | Original issue-body status |
|------|------:|---------|--------|
| `app/lib/presentation/screens/experience/cancel_event_modal.dart` | 154 | `CancelEventModal` referenced only in a docstring inside `close_item_modal.dart:19`. Migration completed in #1797 but the call site was never re-wired. | Listed under "Already migrated" |
| `app/lib/presentation/screens/experience/rsvp_modal.dart` | 404 | `RsvpModal` referenced only inside its own file. Live RSVP UI lives in `experience_rsvp_button.dart` and `experience_event_pane.dart`, neither of which imports `rsvp_modal.dart`. | Wave A ("largest single migration") |
| `app/lib/presentation/screens/experience/event_joiner_modal.dart` | 683 | `EventJoinerModal` referenced only inside its own file. | Wave B |
| `app/lib/presentation/screens/experience/event_organizer_modal.dart` | 907 | `EventOrganizerModal` referenced only inside its own file. | Wave B |
| `app/lib/presentation/screens/gear/loan_borrower_modal.dart` | 702 | `LoanBorrowerModal` referenced only in stale docstring inside `transfer_modal_router.dart:26`. | Wave E2 |
| `app/lib/presentation/screens/gear/loan_list_modal.dart` | 414 | `LoanListModal` referenced only in stale docstring inside `transfer_modal_router.dart:25`. | Wave C |
| `app/lib/presentation/screens/gear/loan_owner_modal.dart` | 817 | `LoanOwnerModal` referenced only by `loan_list_modal.dart` (cascading dead). | Wave E2 |
| `app/lib/presentation/screens/gear/loan_manage_modal.dart` | 604 | `LoanManageModal` referenced only inside its own file. | Un-enumerated |

**Loan-flow note.** Loans are alive — they just don't use modals.
The live loan UX is **inline and menu-driven**:
- Action handlers live in
  [`gear_transfer_handlers_mixin.dart`](../../app/lib/presentation/screens/gear/widgets/gear_transfer_handlers_mixin.dart)
  (`handleStartLoan`, `handleCompleteLoan`, `handleCancelLoan`).
- Owners drive loan actions through the gear "managing menu"
  ([`gear_menu_items.dart`](../../app/lib/presentation/screens/gear/widgets/gear_menu_items.dart)
  and `gear_actions_section.dart`).
- Borrowers express interest inline; no modal opens.
- `transfer_modal_router.dart`'s `_showLoanModal` is intentionally a
  no-op (lines 141–157) — its name is misleading; it logs and
  returns `null` because the loan flow stopped using modals at some
  point.

The four deleted loan-modal files were stale relics of an earlier
modal-based loan UX that was replaced by the menu-driven flow. Modal
infrastructure for loans was never re-wired; only giveaways still use
modals via the router. The deletion does **not** affect loan
functionality.

**Cascading orphans (Phase 7 / Wave F cleanup).** These viewmodels
and state classes are now zero-caller and should be cleaned up in
Phase 7 once the migration is complete:
- `loan_owner_state`, `loan_owner_view_model`
- `loan_borrower_state`, `loan_borrower_view_model`
- `experience_rsvp_view_model`
- `event_modal_state`, `event_modal_view_model`
- `weekly_goal_view_model`

Stale docstring references inside `transfer_modal_router.dart` (lines
24–26) should also be cleaned up — they reference now-deleted
`LoanOwnerModal` / `LoanListModal` / `LoanBorrowerModal`.

## GlassInsetCard design decision

Per the plan and the parent #1797, cards-within-the-glass-sheet (the
transfer-modal cards: `PickupDetailsCard`, `ImpactMetricsCard`, etc.)
will use a new `GlassInsetCard` primitive introduced in Phase 2:

- **Fill:** flat `white/8` alpha (new `AppColors.modalInsetCardBg` token)
- **Border:** `white/12` (new `AppColors.modalInsetCardBorder` token)
- **Blur:** **none** — no inner `BackdropFilter`. Rationale: nesting
  `BackdropFilter`s reads the already-blurred parent texture, which
  on iOS+Metal can produce a visible ring artifact at the inset
  boundary. A flat alpha fill composes correctly over the
  `GlassSurface` parent.
- **Geometry:** new `ModalTheme.insetCardRadius` and
  `ModalTheme.insetCardBorderWidth` constants — no hardcoded numbers
  in the widget.
- **Accessibility:** presentational by default; if a consumer passes
  `onTap`, internally compose `Tappable` rather than exposing a raw
  `GestureDetector`.
- **Tests:** widget test + golden against the dark-scrim backdrop in
  `app/test/presentation/widgets/modal/glass/`.

This note is also reflected in the modal-glass primitives README.

## Theme-aware-color affirmation

This issue introduces no new hardcoded `Color(0xFF...)` literals or
magic geometry numbers outside `AppColors.modal*` and `ModalTheme`.
The white-on-glass deviation from `architecture.md`'s "all colors
must be theme-aware" rule was blessed in #1797 and is documented in
`docs/client/design.md`; nothing in #1802 widens the deviation.

## Inventory

Legend: **disp** = disposition. **wave** = phase assignment.

### Already migrated (uses glass primitives today)

| File | wave | notes |
|------|------|-------|
| `screens/experience/unified_time_modal.dart` | done | shipped in #1797 |
| `screens/experience/widgets/preview/preview_time_picker_sheet.dart` | done | shipped in #1797 |
| `screens/experience/cancel_event_modal.dart` | DELETED | migrated in #1797, zero callers — superseded by `close_item_modal` |
| `widgets/content/close_item_modal.dart` | done | shipped in #1797 (supersedes `cancel_event_modal`) |
| `widgets/content/content_info_modal.dart` | done | shipped in #1797 |
| `widgets/content/content_overflow_menu.dart` | done | shipped in #1797 (verified live: `community_content_view.dart:587`) |
| `widgets/experience/duration_picker_modal.dart` | done | shipped in #1797 |
| `widgets/media/media_picker_dialog.dart` | done | shipped in #1797 |
| `widgets/portfolio/weekly_goal_editor.dart` | DELETED | dead, deleted in this PR |
| `widgets/request/request_help_modal.dart` | done | shipped in #1797 |
| `widgets/settings/danger_zone_choice_modal.dart` | done | shipped in #1797 |

### Phase 1 — Wave A

| File | disp | notes |
|------|------|-------|
| `widgets/location/location_picker_modal.dart` | MIGRATE | live (5 callers); `ModalActionButtons` site |
| `widgets/plus_button_modal.dart` | MIGRATE | live (3 callers); preserve gradient circles |
| ~~`screens/experience/rsvp_modal.dart`~~ | DELETED | dead, removed in Phase 0 |

### Phase 2 — Wave E1 (transfer infrastructure)

Re-skin shared infrastructure once. Auto-migrates any consumer.

- `widgets/transfer/transfer_modal_shell.dart`
- `widgets/transfer/transfer_modal_header.dart`
- `widgets/transfer/transfer_modal_title.dart`
- `widgets/transfer/transfer_phase_bottom_bar.dart`
- `widgets/transfer/transfer_progress_bar.dart`
- `widgets/transfer/transfer_error_display.dart`
- All shared cards: `pickup_details_card`, `impact_metrics_card`, `impact_metric_row`, `countdown_display`, `waiting_on_user_card`, `what_happens_next_card`, `what_they_see_card`, `other_party_sees`, `recipient_selection_card`, `quick_date_chips`, `quick_time_chips`, `duration_chips`, `step_checklist_item`, `transfer_timeline`
- New: `widgets/modal/glass/glass_inset_card.dart`

Note: re-skinning `TransferModalShell` was originally expected to
auto-migrate the loan modals + `arrange_pickup_modal` + `loan_manage_modal`
+ Wave B's `event_organizer_modal` / `event_joiner_modal`. With those
seven modals deleted in Phase 0, the auto-migration scope shrinks to
just `arrange_pickup_modal` (+ `giveaway_receiver_modal` in Wave E2,
which also uses the shell).

### Phase 3 — Wave B remainder (event/experience action sheets)

| File | disp | callers | notes |
|------|------|--------:|-------|
| `screens/experience/mark_completed_modal.dart` | MIGRATE | 2 | |
| `screens/request/mark_fulfilled_modal.dart` | MIGRATE | 1 | date picker already glass |
| `screens/experience/time_poll_propose_modal.dart` | MIGRATE | 2 | pickers already glass |
| `screens/experience/time_poll_vote_modal.dart` | MIGRATE | 7 | |
| `screens/experience/time_poll_confirm_modal.dart` | MIGRATE | 1 | |
| `widgets/experience/attendee_list_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/add_contribution_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/batch_add_contribution_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/batch_add_need_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/edit_contribution_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/view_contribution_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/needs/view_need_sheet.dart` | MIGRATE | 1 | |
| `widgets/experience/experience_participants_header.dart` | MIGRATE | 1 | inline `showAccessibleModal` builder |
| `screens/experience/widgets/preview/participant_picker_sheet.dart` | MIGRATE | 1 | un-enumerated |
| ~~`screens/experience/event_joiner_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |
| ~~`screens/experience/event_organizer_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |

### Phase 4 — Wave C remainder (gear/request/loan action sheets, non-transfer)

| File | disp | callers | notes |
|------|------|--------:|-------|
| `screens/gear/arrange_pickup_modal.dart` | AUTO (Wave E1) | 1 | uses `TransferModalShell` |
| `screens/gear/past_transfer_modal.dart` | MIGRATE | 1 | date picker already glass |
| `widgets/gear/gear_borrower_sheet.dart` | MIGRATE | 2 | |
| `widgets/gear/gear_interest_sheet.dart` | MIGRATE | 2 | |
| `widgets/gear/gear_metadata_sheet.dart` | MIGRATE | 6 | |
| `widgets/request/helper_list_sheet.dart` | MIGRATE | 1 | |
| `widgets/request/request_needs_sheets.dart` | MIGRATE | 1 | |
| `screens/gear/widgets/gear_access_sheet_helper.dart` | MIGRATE | 1 | |
| `widgets/gear/metadata/category_material_editor.dart` | MIGRATE | 1 | |
| ~~`screens/gear/loan_list_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |
| ~~`screens/gear/loan_manage_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |

### Phase 5 — Wave D (community / portfolio / discover / settings)

| File | disp | callers | notes |
|------|------|--------:|-------|
| `widgets/sharing/community_picker_sheet.dart` | MIGRATE | 3 | re-skin `CommunityToggleTile` too |
| `widgets/discover/discover_filter_sheet.dart` | MIGRATE | 2 | |
| `widgets/portfolio/portfolio_filter_sheet.dart` | MIGRATE | 2 | |
| `widgets/feedback/feedback_sheet.dart` | MIGRATE | 4 | |
| `widgets/shadow_user_profile_sheet.dart` | MIGRATE | 3 | |
| `widgets/content/access_sheet.dart` | MIGRATE | 5 | |
| `widgets/content/content_metric_sheet.dart` | MIGRATE | 5 | confirm not impact-detail |
| `widgets/content/media_pick_from_carousel.dart` | MIGRATE | 1 | |
| `widgets/content/qt_attributes_card.dart` | MIGRATE | 1 | inline `showAccessibleModal` |
| `screens/communities/invite_sheet.dart` | MIGRATE | 5 | un-enumerated |
| `widgets/sharing/bulk_community_selection_modal.dart` | MIGRATE | 2 | un-enumerated |
| `widgets/sharing/community_selection_modal.dart` | MIGRATE | 3 | un-enumerated |
| `widgets/sharing/item_share_sheet.dart` | MIGRATE | 4 | un-enumerated |
| `widgets/discover/discover_bottom_sheet.dart` | MIGRATE | 1 | un-enumerated |
| `widgets/chat/reaction/emoji_picker_sheet.dart` | MIGRATE | 2 | un-enumerated |
| `widgets/chat/reaction/reaction_detail_sheet.dart` | MIGRATE | 2 | un-enumerated |
| `screens/workshop/workshop_roster_sheet.dart` | NOT-A-MODAL | 1 | misnamed — full-screen route via `NavigationHelpers.pushScreen` + `Scaffold`, not a `showAccessibleModal` caller. No migration. |
| `screens/experience/widgets/carousel_media_picker_sheet.dart` | MIGRATE | 1 | un-enumerated |
| `widgets/leaderboard/community_detail_modal.dart` | MIGRATE | 1 | un-enumerated; verify not impact-detail |

### Phase 6 — Wave E2 (canonical transfer modal implementations)

| File | disp | callers | notes |
|------|------|--------:|-------|
| `screens/gear/giveaway_giver_modal.dart` | DONE | 1 | migrated in Phase 2; routed through `TransferModalShell` in Phase 6 (unifies loading/error with receiver). |
| `screens/gear/giveaway_receiver_modal.dart` | DONE | 1 | migrated in Phase 2 via `TransferModalShell`. |
| ~~`screens/gear/loan_borrower_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |
| ~~`screens/gear/loan_owner_modal.dart`~~ | DELETED | 0 | dead, removed in Phase 0 |

### Out of scope

- Creation modals: `community_creation_modal`, `experience_creation_modal`, `create_gear_modal`, `request_creation_modal`
- Preview modals: `community_preview_modal`, `experience_preview_modal`, `gear_preview_modal`, `request_preview_modal`
- Impact-detail modals: `co2_detail_modal`, `quality_time_detail_modal`, `value_detail_modal`
- Dialogs: `consent_dialog`, `widgets/transfer/confirmation_dialog.dart`

### Not modal files (caller-screens, content-views, sub-widgets that matched the inventory grep)

- All `*_content_view.dart` (community/experience/gear/request/nudge), `*_screen.dart`, `home_screen.dart`, `chat_bubble.dart`, `message_list.dart`, `empty_content_state.dart`, `community_metrics_new_stage.dart`, `impact_select_chip.dart`, `transfer_modal_router.dart`, `community_edit_screen.dart`, `manage_members_screen.dart`, `profile_edit_screen.dart`, `leaderboard_screen.dart`, `portfolio_*_screen.dart`. These open modals; they are not modals themselves.

## Wave order summary

1. **Phase 0** (complete): triage + 12 dead-modal deletes (~5,266 lines).
2. **Phase 1 — Wave A** (complete): `location_picker_modal`, `plus_button_modal` migrated to glass.
3. **Phase 2 — Wave E1 (infra)** (complete): `GlassInsetCard` primitive added; `TransferModalShell`, `TransferPhaseBottomBar`, `RecipientSelectionCard`, `QuickDateChips`, `QuickTimeChips`, `DurationChips` re-skinned to glass; `giveaway_giver_modal`, `giveaway_receiver_modal`, `arrange_pickup_modal` migrated. **18 orphaned transfer files deleted** (cards belonged to the deleted loan/event modals, surfaced by Phase 0). Wave E1 effectively absorbed Wave E2 — the surviving transfer flows are giveaway-only. Regression tests added: `transfer_phase_bottom_bar_test.dart` + `GlassInsetCard` widget tests + `TransferModalShell` glass-wrap assertion.
4. **Phase 3 — Wave B remainder** (complete except for `mark_completed_modal` / `mark_fulfilled_modal`): migrated `time_poll_propose_modal`, `time_poll_vote_modal`, `time_poll_confirm_modal`, `attendee_list_sheet`, `participant_picker_sheet`, `experience_participants_header` (its inline `showAccessibleModal` builder), and **all 6 `widgets/experience/needs/` sheets via a single `_sheet_chrome.dart` migration** (the shared chrome helpers feed all six). `event_joiner_modal` / `event_organizer_modal` already deleted in Phase 0. **Deferred:** `mark_completed_modal` and `mark_fulfilled_modal` use the dedicated `CompletionColors` palette (the experience-completion / request-fulfillment celebration aesthetic) — same out-of-scope reasoning as `ImpactModalColors` impact-detail modals. Migrating them would be a redesign, not a re-skin; tracking as a separate effort if desired.
5. **Phase 4 — Wave C remainder** (complete except for `past_transfer_modal`): migrated `gear_borrower_sheet`, `gear_interest_sheet`, `gear_metadata_sheet` (+ its 5 sub-editors `brand_model_editor`, `category_material_editor`, `metric_tiles_editor`, `save_bar`, `url_editor`), `helper_list_sheet`, `request_needs_sheets`, `gear_access_sheet_helper`. Removed two more custom-font references (`'Georgia'` from `gear_borrower_sheet` / `gear_interest_sheet` / `helper_list_sheet`, and `_kSerifFont` from `request_needs_sheets`). `loan_list_modal`, `loan_manage_modal` deleted in Phase 0; `request_management_modal` deleted in Phase 0. **Deferred:** `past_transfer_modal` uses `CompletionColors` like `mark_completed_modal` / `mark_fulfilled_modal` — same out-of-scope reasoning (transfer-completion celebration aesthetic, would need a redesign rather than a re-skin).
6. **Phase 5 — Wave D** (complete): community / portfolio / discover / settings + un-enumerated long tail. Migrated `community_picker_sheet` (+ shared `community_toggle_tile`), `discover_filter_sheet`, `portfolio_filter_sheet`, `feedback_sheet`, `shadow_user_profile_sheet`, `access_sheet`, `content_metric_sheet` (incl. its inline `_showPickerSheet`), `media_pick_from_carousel`, `qt_attributes_card`, `invite_sheet`, `bulk_community_selection_modal`, `community_selection_modal`, `item_share_sheet`, `discover_bottom_sheet`, `emoji_picker_sheet`, `reaction_detail_sheet`, `carousel_media_picker_sheet`, `community_detail_modal`. Updated callers (`portfolio_screen`, `portfolio_items_screen`, `leaderboard_screen`, `manage_members_screen`, `experience_content_view`, `chat_bubble`, `message_list`) to pass `barrierColor: AppColors.modalBackdrop`. Dropped the `'Georgia'` font from `portfolio_filter_sheet`, `discover_filter_sheet`, and `content_metric_sheet`. **Reclassified:** `screens/workshop/workshop_roster_sheet.dart` is misnamed — it is a full-screen route (`NavigationHelpers.pushScreen` + `Scaffold`), not a modal — so no glass-sheet migration applies; left as-is.
7. **Phase 6 — Wave E2** (complete): the bulk of Wave E2 was absorbed into Phase 2 (the giveaway modals were migrated alongside the transfer infra; both loan modals were deleted in Phase 0). Phase 6 polish closed the remaining per-flow inconsistency: `giveaway_giver_modal` now routes through `TransferModalShell` (was using `GlassSheet` directly with an ad-hoc Column), unifying loading/error rendering and matching `giveaway_receiver_modal`'s 60% screen-height cap. Existing `transfer_modal_shell_test.dart`, `transfer_phase_bottom_bar_test.dart`, and the giveaway viewmodel tests still pass without re-baselining.
8. **Phase 7 — Wave F** (complete 2026-05-10): cleanup. Deleted `ModalBuilders` (803 lines), `ModalActionButtons` (118 lines), and `modal_builders_test.dart` (663 lines) — zero callers outside their own files. Scrubbed stale doc-comment references to `ModalBuilders`/`ModalActionButtons` from `chip_data.dart` and `glass_search_input.dart`. Updated [docs/client/modals.md](../../client/modals.md) (transfer-modals section rewritten to reflect the giveaway-only reality, retired-widget note replaces deprecation note, "Different surfaces, intentionally" callout added for creation/preview + impact-detail). Removed the "Modal Heights" inconsistency note from [docs/client/design.md](../../client/design.md). Verified `scripts/a11y_allowlist.txt` is still header-only (no allowlist entries). Verified zero direct `showModalBottomSheet` calls bypass `showAccessibleModal`. **Token deletes deferred:** every legacy `transfer*` token (`transferBackgroundCream`, `transferCardBackground`, `transferBorder`, `transferBorderLight`, `transferHighlight`, `transferTextPrimary/Secondary/Muted`) and the `experienceModalBackground`/`lightExperienceModalBackground` pair still have callers in non-modal surfaces — `item_metrics_screen`, `system_pill_content_factory`, `rsvp_toggle_buttons`, `confirmation_dialog`, the four creation modals, and an internal `app_colors.opaqueCardBg` helper. Per the plan's conditional-delete rule, deletes are deferred to whichever effort retires those callers (creation-modals redesign for the experience tokens; non-modal screen redesigns for the transfer-text/border family).
9. **Footer-button standardization** (alongside Phase 7): every glass modal's bottom action row now flows through `GlassFooterButtons` — same shape, position, color as the time modal. Migrated `_sheet_chrome.buildPrimaryButton` + `buildBatchActionRow` (propagates to 6 needs sheets), `request_needs_sheets._buildPrimaryButton`, `access_sheet._buildBottomActions`, `feedback_sheet` (submit + done), `invite_sheet._buildActionButtons`, `item_share_sheet._buildActionButtons`, `shadow_user_profile_sheet._buildInviteButton`, `plus_button_modal._buildBottomButtons`, `community_detail_modal._buildViewCommunityButton`, `community_picker_sheet` (Confirm moved from header to footer), and `location_picker_modal._buildFooterRow`. Added `commonReset` and `locationPickerGetDirections` ARB keys (en + es).
