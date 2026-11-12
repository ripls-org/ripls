# Issue #2526: Converge item sharing onto ShareItem/UnshareItem — **step 3: remove the six deprecated per-type RPCs**

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2526
**Filed:** 2026-06-20 | **Last activity:** 2026-07-22 | **State:** OPEN
**Labels:** none | **Assignees:** unassigned
**Context docs:** `docs/proto_conventions.md` (anchor — §"Removing an RPC" is the governing gate), `docs/server/architecture.md` + `docs/server/conventions.md` (server anchors), `docs/client/architecture.md` + `docs/client/conventions.md` (client anchors), `docs/client/sharing.md`, `docs/workflows/item_sharing.md`, `docs/workflows/experience.md`, `docs/workflows/request.md`, `docs/workflows/loan.md`, `docs/workflows/giveaway.md`, `docs/testing/update_system_tests.md`, `docs/simulation.md`, `docs/reviews/api_design_review_prompt.md`, `docs/reviews/testing_review_prompt.md`, `docs/reviews/tech_debt_review_prompt.md`, and the prior plan `docs/issues/2526-converge-item-sharing.md` (steps 1–2, superseded by this doc for step 3).

> **This is v2 of the #2526 plan.** `docs/issues/2526-converge-item-sharing.md` planned steps 1 and 2 and is now history — both **merged to main** in PR #2754 (`share_rpc`, 2026-07-22). This document plans only the remaining **step 3: delete the six deprecated RPCs**. The earlier doc is left in place; diff against it for the design rationale behind `UnshareItem`.

> **Implementation status (branch `share_item_cleanup`): DONE.** All four phases
> landed as four commits. What the plan got wrong, and what the work found, is
> recorded in *Implementation notes* at the bottom — read that before trusting
> the Open Questions above, three of which were answered differently by the code.

## Problem Statement

`CommunityService.ShareItem` / `UnshareItem` are the uniform add/remove path for
every item type. Six per-type RPCs still sit alongside them doing a strict subset
of the same job, all marked `option deprecated = true` with `TODO(#2526)`:

| RPC | Proto | Handler |
|---|---|---|
| `ShareGear` | `community_service.proto:1159` | `server/services/community/gear_sharing.go:170` |
| `UnshareGear` | `community_service.proto:1175` | `server/services/community/gear_sharing.go:547` |
| `ShareExperience` | `experience_service.proto:946` | `server/services/experience/share.go:24` |
| `UnshareExperience` | `experience_service.proto:955` | `server/services/experience/unshare.go:18` |
| `ShareRequest` | `request_service.proto:832` | `server/services/request/sharing.go:21` |
| `UnshareRequest` | `request_service.proto:842` | `server/services/request/sharing.go:200` |

Plus six request/response message pairs (`community_service.proto:482-536`,
`experience_service.proto:125-137`, `request_service.proto:418-443`).

**The app no longer calls any of them.** Step 2 migrated the client at the
*service-wrapper* layer: `CommunityService.shareGear` / `unshareGear`
(`app/lib/services/community_service.dart:495,594`),
`ExperienceService.shareExperience` / `unshareExperience`, and
`RequestService.shareRequest` / `unshareRequest`
(`app/lib/services/request_service.dart:590,619`) kept their Dart signatures but
now build `ShareItemRequest` / `UnshareItemRequest` and call
`_client.shareItem` / `_client.unshareItem`. Every repo, viewmodel, and screen
call site rides on top of those wrappers, so no Dart code path reaches a
deprecated RPC. Verified by grep across `app/lib` (excluding `data/gen/`): zero
references to `ShareGearRequest` / `ShareExperienceRequest` / `ShareRequestRequest`
or their `Unshare*` counterparts.

**What still calls them is test and simulation infrastructure** — 147 Go call
sites and 9 TypeScript call sites, enumerated below. That is the actual work.

### The removal gate is met

The issue body says removal "must be gated on a minimum app version." **That
premise is stale, and `docs/proto_conventions.md` now says so explicitly:**

> "There is no force-upgrade or minimum-version gate in the codebase, so a build
> that never updates keeps calling forever. One release plus a day is the working
> rule, not a guarantee."

The governing rule (`proto_conventions.md` → *Removing an RPC*) is two clauses:

1. **Prove nothing calls it** — "Repository and service wrappers are not callers;
   check for a viewmodel or screen that actually invokes them." ✅ Done above.
