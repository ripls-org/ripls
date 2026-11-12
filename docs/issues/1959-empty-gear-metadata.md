# Issue #1959: No gear metadata is generated

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1959
**Filed:** 2026-05-16 | **Last activity:** 2026-05-16
**Labels:** user-feedback | **Assignees:** unassigned
**Context docs:** none

**Audit applied (2026-05-16):** This plan was audited against
[docs/client/architecture.md](docs/client/architecture.md),
[docs/client/i18n.md](docs/client/i18n.md),
[docs/client/testing.md](docs/client/testing.md),
[docs/client/accessibility.md](docs/client/accessibility.md), and
[docs/server/observability.md](docs/server/observability.md). The audit
caught (and the plan now corrects) a "viewmodel resolves display strings"
violation around weight-unit formatting — see Open Questions #2 and Phase 1's
"Format-rule constraint" callout below. Other architectural pillars (MVVM
flow, provider lifecycle, async-safety, testing layer, accessibility
primitives, and server logging conventions) were unaffected by the proposal.

## Problem Statement

A user (user10@example.org, iOS 26.4.2, app v0.21.0 build 6957, PROD env)
photographed a buzzsaw inside the unified-create flow, advanced to the preview
card, tapped the "Item details" row, and saw an empty sheet — no brand, model,
estimated value, material, or weight filled in. The reporter clarified in a
follow-up comment that this is **not just a rendering issue**: the AI-derived
metadata never reaches the client surface where it can be edited. From the
user's point of view the AI failed to extract anything beyond the title.

Internally the bug splits in two halves:

1. **Client wiring (root cause, high confidence).** When the unified-create
   stream hits its terminal `final_7` event, the reducer at
   [unified_create_view_model.dart:377-384](app/lib/presentation/viewmodels/unified_create_view_model.dart#L377-L384)
   extracts only `type`, `streamComplete`, `streaming`, and `selectorEnabled`
   from `finalPayload`. It throws away `finalPayload.payload.gear.detectedGear`
   — the `DetectedGearItem` that carries `brand`, `model`, `weightGrams`,
   `valueEstimate`, `category`, and `materialCategory`. As a result
   `state.itemDetails` stays `null`, the preview-card row shows the placeholder
   label, and the bottom sheet (`UnifiedItemDetailsSheet`) opens with empty
   controllers ([unified_item_details_sheet.dart:42-49](app/lib/presentation/widgets/create/unified_item_details_sheet.dart#L42-L49)).

2. **AI extraction recall (contributing, lower confidence).** Even if the
   client wired the data through, the server-side gear detector returns
   strings/floats that are commonly empty/zero for generic power tools without
   a clearly visible brand decal (buzzsaw, hand tools, etc.). The detection
   schema makes every metadata field optional ([schemas.go:167-177](server/ai/schemas.go#L167-L177))
   and the prompt's "REQUIRED" wording for `value_estimate` is not enforced.
   Fixing only the client wiring still leaves the buzzsaw case showing a
   sparsely-populated sheet (brand=empty, value=empty) — better than today
   (everything empty including the few fields the AI did extract) but not
   "rich metadata for every photo." This issue should land the client fix
   first; recall improvement is a separate, slower experiment track.

The reporter's phrasing ("metadata is not getting generated") frames it as a
generation problem, but the evidence (server code clearly builds a populated
`DetectedGearItem`, client code clearly drops it on the floor) points to
wiring being the dominant defect.

## Context from Comments

One follow-up comment from @tescobar (maintainer, also reporter via the
in-app feedback channel):

> This is not just a rendering issue, the metadata is not getting generated
> and shown when tapping on the "Details" on the preview modal during creation.

This is the only signal beyond the screenshot. There's no disagreement,
no proposed workaround, and no other voices yet — so the planning has to
infer based on code state, not crowd-sourced context.

## Potential Solutions

### Option A: Wire `DetectedGearItem` through the unified-create final reducer

- **Approach:** Extend `UnifiedCreateEventFinal` handling in
  [unified_create_view_model.dart:377](app/lib/presentation/viewmodels/unified_create_view_model.dart#L377)
  so that when `finalPayload.payload` is a `gear` variant, the reducer reads
  `finalPayload.gear.detectedGear` and maps it into a new
  `ItemDetailsValue` — provided the user has not already edited
  `UserEditedField.itemDetails` (consistent with the existing
  edit-protection pattern used for `title`, `description`, `mediaIds`).
  Numeric fields get formatted: `weightGrams` displayed in kg or grams via
  the same heuristic the legacy `GearMetadataSheet._initFromDetectedGear`
  uses ([gear_metadata_sheet.dart:221-234](app/lib/presentation/widgets/gear/gear_metadata_sheet.dart#L221-L234));
  `estimatedValueUsd` formatted to 2 decimals.
- **Pros:** Targets the root cause directly. Small diff (~30 lines plus
  tests). No proto, server, or schema changes. Surfaces every metadata
  field the AI already populates today, including the ones product-lookup
  enhances on the server.
- **Cons:** Doesn't help when the AI returns nothing — the sheet is still
  empty in that case. We're back to today's behavior only for items the
  detector can't read.
- **Effort:** Small.

### Option B: A + display detected metadata read-only on the preview card

- **Approach:** Option A, plus a visible "AI suggestion" affordance on the
  preview card itself — even before the user opens the sheet — that shows
  the detected brand/value/weight inline. Mirrors what the legacy
  `gear_preview_modal.dart` does via the metadata chip
  ([gear_preview_modal.dart:479-527](app/lib/presentation/screens/gear/gear_preview_modal.dart#L479-L527)).
- **Pros:** Users see the AI's work without having to tap "Details" at all.
  Reduces friction on the share flow's golden path.
- **Cons:** Larger design + l10n footprint. The unified-create preview
  card currently shows a single "Item details" row by design (see
  `docs/design/unified-create.md`) — promoting metadata to first-class
  preview surface is a UX decision that needs sign-off, not just a fix.
- **Effort:** Medium.

### Option C: Server-side metadata fallback (broader recall fix)

- **Approach:** When `gearDetectionOutput` returns an empty `Brand` or
  `WeightGrams = 0` for a high-confidence detection, run a second
  text-mode AI call from `detection.Title` (e.g., "buzzsaw") through
  `GenerateGearFromText`, which has a different prompt with stronger
  "fill in plausible specs" guidance ([prompts.go:138-246](server/ai/prompts.go#L138-L246)).
  Use those values as defaults, with provenance flagged so the client can
  show them as low-confidence/AI-fill.
- **Pros:** Improves the experience even on hard-to-identify generic items
  where vision genuinely can't read a brand.
- **Cons:** Adds an entire extra AI round-trip on the slowest path; risks
  fabrication (the AI inventing brands for unbranded items). Doesn't fix
  the wiring bug — Option A is still required first.
- **Effort:** Large. Best treated as a separate follow-up.

## Recommendation

**Ship Option A.** It's the smallest change that addresses the user-visible
defect, it doesn't depend on any design or proto decision, and it surfaces
all the server work that already happens (product-lookup enhancement,
material-category mapping, weight estimates) but is currently invisible.

Option B should be a separate issue if product wants always-visible AI
suggestions on the preview card — it overlaps with the broader unified-create
UX track, not this bug fix. Option C is a long-tail recall investment that
belongs in the gear-AI eval workstream, not here.

The legacy single-create flow (`gear_preview_modal.dart`) is not the
active flow in PROD — `Environment.unifiedCreateEnabled` is `true` in
`app/env.prod.json` ([environment.dart:59](app/lib/core/config/environment.dart#L59), `env.prod.json`)
— so the fix only needs to touch the unified-create reducer. The legacy
modal already handles `DetectedGearItem` correctly via
`GearMetadataSummary.fromDetectedGear` and we don't need to disturb it.

## Open Questions

- **Should the wired-in AI metadata be marked as user-edited (i.e., added
  to `state.userEditedFields`) so a flipType re-stream doesn't wipe it,
  or left unmarked so the user's later edits remain authoritative?** —
  Recommendation: leave **unmarked**. Mirrors how `title` and
  `description` work today — they get overwritten on `flipType` unless
  the user has typed in them. Treating AI-detected item details
  identically keeps the mental model consistent. The user only has to
  open the sheet and tap Done to "commit" the AI values as their own.
- **What unit should weight default to when populating from `weightGrams`?**
  — Recommendation: match the legacy sheet's heuristic — kg when
  `weightGrams ≥ 1000`, grams otherwise
  ([gear_metadata_sheet.dart:225](app/lib/presentation/widgets/gear/gear_metadata_sheet.dart#L225)).
  `ItemDetailsValue.weight` is a free-text string today.
  **Audit correction:** the original plan had the viewmodel write
  `"3.2 kg"` directly into `state.itemDetails.weight`. That breaks the
  rule in [docs/client/i18n.md](docs/client/i18n.md) §Best Practices —
  "Viewmodels never resolve strings." Either (a) extend
  `ItemDetailsValue` with a `String weight` (number only, e.g. `"3.2"`)
  + `String weightUnit` (raw `"kg"`/`"g"`/`"lbs"`, not a localized
  label) so the widget formats the display via `context.l10n` at render
  time, or (b) keep the legacy approach of populating only the numeric
  string and letting the user pick the unit on the sheet. **Recommend
  (a)** — mirrors the legacy `WeightUnit` enum mechanics and avoids
  re-entering "user has to fix the AI's default" loops. The unit
  strings `"kg"`/`"g"`/`"lbs"` are wire-level identifiers, not user
  strings, so they don't need l10n keys themselves — only the eventual
  rendered label does.
- **Should we backfill brand/model/value/weight from `DetectedGearItem`
  into the `SaveGear` `GearMetadata` even when the user never opens the
  sheet?** — Recommendation: **yes**. Today
  `_gearMetadataFromItemDetails` ([unified_create_save_actions.dart:163-174](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L163-L174))
  only saves brand + model because that's all the user-typed values
  carry. If the AI extracted weight + value + material but the user
  didn't touch the sheet, those should still persist with
  `PROVENANCE_SOURCE_LLM`. Implement this in Phase 2 once Phase 1 has
  proven the wiring works.
- **Is there a way to distinguish "AI did not detect" from "AI detected
  but client dropped the data" in logs or analytics today?** —
  Recommendation: add a `gen_probe flow=gear mode=streaming-image
  metric=metadata_populated brand=<bool> model=<bool> value=<bool>
  weight=<bool>` log line on `final_4` / `final_7` so we can quantify
  the recall problem (Option C territory) once the wiring problem is
  fixed. Cheap, no PII risk.
- **Does this same wiring gap affect text-mode and URL-mode gear
  creation?** — Recommendation: assume **yes** until verified. Both
  modes go through the same `final_7` reducer and the same
  `state.itemDetails`. Worth a single end-to-end test in each mode in
  Phase 1.

## Implementation Plan

### Phase 1: Wire AI-detected metadata into `state.itemDetails`

**Goal:** Closing the loop so the "Item details" sheet opens pre-filled
with whatever the AI returned. This is the user-visible fix.

**Format-rule constraint (from audit):** the reducer must not emit
display-ready strings.
[docs/client/i18n.md](docs/client/i18n.md) §Best Practices forbids the
viewmodel from resolving user-facing text; that has to happen in the
widget via `context.l10n`. So the reducer writes raw values
(`weight = "3.2"`, `weightUnit = "kg"`, `estValueUsd = "129.99"`); the
sheet/preview card render those into the localized display. This is
consistent with how `ItemDetailsValue` is consumed today — both fields
are already string-typed, and `_formatItemDetails` in
[unified_preview_card.dart:318](app/lib/presentation/widgets/create/unified_preview_card.dart#L318)
only uses `brand`/`model` for the row summary anyway.

**Tasks:**
- [ ] Extend `ItemDetailsValue`
      ([unified_create_state.dart:16-36](app/lib/presentation/viewmodels/unified_create_state.dart#L16-L36))
      with a `String weightUnit` field (allowed values `"kg"`, `"g"`,
      `"lbs"` — wire-level identifiers, not user-visible strings, so no
      l10n keys needed here). Default to `"kg"`. Update the
      `isEmpty` getter to ignore `weightUnit` when `weight` is empty.
- [ ] Add a private mapper `_itemDetailsFromDetectedGear(DetectedGearItem)
      → ItemDetailsValue?` next to the reducer in
      `unified_create_view_model.dart`. Skip placeholders via
      `GearMetadataFormatter.isUnknownOrEmpty` (the legacy sheet uses
      this — keep the gate aligned). Return `null` when every mapped
      field would be empty so the existing
      `state.itemDetails == null` semantic is preserved.
- [ ] Extend `_applyEvent`'s `UnifiedCreateEventFinal` arm
      ([unified_create_view_model.dart:377-384](app/lib/presentation/viewmodels/unified_create_view_model.dart#L377-L384)):
      when `finalPayload.whichPayload() == StreamGenUnifiedCreateFinal_Payload.gear`
      AND the user has not edited `UserEditedField.itemDetails`, set
      `state.itemDetails` from the mapper output. No `ref.mounted`
      check needed — the reducer is synchronous and the
      `unifiedCreateViewModelProvider` is intentionally not autoDispose
      ([unified_create_view_model.dart:448-453](app/lib/presentation/viewmodels/unified_create_view_model.dart#L448-L453)).
- [ ] Numeric formatting in the mapper: convert `weightGrams.mean` to
      kg (`mean / 1000`, 1 decimal) when `≥ 1000`, otherwise grams
      (rounded). Set `weightUnit` to `"kg"` or `"g"` accordingly. Value
      goes in as `estimatedValueUsd.toStringAsFixed(2)`. **Do NOT
      include unit suffixes or currency symbols in the strings** —
      that's the widget's job.
- [ ] Update `UnifiedItemDetailsSheet`
      ([unified_item_details_sheet.dart](app/lib/presentation/widgets/create/unified_item_details_sheet.dart))
      to seed its weight controller from `initial.weight` only and
      surface a unit toggle / selector that reads/writes
      `initial.weightUnit`. The localized unit labels come from the
      same l10n keys the legacy `MetricTilesEditor` uses (or new keys
      `a11yWeightUnitKg` / `Lbs` / `G` if they don't exist yet — add
      to both `app_en.arb` and `app_es.arb`).
- [ ] Skip `materialCategory` for Phase 1. `ItemDetailsValue.material`
      is a `String` today but `DetectedGearItem.materialCategory` is
      the proto enum `MaterialCategory`. Mapping requires either a
      localized label (more viewmodel-side string work, banned by
      i18n.md) or extending `ItemDetailsValue` with the enum directly.
      Defer to Phase 2 — material is rarely AI-visible anyway for
      the photo-of-power-tool case this issue is reported against.
- [ ] Unit-test the reducer in `app/test/presentation/viewmodels/`
      using `ProviderContainer` per
      [docs/client/testing.md](docs/client/testing.md) §1: (a) gear
      final with full metadata → state.itemDetails populated, weight
      string carries no unit suffix; (b) gear final with all-empty
      metadata → state.itemDetails left `null`; (c) gear final after
      user edited itemDetails (UserEditedField.itemDetails present in
      state) → no overwrite; (d) event/request final →
      state.itemDetails untouched. Use `addTearDown(() =>
      container.dispose())` to avoid the leak pitfall called out in
      testing.md §Pitfall 1.
- [ ] Add a disposal-safety test alongside per testing.md §Testing
      Stream Management — start a stream, dispose the container before
      the final event fires, confirm no throw.
- [ ] Add one widget test under `app/test/presentation/widgets/` that
      opens `UnifiedItemDetailsSheet` after a mock gear final and
      verifies the controllers carry the detected values, including
      the localized unit suffix on the rendered weight tile (uses the
      `localizationsDelegates` helper from
      `app/test/helpers/l10n_helpers.dart`).
- [ ] Manual repro of the original buzzsaw scenario on iOS simulator
      against dev: photograph any branded item, advance to the preview
      card, tap Item details, confirm fields are pre-filled and the
      weight tile shows the unit in both `en` and `es` locales.

**Validates:** The user-reported defect — empty sheet for items where
the AI did return metadata — is gone, without violating the
i18n-and-viewmodels rule.

### Phase 1.5: l10n + accessibility hygiene

**Goal:** The new weight-unit display in `UnifiedItemDetailsSheet`
respects the project's accessibility and i18n contracts.

**Tasks:**
- [ ] Confirm the new unit toggle inside the sheet (kg / g / lbs) is
      built from the `Toggle` primitive in
      [app/lib/presentation/widgets/accessibility/](app/lib/presentation/widgets/accessibility/),
      not a raw `ToggleButtons`. The legacy `_WeightUnitToggle` in
      `metric_tiles_editor.dart` predates the a11y migration and is
      allowlisted — the *new* code in `UnifiedItemDetailsSheet` should
      follow [docs/client/accessibility.md](docs/client/accessibility.md)
      §"When to reach for which primitive".
- [ ] Every new label (e.g. unit suffix on the weight readout, sheet
      title, the unit toggle's `semanticsLabel`) comes from
      `context.l10n.*`. Add keys to both `app_en.arb` and
      `app_es.arb` before `npm run generate`. Use the `a11y` prefix
      for semantics-only strings.
- [ ] Run `npm run lint:dart:i18n` and `npm run lint:dart:a11y`
      locally before pushing — CI rejects new violations per
      accessibility.md §Lint rules.

**Validates:** No new entries in the a11y allowlist; no hardcoded
strings; new sheet works under both `en` and `es` locales.

### Phase 2: Persist AI metadata on `SaveGear` even when the sheet isn't opened

**Goal:** AI-extracted weight/value/material survive the save even if
the user never taps the Item details row.

**Tasks:**
- [ ] Extend `_gearMetadataFromItemDetails`
      ([unified_create_save_actions.dart:163-174](app/lib/presentation/viewmodels/unified_create_save_actions.dart#L163-L174))
      to read from both `state.itemDetails` AND
      `state.detectedGearMetadata` (a new state field carrying the raw
      `DetectedGearItem` from the final event, or the existing
      `ItemDetailsValue` if Phase 1's wiring is sufficient).
- [ ] Provenance: user-typed fields keep
      `PROVENANCE_SOURCE_USER`, untouched fields stamped
      `PROVENANCE_SOURCE_LLM`. Track which fields the user touched via
      either (a) the existing `UserEditedField.itemDetails` flag
      (coarse), or (b) extending `ItemDetailsValue` with per-field
      provenance (precise). Recommend (a) first as a one-bit hack —
      pursue (b) only if analytics show user trust issues.
- [ ] Round-trip test: detected gear → SaveGear → GetGear → verify
      AI-sourced fields appear with LLM provenance.
- [ ] Update `docs/design/unified-create.md` § Item details to note
      the new behavior.

**Validates:** AI's work isn't wasted on the silent-share path (user
takes photo, sees title/description fill in, taps Share without ever
opening Item details).

### Phase 3: Recall instrumentation

**Goal:** Quantify how often the AI returns empty metadata in
production so we can decide whether Option C is worth pursuing.

**Tasks:**
- [ ] Extend the existing structured log line at
      [gen_ai_streaming.go:199-204](server/services/gear/gen_ai_streaming.go#L199-L204)
      with one bool per metadata field: `brand_present`,
      `model_present`, `value_estimate_present`, `weight_grams_present`.
      The line is already an `Info` call obtained via
      `logging.LoggerWithContext(ctx).With("operation",
      "StreamGenGear", ...)` so the additions automatically pick up
      `request_id`, `user_id`, and `user_email` per
      [docs/server/observability.md](docs/server/observability.md)
      §Standard Field Names. **Do not** introduce a field named
      `gear_id` here — there isn't one yet at this point in the
      stream (gear is saved later, not detected) — keep the
      already-present `media_id` instead.
- [ ] If the unified-create path emits its own equivalent line, add
      the same bools there too — keep field names identical so a
      single Cloud Logging filter matches both surfaces.
- [ ] Add the equivalent client-side `gen_probe` line in
      `gen_gear_streaming_actions.dart`'s `final_4` handler. Use the
      existing `Logger('GenGearStreamingActions')` instance — no new
      logging plumbing needed.
- [ ] Wait 2 weeks; pull aggregates from Cloud Logging via the
      `rpc_errors` infrastructure (observability.md §Log-Based
      Metrics) extended with a per-field log-based metric; decide
      whether to pursue Option C.

**Validates:** A measured baseline for the recall problem, separate
from the wiring problem this issue resolves. Bools are
field-presence flags only — never log the brand/model values
themselves, in case an AI hallucinates a personal-identifier into
the output (defense-in-depth around observability.md §Sensitive
Data Protection).

## Architecture Audit Summary

A complete pass against the project's architecture and convention docs:

| Pillar | Source | Verdict |
|---|---|---|
| MVVM layering (ViewModel-only reducer, no service/repo touch) | [docs/client/architecture.md](docs/client/architecture.md) §Key Architectural Rules | ✅ Plan stays in the ViewModel layer; no Repository / Service edits. |
| Provider lifecycle | [docs/client/architecture.md](docs/client/architecture.md) §Async Safety | ✅ `unifiedCreateViewModelProvider` is intentionally not autoDispose and the reducer is synchronous, so no `ref.mounted` checks needed. |
| Caching | [docs/client/caching.md](docs/client/caching.md) | ✅ N/A — no repository state involved; the streamed metadata never crosses the cache boundary. |
| i18n: viewmodel doesn't resolve strings | [docs/client/i18n.md](docs/client/i18n.md) §Best Practices | ⚠️ **Fixed during audit** — Phase 1 was rewritten so the reducer emits raw numeric values + a wire-level unit identifier, with the widget doing the localized rendering. See Open Question #2 and Phase 1 "Format-rule constraint". |
| i18n: new strings in both locales | [docs/client/i18n.md](docs/client/i18n.md) | ✅ Phase 1.5 explicitly requires the new keys ship in `app_en.arb` and `app_es.arb`. |
| Accessibility primitives | [docs/client/accessibility.md](docs/client/accessibility.md) §"When to reach for which primitive" | ✅ Phase 1.5 calls out `Toggle` (not raw `ToggleButtons`) for the new unit selector. |
| Testing layer (ViewModel-first, ProviderContainer) | [docs/client/testing.md](docs/client/testing.md) §1 + §Common Testing Pitfalls | ✅ Phase 1's test list uses `ProviderContainer` with `addTearDown(dispose)` and covers state transitions, edge cases, and a disposal-safety case. |
| Server observability — masking, levels, field names | [docs/server/observability.md](docs/server/observability.md) §Standard Field Names + §Sensitive Data Protection | ✅ Phase 3 extends an existing `Info` line, reuses `LoggerWithContext` ambient fields, logs presence bools only (no AI-extracted values), and avoids the reserved `request_id` field. |
| Request ID glossary | [docs/server/observability.md](docs/server/observability.md) §Request ID glossary | ✅ Phase 3 sticks to `media_id` (not `request_id` / `target_request_id`) since this is a gear-detection path, not a `models.Request` path. |
| Server architecture — service independence | [docs/server/architecture.md](docs/server/architecture.md) §Service Independence | ✅ No server-side changes proposed beyond a log-field extension; no inter-service coupling introduced. |

## Related Issues

- #1777 — Gear detection prompt A/B (the v2 prompt was promoted in
  May 2026). Touches the same prompt
  ([prompts.go:79-136](server/ai/prompts.go#L79-L136)) but with a focus
  on title/brand/model separation, not the recall of empty fields.
- #1907 / #1932 — Unified-create classifier work. Same plumbing, same
  reducer that drops `detectedGear` today. The fix in this issue should
  not conflict with their direction.
- #1648 — Image-mode community creation. Sibling path, also runs
  through `StreamGenUnifiedCreate` — worth checking whether community
  has the equivalent "drop the AI fields on the floor" gap.
- #1157 — Streaming preview-modal work (legacy single-create gear).
  Established the `state.detectedGear` pattern that the unified flow
  failed to mirror.
