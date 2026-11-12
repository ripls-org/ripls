# Issue #2404: Migrate Android build toolchain together: AGP 8→9 + Gradle 8→9 (coupled majors)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2404
**Filed:** 2026-06-08 | **Last activity:** 2026-06-08
**Labels:** dependencies | **Assignees:** unassigned
**Context docs:** none provided (this plan reads `docs/dependency_automation.md` §6–8 as background)

## Problem Statement

Renovate wants to bump the Android build toolchain to its v9 line:

- **Android Gradle Plugin** `8.13.2 → 9.2.1` (`app/android/settings.gradle.kts`, plugin
  `com.android.application`)
- **Gradle wrapper** `8.14.5 → 9.5.1` (`app/android/gradle/wrapper/gradle-wrapper.properties`)

It opened them as two PRs (#2391 AGP, #2389 Gradle), both of which failed the required
**Android Release Build** (`release-build`) check. They're interdependent majors: AGP 9
*requires* Gradle ≥ 9.4.1 and drops APIs AGP-8-era Gradle relied on, so neither PR can go
green alone. Both are now closed in favor of doing the migration as **one coordinated
branch**, validated by a human against the real `release-build` job (the §8
`claude_fix_renovate_ci` auto-fixer can't run the Android release build locally, so it's the
wrong tool — and the issue documents exactly this).

The work has two distinct halves:

1. **Renovate config** — group AGP + the Gradle wrapper in `renovate.json` so future
   toolchain majors arrive as **one** PR and these don't reopen as two.
2. **The migration itself** — flip the two version literals on one branch and resolve
   whatever AGP-9 / Gradle-9 fallout the `release-build` job surfaces.

**A key reframing from codebase research:** the migration is far less open-ended than the
issue's task list implies, because **`versions.env` already satisfies every AGP 9.2 floor**,
the Flutter migrator has **already written the AGP-9 opt-out flags** into `gradle.properties`,
and we're **already on the AGP-9-supporting Flutter line** (3.44.1 — 3.44 is where Flutter
added AGP 9 support, and it's the latest stable). So the only real work beyond flipping two
version literals is **upgrading the third-party plugins that don't yet support AGP 9** — not
JDK, NDK, Kotlin, compileSdk, DSL, or the Flutter SDK itself. Per maintainer direction we take
that head-on (Option B below) and **push every dependency forward together as far as we can**,
rather than doing the minimal toolchain-only bump and stopping.

## Context from Comments

**The issue has no comments.** All context lives in the description and the two closed PRs.
That's fine here — the issue is unusually well-specified (it enumerates the coupling, the
observed failure, why the auto-fixer is excluded, and a task checklist). The only thing the
codebase can't answer on its own is **which specific third-party plugins need an AGP-9-ready
bump** to clear the "40 packages incompatible" failure — and that's settled empirically by the
first `release-build` run, not by discussion. That becomes the headline open question below.