2. **Wait one shipped release plus ~24 hours after the client stopped calling it.**
   The migration merged 2026-07-22 (PR #2754) and shipped in **v0.28.0
   (2026-07-28)**. Three further releases have shipped since — v0.28.1 (07-30),
   v0.28.2 (07-31), v0.28.3 (08-01) — and today is **2026-08-08**. ✅ The gate
   is met with four releases and a week of slack against a one-release-plus-a-day
   minimum.

The same section adds a judgement clause: *"prefer to let an RPC removal ride a
release further than the minimum when the surface it breaks is one a stranded
build would hit on launch."* Sharing is not a launch surface — a stranded
pre-v0.28.0 build gets `UNIMPLEMENTED` only when its owner taps a community
toggle or a share/remove action, not on cold start. The extra ride has already
happened anyway.

`lint:proto:breaking` will **not** catch this either way. `buf.yaml` runs
`breaking.use: [WIRE]` only (#2828), and `RPC_NO_DELETE` / `SERVICE_NO_DELETE` /
`MESSAGE_NO_DELETE` are all `FILE`/`PACKAGE`-scoped — `buf.yaml`'s own comment
block calls this out as a known gap. So deleting the six RPCs *together with*
their twelve message types passes cleanly, and a green gate is not evidence of
anything. (WIRE *would* catch repointing a surviving RPC at a different
request/response type — not applicable here, since both ends go at once.)
Nothing gets `reserved` — RPC names are not reused like field numbers.

### What is *not* being removed

The removal deletes **only the RPC handlers and their message types**. The
behavior cores each handler delegates to are live production code reached through
`ShareItem` / `UnshareItem` and must survive untouched:

- `ShareGearToCommunity` / `ShareGearToCommunityWithAvailability` / `UnshareGearFromCommunity` (`community/gear_sharing.go:235,245,567`)
- `ShareExperienceToCommunity` / `shareExperienceToCommunity` / `UnshareExperienceFromCommunity` (`experience/share.go:122`, `experience/unshare.go:39`)
- `ShareRequestToCommunity` / `UnshareRequestFromCommunity` (`request/sharing.go:110,221`)
- Every `Verify*Owner` / `Verify*Viewer` registry method value.

`UpdateGearSharing` and `SetGearAvailability` are **not** deprecated and stay.

### Behavior that leaves with the RPCs

Two checks live only in the deprecated handlers and have **already been dead in
production since v0.28.0** — the client stopped exercising them then. Removing
the RPCs makes that de jure:

- **`AlreadyExists` on re-share.** `ShareExperience` returns
  `experience_already_shared_to_community` (`experience/share.go:68`);
  `ShareRequest` has an equivalent (`TestShareRequest_AlreadyShared`).
  `ShareItem` is idempotent by design and returns success.
- **Silent skip of non-member communities.** `ShareRequest` *warns and skips* a
  community the caller isn't a member of (`request/sharing.go:34`, asserted by
  `TestShareRequest_NonMemberCommunitySkipped`). `ShareItem` **errors** on that
  target (`share_item.go:203`). This is an inverted contract, not a preserved
  one — the test cannot be ported, only deleted.
- **Nothing else.** `ShareItem` enforces `RequireMemberOfActiveCommunity` per
  target community (`share_item.go:203`) and owner-only audience mutation
  (`:113`), so the membership and ownership gates are fully preserved.

Both deltas have been the client's live behavior since v0.28.0. The tests
asserting them (`request/sharing_test.go:83,210`, the `AlreadyExists` case in
`experience/share_test.go:71`) must be *deleted* rather than retargeted — the
contracts they assert no longer exist.

**One user-visible error code dies with them.**
`experience_already_shared_to_community` is produced *only* by the deprecated
handler, and it is a full l10n chain: `experience/share.go:68` →
`app/lib/core/errors/rpc_error_handler.dart:238-239` →
`rpcErrorExperienceAlreadySharedToCommunity` in `app_en.arb:1014-1015` and
`app_es.arb:327`. All four go together, or `npm run lint:dart:arb-orphans`
fails (and translators keep paying for dead copy in every locale). Checked the
other codes in the doomed handlers: `experience_owner_required_for_share`
survives via `VerifyExperienceOwner`, `gear_owner_required_for_share` via
`VerifyGearOwner`, and `request_creator_required_for_share` is emitted twice —
`request/sharing.go:48` (dies) and `:82` inside the surviving owner check
(lives) — so only the experience code orphans. **Re-run that sweep at
implementation time** rather than trusting this list.

## Context from Comments

**The issue has zero comments.** The body is the sole spec and it carries a
now-superseded premise (min-version gating). The prior plan doc
(`2526-converge-item-sharing.md`) plus its audit notes are the real discussion
record, and its "Deferred" section is exactly this work. Everything the plan
below decides that is *not* in the issue body — the soak accounting, the
per-bucket migration targets, the `AlreadyExists` coverage loss — is called out
explicitly here rather than assumed.

## Codebase findings (research)

### Caller inventory (the actual scope)

**Go — 147 call sites**, all test or simulation code, none production:

| Bucket | Sites | Files | Migration target |
|---|---|---|---|
| Unit tests under `server/services/**` | 95 | ~33 | the in-package **core** (`ShareExperienceToCommunity`, `ShareGearToCommunityWithAvailability`, `ShareRequestToCommunity`, `Unshare*FromCommunity`) |
| `server/integration_tests/**` | 45 | 16 | over-the-wire `communityClient.ShareItem` / `UnshareItem` |
| `server/e2e_*.go` (`e2e_loan_test.go`, `e2e_giveaway_test.go`, `e2e_request_test.go`) | 3 | 3 | same as integration |
| `server/simulation/executor.go` | 4 | 1 | `client.Community().ShareItem(...)` |

Almost all 95 unit-test sites are **setup**, not assertions — "put this experience
in this community, then test the thing I actually care about." Those become a
one-line call to the core. The exceptions are the tests that assert per-type
sharing *behavior* and must be retargeted (not deleted) because the behavior is
still live through `ShareItem`:

- `community/gear_sharing_test.go` — `TestService_ShareGear` (`:17`),
  `_SystemMessages` (`:250`), `_AnchorMessageDeduped` (`:355`),
  `_GiveawayConversation` (`:447`)
- `community/gear_sharing_unshare_test.go` — `TestService_UnshareGear` (`:12`)
- `experience/share_test.go` — `TestService_ShareExperience` (`:71`),
  `_OwnerAutoRSVP` (`:269`)
- `request/sharing_test.go` — `TestShareRequest_Success` (`:19`), `_NonOwner`
  (`:174`), `_NonMemberCommunitySkipped` (`:210`), `TestUnshareRequest_Success`
  (`:256`), `_LastCommunity` (`:323`), `_NonOwner` (`:358`),
  `_NotSharedWithCommunity` (`:402`)

**Watch-out:** the cores do **not** perform the owner/membership checks — auth is
hoisted to the caller (`ShareItem`/`UnshareItem`). So `_NonOwner` /
`_NonMemberCommunitySkipped` style tests cannot simply swap the callee: they
assert an auth contract that now lives on `ShareItem`. Each needs to either move
to the `community` package's `share_item_test.go` / `unshare_item_test.go`
(which already have `TestShareItem_OwnerVerificationFails`,
`TestShareItem_ShareToCommunitiesRequiresOwner`, `TestShareItem_MemberOnly`) or
retarget at the type's `Verify*Owner` method value directly. Do **not** delete
them silently. `UnshareRequestFromCommunity` *does* keep its own owner check and
last-community guard inside the core (`request/sharing.go:221`), and
`UnshareExperienceFromCommunity` keeps ownership + active-community checks
(`experience/unshare.go:52,63`) — verify per type rather than assuming.

**TypeScript — 9 call sites** in `e2e/`:

- `e2e/lib/seed/gear.ts:117` — `shareGear` purely to **pin availability** on the
  per-item community. `SaveGearRequest.availability` (field 10, insert-only,
  `gear_service.proto:66`) already exists and the seed's `SaveGear` is always an
  insert → pass `availability` there and delete the `shareGear` call and its
  `CommunityService` client construction entirely.
- `e2e/lib/seed/experiences.ts:98` — `shareExperience` to seed community
  membership → `communityClient.shareItem({ experienceId, shareToCommunityIds: [communityId] })`.
- Spec-level shares to *additional* communities:
  `people-directory.spec.ts:94`, `library-shelf-gear.spec.ts:103,108,197,202`,
  `universal-search.spec.ts:76`, `walkthroughs/gear.spec.ts:170` → `shareItem`
  with `shareToCommunityIds`, plus `setGearAvailability` where the spec depends
  on `FOR_GIVEAWAY` (availability is item-wide since #2492/#2687, so it is one
  call per gear, not one per community). The `shareItem` pattern is already
  established in e2e (`community-promote-nudge.spec.ts:240`,
  `event-invite-home-multi-client.spec.ts:91`,
  `item-creation-adhoc-community.spec.ts:61`).

**Dart — near-zero migration, but not zero.** The service wrappers, repos,
viewmodels, and screens keep working unchanged; the generated stubs disappear
silently. The one hand-written change is the orphaned
`experience_already_shared_to_community` error mapping and its two ARB entries
(above). `npm run generate && npm run lint:dart` proves no Dart *code* reads a
deleted symbol (deleting a proto symbol is a Dart compile error, per
`proto_conventions.md`); `npm run lint:dart:arb-orphans` catches the copy.

**Round-trip tests are in the 95.** `experience/roundtrip_test.go:152` and
`experience/undo_roundtrip_test.go:36` both use `ShareExperience` as setup.
`proto_conventions.md` expects round-trip coverage to exist for these types —
retarget them at the core, never delete them to make the migration smaller.

### Lint carve-out to retire

`server/.golangci.yml:384-390` holds an `SA1019` exclusion scoped by regex to
exactly these six RPC names, with the comment *"Remove when #2526 lands."*
Deleting it is part of this change, and it is also the **self-check**: if the
exclusion is gone and `npm run lint:go:vet` is green, no Go caller survived. The
~14 inline `// SA1019: ...` explanatory comments scattered through
`server/integration_tests/**` and `server/simulation/executor.go` go with their
call sites.

### Proto-reader surfaces (all four must be run)

Per the #2834/#2835 removal experience, a proto symbol has four independent
readers and CI splits them across workflows — a locally-green Go build proves
nothing about the other three:

1. Go — `go build ./... && go test ./...` (+ `-tags=integration`)
2. Dart — `npm run generate && npm run lint:dart` (`flutter analyze`)
3. e2e protobuf-es — `npm run lint:e2e:types` (`tsc --noEmit` in `e2e/`) —
   **run this locally**, it is the one most often missed
4. Web bundle — `npm run build:e2e` / the Playwright workflow suite

### Docs carrying stale references

- `docs/workflows/item_sharing.md:36-37` — "deprecated pending removal (#2526)"
- `docs/client/sharing.md:104` — same, plus the wrapper-layer explanation (the
  wrappers stay; only the "pending removal" clause changes)
- `docs/workflows/experience.md:81-83,270,280,283,290` — line-range citations
  into `share.go` / `unshare.go` that shift when the handlers are deleted
- `docs/workflows/request.md:108,172,182,185,193,638,677` — same, incl. the
  notification-table parenthetical "not on subsequent `ShareRequest` calls"
- `docs/workflows/loan.md:181` and `docs/workflows/giveaway.md:192` — these name
  `CommunityService.ShareGear` as the *availability-set* location. **Already
  stale independent of this change** (availability went item-wide in #2492/#2687;
  the live path is `SetGearAvailability` / `UpdateGearSharing`). Fix while here.
- `docs/proto_conventions.md` — worth adding #2526 as the worked example for
  *Removing an RPC*, the way #2827/#2835 is the worked example for removing a field.
- **`buf.yaml`'s `breaking:` comment block is itself stale.** Its closing lines
  say the policy is "wait the same **two release cycles**" and point at
  `docs/proto_conventions.md → Deprecating and removing fields`. Both are wrong
  since #2835: the wait is **one shipped release plus ~24 hours**, and the
  section is now titled *Removing an RPC*. This change is the natural moment to
  fix it — a reader following that pointer today lands nowhere and infers a
  policy twice as strict as the real one.
- `docs/testing/update_system_tests.md` — the workflow docs' Location /
  Implementation blocks map **1:1** to `server/integration_tests/**`. Editing
  those blocks (above) without re-syncing the tests breaks that contract; run
  `/ripls-update-system-tests` after the workflow-doc edits rather than hand-
  patching.

## Potential Solutions

The removal itself is settled; the open decision is **how the 147 Go test call
sites move**.

### Option A: Retarget by bucket — unit tests → cores, wire tests → `ShareItem`

- **Approach:** Unit tests under `server/services/**` call the in-package core
  directly (`ShareExperienceToCommunity(ctx, expID, cID, ownerID)`); integration,
  `e2e_*`, and simulation build a `CommunityServiceClient` and call `ShareItem` /
  `UnshareItem`. Add a small per-package test helper (e.g.
  `shareExperienceForTest(t, service, expID, cID, ownerID)`) so the 95 unit sites
  become a one-token change and future setup is uniform. Auth-contract tests move
  to `community/share_item_test.go` / `unshare_item_test.go` or retarget at
  `Verify*Owner`.
- **Pros:** Every test ends up exercising the code path that is actually live.
  Unit tests stay in-package with no cross-service wiring (preserves the
  service-independence rule in `server/architecture.md` — `experience` tests must
  not need a `community` service). Wire tests keep genuine RPC coverage. The
  helper makes the bulk edit mechanical and reviewable.
- **Cons:** Largest single diff of the three. The unit-test path loses the
  auth-check-at-the-boundary shape (the core skips owner/membership), so a few
  tests need real thought rather than a swap.
- **Effort:** Medium-large — but ~90% mechanical.

### Option B: Test-only shim RPCs behind a build tag / dev-mode guard

- **Approach:** Keep thin `ShareGear`-shaped helpers alive for tests only,
  outside the proto surface.
- **Pros:** Tiny diff; tests untouched.
- **Cons:** Re-creates the thing being deleted, one layer down. The point of
  #2526 is one path, and a test-only second path means test coverage stops
  matching production. Also leaves the `SA1019` carve-out's intent unmet.
- **Effort:** Small — and wrong.

### Option C: Split into two PRs — proto+server removal, then test migration

- **Approach:** Can't be done in that order (deleting the RPC breaks the tests in
  the same commit). The only viable split is: PR 1 migrates all callers off the
  deprecated RPCs while they still exist; PR 2 deletes proto + handlers +
  carve-out.
