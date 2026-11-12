# Issue #2012: Consolidate polymorphic entity-card proto messages (AvailableNowItem / BriefCTARow / DailyItem)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2012
**Filed:** 2026-05-18 | **Last activity:** 2026-05-18
**Labels:** none | **Assignees:** unassigned
**Context docs:** none — the related plan [docs/issues/2011-known-for-chip-detail.md](./2011-known-for-chip-detail.md) (the audit that surfaced this issue) is the relevant background.
**Audit:** Reviewed 2026-05-18 against [docs/client/architecture.md](../client/architecture.md), [docs/client/caching.md](../client/caching.md), [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/testing.md](../client/testing.md), [docs/server/architecture.md](../server/architecture.md), [docs/server/observability.md](../server/observability.md), and [docs/proto_conventions.md](../proto_conventions.md). Material changes are marked **[audit]** inline. Audit summary table at the end.

> **⚠ Compatibility stance (2026-05-18 follow-up):** No proto field is **removed** in this work. Every field migrated into the shared item shape is marked `[deprecated = true]` but **stays on the wire for at least two full release cycles** so old clients in the field keep working. The server populates both old and new representations across both cycles. Eventual deletion of deprecated fields is tracked in [**#2020**](https://github.com/Type2Fun-Foundation/ripls/issues/2020) — every deprecated field gets an inline proto `TODO(#2020)` comment pointing at it. **There is no Phase 6 in this work that breaks clients.**

> **🔧 Implementation note (2026-05-18 follow-up):** During implementation, the originally-proposed `EntityCardCore` message and `EntityKind` enum were renamed to align with existing proto conventions and to keep UX framing ("card") out of `.proto` files:
> - The new universal shape is **`api.Item`** (defined in `proto/ripls/api/item.proto`), not `EntityCardCore`.
> - The discriminator enum is **`api.ItemKind`**, not `EntityKind`.
> - Rather than minting a new shape, the existing `AvailableNowItem` was promoted to `Item` (its surface-specific `mini_label` and renamed-from-`detail` `subtitle` are now `optional`). The legacy `AvailableNowKind` enum was retired; `Item` uses `ItemKind` directly.
> - `BriefCTARow.core` → `BriefCTARow.item`. `DailyItem.core` → `DailyItem.item`.
> - The Go helper package `server/entitycard/` was renamed to `server/itemkind/`. The Dart navigation helper `pushToEntity` was renamed to `pushToItem`.
>
> The body of this plan retains the original `EntityCardCore` / `EntityKind` terminology for historical fidelity — read it as "the shared item shape" and "the unified item-kind enum" respectively. The acceptance criteria, phases, and audit findings are unchanged in substance.

## Problem Statement

The codebase carries three near-duplicate "polymorphic entity-card" proto messages that all wrap the same underlying entities (gear / experience / request / giveaway / transfer). Each was added for a specific surface and diverged in field names and discriminator type:

| Message | Lives in | Surface | Discriminator | Total fields |
|---------|----------|---------|---------------|--------------|
| [`AvailableNowItem`](../../proto/ripls/api/available_now.proto) | `available_now.proto` | Profile + workshop "Available Now" rails | `AvailableNowKind` enum (gear / experience / giveaway / request) | 6 |
| [`BriefCTARow`](../../proto/ripls/api/workshop_service.proto#L217) | `workshop_service.proto` | Workshop "Three ways to keep it going" postcards + per [`profile/postcards.go`](../../server/services/profile/postcards.go) the user-profile postcards | `cta_action` *string* (gear/experience/request — and overloaded with non-entity action names like `schedule_repeat` per [`BriefLever`](../../proto/ripls/api/workshop_service.proto#L127)) + `is_primary` flag | 11 |
| [`DailyItem`](../../proto/ripls/api/portfolio.proto#L377) | `portfolio.proto` | Portfolio / inbox daily list | `DailyItemType` enum (transfer / giveaway / experience / request / community milestone) | 39 |

All three carry a shared `{context_id, kind discriminator, title, subtitle, optional media_id}` skeleton plus surface-specific extras (`mini_label`, `atmosphere_line` / `headline_emphasis`, unread count, last-activity timestamp, owner avatar, action button state, etc.). The discriminator type drifted between two enums (`AvailableNowKind`, `DailyItemType`) and a free-form string (`cta_action`).

Tap-routing has the same shape: each surface has its own switch statement that maps the discriminator to one of three screens (`GearScreen` / `ExperienceScreen` / `RequestScreen`):

- [`app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart`](../../app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart) — switches on `row.ctaAction` (string).
- [`app/lib/presentation/screens/workshop/workshop_screen.dart:260`](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L260) — `_itemTypeFor()` maps `AvailableNowKind` → string, then `pushToItemScreen()`.
- [`app/lib/presentation/screens/users/user_screen.dart:145`](../../app/lib/presentation/screens/users/user_screen.dart#L145) — duplicate of the workshop helper.
- [`app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart:85`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L85) — `_navigateToItem()` switches on `DailyItemType` directly (does NOT go through `pushToItemScreen` — also passes `initialCommunityId` and `initialTab`).

The single tap helper at [`navigation_helpers.dart:57`](../../app/lib/core/utils/navigation_helpers.dart#L57) is a string-keyed switch, so each surface translates its discriminator into a string just to call it.

This issue tracks pulling the three shapes into one card message and one routing helper. **No user-visible change.** Pure refactor.

## Context from Comments

No comments on the issue yet. The body was authored as a forward-looking refactor brief filed during the [#2011 audit](./2011-known-for-chip-detail.md), so there's no contradicting follow-up to reconcile. The #2011 plan landed an explicit acknowledgement (under "Follow-up: consolidate polymorphic entity-card messages") that it picked `AvailableNowItem` for the chip-detail RPC specifically to avoid minting a fourth shape — making the four-way consolidation case slightly stronger.

## Client field-usage verification (2026-05-18)

Before drafting deprecation rules, I ripgrep'd every field this plan touches across `app/lib/` and `app/test/`. Results:

**`AvailableNowItem`** — every field is read on the client:
- `contextId` (6 reads), `kind` (6 reads), `title` (heavily used), `detail` (3 reads in `available_now_rail.dart`), `miniLabel` (2 reads), `mediaId` (multiple reads).

**`BriefCTARow`** — most fields are read; two are **vestigial today**:
- Active: `kicker` (5), `headline` (4), `atmosphereLine` (1), `actionLabel` (5), `ctaAction` (3 — `user_profile_lever_dispatcher.dart`, `nudge_content_view.dart`), `contextId` (6), `thumbnailMediaId` (6).
- **`thumbnailCaption` — UNUSED** anywhere in `app/lib/` or `app/test/`.
- **`headlineEmphasis` — UNUSED** anywhere in `app/lib/` or `app/test/`.
- (`communityId` and `isPrimary` reads exist but in unrelated contexts — the BriefCTARow-specific reads are present but not the dominant signal in the grep counts.)

**`DailyItem`** — every field this plan would route through `core` is actively read on the client:
- `title` (heavy use in `inbox_row.dart` lines 114, 121, 288 + `magazine_item.dart` lines 73–76), `itemType` (routing in `inbox_row.dart` lines 184, 363–436), `itemId` (22), `contentId` (26), `description` (used in `inbox_row.dart:485` as fallback when no message preview), `backgroundMediaId` (6 sites including `inbox_row.dart` lines 154, 185).

**`BriefLever`** — one vestigial field:
- Active: `ctaAction` (1 read in `nudge_content_view.dart`).
- **`visibleText` — UNUSED** anywhere in `app/lib/` or `app/test/`. (Comment on the proto says it's the underlined inline lever text, but the client doesn't read it — the AI brief paragraph that surfaced it was deprecated in the home design, per `BriefPayload.segments` being marked deprecated.)

**Why this matters for the plan:** Because **every wire field this refactor touches is actively read on the client** (excluding the two vestigial `BriefCTARow` fields and `BriefLever.visibleText` which are unrelated to the consolidation), the server must keep populating the legacy top-level fields **for at least two full release cycles after this work lands**. Old clients in the wild continue to read those fields and would render blank rows / blank tiles if the server stopped emitting them prematurely. Eventual removal is tracked in [**#2020**](https://github.com/Type2Fun-Foundation/ripls/issues/2020), which gates removal on telemetry confirming no app version older than the migration cutoff is still in active use. The three vestigial fields (`thumbnailCaption`, `headlineEmphasis`, `BriefLever.visibleText`) are noted in #2020 as cleanup candidates for the same deletion pass but are **not** in scope for this work — touching them risks scope creep on a pure refactor.

## Potential Solutions

### Option A: Single shared `EntityCard` message with extension `oneof`
- **Approach:** Mint one shared message in a new `proto/ripls/api/entity_card.proto` carrying the common core (`context_id`, `EntityKind kind`, `title`, optional `subtitle`, optional `media_id`) plus an `extension` oneof that carries the surface-specific overlay (`AvailableNowExtension { mini_label }`, `PostcardExtension { kicker, atmosphere_line, action_label, action_name, is_primary, thumbnail_caption, headline_emphasis, community_id }`, `InboxItemExtension { unread_count, last_activity_at, owner, action_button_label, action_button_style, state_label, ... }`). Add one unified `EntityKind` enum that supersets `AvailableNowKind` + `DailyItemType` (gear / experience / request / giveaway / transfer / community). Server-side aggregators emit the shared shape with the right extension populated. Client gets one `pushToEntity(card)` helper that switches on `kind` and reads any extra navigation hints (initial community, initial tab) from the extension.
- **Pros:** Satisfies the issue's literal acceptance criterion ("All three callers emit and consume a single shared card message"). One tap helper across every surface. Future surfaces (e.g. the #2011 chip-detail) get a single, obvious shape to reuse. Future fields on the core (e.g. structured impact line) ship everywhere at once. The oneof keeps surface-specific bloat off the wire — an Available Now tile never carries inbox state.
- **Cons:** Largest proto refactor. Every aggregator, every serializer, every test changes. Field renumbering on the migrated response messages means a coordinated client+server release (Ripls ships them together, but in-flight users on an old client during deploy see broken responses). The `BriefCTARow.cta_action` string today encodes *both* entity kinds (`"gear"`) and non-entity actions (`"schedule_repeat"`, `"revive_experience"` per [`BriefLever.cta_action`](../../proto/ripls/api/workshop_service.proto#L132)); separating those concerns is non-trivial and is a behavior shift, not a pure rename.
- **Effort:** Large. Roughly: proto file + server aggregators (3 packages) + Dart consumers (4 screens + 1 dispatcher) + server tests + client tests + #2011 chip-detail follow-up to point at the new shape. ~2 weeks of focused work with a coordinated release.

### Option B: Shared `EntityCardCore` submessage, surface messages keep their identity
- **Approach:** Extract just the common skeleton into a `EntityCardCore { context_id, EntityKind kind, title, optional subtitle, optional media_id }` submessage and embed it in each of the three existing messages: `AvailableNowItem { EntityCardCore core, string mini_label }`, `BriefCTARow { EntityCardCore core, ... existing extras }`, `DailyItem { EntityCardCore core, ... existing extras }`. Unify the discriminator enum once. The tap-routing helper takes `EntityCardCore` and works for all three surfaces.
- **Pros:** Hits the *spirit* of the consolidation (one shared core, one shared routing helper, one shared enum) without renumbering every field on three large messages. Inbox-specific fields stay on `DailyItem` where they evolve at their own cadence. Smaller blast radius on the wire — only the *fields that move into the core* need new tag numbers, and we can sequence the moves field-by-field. Each surface keeps independent evolution velocity (the original "why not now" argument from the issue body).
- **Cons:** Strictly speaking, doesn't satisfy the literal acceptance criterion ("single shared card message"). The three message names still exist; the embedded core is one indirection level for any reader. Some future "shared logic" code still needs to ask "which of the three am I looking at" for the surface-specific overlay — the oneof in Option A makes that explicit, embedding doesn't.
- **Effort:** Medium. Roughly half of Option A — the core fields move once, the extras stay put.

### Option C: Client-side helpers only, leave proto messages untouched
- **Approach:** Introduce a Dart `EntityCard` abstraction that wraps any of the three proto types and exposes a uniform `{contextId, kind, title, subtitle, mediaId}` accessor + a unified `tap(BuildContext)` helper. Server protos and aggregators stay exactly as they are. The four duplicate tap-routing switches collapse to one helper.
- **Pros:** Smallest, lowest-risk change. No wire-format churn, no coordinated releases. Eliminates the four duplicate Dart switches (the most visibly painful duplication today). Buys time to learn whether the surfaces actually keep diverging or stabilize.
- **Cons:** Server side stays duplicated. New surfaces (#2011 chip-detail being the immediate one, but not the last) still face the "which of the three shapes do I reach for" decision. Doesn't satisfy the issue's literal acceptance criterion. The Dart abstraction adds a layer that future readers have to unpack to understand the wire shape.
- **Effort:** Small. ~1-2 days of client-only work.

## Recommendation

**Option B.** It hits the actual harm the issue describes — the four duplicate tap-routing switches, the drifting discriminators, the "which of these shapes do I pick" decision new surfaces face — without paying for renumbering the 39 fields of `DailyItem` or untangling `BriefCTARow.cta_action`'s overloaded string in a single PR.

The literal acceptance criterion ("single shared card message") is slightly off-target: the surface-specific extras genuinely *don't* want to live on the same message — an Available Now tile never carries inbox unread state, and a postcard never carries a scheduled-at timestamp. A oneof in a single message expresses that, but so does an embedded core submessage, and the latter is much less wire churn. The three messages remain as overlays, but the *core* — the part that actually duplicates — is shared.

Option A is the right answer if the three surfaces are converging (e.g. if #2011 chip-detail becomes the third surface for `AvailableNowItem` and a fourth surface for the unified card lands soon after, and a fifth, and so on). Right now there are exactly three surfaces, they've been stable for several releases, and the issue body itself flags that "three shapes evolve at different cadences — locking them together adds coordination cost on every future surface tweak." Option B gets the consolidation benefits while keeping that cadence cost off the table.

Option C is too thin — it leaves the server-side duplication in place and doesn't help #2011 (or the next new surface) make a non-arbitrary choice. The client-only helper from Option C *is* a worthwhile sub-step inside Option B's Phase 4, but it isn't sufficient on its own.

**Recommended sequencing:** Land the unified `EntityKind` enum + shared `EntityCardCore` submessage in a single proto-level PR (Phase 1), migrate one surface at a time to embed it (Phases 2–4), then collapse the four duplicate Dart tap switches into one helper (Phase 5). Each phase is independently shippable and reversible.

## Open Questions

- **Does `EntityKind` need a `TRANSFER` variant, a `GEAR` variant, or both?** — Recommendation: keep both as distinct enum values. `DailyItem.item_type == DAILY_ITEM_TYPE_TRANSFER` today routes to `GearScreen` (its `content_id` is `gear_id`), so on tap-routing they collapse, but on the wire they carry different semantics: TRANSFER means "a loan of gear with state" (passive inbox view), GEAR means "the item itself" (browsing / borrowing view). Collapsing them would lose information the inbox uses for state labels. Better to keep both kinds and let the tap helper map `TRANSFER` → `GearScreen` (just like the discriminator does today). Same reasoning for keeping `GIVEAWAY` distinct from `GEAR` — a giveaway is gear in a specific terminal state.
- **What about `DAILY_ITEM_TYPE_COMMUNITY` (community-milestone events)?** — Recommendation: include `ENTITY_KIND_COMMUNITY` in the unified enum even though only `DailyItem` uses it today. The cost is one extra enum value; the value is consistency (`EntityKind` is canonical entity discriminator across the codebase). Tap routing on `COMMUNITY` is currently a no-op on the inbox — preserve that.
- **How do we untangle `BriefCTARow.cta_action`'s overloaded string?** — Recommendation: split into two fields during migration. `EntityCardCore.kind` (the new enum) carries the entity discriminator; a new `BriefCTARow.action_name` string field carries the non-entity action verbs (`schedule_repeat`, `revive_experience`, `propose_share`, etc.). Mark the old `cta_action` field `[deprecated = true]` and have the server populate both during migration so old clients keep working. Drop `cta_action` after one release cycle. This is also the moment to fix the broader smell — `BriefLever` uses the same string, and its values are *exclusively* non-entity actions; rename its field to `action_name` for clarity.
- **Where does `EntityCardCore` live — its own file or inside `available_now.proto` (the smallest of the three)?** — Recommendation: new `proto/ripls/api/entity_card.proto`. The shape will be imported by `available_now.proto`, `workshop_service.proto`, `portfolio.proto`, and (per the [#2011 plan](./2011-known-for-chip-detail.md)) `GetCategoryDetailResponse`. A dedicated file with no inbound dependencies avoids retriggering the cyclic-import problem `available_now.proto` was already extracted to solve (see its file-level comment).
- **Does the inbox screen's tap routing (`initialCommunityId`, `initialTab` hints) belong on the unified helper, or stay on the inbox path?** — Recommendation: keep the inbox extras on `InboxItemExtension`, and have the unified tap helper read them via an optional `EntityCardCore.tap_hints` field — but only after Phase 5. In Phase 5's initial pass the inbox keeps its own `_navigateToItem` because it routes via `pushScreen` (not `pushToItemScreen`) and threads tab + community hints; refactoring `NavigationHelpers.pushToItemScreen` to take optional hints is a follow-up that can land on its own merits later.
- **Are there other shapes we should sweep up while we're here?** — Recommendation: no — keep the scope to the three named in the issue body. `SharedItem` (also in `portfolio.proto`, line 252) and `MetricLineItem` (line 356) look similar at a glance but carry different semantics (inventory rows vs. itemized metric breakdowns) and don't appear in any tap-routing switch. The [`#1999`](https://github.com/Type2Fun-Foundation/ripls/issues/1999)-era `KnownForChips` items aren't entity cards. Scope creep on a pure refactor inflates blast radius without changing the outcome.
- **Is on-wire compatibility a hard requirement?** — Recommendation: treat it as a soft requirement (graceful degrade during rolling release) but not blocking. Ripls ships client + server together, the wire is internal, and there's no third-party consumer of these messages. Any field that moves into `EntityCardCore` gets a new tag number; in the same release, we mark the old field `[deprecated = true]` and have the server populate both. After one release cycle, drop the deprecated field. This costs one extra deploy per migrated field but keeps in-flight users on old clients functional. Acceptance-criterion language ("Existing on-wire data round-trips") is satisfied because no stored data carries these wire messages — they're response-only.
- **Does the new shape ship as part of the [#2011 chip-detail RPC](./2011-known-for-chip-detail.md), or independently?** — Recommendation: independently, but sequenced before the #2011 RPC lands if practical. The #2011 plan explicitly chose `AvailableNowItem` to avoid minting a fourth shape; if `EntityCardCore` exists by then, #2011 can carry it directly and the "follow-up: consolidate" section of that doc collapses to "done." If the sequencing doesn't work out, #2011 lands first against `AvailableNowItem` and gets migrated as part of this issue's Phase 2.

## Implementation Plan

### Phase 1: Add `EntityCardCore` + unified `EntityKind` enum (proto + Go scaffolding, no migration)

**Goal:** The new shape exists on the wire and in generated code. Nothing emits or consumes it yet. Establishes the contract every later phase migrates onto.

**Tasks:**
- [ ] Create `proto/ripls/api/entity_card.proto` containing:
  - `EntityKind` enum: `ENTITY_KIND_UNSPECIFIED = 0`, `ENTITY_KIND_GEAR = 1`, `ENTITY_KIND_EXPERIENCE = 2`, `ENTITY_KIND_REQUEST = 3`, `ENTITY_KIND_GIVEAWAY = 4`, `ENTITY_KIND_TRANSFER = 5`, `ENTITY_KIND_COMMUNITY = 6`. Each value documented per the proto-comment conventions in [CLAUDE.md](../../CLAUDE.md) §"Proto Comments" (no vendor names, no transport plumbing, no server-side mechanics — just what the value means to a client).
  - `EntityCardCore` message with `string context_id = 1`, `EntityKind kind = 2`, `string title = 3`, `optional string subtitle = 4`, `optional string media_id = 5`. Use `optional` per [CLAUDE.md](../../CLAUDE.md) §"Proto Field Conventions" for any field that's conditionally present.
  - Package + go_package directives matching the existing `ripls/api/` files.
- [ ] Add a Go helper package `server/entitycard/` with `Kind(...)` conversion utilities: `FromAvailableNowKind(api.AvailableNowKind) api.EntityKind`, `FromDailyItemType(api.DailyItemType) api.EntityKind`, `FromCTAActionString(string) (api.EntityKind, bool)`, and the inverse `ToScreenRouteName(api.EntityKind) string` (returns `"gear"` / `"experience"` / `"request"` etc. for the tap helper). Pure functions, exhaustively covered by table-driven unit tests. **[audit]** Include a `doc.go` with a `// Package entitycard ...` comment (per [CLAUDE.md](../../CLAUDE.md) §"Style Guidelines" — the `revive.package-comments` rule fails the build if missing) and a `README.md` per the multi-file-directory rule. The library is a shared utility, not a service — services depend on it directly per [docs/server/architecture.md](../server/architecture.md) §"Shared Functionality in Libraries". No interface; pure functions with no external dependencies don't need one.
- [ ] Define the round-trip test pattern (per [`docs/proto_conventions.md`](../proto_conventions.md)) for the conversion helpers: every `AvailableNowKind` and `DailyItemType` enum value round-trips through `EntityKind` and back without information loss. The test fails the build if a new enum value is added on either side without a conversion arm.
- [ ] Add an [audit] **classification ticket** for any `CommunityEventType` values that might be added in later phases. None expected for a pure refactor, but per [CLAUDE.md](../../CLAUDE.md) §"Adding New CommunityEventType Values" the test `TestEveryEventTypeClassified` will flag any new values; this phase doesn't add any.
- [ ] Run `npm run generate`, `npm run build`, and `go build -tags=integration ./...` per [CLAUDE.md](../../CLAUDE.md) §"Go Server Build Verification". No call sites change yet — this should be a pure-additive compile.

**Validates:** The new shape compiles and the conversion helpers cover every existing discriminator value. Nothing user-visible changed.

### Phase 2: Migrate `AvailableNowItem` (smallest, most contained)

**Goal:** `AvailableNowItem` embeds `EntityCardCore` and the existing client tap-routing reads through the core. Establishes the migration pattern on the lowest-risk surface.

**Tasks:**
- [ ] Update `proto/ripls/api/available_now.proto`:
  - Add `EntityCardCore core = 7;` field. Keep the existing `context_id` (1), `kind` (2), `title` (3), `detail` (4), `mini_label` (5), `media_id` (6) fields with `[deprecated = true]` annotations on `context_id`, `kind`, `title`, `detail`, `media_id`. Keep `mini_label` undeprecated (it's surface-specific and stays).
  - Add a comment explaining that `core` is the canonical source and that **the server populates both `core` and the deprecated top-level fields for at least two release cycles** so older clients (which only read the top-level fields) continue to work. Removal is tracked in [#2020](https://github.com/Type2Fun-Foundation/ripls/issues/2020).
  - Each deprecated field gets an inline proto comment: `// TODO(#2020): remove after all clients have migrated to EntityCardCore.`
- [ ] Update the server aggregator at [`server/available_now/available_now.go`](../../server/available_now/available_now.go):
  - In `gatherGear` (line 197), `gatherRequests` (line 324), `gatherExperiences` (line 449): populate the new `Core: &api.EntityCardCore{...}` field with `ContextId`, `Kind` (via `entitycard.FromAvailableNowKind`), `Title`, `Subtitle: detail` (note rename — `detail` becomes `subtitle` on the core), `MediaId`. **Continue populating the deprecated top-level fields with the same values across the entire deprecation window** (two release cycles, per #2020). Removing dual population early would render blank tiles for any old client in the field, which still reads only the top-level fields.
  - Add a unit test that for every kind, `core.context_id == top-level context_id`, `core.kind` round-trips, etc. — catches drift between the two populations.
- [ ] Update the Dart consumers to read through `.core` when present:
  - [`workshop_screen.dart:260`](../../app/lib/presentation/screens/workshop/workshop_screen.dart#L260) `_openAvailableNowItem` reads `item.core.contextId` and `item.core.kind` (the new `EntityKind` enum) and routes via `entitycard_routing.pushToEntity(context, item.core)` — a new helper added in this phase under [`app/lib/core/utils/`](../../app/lib/core/utils/). Falls back to the old fields if `core` is unset (covers any in-flight responses during rollout). Drop `_itemTypeFor()` in Phase 5 once all surfaces migrated.
  - [`user_screen.dart:145`](../../app/lib/presentation/screens/users/user_screen.dart#L145) — same swap.
- [ ] **[audit]** ARB strings unchanged — this is a wire-shape refactor with **zero user-visible copy changes**, so neither `app_en.arb` nor `app_es.arb` need edits per [docs/client/i18n.md](../client/i18n.md). No new tappable surfaces are introduced (existing taps still happen at existing widgets, which already use `Tappable` / `IconAction`), so no [docs/client/accessibility.md](../client/accessibility.md) work. No new RPCs, repositories, or cache namespaces, so no [docs/client/caching.md](../client/caching.md) changes. Call this out explicitly in the PR description so reviewers don't expect them.
- [ ] **[audit]** After regenerating Dart code (`npm run generate`), re-run `flutter pub run build_runner build` to regenerate any Mockito mocks that touch repositories returning the affected types (e.g. `workshop_repository.dart`, `profile_repository.dart`, `portfolio_repository.dart`). Per [docs/client/testing.md](../client/testing.md) §"Mock Generation", generated mocks are type-safe but need rebuild_runner to pick up new fields on the proto-generated classes.
- [ ] **[audit]** Tests:
  - Server unit test: `gather*` round-trip — emit, encode, decode, assert `core` and deprecated fields agree.
  - Client viewmodel test (the workshop available-now rail's existing test): cover the case where `item.core` is set and the case where only the deprecated fields are set; both route to the same screen.
  - Run `npm run test:app -- --reporter expanded` per [CLAUDE.md](../../CLAUDE.md) §"Testing".
- [ ] **[audit]** Logging: no new RPCs in this phase, so no new logging fields. Existing aggregator logging unchanged.

**Validates:** The Available Now rail renders identically on the workshop and user-profile surfaces, with the server emitting both the new core and the legacy fields, and the client reading either.

### Phase 3: Migrate `BriefCTARow` (also splits `cta_action` cleanly)

**Goal:** `BriefCTARow` embeds `EntityCardCore`, and the overloaded `cta_action` string is cleanly separated into entity discriminator (on the core) vs. non-entity action verb (on the row).

**Tasks:**
- [ ] Update `proto/ripls/api/workshop_service.proto`:
  - Add `EntityCardCore core = 12;` on `BriefCTARow`.
  - Add `optional string action_name = 13;` for non-entity actions (`schedule_repeat`, `revive_experience`, `propose_share`, `seed_subhost`, `adjust_event`, `post_in_chat`, `ask_for_help`). Comment that this is mutually exclusive with `core.kind` — when `core.context_id` is non-empty the row is an entity card, otherwise it's a non-entity action.
  - Mark `cta_action` (5), `context_id` (6), `thumbnail_media_id` (8) as `[deprecated = true]`. Each deprecated field gets an inline `// TODO(#2020): remove after all clients have migrated to EntityCardCore.` comment. Keep `kicker`, `headline`, `atmosphere_line`, `action_label`, `community_id`, `thumbnail_caption`, `headline_emphasis`, `is_primary` — all surface-specific overlay fields that stay on the row. (Note: `thumbnail_caption` and `headline_emphasis` are vestigial on the client today — see [Client field-usage verification](#client-field-usage-verification-2026-05-18). They're noted in #2020 as separate cleanup candidates but **stay on the wire in this work**.)
  - Note: `BriefCTARow.headline` and `BriefCTARow.atmosphere_line` are subtly different from `EntityCardCore.title` / `subtitle` — `headline` carries an italic-emphasis fragment via `headline_emphasis`. Keep `headline` + `headline_emphasis` on the row (the surface needs the structured emphasis); the core gets the plain `title` (server denormalizes `headline` minus emphasis into `core.title` so consumers that don't care about the emphasis just read `core.title`).
- [ ] Update the server aggregators emitting `BriefCTARow`:
  - [`server/services/workshop/brief.go:364`](../../server/services/workshop/brief.go#L364): populate `core` from the entity referenced by what's currently `context_id`. Populate `action_name` from the `cta_action` string when the action is non-entity (use a small allow-list of known non-entity verbs; anything else routes through `entitycard.FromCTAActionString` to populate `core.kind`).
  - [`server/services/profile/postcards.go`](../../server/services/profile/postcards.go): same.
  - Unit test the split: a row with `cta_action="gear"` produces `core.kind=ENTITY_KIND_GEAR` and `action_name=""`; a row with `cta_action="schedule_repeat"` produces `action_name="schedule_repeat"` and `core` empty.
- [ ] Update the Dart consumers:
  - [`user_profile_lever_dispatcher.dart`](../../app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart): read through `row.core` when set, route via the new `pushToEntity(context, row.core)` helper. The non-entity action branch (`schedule_repeat` etc.) stays a no-op in this dispatcher (it currently switches only on entity values; non-entity values were already an implicit no-op via the unmatched `default`).
  - [`nudge_content_view.dart:98`](../../app/lib/presentation/widgets/nudge/nudge_content_view.dart#L98) `_openAction`: same swap.
- [ ] **[audit]** Same string-overload smell at [`BriefLever.cta_action`](../../proto/ripls/api/workshop_service.proto#L132), where the field is *only* non-entity actions. Don't *rename* the field — that breaks the wire. Instead, add `optional string action_name = 4;` (next free tag) and mark `cta_action` `[deprecated = true]` with `// TODO(#2020): remove after all clients have migrated.` Server populates both across the two-release-cycle deprecation window; final removal is tracked in #2020 alongside the other deprecated fields. Same migration rhythm as `BriefCTARow.cta_action`.
- [ ] **[audit]** Tests: server round-trip + client viewmodel coverage for both entity and non-entity rows. The existing [`server/services/workshop/brief_test.go`](../../server/services/workshop/brief_test.go) gets new cases.

**Validates:** The "Three ways to keep it going" postcards render and route identically, with the entity / non-entity split now explicit on the wire.

### Phase 4: Migrate `DailyItem` (largest message, most extras stay put)

**Goal:** `DailyItem` embeds `EntityCardCore` for the routing skeleton; the 30+ inbox-specific fields stay on `DailyItem` unchanged. The inbox tap-routing collapses to read through `.core`.

**Tasks:**
- [ ] Update `proto/ripls/api/portfolio.proto`:
  - Add `EntityCardCore core = 40;` on `DailyItem` (next free tag number).
  - Mark `item_type` (10), `item_id` (11), `content_id` (19), `title` (2), `description` (20), `background_media_id` (14) as `[deprecated = true]`. Each gets an inline `// TODO(#2020): remove after all clients have migrated to EntityCardCore.` comment. **Server continues to populate them across the two-release-cycle deprecation window** — every one of these fields is actively read on the client today (verified via grep across `app/lib/`; see [Client field-usage verification](#client-field-usage-verification-2026-05-18)), so dropping any of them early renders blank inbox rows for old app versions still in the field.
  - The mapping: `core.context_id` = whichever id the existing tap path reads (i.e. `content_id`, which is `gear_id` for TRANSFER/GIVEAWAY and `item_id` for EXPERIENCE/REQUEST — verify by reading [`portfolio_inbox_screen.dart:85`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L85) which only reads `content_id`). `core.kind` from `entitycard.FromDailyItemType(item_type)`. `core.title = title`. `core.subtitle = description`. `core.media_id = background_media_id`.
  - Note: `DAILY_ITEM_TYPE_COMMUNITY` maps to `ENTITY_KIND_COMMUNITY`. Tap routing for community-kind items remains a no-op in the inbox (no community detail screen at this discriminator level).
- [ ] Update the server aggregator at [`server/services/portfolio/assembly.go`](../../server/services/portfolio/assembly.go):
  - In each of the five `DailyItem` constructors (lines 516, 684, 796, 885, 953): populate `Core: &api.EntityCardCore{...}` alongside the existing field population. Add a small helper in the same file to keep the boilerplate centralized.
  - Update [`server/services/portfolio/inbox_feed.go:111`](../../server/services/portfolio/inbox_feed.go#L111) if any post-assembly mutations touch the routing fields (they currently don't, but verify).
- [ ] **[audit]** `MetricLineItem.item_type` and `SharedItem.item_type` and `DailyAction.item_type` (all in `portfolio.proto`) use `DailyItemType` too. Decide per-call whether to migrate them in this phase or leave them as out-of-scope. **Recommendation: leave them.** They're not entity *cards* (no `context_id` + `title` + `media_id` skeleton), they're metric-breakdown rows. Migrating them would expand scope beyond the issue's three named messages. Mark this explicitly as a non-goal in the PR description so reviewers don't ask.
- [ ] Update the Dart consumer at [`portfolio_inbox_screen.dart:85`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart#L85):
  - `_navigateToItem` reads `item.core.kind` and `item.core.contextId`. Routes via the existing `pushScreen` path (NOT `pushToItemScreen` — the inbox passes `initialCommunityId` and `initialTab`, which `pushToItemScreen` doesn't support). This means the inbox keeps its own helper in Phase 4; Phase 5 unifies.
  - [audit] Check `magazine_item.dart:72,203` (`DailyItemType` branching for filtering/counting). Those reads are not tap-routing — they're "is this item watchable" filters. Leave them on `item_type` until Phase 5 collapses the discriminator; migrating them is mechanical but not blocking.
- [ ] **[audit]** Tests:
  - Server: [`server/services/portfolio/assembly_test.go`](../../server/services/portfolio/assembly_test.go) gets a new round-trip test ensuring `core` and the deprecated fields agree across every constructor.
  - Client: [`app/test/presentation/viewmodels/portfolio_inbox_view_model_test.dart`](../../app/test/presentation/viewmodels/portfolio_inbox_view_model_test.dart) and [`app/test/presentation/widgets/portfolio/inbox_row_test.dart`](../../app/test/presentation/widgets/portfolio/inbox_row_test.dart) need updates for the new `.core` reads.
  - Query budget unchanged (the assembly path doesn't get new queries) — assert via `storage.AssertMaxQueries` per [CLAUDE.md](../../CLAUDE.md) §"SQL Efficiency".
- [ ] **[audit]** Logging: no new RPCs introduced in this phase, so no new `operation` fields land. The existing aggregator logging in [`server/services/portfolio/`](../../server/services/portfolio/) must not regress — keep every `logging.LoggerWithContext(ctx).With(...)` chain intact, and do not strip context. Per [docs/server/observability.md](../server/observability.md) §"Logging Usage Pattern", aggregator log lines stay tagged with their existing `operation` (e.g. `GetPortfolioInboxFeed`) so the `rpc_errors` alert continues to bucket correctly.

**Validates:** The inbox renders identically, with the routing skeleton now reading from `core` instead of the deprecated top-level fields. The 30+ inbox-specific fields are untouched.

### Phase 5: Collapse the four duplicate Dart tap-routing switches into one helper

**Goal:** Every entity-card tap on every surface goes through one `pushToEntity(context, card, {hints})` helper. The duplicated switches disappear.

**Tasks:**
- [ ] Add `pushToEntity(BuildContext context, EntityCardCore card, {String? initialCommunityId, int? initialTab})` in [`app/lib/core/utils/navigation_helpers.dart`](../../app/lib/core/utils/navigation_helpers.dart). Internally:
  - Switches on `card.kind`. `GEAR` / `GIVEAWAY` / `TRANSFER` → `GearScreen`; `EXPERIENCE` → `ExperienceScreen`; `REQUEST` → `RequestScreen`; `COMMUNITY` → no-op (or future community detail screen — TBD per the open question).
  - Threads `initialCommunityId` and `initialTab` through when supplied (the inbox needs them; other surfaces don't).
  - Routes via `pushScreen` (uses `SwipeToCloseMixin`), not `pushToMaterialPageRoute` — per [docs/client/architecture.md](../client/architecture.md) §"Navigation & Gestures".
- [ ] Replace the existing `pushToItemScreen(itemId, itemType)` body with a thin shim that constructs an `EntityCardCore` from the string `itemType` and delegates to `pushToEntity`. Keep the function so callers don't break, but mark it `@Deprecated('use pushToEntity')` for a cycle, then remove.
- [ ] Delete the four duplicate switches:
  - [`workshop_screen.dart`](../../app/lib/presentation/screens/workshop/workshop_screen.dart) `_itemTypeFor` (line 275).
  - [`user_screen.dart`](../../app/lib/presentation/screens/users/user_screen.dart) `_itemTypeFor` (line 145).
  - [`portfolio_inbox_screen.dart`](../../app/lib/presentation/screens/portfolio/portfolio_inbox_screen.dart) `_navigateToItem`'s switch (line 85) — replaced by a single `NavigationHelpers.pushToEntity(context, item.core, initialCommunityId: communityId, initialTab: initialTab)` call.
  - [`user_profile_lever_dispatcher.dart`](../../app/lib/presentation/widgets/user_profile/user_profile_lever_dispatcher.dart) `openPostcard` — replaced with the same `pushToEntity` call.
- [ ] Collapse `magazine_item.dart`'s `DailyItemType` branching (line 72, 203) onto `EntityKind` where it reads from `.core.kind`. Filtering / counting logic stays the same.
- [ ] **[audit]** Accessibility: any new tap target uses `Tappable` from [`widgets/accessibility/`](../../app/lib/presentation/widgets/accessibility/), per [docs/client/accessibility.md](../client/accessibility.md). The helper itself doesn't render a widget, so the rule applies to callers — Phase 5 doesn't introduce new callers, only collapses existing ones.
- [ ] **[audit]** Tests:
  - New `navigation_helpers_test.dart` covering every `EntityKind` value and the `initialCommunityId` / `initialTab` threading.
  - The four existing call-site tests get re-pointed at the new helper.
  - Run `npm run test:app -- --reporter expanded` and `npm run lint:nav` per [docs/client/architecture.md](../client/architecture.md).

**Validates:** All four entity-card surfaces route through one helper. The next surface that adds an entity-card list has one obvious destination.

### Phase 6: ~~Remove deprecated top-level fields~~ — *deferred to #2020*

**This phase is intentionally out of scope for this work.** Removing the deprecated fields is a wire-incompatible change that would break any app version in the field still reading those fields. Per [Client field-usage verification](#client-field-usage-verification-2026-05-18), every deprecated field is actively read on the client today, so the breakage would be immediate and user-visible.

The deletion work — drop the deprecated fields, drop server dual-population, drop client fallback reads, retire `AvailableNowKind`, remove the `@Deprecated('use pushToEntity')` shim — is tracked in **[#2020](https://github.com/Type2Fun-Foundation/ripls/issues/2020)**. That issue gates the work on:

1. **At least two full release cycles** elapsed since #2012 landed in production.
2. Telemetry confirming no app version older than the #2012 migration cutoff is still in active use.
3. Reviewer sign-off in the #2020 PR description acknowledging the rollout-pace check passed.

Every deprecated field in the proto files carries an inline `// TODO(#2020): remove after all clients have migrated.` comment, so the link is reachable from the source as well as from this plan.

**Validates:** The wire format carries one canonical shape per surface. The discriminator is one enum. The codebase has lost ~150 lines of duplicated switches and field declarations with no behavior change.

## Follow-up: Deletion of deprecated fields

Tracked at **[#2020](https://github.com/Type2Fun-Foundation/ripls/issues/2020)**. Summary of what #2020 owns:

- Drop the `[deprecated = true]` fields from `available_now.proto`, `workshop_service.proto`, and `portfolio.proto` (the full list is in #2020's body).
- Drop the dual-population code in `server/available_now/`, `server/services/workshop/`, `server/services/profile/`, `server/services/portfolio/`.
- Drop the client fallback reads (`if .core unset then read top-level`) added in Phases 2–4.
- Drop the `@Deprecated('use pushToEntity')` shim around `pushToItemScreen` added in Phase 5.
- Retire `AvailableNowKind` (now subsumed by `EntityKind`); decide whether `DailyItemType` survives (still referenced by `MetricLineItem` / `SharedItem` / `DailyAction`).
- Sweep up the vestigial fields surfaced in this audit (`BriefCTARow.thumbnail_caption`, `BriefCTARow.headline_emphasis`, `BriefLever.visible_text`) — same deletion pass.

**Prerequisites #2020 gates on:** two full release cycles elapsed since #2012 landed in production, plus telemetry confirming no app version older than the #2012 cutoff is still active.

## Related Issues

- **[#2020 — Delete deprecated entity-card fields from #2012 consolidation](https://github.com/Type2Fun-Foundation/ripls/issues/2020)** — The follow-up deletion issue filed alongside this plan. Owns the wire-break cleanup once the deprecation window has elapsed.
- **[#2011 — Tapping a 'known for' chip opens a detail screen](https://github.com/Type2Fun-Foundation/ripls/issues/2011)** — The audit on #2011's plan filed this issue. #2011 chose `AvailableNowItem` directly to avoid minting a fourth shape; once Phases 1–2 of this issue land, the #2011 RPC response should be updated to carry `EntityCardCore` instead. If #2011 ships first, its migration becomes part of this issue's Phase 2.
- **[#1999 — Workshop screen mirror of the user profile](https://github.com/Type2Fun-Foundation/ripls/issues/1999)** — Established the user-profile postcards (BriefCTARow on `ProfileService`) that Phase 3 migrates.
- **[#1996 — Self-view available-now items](https://github.com/Type2Fun-Foundation/ripls/issues/1996)** — Established the Available Now rail's scope rules that Phase 2 inherits unchanged.

## Audit Findings (2026-05-18)

Material changes from the pre-audit draft, with the architectural rule each one comes from:

| # | Change | Source rule |
|---|--------|-------------|
| 1 | **`BriefLever.cta_action` is *added-then-deprecated*, not *renamed***. Proto field rename is a wire break — every existing client and stored serialized message would mis-decode. The plan now adds `optional string action_name = 4;` and marks `cta_action` `[deprecated = true]`, with the deprecated field dropped in Phase 6. Same migration rhythm as `BriefCTARow.cta_action`. | [CLAUDE.md](../../CLAUDE.md) §"Proto Field Conventions" + general proto wire-compatibility rules. |
| 2 | **`server/entitycard/` package gets `doc.go` + `README.md` callouts**. Pre-audit draft mentioned them; audit promotes them to required and ties them to the lint rule that fails the build (`revive.package-comments` at `severity: error`). The library is a shared utility, not a service — services depend on it directly without an interface. | [CLAUDE.md](../../CLAUDE.md) §"Style Guidelines" + [docs/server/architecture.md](../server/architecture.md) §"Shared Functionality in Libraries". |
| 3 | **Pure-refactor scope made explicit per layer**: no ARB changes, no a11y work, no caching changes. Pre-audit draft was silent on what *doesn't* need touching; reviewers might otherwise expect i18n / a11y / caching diffs because most planned work in this repo includes them. Explicit non-goals in the PR description. | [docs/client/i18n.md](../client/i18n.md), [docs/client/accessibility.md](../client/accessibility.md), [docs/client/caching.md](../client/caching.md) — all checked, all not-applicable here, called out for clarity. |
| 4 | **Mockito mock regeneration after proto changes is a required step**. When the generated Dart code grows (new `EntityCardCore` class on `AvailableNowItem`/`BriefCTARow`/`DailyItem`), any `@GenerateMocks([WorkshopRepository, ...])` need a `flutter pub run build_runner build` to pick up the new fields. Easy to miss; surfaces as compile errors in test files only. | [docs/client/testing.md](../client/testing.md) §"Mock Generation". |
| 5 | **Aggregator logging must not regress** during the migration. Pre-audit draft mentioned "existing logging unchanged" but didn't tie it to the alert path. Audit notes that `rpc_errors` alerting buckets by `operation`, so keeping every `logging.LoggerWithContext(ctx).With(..., "operation", ...)` chain intact is load-bearing — strip it and the alerts go silent on the affected RPC. | [docs/server/observability.md](../server/observability.md) §"Standard Field Names" + §"Cloud Monitoring Integration" (rpc_errors metric). |
| 6 | **Round-trip test pattern for the conversion helpers tied to `docs/proto_conventions.md`**. Every `AvailableNowKind` and `DailyItemType` enum value must round-trip through `EntityKind` and back without information loss. The test is the early-warning if a new enum value is added on either side without a conversion arm — same shape as the existing proto-conversion round-trip tests. | [docs/proto_conventions.md](../proto_conventions.md) + [CLAUDE.md](../../CLAUDE.md) §"Style Guidelines". |
| 7 | **Connect error handling is not in scope** for this refactor (no new RPCs, no new error-wrapping sites). Pre-audit draft briefly mentioned `connecterr.Internal`; audit removed that mention to keep the plan honest about scope — adding it would suggest new RPCs are being added when they are not. | [docs/server/architecture.md](../server/architecture.md) §"Error wrapping". |
| 8 | **Phase 6 has a soft release-cadence prerequisite, not a code prerequisite**. The "wait one release cycle" rule depends on rollout pace, not on PR readiness. Audit flags this as a PR-description checklist item so reviewers can sanity-check via telemetry before merging Phase 6, rather than failing the merge gate on a timing window. | Operational pattern; not a doc rule per se. |
| 9 | **Each migrated message keeps `optional` on its newly-added `core` field considered carefully**: `EntityCardCore core` is a message field, so it's already pointer-typed in Go and `has*()`-checkable in Dart without the explicit `optional` keyword. The plan does NOT add `optional` to the `core` field itself — it's the default behavior for sub-messages. Optional applies to the *scalar* fields *inside* `EntityCardCore` (`subtitle`, `media_id`). | [CLAUDE.md](../../CLAUDE.md) §"Proto Field Conventions" — the "no Go impact for optional message fields" note. |
| 10 | **No `Tappable` / accessibility primitive changes needed** in Phase 5 even though tap-routing collapses to one helper. The existing tappable widgets (the cards / postcards / inbox rows that already use `Tappable`) keep their semantics; only the *handler body* changes. CI's `npm run lint:dart:a11y` continues to pass because no new `GestureDetector` / `InkWell` is added. | [docs/client/accessibility.md](../client/accessibility.md) §"Lint rules". |
| 11 | **No proto field is removed in this work.** Every wire field this refactor touches is actively read on the client today (verified by ripgrep across `app/lib/` — see [Client field-usage verification](#client-field-usage-verification-2026-05-18)). The server populates both `core` and the deprecated top-level fields **for at least two release cycles** so old clients in the field keep working. Removal is tracked in **[#2020](https://github.com/Type2Fun-Foundation/ripls/issues/2020)** and gated on rollout telemetry. Every deprecated proto field carries an inline `// TODO(#2020): remove after all clients have migrated.` comment for source-level discoverability. | User direction (2026-05-18) — don't break clients. |
| 12 | **Phase 6 is intentionally deferred to #2020, not deleted from the doc.** Keeping the section header with a "deferred" note is more useful than dropping it silently — future readers see the deletion was considered and tracked, not forgotten. The substance of Phase 6 (what gets dropped) lives in #2020's body. | User direction (2026-05-18) — track deletion in a Git issue, not in this plan. |

No changes were made to the problem statement, the recommendation, or the related-issues list — those held up against the reference docs.