Observed failure quoted on the AGP-only PR (#2391):

```
FAILURE: Build failed with an exception.
40 packages have newer versions incompatible with dependency constraints.
```

That specific message is the AAR-metadata / dependency-constraint check. On an AGP-9-only PR
it's expected (AGP 9 ran against a Gradle 8 wrapper it's incompatible with, and against
plugin AARs that haven't published AGP-9-compatible constraints). Whether it *persists* once
both halves are bumped together is the crux of Phase 2.

## Findings from codebase + upstream research

This is the load-bearing section — it converts most of the issue's "check X" tasks from open
questions into already-answered facts, and isolates the one that isn't.

### AGP 9.2 requirements vs. what the repo already has

Per the [AGP 9.2.0 release notes](https://developer.android.com/build/releases/agp-9-2-0-release-notes):

| AGP 9.2 requires | Repo has today | Source | Status |
|---|---|---|---|
| Gradle ≥ **9.4.1** | bumping wrapper to **9.5.1** | `gradle-wrapper.properties` | ✓ satisfied by this bump |
| JDK ≥ **17** | `JAVA_VERSION=17` | `versions.env` | ✓ already met — **no runner rebuild for JDK** |
| Build-tools / `compileSdk` **36** | app forces `compileSdk = 36` (and subproject loop forces 36) | `app/android/app/build.gradle.kts`, `app/android/build.gradle.kts` | ✓ already met |
| NDK default **28.2.13676358** | `NDK_VERSION=28.2.13676358` | `versions.env` | ✓ **exact match** to AGP 9.2's default |
| KGP ≥ **2.2.10** (AGP 9 hard floor; auto-upgrades lower) | Kotlin `2.3.21` | `settings.gradle.kts` | ✓ already above floor → **Kotlin bump NOT forced** |

The headline consequence: **the toolchain *around* AGP/Gradle is already in spec.** The issue
asks us to "check the JDK floor," "check Kotlin compatibility," and "mind compileSdk/ndkVersion"
— all three already pass. In particular, AGP 9 forcing a Kotlin floor was the stated trigger
for *maybe* grouping Kotlin and pulling in #2383 (2.4.0); since 2.3.21 already clears the 2.2.10
floor, **neither is required**.

### AGP-9 migration items that are already done or low-risk

- **Opt-out flags already present.** `app/android/gradle.properties` carries
  `android.builtInKotlin=false` and `android.newDsl=false`, both "added automatically by
  Flutter migrator" (per the inline comments; committed in `b8f3e8316`). These are exactly the
  AGP-9 escape hatches: `newDsl=false` keeps the legacy variant DSL (which our code uses, see
  below) working under AGP 9, and `builtInKotlin=false` keeps the separate `kotlin-android`
  plugin path valid (AGP 9 enables built-in Kotlin by default, and *applying* `kotlin-android`
  under built-in-Kotlin is a hard build failure — so this flag is what prevents that). Both
  opt-outs are slated for removal in AGP 10, so this migration intentionally leaves them set
  and defers the "remove the legacy path" work to the AGP-10 cycle.
- **R8 `-keepattributes` tightening is a non-issue for our rules.** AGP 9.2 makes
  `-keepattributes *` stop matching runtime-invisible annotations. Our
  `app/android/app/proguard-rules.pro` (25 lines) has **no `-keepattributes` directives at
  all**, so our custom rules are unaffected. Flutter's bundled `proguard-android-optimize.txt`
  and plugin-consumer rules are the only place this could bite; the `release-build` job
  (`flutter build apk --release`, which runs R8) is the detector.
- **Removed AGP-9 features we don't use:** density-split APKs (we ship via app bundle /
  standard APK) and Wear-app embedding. No action.

### The one genuine risk: legacy DSL usage + Flutter/plugin AGP-9 readiness

- **Legacy DSL usage in our build.** `app/android/build.gradle.kts` reaches into
  `com.android.build.gradle.LibraryExtension` in two `subprojects {}` loops (the
  missing-namespace fix and the `compileSdk = 36` override for old plugin AARs). That's the
  *old* AGP API surface AGP 9 hides by default. With `android.newDsl=false` it should remain
  available — but this is the most likely spot for a compile-time break in the build scripts,
  and it must be confirmed against `release-build`, not assumed.
- **We are already on the AGP-9-supporting Flutter line.** Flutter **3.44** is the release
  that introduced AGP 9 support (added the `builtInKotlin`/`newDsl` opt-out migrators, bumped
  the KGP-error minimum, added `CommonExtension` wrappers for the 8.x→9.0 signature change),
  and **3.44 is the latest stable as of mid-2026**. The repo is pinned to **3.44.1** — a patch
  of that line. So the **Flutter SDK is *not* the missing lever**; bumping `FLUTTER_VERSION`
  has nowhere higher to go on stable, and the opt-out flags this Flutter added are already in
  our `gradle.properties`.
- **The real ecosystem blocker is third-party plugin readiness, not the SDK.** Flutter's
  umbrella [flutter/flutter#181557 "Flutter Supports AGP 9"](https://github.com/flutter/flutter/issues/181557)
  and its sub-issue [#181383 "Flutter maintained plugins should support AGP 9.0"](https://github.com/flutter/flutter/issues/181383)
  are **open, P1, in progress as of mid-2026**. Plugins still applying `kotlin-android` or the
  legacy DSL fail under AGP 9 (e.g.
  [wakelock_plus#117](https://github.com/fluttercommunity/wakelock_plus/issues/117): "AGP 9
  incompatibility — kotlin-android plugin must be removed"). Our app pulls in a large Android
  plugin set — first-party (`camera`, `video_player`, `share_plus`, `url_launcher`,
  `google_maps_flutter`, the `firebase_*` family, `image_picker`, `geolocator`) and community
  (`flutter_local_notifications`, `add_2_calendar`, `app_badge_plus`, `flutter_secure_storage`,
  `flutter_timezone`, `play_install_referrer`, `gal`, `device_info_plus`). The "40 packages …
  incompatible with dependency constraints" failure is most plausibly **plugin AAR metadata /
  legacy-Kotlin-plugin application**, not our own config. So the path to green is
  **upgrading the offending plugins to AGP-9-ready releases**, which is exactly what Option B
  (below) does.
- **`mapbox_maps_flutter` is pinned and Renovate-disabled — bump it forward with the rest.**
  It's pinned **exact** to `2.19.1` and Renovate-disabled. Per maintainer direction we push
  everything forward together: if AGP 9 needs a newer mapbox, we bump it as part of this branch
  like any other plugin — the Renovate disable is a process default, not a merge gate, and no
  special manual-QA step is assumed. (`protobuf`, `mockito`/`analyzer` are also capped, but
  those are codegen/test-stack pins unlikely to gate the Android release build.) The only thing
  that genuinely can't be solved from our repo is a plugin with **no AGP-9-compatible release
  published at all** — that's the lone Option-C trigger.

### How this interacts with the dependency-automation machinery

- **Managers involved:** `gradle` (the plugins in `settings.gradle.kts`, incl.
  `com.android.application`) and `gradle-wrapper` (the wrapper properties, depName `gradle`).
  Both are in the §3 auto-merge manager set, but the global "majors always wait" rule keeps a
  coupled *major* PR manual — which is exactly what we want.
- **Not a `needs runner rebuild` change.** The Android SDK/AGP is *not* baked into the runner
  image (`docs/dependency_automation.md` §5: AGP auto-downloads build-tools/platform/cmake at
  build time; only `cmdline-tools`, `platform-tools`, NDK, and JDK are baked). AGP/Gradle live
  in-repo. So this PR does **not** touch `versions.env` and does **not** need a runner-image
  republish — it just needs `release-build` to pass on the existing image (which already has
  JDK 17 + NDK 28.2.13676358).
- **`dev.flutter.*` plugins are already Renovate-disabled** (SDK-provided via `includeBuild`),
  so grouping AGP/Gradle won't collide with them.

## Potential Solutions

The two halves are independent design decisions, so each gets its own option set.

### Half 1 — Renovate grouping

#### Option A: Group AGP + Gradle wrapper only (recommended)
- **Approach:** One `packageRule` with `groupName: "android gradle toolchain"` matching
  `["com.android.application", "gradle"]` (optionally `com.android.library` defensively),
  spanning the `gradle` and `gradle-wrapper` managers. Sets **only** `groupName` — no
  `automerge` override — so the existing rules decide merge policy (minor/patch auto-merge via
  the §3 manager rule, majors manual via the global major rule).
- **Pros:** Minimal, surgical; directly satisfies "arrive as one PR" and "don't reopen as
  two"; doesn't disturb the carefully-ordered automerge/label invariant in `renovate.json`.
- **Cons:** Couples routine Gradle wrapper patches with AGP patches into one PR — but they're
  the same toolchain and both auto-merge on green anyway, so this is a feature.
- **Effort:** Small.

#### Option B: Also permanently group Kotlin (`org.jetbrains.kotlin.android`)
- **Approach:** Add Kotlin to the group from Option A.
- **Pros:** Would auto-couple a future AGP-forced Kotlin floor.
- **Cons:** **Not justified.** AGP 9 only needs KGP ≥ 2.2.10 and we're at 2.3.21, so Kotlin
  isn't coupled in practice. Permanently grouping it drags unrelated Kotlin minor bumps (which
  often need their own code migration) into the toolchain PR and couples #2383 to this for no
  reason.
- **Effort:** Small, but adds long-term false coupling.

#### Option C: Leave Renovate alone, do a one-off manual bump
- **Approach:** Skip the grouping; just bump both files by hand on a branch.
- **Pros:** Fastest path to *this* migration.
- **Cons:** Fails the issue's first task outright — the next AGP/Gradle major reopens as two
  failing PRs and we repeat this exercise. Rejected.
- **Effort:** Small now, recurring cost forever.

### Half 2 — The migration branch

> **Chosen: Option B** (per maintainer direction — go straight for the full
> toolchain-plus-plugin upgrade rather than the minimal toolchain-only bump, and **push every
> dependency forward together as far as we can**). Corrected for the research above: because
> we're already on the AGP-9 Flutter line (3.44.1, latest stable), Option B is **plugin
> upgrades coupled with the toolchain bump**, *not* a Flutter SDK jump — which makes it lighter
> than a Flutter upgrade would be (likely **no `FLUTTER_VERSION` change → no runner rebuild**).
> No plugin bump is assumed to require manual QA; pinned/disabled plugins (incl.
> `mapbox_maps_flutter`) get bumped forward with everything else.

#### Option A: Toolchain-only bump, no plugin upgrades
- **Approach:** Flip only AGP→9.2.1 and Gradle→9.5.1, keep all plugins at their current pins,
  and try to make `release-build` pass with build-script/proguard fixes alone.
- **Pros:** Smallest possible diff; isolates "is it *our* config" from "is it the plugins."
- **Cons:** Almost certainly walls out — the "40 packages incompatible" failure points at
  plugin AAR metadata / legacy-Kotlin-plugin application, which can't be fixed without
  upgrading the plugins. Useful only as a *diagnostic first push*, not a landing strategy.
- **Effort:** Small, but expected to be insufficient.

#### Option B (CHOSEN): Couple the AGP-9-incompatible plugin upgrades with the toolchain bump
- **Approach:** One branch flips AGP→9.2.1 and Gradle→9.5.1 **and** upgrades the Android
  plugins that don't yet support AGP 9 to their AGP-9-ready releases (driven by what
  `release-build` reports failing). Keep the `builtInKotlin=false`/`newDsl=false` opt-out flags
  (the supported Flutter-3.44 transition path). Bump to the newest `3.44.x` Flutter patch *only
  if* one exists with relevant plugin/AGP fixes — otherwise `FLUTTER_VERSION` stays put.
- **Pros:** The actual path to green given the failure is plugin-side; stays an Android/pub PR
  (no SDK jump) so — assuming `FLUTTER_VERSION` is unchanged — it avoids `needs runner rebuild`
  / §5 candidate validation / runner republish entirely. Delivers v9 in one coordinated change,
  matching the issue's intent.
- **Cons:** Pub-dependency churn across many plugins, each a potential code-migration surface
  (Riverpod/go_router-style breaking minors have bitten before — that's *why* `pub` is
  manual-review in `renovate.json`). We push all of them forward together and let
  `flutter analyze` / `flutter test` / `release-build` catch the fallout.
- **Effort:** Medium–Large (dominated by however many plugins need bumping and how much Dart-side
  migration their newer majors carry).

#### Option C: Defer and document a hold (fallback only)
- **Approach:** Used **only** if some plugin has **no AGP-9-compatible release published at
  all** (genuinely unfixable from our repo). Add `allowedVersions: "<9.0.0"` caps on
  `com.android.application` and `gradle` in `renovate.json`, document the reason in the
  *Held-back dependencies* section of `docs/dependency_automation.md`, and revisit when the
  blocking plugin ships AGP-9 support.
- **Pros:** Stops the two PRs reopening red every Monday; makes the hold explicit and §8
  auto-fixer-aware (its pre-gate skips capped deps); keeps the backlog honest.
- **Cons:** Doesn't deliver v9. Documented-stalemate outcome, reserved for a true upstream gap —
  not for any plugin we *could* bump.
- **Effort:** Small.

## Recommendation

**Half 1: Option A** (group AGP + Gradle wrapper, exclude Kotlin). It's the minimal change
that satisfies "one PR / don't reopen as two," and the research shows Kotlin grouping buys
nothing because the floor is already cleared.

**Half 2: Option B, per maintainer direction** — couple the AGP-9-incompatible plugin upgrades
with the toolchain bump on one branch and **push every dependency forward together as far as we
can**, driving straight for a green `release-build`. The research reshapes what Option B costs:
since we're already on the AGP-9 Flutter line (3.44.1, latest stable), this is **not** a Flutter
SDK upgrade — it's a toolchain bump plus `pub` plugin bumps, which (with `FLUTTER_VERSION`
unchanged) keeps it out of `needs runner rebuild` territory. Use Option A purely as a
**diagnostic first push** to read the failure set, then bump the offending plugins — including
pinned/Renovate-disabled ones like `mapbox_maps_flutter`, which we treat as ordinary bumps with
no special manual-QA gate. Option C is reserved for the single case where a plugin has **no
AGP-9-compatible release at all** and can't be fixed from our repo.

The migration must be a **single branch** validated by a human against the **required
`release-build` job plus a local `flutter build apk --release`** — never split, and never
handed to the §8 auto-fixer (it can't run the Android release build). Because plugin majors can
carry Dart-side API changes, the branch must also be green on `flutter analyze` + `flutter
test` (the normal automated gates) before merge — no manual QA step is assumed.

## Open Questions

- **Which specific plugins lack an AGP-9-compatible release?** — Recommendation: treat as
  **unknown until measured**, then bump every one that has a fix. Push the Half-1 grouping +
  Half-2 toolchain bump as a diagnostic first push, read the `release-build` log, and bump the
  named plugins forward (including pinned/disabled ones like `mapbox_maps_flutter`). The plan
  defers *which* plugins to data, not *whether* to bump them — under the "push everything
  forward together" directive, the answer for any plugin with an AGP-9 release is "bump it."
  Only a plugin with **no** AGP-9 release at all triggers Option C.
- **Should we also move to the newest `3.44.x` Flutter patch?** — Recommendation: only if a
  patch exists carrying relevant AGP-9/plugin fixes; otherwise leave `FLUTTER_VERSION=3.44.1`.
  Keeping it unchanged is what keeps this out of `needs runner rebuild` / §5 candidate-image
  territory. If a patch *is* needed, that single line becomes a `needs runner rebuild` change
  and rides the §5 → `build_runner_image` → `recreate_runners` machinery — flag it loudly in
  the PR if so.
- **Keep `android.builtInKotlin=false` / `android.newDsl=false`, or migrate off them now?** —
  Recommendation: **keep them.** They're the supported AGP-9 transition path and removing them
  means rewriting the legacy-DSL `subprojects {}` loops in `build.gradle.kts` and dropping the
  separate `kotlin-android` plugin — a larger change that belongs to the AGP-10 prep cycle (a
  future issue), not this coupled-major bump. File that as a follow-up TODO if we want it
  tracked.
- **Remove `android.enableJetifier=true`?** — Recommendation: **out of scope for this issue,
  but worth a quick check.** Jetifier is deprecated in AGP 9 (default `false`, removed in
  AGP 10). It's only needed if some dependency still references the old `android.support.*`
  libraries. Try dropping it in a *separate* follow-up and see if `release-build` still passes;
  don't bundle that risk into the toolchain bump.
- **Does the coupled major PR route correctly through human review once grouped?** —
  Recommendation: yes by construction — the global "majors always wait" rule keeps
  `automerge:false`, and the group rule sets only `groupName`. Confirm the grouped PR is
  *not* auto-labeled `automerge` (the invariant: `automerge:true` IFF the `automerge` label).
- **Exact required-check name** — Recommendation: it's the **Android Release Build** workflow's
  `release-build` job (`.github/workflows/release_build_android.yaml`). Validate against that
  exact job; on PRs it gates real work behind a `changes` filter, but a branch touching
  `app/**` will trigger the full build.

## Implementation Plan

### Phase 1: Group AGP + Gradle in `renovate.json`
**Goal:** Future toolchain majors arrive as one PR; #2389/#2391 don't reopen as two.
**Tasks:**
- [ ] Add a `packageRule` with `groupName: "android gradle toolchain"`,
  `matchPackageNames: ["com.android.application", "com.android.library", "gradle"]`, setting
  **only** `groupName` (no `automerge`, no label) so the existing major/minor rules still
  govern merge policy.
- [ ] Place it among the other `groupName` rules (its position doesn't affect automerge since
  it sets none, but keep it near the onnx/go-toolchain grouping rules for readability).
- [ ] Add a one-line `description` on the rule (matches the file's convention) explaining the
  AGP↔Gradle coupling and pointing at this issue.
- [ ] Do **not** add `org.jetbrains.kotlin.android` (Kotlin floor already cleared; avoid false
  coupling).
**Validates:** Renovate now proposes AGP + Gradle wrapper as a single branch. (Sanity-check by
reasoning about the rule against the two closed PRs; full confirmation comes when Renovate next
runs, but the change is self-evidently correct from the manager/packageName mapping.)

### Phase 2: Coupled toolchain bump (diagnostic first push)
**Goal:** Move both toolchain literals together and read the real failure set off CI.
**Tasks:**
- [ ] `settings.gradle.kts`: `com.android.application` `8.13.2 → 9.2.1`.
- [ ] `gradle-wrapper.properties`: `distributionUrl` → `gradle-9.5.1-all.zip`.
- [ ] Leave `gradle.properties` opt-out flags (`builtInKotlin=false`, `newDsl=false`) and
  Kotlin `2.3.21` **unchanged** (research shows neither needs to move).
- [ ] Push and read the first `release-build` log; **enumerate the incompatible packages**
  (the "40 packages" list) and the failure class (DSL vs AAR-metadata vs R8).
**Validates:** Produces the concrete plugin hit-list that drives Phase 3 — converts "which
plugins?" from a guess into a CI-reported fact.

### Phase 3: Push every dependency forward together
**Goal:** Bump everything that can move to its AGP-9-ready release on the same branch.
**Tasks:**
- [ ] For each plugin the Phase-2 log flags incompatible, bump it in `app/pubspec.yaml` to its
  AGP-9-compatible release — **including pinned/Renovate-disabled ones** (`mapbox_maps_flutter`
  especially); no manual-QA gate is assumed, treat them as ordinary bumps.
- [ ] Fix any build-script DSL breaks (e.g. `com.android.build.gradle.LibraryExtension` in the
  `build.gradle.kts` `subprojects {}` loops); confirm `newDsl=false` is honored.
- [ ] Adjust our config only if needed (extend the existing `resolutionStrategy.force` /
  `compileSdk = 36` override loop; touch `proguard-rules.pro` only on a real R8 regression).
- [ ] Resolve any Dart-side API migrations the plugin majors carry (Riverpod/go_router-style
  breakers) until `flutter analyze` + `flutter test` are green.
- [ ] If — and only if — a flagged plugin has **no** AGP-9 release published at all, peel it off
  to Option C (cap + *Held-back dependencies* entry); everything else moves forward.
**Validates:** A single coordinated branch where the toolchain and its whole plugin tail are
AGP-9-ready together.

### Phase 4: Validate and land
**Goal:** Green on every automated gate, then merge as one coordinated PR.
**Tasks:**
- [ ] Required **Android Release Build** (`release-build`) green.
- [ ] Local `cd app && flutter build apk --release --dart-define-from-file=env.local.json`
  succeeds.
- [ ] `flutter analyze` + `flutter test` green (catches plugin-major Dart API fallout). Any
  Dart edited to accommodate a plugin major must keep its existing **ViewModel tests** green
  and add coverage if the change introduces new branching — per `docs/client/testing.md`
  (ViewModel-first, `ProviderContainer` + Mockito).
- [ ] **Coverage caveat (not a gate):** the automated checks do **not** exercise Android
  *runtime* behavior — `release-build` only proves the APK compiles + R8 succeeds,
  `flutter test` covers ViewModel logic, and the `e2e/` Playwright harness is Flutter-Web only
  (iOS/Android native UI automation is out of scope post-#1887, per `docs/client/testing.md`).
  So a plugin major with user-visible Android surface (`camera`, `mapbox_maps_flutter`,
  `flutter_local_notifications`, `video_player`) could regress at runtime without any check
  catching it. Per maintainer direction this is **not** treated as a blocking manual-QA gate —
  just flag such plugins in the PR body so the reviewer can decide whether a smoke check is
  worthwhile.
- [ ] Confirm the grouped PR is routed to human review (major → `automerge:false`, no
  `automerge` label) and merge on approval.
- [ ] If a `3.44.x` Flutter patch had to be pulled in, flag the `versions.env`
  `needs runner rebuild` path in the PR body so the runner image republishes post-merge.
**Validates:** v9 lands in one coordinated change — toolchain + plugins — with no split PRs and
no silent red.

## Related Issues
- **#2391** (closed) — AGP `8.13.2 → 9.2.1`, the AGP half; closed in favor of #2404.
- **#2389** (closed) — Gradle wrapper `8.14.5 → 9.5.1`, the Gradle half; closed in favor of #2404.
- **#2383** — wants Kotlin `org.jetbrains.kotlin.android` 2.3.21 → 2.4.0. Research shows AGP 9
  does **not** force this (KGP 2.2.10 floor already cleared), so it stays independent — do not
  fold it into #2404.
- **#2416** — `versions.env` Renovate hygiene (Java major-only tracking, onnxruntime tarball
  ceiling). Adjacent toolchain-automation cleanup; no direct dependency, but the same
  `needs runner rebuild` machinery would apply only if Phase 4 pulls in a newer `3.44.x` patch.
- **#2287 / #2395** — the runner-image + candidate-validation machinery. Exercised **only** if
  Phase 4 has to pull in a newer `3.44.x` Flutter patch (a `versions.env` `needs runner rebuild`
  change). The expected plugin-only Option-B path leaves `FLUTTER_VERSION` untouched and never
  touches this machinery.
- **#2396** — the §8 `claude_fix_renovate_ci` auto-fixer; explicitly the *wrong* tool for this
  migration (can't run `release-build`), per the issue and `docs/dependency_automation.md` §8.

## Audit Notes (`/ripls-audit`)

Audited against `docs/client/{architecture,caching,i18n,accessibility,testing}.md` and
`docs/server/{architecture,observability}.md`.

- **Most standard dimensions don't apply.** This is a build-toolchain / dependency-automation
  plan: it changes `renovate.json`, `settings.gradle.kts`, `gradle-wrapper.properties`,
  `gradle.properties`, `app/pubspec.yaml`, and exercises the CI `release-build` job. It adds no
  client UI/ViewModel/repository code, no server RPC/storage/logging code, no user-facing
  strings, and no media display. So MVVM-layering, Stash caching, server/client boundary,
  observability/structured-logging, i18n ("Event" not "Experience"), and a11y-label
  conventions have no surface area here — checked, nothing to enforce.
- **Testing alignment (applied):** Phase 4 now states that any Dart touched by a plugin major
  must keep its **ViewModel tests** green (and add coverage for new branching), matching the
  ViewModel-first / `ProviderContainer` + Mockito convention in `docs/client/testing.md`.
- **Android-runtime coverage gap (disclosed, not gated):** per `docs/client/testing.md`, the
  automated gates can't see Android native runtime behavior (`release-build` = compile + R8;
  `flutter test` = ViewModel logic; `e2e/` = Flutter Web only; mobile-native UI automation out
  of scope post-#1887). A plugin major could therefore regress at runtime on Android uncaught.
  Recorded as a PR-body flag for reviewer judgement — **not** a manual-QA gate, per maintainer
  direction.
- **No contradictions** with existing architecture decisions; the plan is consistent
  throughout with the dependency-automation machinery in `docs/dependency_automation.md`
  (§3 auto-merge managers, §5 `needs runner rebuild` / candidate validation, §8 auto-fixer
  exclusion).