- **Pros:** PR 1 is a pure no-op refactor that must keep every test green — a
  strong safety property, and it makes PR 2 a ~200-line deletion that is trivially
  reviewable. Also de-risks: if PR 1 surfaces a coverage gap, PR 2 waits without
  anything half-removed on main.
- **Cons:** Two review cycles; PR 1 alone delivers nothing user-visible.
- **Effort:** Same total, split.

## Recommendation

**Option A, structured as Option C's two-PR split.**

The migration content is Option A — retarget by bucket, cores for unit tests,
`ShareItem`/`UnshareItem` over the wire. Ship it as two PRs because the
caller-migration PR has an unusually strong invariant: *every existing test must
stay green with no assertion changes except the `AlreadyExists` deletions*. That
invariant is the whole safety argument for deleting production RPC handlers, and
it is far easier to hold a reviewer to it when the deletion is not in the same
diff. The second PR is then a mechanical subtraction whose CI signal (Go + Dart +
e2e-types + Playwright, with the `SA1019` carve-out removed) is the proof.

If the two-PR split is unwanted, do it as one PR with the migration and the
deletion in **separate commits** in that order, so the green-tests-before-deletion
property is still visible in review.

Explicitly **not** in scope: unifying the divergent unshare semantics
(soft-archive for requests vs hard-delete for gear/experience, the
last-community guard, the `GEAR_UNSHARED` event). That was deferred by the v1
plan's Open Question 5 and stays deferred — it is a behavior change, this is a
subtraction.

