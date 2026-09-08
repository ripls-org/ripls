---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Client creation flows — the Unified Create modal funnels Text/Image/URL input into a streamed server result rendered as a glass preview card; covers entry points, two-stage state machine, and legacy fallbacks.
  globs: [app/lib/presentation/screens/create/**, app/lib/presentation/widgets/creation/**, app/lib/presentation/viewmodels/unified_create_view_model.dart, app/lib/presentation/viewmodels/unified_create_state.dart]
  triggers: [unified-create, create-modal, generate, fab, blank-create, preview-card]
  lens: [client, workflow]
  domain: client
freshness:
  verified_commit: "82c3518d0"
  verified_on: "2026-08-17"
---
# Client Creation Flows

## Overview

The Ripls client funnels almost every "share something" gesture into a single
**Unified Create** modal. The user opens it from the floating `+` FAB in the
home screen's bottom nav, picks an input mode (Text, Image, or URL), and the
server streams back a single result the client renders into a glass preview
card. The server infers the type (Event / Item / Request); the user can flip
it at any time and the stream re-issues.

Community creation is **out of scope** for unified create — it stays on its
own dedicated AI flow (see [Community Creation](#community-creation-flow) below).
The legacy per-type modals (Experience, Request) still exist on disk and are
used as flag-off fallbacks, but unified create is enabled in every env
(`ENABLE_UNIFIED_CREATE=true` in [env.local.json](../../app/env.local.json),
[env.dev.json](../../app/env.dev.json), [env.prod.json](../../app/env.prod.json)).

See [docs/design/unified-create.md](../design/unified-create.md) for the
design rationale, RPC contract, and LLM strategy.

---

## Entry Point: Floating Create FAB

The bottom-right `+` FAB in [home_screen.dart:466-499](../../app/lib/presentation/screens/home/home_screen.dart#L466-L499)
is the canonical entry point. Tapping it runs [home_screen.dart:69-127](../../app/lib/presentation/screens/home/home_screen.dart#L69-L127):

1. Request location with `suppressSettingsDialog: true` (a user gesture is a
   valid moment for the first OS prompt; we don't pester after `deniedForever`).
2. Read `unifiedCreateEnabledProvider` ([feature_flags.dart](../../app/lib/core/config/feature_flags.dart)).
3. **Flag on (default everywhere):** open `UnifiedCreateModal.show(context, ref)`.
   If it pops with a non-null `UnifiedSaveResult`, run
   `postCreationServiceProvider.handlePostCreation()` then open the Share sheet
   *after* the modal is gone — running them earlier races with the new
   ContentView mount and leaves the feed stuck on a spinner.
4. **Flag off (legacy fallback only):** open [PlusButtonModal](../../app/lib/presentation/widgets/plus_button_modal.dart),
   which routes "Lend or Giveaway", "Invite", "Ask", "Invite User" to per-type
   modals.

Other "blank create" affordances go through [blank_create_dispatcher.dart](../../app/lib/presentation/screens/create/blank_create_dispatcher.dart):

- `openBlankCreate(context, ref)` — flag-gated dispatcher used for general
  create CTAs.
- `openBlankCreateGear(context, ref)` — **always** opens `UnifiedCreateModal`
  (gear has no legacy fallback anymore), seeded with
  `targetType: DETECTED_CONTENT_TYPE_GEAR` so the composer opens on the
  camera tab and the stream is dispatched with `force_type` (classifier
  skipped) (#2936).
- `openBlankCreateExperience(context, ref)` — flag-gated; seeds
  `targetType: DETECTED_CONTENT_TYPE_EVENT` so the composer opens on the
  Text tab with an event-specific hint instead of the mixed example tour
  (#2936). Falls back to `ExperienceCreationModal` when the flag is off.
- `openBlankCreateRequest(context, ref)` — flag-gated; seeds
  `targetType: DETECTED_CONTENT_TYPE_REQUEST` (same classifier-skip as
  above) and returns a sentinel string so callers that branch on null vs.
  non-null work under both flag states.
- `openSuggestedCreate(context, ref, prompt, startUnixSec)` — opens the unified
  modal **pre-filled** from a calendar open-day suggestion: seeds the prompt and
  the event start time (via `UnifiedCreateModal.show`'s `initialPrompt` /
  `initialStartUnixSec` params) and starts generation so the user lands on the
  streamed preview. Returns `true` when the user actually saved (and then runs
  the post-creation flow + opens the Share sheet); falls back to
  `openBlankCreate` when the flag is off.
- `openRepeatDraft(context, ref, experienceId)` — pre-fills
  [ExperiencePreviewModal](../../app/lib/presentation/screens/experience/experience_preview_modal.dart)
  from a prior instance of `experienceId` via `WorkshopRepository.generateDraft()`,
  skipping the input/generating steps entirely. Falls back to `openBlankCreate`
  whenever the draft can't be used (empty id, RPC failure, or a nameless draft).
  Shared by the wrapped-up event's "Schedule the next one" chip and the Home
  inbox's workshop-surface nudge CTAs — see
  [nudges.md](../nudges.md#host-prompts-on-the-inbox).

---

## Unified Create Flow

### Two-stage state machine

[UnifiedCreateState](../../app/lib/presentation/viewmodels/unified_create_state.dart) tracks one
top-level `CreateStage`:

- `CreateStage.input` — bottom drawer with Text / Image / URL tabs.
- `CreateStage.preview` — hero media background + glass preview card.

The transition is one-way per session: the user types/captures, taps
**Generate**, and the state flips to `preview`. Closing the modal disposes the
view model's stream subscription and any owned `VideoPlayerController`; the
provider itself is **not** `autoDispose` because the save closure runs after
the modal pops (see comments at [unified_create_view_model.dart:751-756](../../app/lib/presentation/viewmodels/unified_create_view_model.dart#L751-L756)).
The state is explicitly `reset()` on every `show()`.

### Input stage

[UnifiedCreateInputDrawer](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart) renders
a glass-surface bottom drawer with three pill tabs:

- **Text** — 4-5 line `_BorderlessGlassField`. For the undeclared blank
  create (`state.targetType == null`), hint text **cycles every 3 seconds**
  through 12 shuffled example prompts (mix of Request / Event / Item
  examples — gives a tour of what the tool can produce). When an
  intent-specific entry point seeded `targetType` (e.g.
  `openBlankCreateExperience`), the cycle never starts and one static
  type-specific hint shows instead (#2936). Tapping **Generate** calls
  `vm.start()`.
- **Image** — the drawer collapses to just the tab strip; the live camera
  preview, coaching carousel, and shutter live in [UnifiedCreateCameraLayer](../../app/lib/presentation/widgets/creation/unified_create_camera_layer.dart)
  behind the drawer. On capture, `vm.captureAndStartFromCamera(file)` uploads
  via `MediaRepository.addMedia()` and starts the stream. Gallery picks go
  through `vm.pickAndStartFromGallery()`. On web, gracefully degrades to
  gallery-only (no live camera).
- **URL** — single-line `_BorderlessGlassField` with a `link` prefix icon. Hint
  cycles every 3 seconds through 10 product + event site URLs (REI, Amazon,
  Patagonia, Eventbrite, Meetup, Partiful, …). `textCapitalization.none`
  because URLs are case-sensitive.

The `Generate` primary button is disabled until the active panel has a
non-empty input.

### Streaming

`vm.start()` flips `stage → preview`, clears type-specific fields, and
dispatches the [StreamGenUnifiedCreate](../../proto/ripls/api/unified_create_service.proto) RPC via
[UnifiedCreateStreamingActions](../../app/lib/presentation/viewmodels/unified_create_streaming_actions.dart). The
streaming actions facade picks exactly one of `text` / `mediaId` /
`websiteUrl` from the active input mode — leaving stale data in another panel
would trip the server's "exactly one input" validation.

Wire events are normalised into a sealed `UnifiedCreateEvent` union before the
reducer applies them:

| Wire event   | Reducer action |
|--------------|---------------|
| `type`       | Set `state.type`, unlock `selectorEnabled`. |
| `title`      | Set `state.title` (skipped if user-edited). |
| `description`| Set `state.description` (skipped if user-edited). |
| `geocoded`   | Set `state.location` — an AI-predicted `GeocodedLocation` not yet persisted. |
| `mediaReady` | Wrap each `MediaCandidate` in a `ProtoSlot`, set `mediaIds` + `mediaCandidates`. Triggers `_loadPreviewVideoIfNeeded` (events can stream a video). |
| `time`       | Set `state.eventTime` (events only). |
| `final`      | Mark `streamComplete=true`, set final `type`, map `DetectedGearItem` → `ItemDetailsValue` for gear. |
| `error`      | Surface message via `errorMessage`. |

A 45-second hard ceiling guards against a hung Vertex backend
(`_streamTimeout` at [unified_create_view_model.dart:514](../../app/lib/presentation/viewmodels/unified_create_view_model.dart#L514)) — the most common local-dev failure is
Vertex not being able to reach `LocalBucketStorage` URLs in image mode.

### Preview stage

The preview composites three layers in [unified_create_modal.dart](../../app/lib/presentation/screens/create/unified_create_modal.dart):

1. **Hero media background.** `_HeroMediaBackground` renders the first media
   in `state.mediaIds` full-bleed. Image media → [BackgroundMediaImage](../../app/lib/presentation/widgets/media/background_media_image.dart)
   (uses `mediaRepository.getHeroMediaUrl`). Video media (event mode) →
   `VideoPlayer` driven by an owned `VideoPlayerController`. A transient
   `candidatePreviewPosterUrl` is rendered immediately on candidate-swap so
   the user gets instant feedback while the import resolves. A black dim is
   layered on top for legibility.
2. **Replace Background button.** Glass-circle pencil button above the
   preview card. Opens [MediaPickerDialog](../../app/lib/presentation/widgets/media/media_picker_dialog.dart)
   with: Photos, Camera, Video, plus the streamed `mediaCandidates` row.
   Tapping a candidate runs `vm.useCandidate(index)` which imports it via
   `MediaRepository.addMediaFromUrl()` and **swaps reversibly** — the
   previously-active media is placed back at the tapped index so a second tap
   restores it.
3. **[UnifiedPreviewCard](../../app/lib/presentation/widgets/create/unified_preview_card.dart).**
   The glass card itself. Contains:
   - **Building pill** ([UnifiedBuildingPill](../../app/lib/presentation/widgets/create/unified_building_pill.dart)) — animated "Building…" indicator while streaming.
   - **[UnifiedTypeSelector](../../app/lib/presentation/widgets/create/unified_type_selector.dart)** — 3-segment pill (Request / Event / Item). Tapping a non-active segment calls `vm.flipType(newType)`. Locked (`selectorEnabled=false`) until the first `type` event arrives and again during any flip-triggered re-stream.
   - **Title + description fields** — synced to streaming events while the user hasn't edited them; once edited, the field is added to `userEditedFields` and incoming wire events for it are dropped.
   - **Per-type rows:**
     - Event → time picker (opens `PreviewTimePickerSheet`).
     - Gear → "Item details" row (opens [UnifiedItemDetailsSheet](../../app/lib/presentation/widgets/create/unified_item_details_sheet.dart) for brand / model / value / weight / material) + [UnifiedLendGiveToggle](../../app/lib/presentation/widgets/create/unified_lend_give_toggle.dart).
     - Request → [SeedNeedsField](../../app/lib/presentation/widgets/creation/seed_needs_field.dart): the claimable needs (one, several, or none) the AI extracted from the text, as removable chips plus an inline add field — editable before save so the request is born with exactly the needs list shown (#2702, #2731).
   - **Location row** — opens the shared location picker via [LocationPickerHelper](../../app/lib/core/utils/location_picker_helper.dart). Uses the AI-predicted `state.location` until the user explicitly picks one.
   - **[UnifiedPrimaryButton](../../app/lib/presentation/widgets/create/unified_primary_button.dart)** — a per-type "Save {type}" label (`_saveButtonLabel`). Disabled until `state.isContentValid` (the item's content is ready, independent of audience). There is no community picker here — the audience is picked later in the Share sheet the home screen opens after the modal pops (see gating below).

### Type flip

`vm.flipType(newType)` cancels the in-flight stream, clears type-specific
fields the user hasn't edited (notably `eventTime`), and re-issues the stream
with `force_type` set — the server skips the classifier and goes directly to
the per-type generator. Cross-type fields (title, description, location,
media) are preserved client-side via the `userEditedFields` set;
the reducer drops incoming events for any field the user has touched.

### Save gating

The Save button gates on
[UnifiedCreateState.isContentValid](../../app/lib/presentation/viewmodels/unified_create_state.dart),
which checks content readiness independent of audience (so the button is
tappable with no audience picked — creation no longer chooses an audience at
all):

- All types — `streamComplete=true`, `type != null`.
- **Event** — title ≥3 chars, description ≥10 chars.
- **Gear / Request** — title and description non-empty.

`isSaveable` is an alias for `isContentValid` (creation provisions a per-item
community server-side, so there is no client-side audience gate). Audience
expansion is handled later by the Share sheet, not in this modal.

### Save dispatch

Tapping Save calls
[UnifiedCreateSaveActions.save(state)](../../app/lib/presentation/viewmodels/unified_create_save_actions.dart),
which routes by `type` to the matching per-type `Save*` call. No audience is
picked here — the server provisions each item's per-item community (events /
requests at creation, gear on first `ShareItem`):

| Type | Save path |
|------|-----------|
| Event | `experienceService.saveExperience()` returns the id. |
| Gear | `gearService.saveGear()` returns the id. The Lend/Give availability is applied later by the Share sheet / `ShareItem`, not here. |
| Request | `requestRepository.submitRequest()` (no community id — the server defaults the request to its per-item community). Goes through the repository (not the service) so feed / search / daily caches invalidate. |

Before each save, `_resolveLocationId(state)` materializes the AI-predicted
`GeocodedLocation` into a saved `locationId` via
`locationRepository.saveLocation()` when the user never opened the picker —
without this the new entity would land with no location.

Media is plumbed through: the request path passes `state.mediaIds` so the
server doesn't spawn a second async stock-image fetch that would (a) land
after the feed refresh and (b) usually pick a different image.

On success the modal pops with the `UnifiedSaveResult` (entity id, type, item
name); the home-screen `await` resumes, runs `handlePostCreation()` (feed
refresh + navigate to feed tab), then opens
[ItemShareSheet](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart)
for the new item. That sheet finds-or-provisions the item's per-item community
via `CommunityRepository.shareItem`, renders the open link as a QR code, and
offers two audience rows: **"Invite people"** — opens
[InviteMembersSheet](../../app/lib/presentation/widgets/sharing/invite_members_sheet.dart),
which adds existing Ripls members directly via `shareItem` (a `member_user_id`
invitee, no SMS consent needed); off-app phone/email invites are surfaced as
"coming soon" there, gated on platform SMS (A2P 10DLC) — and **"Invite
community"**, the additive multi-select community picker.

---

## Community Creation Flow

Community creation is **not** part of unified create, and it no longer uses AI
to write any text. It is a single modal where the user types the community's
**name** and an optional **description**; the only AI involvement is finding a
relevant **background image**.

1. **Entry point:** Workshop screen ([workshop_screen.dart](../../app/lib/presentation/screens/workshop/workshop_screen.dart)) opens [CommunityCreationModal](../../app/lib/presentation/screens/communities/community_creation_modal.dart).
2. **Input:** one glass card with a required **name** field (no minimum length)
   and an optional **description** field — both user-entered, never AI-generated.
3. **Background image:** when the user finishes the name field,
   [GenCommunityNotifier.fetchBackground()](../../app/lib/presentation/viewmodels/gen_community_view_model.dart)
   streams `StreamGenCommunity` (keyed on the name/description text) and renders
   the returned stock image, which the user can swap via **Replace Background**
   (camera / gallery / streamed candidates). The fetch is best-effort — a
   failure never blocks creation.
4. **Create:** the Create button (enabled once the name is non-empty) calls
   `CommunityRepository.createCommunity()` which invalidates `community:*` +
   feed caches.

---

## Legacy Per-Type Modals (Flag-Off Fallback)

These survive on disk for the unused `ENABLE_UNIFIED_CREATE=false` path. They
are not exercised in any current env build.

| Type | Creation modal | Preview modal |
|------|----------------|---------------|
| Event | [experience_creation_modal.dart](../../app/lib/presentation/screens/experience/experience_creation_modal.dart) | [experience_preview_modal.dart](../../app/lib/presentation/screens/experience/experience_preview_modal.dart) |
| Request | [request_creation_modal.dart](../../app/lib/presentation/screens/request/request_creation_modal.dart) | [request_preview_modal.dart](../../app/lib/presentation/screens/request/request_preview_modal.dart) |
| Gear | — (no legacy modal remains; `openBlankCreateGear` always goes through unified create) | — |

[ExperiencePreviewModal](../../app/lib/presentation/screens/experience/experience_preview_modal.dart)
is still pushed from a Workshop CTA via [GenExperienceViewModel](../../app/lib/presentation/viewmodels/gen_experience_view_model.dart) (separate from
home-screen create), so it's not strictly dead — but the home-screen `+`
button never reaches it when the flag is on.

---

## Cache Invalidation

Per-type repositories invalidate caches after a save, so the unified flow
benefits transitively — the save actions call the same `Repository.save*`
methods. Coverage:

- **Experience:** `experience:*`, `community:*`, feed.
- **Community:** `community:*`, feed.
- **Gear:** `gear:*`, `community:*`, feed.
- **Request:** `request:*`, `community:*`, feed.

See [docs/client/caching.md](caching.md).

---

## File References

### Unified Create

**UI Layer:**
- [unified_create_modal.dart](../../app/lib/presentation/screens/create/unified_create_modal.dart) — entry-point modal, stage compositing, save dispatch.
- [unified_create_input_drawer.dart](../../app/lib/presentation/screens/create/unified_create_input_drawer.dart) — Text / Image / URL drawer.
- [blank_create_dispatcher.dart](../../app/lib/presentation/screens/create/blank_create_dispatcher.dart) — flag-gated entry helpers.
- [unified_create_camera_layer.dart](../../app/lib/presentation/widgets/creation/unified_create_camera_layer.dart) — live-camera layer for Image mode.

**Preview Card Widgets:**
- [unified_preview_card.dart](../../app/lib/presentation/widgets/create/unified_preview_card.dart)
- [unified_type_selector.dart](../../app/lib/presentation/widgets/create/unified_type_selector.dart)
- [unified_building_pill.dart](../../app/lib/presentation/widgets/create/unified_building_pill.dart)
- [unified_item_details_sheet.dart](../../app/lib/presentation/widgets/create/unified_item_details_sheet.dart)
- [unified_lend_give_toggle.dart](../../app/lib/presentation/widgets/create/unified_lend_give_toggle.dart)
- [unified_primary_button.dart](../../app/lib/presentation/widgets/create/unified_primary_button.dart)

**Share / invite (opened after Save):**
- [item_share_sheet.dart](../../app/lib/presentation/widgets/sharing/item_share_sheet.dart) — `ItemShareSheet`: per-item share sheet (QR link + "Invite people" + "Invite community").
- [invite_members_sheet.dart](../../app/lib/presentation/widgets/sharing/invite_members_sheet.dart) — `InviteMembersSheet`: the wired "Invite people" sheet — chip-completes existing Ripls members and adds each via `shareItem`.

**State / Logic:**
- [unified_create_state.dart](../../app/lib/presentation/viewmodels/unified_create_state.dart) — `UnifiedCreateState`, `CreateStage`, `CreateInputMode`, `TransferIntent`, `UserEditedField`, the `isContentValid` / `isSaveable` gates.
- [unified_create_view_model.dart](../../app/lib/presentation/viewmodels/unified_create_view_model.dart) — input collection, streaming dispatch, type-flip re-streaming, candidate swap, video-controller lifecycle.
- [unified_create_streaming_actions.dart](../../app/lib/presentation/viewmodels/unified_create_streaming_actions.dart) — sealed `UnifiedCreateEvent` union and wire-to-event mapping.
- [unified_create_save_actions.dart](../../app/lib/presentation/viewmodels/unified_create_save_actions.dart) — per-type save + share routing, predicted-location materialization.
- [replace_media_slot.dart](../../app/lib/presentation/viewmodels/replace_media_slot.dart) — `ProtoSlot` / `MediaIdSlot` sealed union for the replace-media row.
- [unified_create_service.dart](../../app/lib/services/unified_create_service.dart) — Connect-Go RPC client.

**Feature flag:**
- [environment.dart](../../app/lib/core/config/environment.dart) — `ENABLE_UNIFIED_CREATE` (build-time).
- [feature_flags.dart](../../app/lib/core/config/feature_flags.dart) — `unifiedCreateEnabledProvider` (Riverpod, test-overridable).

### Home Screen Entry

- [home_screen.dart](../../app/lib/presentation/screens/home/home_screen.dart) — `_onAddTap`, `_buildCreateFab`, bottom-nav layout.
- [plus_button_modal.dart](../../app/lib/presentation/widgets/plus_button_modal.dart) — flag-off chooser sheet.
- [post_creation_service.dart](../../app/lib/services/post_creation_service.dart) — `handlePostCreation()` (feed refresh + tab nav).

### Community Creation

- [community_creation_modal.dart](../../app/lib/presentation/screens/communities/community_creation_modal.dart) — the single create modal (name + optional description + replaceable background image).
- [gen_community_view_model.dart](../../app/lib/presentation/viewmodels/gen_community_view_model.dart) — creation state, background-image streaming, media replacement, and submission.
- [community_edit_view_model.dart](../../app/lib/presentation/viewmodels/community_edit_view_model.dart)
- [community_repository.dart](../../app/lib/data/repositories/community_repository.dart)

### Protocol Buffers

**API:**
- [proto/ripls/api/unified_create_service.proto](../../proto/ripls/api/unified_create_service.proto) — `StreamGenUnifiedCreate`, `DetectedContentType`, `StreamGenUnifiedCreateResponse`.
- [proto/ripls/api/gen_stream.proto](../../proto/ripls/api/gen_stream.proto) — shared `MediaReady`, `MediaCandidate`, `GenStreamErrorCode`.
- [proto/ripls/api/experience_service.proto](../../proto/ripls/api/experience_service.proto)
- [proto/ripls/api/gear_service.proto](../../proto/ripls/api/gear_service.proto)
- [proto/ripls/api/request_service.proto](../../proto/ripls/api/request_service.proto)
- [proto/ripls/api/community_service.proto](../../proto/ripls/api/community_service.proto)

---

## Related Docs

- [docs/design/unified-create.md](../design/unified-create.md) — full design doc (RPC contract, LLM strategy, latency targets, rollout).
- [docs/client/caching.md](caching.md) — repository cache architecture.
- [docs/client/geospatial.md](geospatial.md) — shared location-picker components.