## Open Questions

- **1. One PR or two?** — Recommendation: **two** (migrate, then delete), per
  above. The green-before-delete invariant is the safety case for removing
  production handlers, and it reads much better as its own diff. Fall back to one
  PR with two ordered commits if review bandwidth is the constraint.
- **2. Do the tests for the two dying contracts get deleted or ported?** —
  Recommendation: **delete all three** — `TestShareRequest_AlreadyShared`
  (`request/sharing_test.go:83`), the `AlreadyExists` case inside
  `experience/share_test.go:71`, and `TestShareRequest_NonMemberCommunitySkipped`
  (`:210`). The first two assert a contract `ShareItem` deliberately inverts
  (idempotent, not `AlreadyExists`); the third asserts a *silent skip* where
  `ShareItem` errors, so porting it would assert the opposite of the live
  behavior. In their place, confirm `share_item_test.go` covers (a) re-share is
  idempotent and (b) a non-member target community errors —
  `TestShareItem_MemberOnly` (`:132`) likely covers (b); add (a) if missing.
  Call the three deletions out in the PR body so the coverage change is
  deliberate rather than incidental.
- **2b. Who removes the orphaned `experience_already_shared_to_community`
  copy?** — Recommendation: **the same PR that deletes the handler** (Phase 3),
  all four sites at once — server code string, `rpc_error_handler.dart` case
  arm, `app_en.arb` key + `@`-description, `app_es.arb` key. Splitting it leaves
  either a mapping for an unreachable code or an ARB orphan that fails CI. Do
  **not** reach for `scripts/arb_orphan_allowlist.txt` — it may shrink, never
  grow.
- **3. Where do the auth-contract tests land (`TestShareRequest_NonOwner`,
  `_NonMemberCommunitySkipped`, `TestUnshareRequest_NonOwner`)?** —
  Recommendation: **check per type first.** `UnshareRequestFromCommunity` and
  `UnshareExperienceFromCommunity` keep their own owner checks inside the core,
  so their `_NonOwner` tests retarget with no move. The `Share*` side hoists auth
  to `ShareItem`, so those cases belong in
  `server/services/community/share_item_test.go` alongside the existing
  `TestShareItem_OwnerVerificationFails` / `_ShareToCommunitiesRequiresOwner` /
  `_MemberOnly` — verify they're not already redundant there before porting.
- **4. Add a per-package test helper, or edit 95 call sites inline?** —
  Recommendation: **helper.** One `shareExperienceForTest` / `shareGearForTest` /
  `shareRequestForTest` per package turns the bulk edit into a single reviewable
  definition plus 95 identical call swaps, and gives the next person an obvious
  place to hang setup. Per CLAUDE.md the swaps go through the Edit tool, not
  `sed`/`perl` — a scripted replace here would silently no-op on anchor drift.
- **5. Does the e2e gear seed drop `shareGear` entirely, or swap it for
  `shareItem` + `setGearAvailability`?** — Recommendation: **drop it entirely**
  and pass `availability` to `SaveGear` (field 10 is insert-only and the seed
  always inserts). It removes a whole `CommunityService` client construction from
  the seed path and is strictly less code. Verify the gear service honors
  `SaveGearRequest.availability` on the per-item community before committing to
  this; fall back to `setGearAvailability` after `SaveGear` if it doesn't.
- **6. Should `docs/proto_conventions.md` gain #2526 as the RPC-removal worked
  example?** — Recommendation: **yes**, one sentence in *Removing an RPC*
  mirroring how #2827/#2835 anchors the field-removal section. It records that
  the "prove nothing calls it" clause was satisfied at the *wrapper* layer, which
  is precisely the subtlety that section warns about.
- **7. Anything to do about pre-v0.28.0 builds in the field?** —
  Recommendation: **no.** `proto_conventions.md` is explicit that no
  force-upgrade or min-version gate exists and that one-release-plus-a-day is the
  working rule, not a guarantee. Sharing is not a launch-path surface, and four
  releases have shipped. Building version telemetry to get certainty is a
  separate project the doc already flags; it should not block this.

## Implementation Plan

### Phase 1: Migrate every Go caller off the deprecated RPCs

**Goal:** No Go code calls `Share*`/`Unshare*` per-type RPCs; every existing test
stays green with no assertion changes beyond the deliberate `AlreadyExists`
deletions. The RPCs still exist — this phase is a pure no-op refactor.
**Tasks:**
- [ ] Add per-package test helpers (`shareExperienceForTest`, `shareGearForTest`,
      `shareRequestForTest`, and unshare counterparts) wrapping the live cores.
- [ ] Migrate the 95 `server/services/**` unit-test call sites onto the helpers
      (Edit tool, not scripted replace).
- [ ] Retarget the per-type behavior tests listed in *Codebase findings* — gear
      system-messages / anchor-dedupe / giveaway-conversation, experience
      owner-auto-RSVP, request share/unshare success + guards — at the cores so
      the live behavior stays covered.
- [ ] Retarget the round-trip tests (`experience/roundtrip_test.go:152`,
      `undo_roundtrip_test.go:36`) at the core — do not delete them.
- [ ] Resolve the auth-contract tests per Open Question 3; delete the three
      dying-contract tests per Open Question 2 and add the `ShareItem`
      idempotency assertion if missing.
- [ ] Migrate 45 `server/integration_tests/**` + 3 `server/e2e_*.go` sites to
      `communityClient.ShareItem` / `UnshareItem`; where a gear test relied on
      `ShareGear`'s `availability`, follow with `SetGearAvailability`.
- [ ] Migrate the 4 `server/simulation/executor.go` sites; drop their inline
      `SA1019` comments.
**Validates:** the cores and `ShareItem`/`UnshareItem` are a complete functional
replacement — proven by the whole Go suite passing unchanged.
**Checks:** `npm run generate`, `go build ./...`, `go test ./server/...`,
`go test -tags=integration ./server/...`, `npm run lint:go`.

### Phase 2: Migrate the e2e TypeScript callers

**Goal:** No TS calls a deprecated RPC.
**Tasks:**
- [ ] `e2e/lib/seed/gear.ts` — pass `availability` to `SaveGear`, delete the
      `shareGear` call and its now-unused `CommunityService` client (Open
      Question 5).
- [ ] `e2e/lib/seed/experiences.ts` — swap `shareExperience` for
      `shareItem({ experienceId, shareToCommunityIds: [...] })`.
- [ ] Migrate the 7 spec-level `shareGear` sites (`people-directory`,
      `library-shelf-gear` ×4, `universal-search`, `walkthroughs/gear`) to
      `shareItem` + `setGearAvailability` where giveaway mode matters.
- [ ] Re-render any walkthrough reel whose seeding changed (`walkthroughs/gear.spec.ts`).
**Validates:** the unified path seeds e2e worlds identically.
**Checks:** `npm run lint:e2e:types`, `npm run build:e2e`, the Playwright
workflow suite.

### Phase 3: Delete the RPCs, messages, handlers, and the lint carve-out

**Goal:** The six RPCs and their twelve message types are gone from the proto
surface and the server.
**Tasks:**
- [ ] Proto: delete the six `rpc` declarations and the twelve request/response
      messages (`community_service.proto:482-494,527-536,1153-1179`;
      `experience_service.proto:125-137,940-957`;
      `request_service.proto:418-443,826-844`). **Reserve nothing** — RPC names
      are not reused.
- [ ] Server: delete the six handler funcs, keeping every `*ToCommunity` /
      `*FromCommunity` core and `Verify*` method value intact.
- [ ] Sweep the deleted handlers for `connecterr.UserVisible` codes with no
      surviving producer, and remove each one's full chain. Known: only
      `experience_already_shared_to_community` (`experience/share.go:68` →
      `rpc_error_handler.dart:238-239` → `app_en.arb:1014-1015` +
      `app_es.arb:327`). Re-verify rather than trusting that list.
- [ ] Delete the `SA1019` exclusion at `server/.golangci.yml:384-390` and its
      comment block.
- [ ] Confirm no `TODO(#2526)` markers remain anywhere.
- [ ] Regenerate and verify all four proto-reader surfaces.
**Validates:** the deprecation lifecycle closes; a green build with the carve-out
removed is the proof no caller survived.
**Checks:** `npm run generate`, `go build ./... && go test ./server/...`
(+ `-tags=integration`), `npm run lint:dart`, `npm run lint:dart:arb-orphans`,
`npm run lint:e2e:types`, `npm run lint`, the Playwright suite.
`npm run lint:proto:breaking` will pass and means nothing here (WIRE-only, see
above) — do not treat it as evidence.

### Phase 4: Docs and issue closure

**Goal:** No durable doc still describes the per-type RPCs as existing.
**Tasks:**
- [ ] `docs/workflows/item_sharing.md`, `docs/client/sharing.md` — drop the
      "deprecated pending removal (#2526)" clauses; the client wrappers stay and
      the docs should say so plainly.
- [ ] `docs/workflows/experience.md`, `docs/workflows/request.md` — remove the
      legacy-RPC rows and fix the `share.go` / `unshare.go` / `sharing.go` line
      citations that shift; fix the request notification-table parenthetical.
- [ ] `docs/workflows/loan.md:181`, `docs/workflows/giveaway.md:192` — repoint
      the availability-set location at `SetGearAvailability` / `UpdateGearSharing`
      (stale independent of this change).
- [ ] `docs/proto_conventions.md` — add #2526 as the *Removing an RPC* worked
      example (Open Question 6).
- [ ] `buf.yaml` — fix the stale `breaking:` comment ("two release cycles" →
      one shipped release plus ~24h; repoint at *Removing an RPC*).
- [ ] Re-sync the system tests to the edited workflow docs via
      `/ripls-update-system-tests` (the 1:1 contract in
      `docs/testing/update_system_tests.md`).
- [ ] Advance `freshness:` stamps on every touched doc; run
      `npm run generate:context-map` if any `context:` block changes.
- [ ] Close #2526.
**Validates:** `npm run lint:context-map` green; the workflow docs and their
system tests still agree; #2526's three-step plan is complete.

## Related Issues
- **#2492** — phone-first pivot, CREATE-1; introduced `ShareItem`, the design
  this converged onto.
- **#2630** — tiered `ShareItem` permissions (link-only reshare for members,
  owner-only audience mutation); the auth contract the migrated tests assert.
- **#2687** — item-wide gear availability via `SaveGearRequest.availability`;
  underpins dropping the e2e seed's `ShareGear` availability-pin.
- **#2835 / #2834** — the prior post-soak proto removal; source of the
  four-proto-reader-surfaces checklist and the one-shipped-release soak policy
  now codified in `proto_conventions.md`.
- **#1611** — Go lint enforcement; owner of the `SA1019` carve-out this change
  retires.
- **#2828** — set `buf breaking` to `WIRE`-only; the reason a green
  `lint:proto:breaking` is not evidence here.

---

## Implementation notes (what the work actually found)

Four commits on `share_item_cleanup`, in the order the plan laid out. The plan
held up structurally; three of its specific claims did not.

**1. There was no `AlreadyExists` test to delete, and `TestShareRequest_AlreadyShared`
asserts the opposite of what its name suggests.** Open Question 2 said to delete
three tests. In fact: `TestService_ShareExperience` never had an `AlreadyExists`
subtest — the contract existed in the handler and was simply untested — and
`TestShareRequest_AlreadyShared` asserts **idempotence** (no duplicate junction
row), which is exactly what `ShareItem` relies on. It was renamed and retargeted,
not deleted. Only **one** test was genuinely deleted:
`TestShareRequest_NonMemberCommunitySkipped`, whose silent-skip contract
`ShareItem` inverts into an error. Its replacement is a new
`TestShareItem_NonMemberTargetCommunityRejected`.

**2. The availability call must come BEFORE the share, and the integration suite
caught it.** The plan (and the first implementation) treated `SetGearAvailability`
as a follow-up to `ShareItem`. `TestGiveaway_Example1_GivingAwayItem` failed with
"User B did not receive 'New giveaway' notification": the `GEAR_SHARED` community
event picks its copy from the availability at *share* time, so sharing first and
setting the mode second emits a "New loan" notification that the later flip cannot
retract. Every helper and call site now sets availability first. This is not a
test artifact — it is why `SaveGear` carries the creation-time choice into the
per-item community's first share (#2687), and it is now recorded in
`docs/workflows/giveaway.md`.

**3. Removing the handlers cascaded twice, exactly as #2835 predicted.** Dropping
the `SA1019` carve-out surfaced two unused test helpers; dropping the
`AlreadyExists` path made `shareExperienceToCommunity`'s `newlyShared` return dead
and tripped `unparam`, collapsing the signature to `error`. Separately,
`server/integration_tests/test_helpers.go` was already at 981 lines, so the new
wire helpers pushed it past the 1,000-line gate — they live in a sibling
`sharing_helpers.go` rather than behind a `go-line-count-allow` hatch.

**Coverage added rather than lost.** `ShareGearToCommunity` — the ItemSharer hook
ShareItem actually calls for gear, and the one that inherits existing availability
— had no unit test at all; it has one now. `TestService_UnshareGear`'s "unsharing
is idempotent" subtest was vacuous (the preceding subtest re-shared the gear, so
its "unshare again" was a first unshare); it now unshares twice.

**Verification.** All four proto readers green: `go build ./server/...` +
`go test ./server/...`, `flutter analyze` + `flutter test` (4310 passed),
`npm run lint:e2e:types`, and `npm run build:e2e`. Integration suite green
(84 tests) both before and after the deletion; the three `TestEndToEnd_*`
lifecycle tests green; full `npm run lint` green with the `SA1019` carve-out
removed — which is the evidence no Go caller survived.
`npm run lint:proto:breaking` also passes, and means nothing, as predicted.

**Not done, deliberately.** Freshness stamps were not advanced:
`docs/context_map.md` says only the `ripls-doc-freshness` skill writes them, and
a hand-edited stamp would rubber-stamp unverified prose. The nightly ratchet will
pick these docs up because their globbed code changed.

---

## Audit notes (ripls-audit)

**Engaged dimensions:** `api_design` (RPC + message removal, backwards
compatibility), `testing` (147 call sites, coverage deltas), `tech_debt`
(deprecation lifecycle, doc/comment staleness), `code_quality` (TODO hygiene,
dead code), `error_handling` (orphaned `connecterr` codes), `architecture`
(service independence in the unit-test migration target). Not engaged:
accessibility, performance, dependency, observability (no logging surface
changes — the deleted handlers' `slog` lines live in the surviving cores).

**Docs consulted:** `docs/proto_conventions.md` (anchor — §*Removing an RPC* is
the governing gate, plus §*Removing a field from `proto/ripls/api`* for the soak
policy), `docs/server/architecture.md` + `docs/server/conventions.md` (server
anchors), `docs/client/architecture.md` + `docs/client/conventions.md` (client
anchors), `docs/testing/architecture.md`, `docs/testing/update_system_tests.md`,
`docs/client/sharing.md`, `docs/workflows/item_sharing.md`,
`docs/workflows/experience.md`, `docs/workflows/request.md`,
`docs/workflows/loan.md`, `docs/workflows/giveaway.md`, `docs/simulation.md`,
and the review rubrics `api_design`, `testing`, `tech_debt`, `code_quality`,
`error_handling`. Also read as primary sources: `buf.yaml`, `server/.golangci.yml`,
`scripts/check_arb_orphans.js`, `scripts/check_l10n_toml_parity.js`.

**Findings applied to the draft:**

1. **`api_design` §4 (backwards compat) — the `buf breaking` claim was
   under-specified.** The draft said the gate "will not catch this" without
   establishing that deleting the twelve *messages* is also safe.
   `MESSAGE_NO_DELETE` is `FILE`-scoped and `buf.yaml` runs `WIRE` only (#2828),
   so it is — but that had to be verified, not assumed. Rewritten with the
   citation, including the one WIRE rule that *is* adjacent
   (`RPC_SAME_REQUEST_TYPE`) and why it doesn't apply.
2. **`error_handling` — a live user-visible error code was going to be
   orphaned.** `experience_already_shared_to_community` has no surviving
   producer and carries a four-site l10n chain through `rpc_error_handler.dart`
   into `app_en.arb` + `app_es.arb`. The draft's "Dart — zero migration" was
   wrong. Corrected, with the sweep added to Phase 3 and the two sibling codes
   (`experience_owner_required_for_share`, `request_creator_required_for_share`)
   checked and confirmed to survive.
3. **`testing` §4 (assertion quality) — a third dying contract was missed.**
   `ShareRequest` *silently skips* non-member communities; `ShareItem` errors on
   them. `TestShareRequest_NonMemberCommunitySkipped` therefore asserts the
   inverse of live behavior and cannot be ported. Added alongside the two
   `AlreadyExists` cases, with the replacement coverage named.
4. **`testing` §1 (critical-path coverage) — round-trip tests were invisible in
   the "95 setup call sites" framing.** `experience/roundtrip_test.go` and
   `undo_roundtrip_test.go` both seed via `ShareExperience`;
   `proto_conventions.md` expects that coverage to exist. Called out explicitly
   so a bulk migration doesn't quietly drop them.
5. **`tech_debt` §6 (outdated docs / stale comments) — `buf.yaml`'s own comment
   block is stale.** It cites "two release cycles" and a
   `proto_conventions.md` section title that no longer exists (both superseded
   by #2835). A reader following that pointer today infers a policy twice as
   strict as the real one. Added to Phase 4.
6. **`docs/testing/update_system_tests.md` — the 1:1 workflow-doc ↔ system-test
   contract.** Phase 4 edits `workflows/experience.md` and `workflows/request.md`
   Location/Implementation blocks, which is exactly what that contract binds.
   Added the `/ripls-update-system-tests` re-sync step.
7. **`lint:dart:arb-orphans` added to Phase 3's check list**, and the
   `arb_orphan_allowlist.txt` escape hatch explicitly ruled out (it may shrink,
   never grow).

**No blocking compliance issues.** The removal gate in
`docs/proto_conventions.md` §*Removing an RPC* is met on both clauses with wide
margin (four shipped releases against a one-release minimum; zero live callers
proven at the layer the doc warns about — viewmodels/screens, not wrappers). The
unit-test migration target (in-package cores, not a cross-service `ShareItem`
call) is the one that preserves `server/architecture.md`'s service-independence
rule, which is why Option A specifies it that way rather than routing every test
through `CommunityService`. The issue body's "minimum app version" premise is
contradicted by the codebase and is corrected in the plan rather than carried
forward.
