# Issue #2643: server/main.go deep refactor: delete completed one-shot migrations/backfills and dead code, factor startup into libraries

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2643
**Filed:** 2026-07-05 | **Last activity:** 2026-07-05
**Labels:** enhancement, review:tech_debt | **Assignees:** unassigned
**Context docs:** `docs/server/architecture.md` (anchor), `docs/server/conventions.md` (anchor), `docs/server/observability.md` (§ log-based metrics — statslog contract), `docs/users/priorities.md` (anchor; not engaged — no product surface), review-prompt "Areas to Review" digests for code_quality, tech_debt, observability, error_handling, security, testing, architecture. UI investigation skipped: server-internal refactor with no UI-observable behavior.

## Problem Statement

`server/main.go` is 1,945 lines — the flag parser, secret resolver, dependency composition root, one-shot data-migration runner, background-job scheduler, HTTP route table, and middleware stack in one file. Three distinct problems compound:

1. **Completed transition code still runs on every startup.** Six idempotent one-shot migrations/backfills (conversation-ID migration, provisional_user rename, category backfill, impact backfill, community-cascade backfill, share-link backfill) all shipped to production between v0.3.0 and v0.25.0; prod is on v0.26.2. Their work is done. They cost startup time, log noise, and reader attention, and one of them (#2056) has an open issue tracking exactly this removal whose condition ("verified ≥1 release") passed four releases ago.
2. **Dead code.** `jobs.NewCommunityConversationBackfillJob` was never wired into main.go in its entire git history; `storage.BackfillVideoThumbnails` has no non-test caller. (Verified by grep across the repo and `git log -S` across all history.)
3. **~800 lines of main.go belong in libraries** per the documented shared-library pattern (`docs/server/architecture.md` § Shared Functionality Patterns): flag/config parsing, a six-times-copy-pasted periodic-job loop, stats-logging tickers, the stock-media provider-chain assembly, `initialize*` wrappers, and 17 near-identical Connect handler registrations.

The review that produced the issue also surfaced two latent inconsistencies to fix in passing: the ESM/Workshop/Profile handlers are registered **without** the shared `commonOpts` interceptors, and production constructs two Firebase apps where one suffices.

Codebase research settled the risk profile of both:

- **`commonOpts` omission is a consistency gap, not a live bug.** `ratelimit.AllRules()` defines rules only for Login, User.AddPhoneNumber, Media.AddMediaFromURL, and the five streaming Gen* RPCs — none for ESM/Workshop/Profile — and `ratelimit.Interceptor` is pass-through for procedures with no matching rule. `middleware.PreferredLanguageInterceptor` short-circuits when the caller sends Accept-Language (the Flutter client always does). So unifying registration changes no current behavior; it prevents the *next* rule addition from silently not applying to these services.
- **The statslog move has a hard external contract.** `terraform/modules/monitoring/log_metrics.tf` filters on the literal strings `jsonPayload.message="db pool stats"` and `"go runtime stats"`, and extracts the `db_pool_*` / `go_goroutines` / `heap_inuse_bytes` / `gc_pause_ms` fields (#1613). `server/statslog/doc.go` and `README.md` explicitly document that the tickers live in main.go — both must be updated when the tickers move, and messages/fields/intervals must not change.

## Context from Comments

No comments — the issue was filed 2026-07-05 from a comprehensive line-by-line review of main.go conducted the same day; the issue body *is* the research record (grep-verified caller inventories, `git log -S` dating, release-tag containment for each transition block). No maintainer direction beyond the issue text exists yet, so the Open Questions below capture the decision points a reviewer should confirm.

## Potential Solutions

### Option A: Delete-only (minimum scope)
- **Approach:** Remove the six completed transition blocks and the two dead-code orphans; touch nothing else.
- **Pros:** Smallest diff; closes #2056; removes real startup work and ~600 lines repo-wide with near-zero design risk.
- **Cons:** main.go only shrinks by ~60 lines (the implementations live elsewhere); none of the issue's factoring goals are met; the copy-pasted job boilerplate and registration blocks keep accreting.
- **Effort:** Small.

### Option B: Phased delete-then-extract PR series (recommended)
- **Approach:** Five small PRs in dependency order: (1) delete transition + dead code; (2) extract `server/config` + move CORS/host middleware; (3) factor the periodic-job runner and statslog tickers; (4) move `initialize*` helpers and the stock-media chain into their owning packages, share one Firebase app; (5) table-driven handler registration + `commonOpts` unification + split the remaining composition root into focused `package main` files.
- **Pros:** Each PR is independently shippable, reviewable, and revertable; deletion first shrinks every later diff; behavior-affecting changes (interceptor unification) are isolated in their own reviewable step; matches the architecture doc's shared-library pattern and the <500-line file guidance.
- **Cons:** Five review cycles; brief window where main.go is in an intermediate style.
- **Effort:** Medium (spread across small PRs).

### Option C: Big-bang single PR, including an `app` package
- **Approach:** One PR that deletes the transition code and moves the *entire* composition root (service graph, routes, jobs) into a new importable `server/app` package with a `Run(cfg)` entry point.
- **Pros:** One review; main.go becomes ~20 lines; an importable server could theoretically be embedded in tests.
- **Cons:** A several-thousand-line diff mixing deletions, moves, and behavior fixes is effectively unreviewable; nothing needs an importable server (e2e tests deliberately exec the built binary with flags); an `app` package for a single binary is speculative abstraction, against the project's minimal-dependencies preference.
- **Effort:** Large, high risk.

## Recommendation

**Option B.** It delivers everything the issue asks for while keeping each change independently verifiable. The deletion phase is pure win and should land first — it also closes #2056. The extraction phases follow the documented shared-library pattern without inventing an `app` abstraction no consumer needs: the composition root itself stays in `package main`, split across focused files (`wiring.go`, `routes.go`, `jobs.go`), which achieves the readability goal (each file <500 lines per the architecture doc) with zero new API surface.

> **Decision (Bryan, 2026-07-05):** Option B approved, delivered as **one PR** instead of five. The phases below become ordered, self-contained commits on the `remove_old_startup_code` branch so the review can still walk the change incrementally (deletion commit first, then one commit per extraction), with a single CI/review cycle and one merge that closes #2056.

Two constraints shape every phase:

- **Flag surface is frozen.** Deploy configs (Cloud Run args, `server/entrypoint.sh`, `scripts/run_server_with_sm_secrets.sh`, e2e `startSharedE2EServer`) pass flags by name; the config extraction must not rename any flag or change any default.
- **Log contracts are frozen.** The statslog message strings/fields/intervals (Terraform metric extractors, #1613) and the per-job error messages (`errs.LogJobError` msg strings) must survive the moves byte-identical.

## Open Questions

- **How do we verify the migrations actually completed in prod before deleting them?** — Recommendation: check prod startup logs (Cloud Logging, readable as user2@example.org) for each block's no-op log line from the current v0.26.2 instances (e.g. the provisional_user rename and share-link backfill both log row counts; zero = done). Fallback: spot-check the prod schema (`provisional_user` table exists; `share_link` populated; no `shadow_user` table). This is a 10-minute check that converts "should be safe" into "verified safe" — worth doing because if a migration had *not* run, deleting it strands data in the old shape and fails at runtime, not at startup.
- **Is the ESM/Workshop/Profile `commonOpts` omission intentional?** — Recommendation: treat as oversight and unify. Research shows zero behavior change today (no rules match those services; PreferredLanguageInterceptor no-ops for the Flutter client) and the unification prevents future rules from silently skipping these services. Call the change out explicitly in the Phase 5 PR description so a reviewer can veto with context.
- **Where does the periodic-runner helper live?** — Recommendation: `server/jobs` (e.g. `jobs.RunPeriodic(ctx, name, interval, fn)` layered on `logging.GoSafe` + `errs.NewTransientStreak`/`errs.LogJobError`). The jobs README already scopes the package to "background jobs that run at startup or on a schedule"; `errs` stays a leaf, `logging` stays generic. The scheduled-notification reconciler/dispatcher loops adopt the same helper.
- **Does `category.Backfill` removal need #2013's sign-off?** — Recommendation: no — the TODO couples two conditions ("run against prod" and "AI categorization shipped"), but only the first gates the *backfill call*: once legacy rows are populated (v0.22.0 shipped 4 releases ago) new rows get categories at save time via the keyword fallback, which stays. Comment on #2013 that the backfill is gone and the issue's remaining scope is the AI categorizer.
- **Delete or wire the never-run `CommunityConversationBackfillJob`?** — Recommendation: delete. Its `TODO(#1637)` feature (community Discuss tab) hasn't shipped; if it does ship and old communities need conversations, the job is one `git revert` away. Leave a note on #1637 pointing at the deleting commit.

## Implementation Plan

Phases land as ordered commits in a single PR (per the decision above). Each phase's validation gate runs before its commit; the full gate (build, tests, integration-tag build, e2e) runs once more at the end before the PR opens.

### Phase 1: Verify, then delete completed transition code and dead code
**Goal:** Every one-shot migration/backfill whose work is done in prod is gone, along with never-wired dead code. Pure deletion; no moves.

**Verification record (prod Cloud Logging, ripls-prod, 2026-07-05):** every recent v0.26.2 instance start shows — `conversation_id migration complete` with gear/request/experience_migrated all **0**; `share_link backfill job complete` with inserted **0** (skipped=28 legacy rows already mirrored, errors=0); `experience/request category backfill complete` with updated **0**; `community cascade backfill job completed` re-scanning the same soft-deleted parents idempotently (counts are parents scanned, not repairs — verified in code); `impact backfill job completed` with all transaction counts **0**. The provisional_user rename ran exactly once (2026-06-25 16:44 UTC: `dropped legacy shadow_user table` + 4 × `migrated provisional reference column`) and has logged nothing since — its no-op path is silent. One extra finding: the impact backfill's gear-enrichment half re-enriches the same 3–4 gear on *every* startup (`gear_enriched=3-4`, never converging, because `applyGearGeneration` only conditionally persists fields the AI fails to produce) — a stuck retry loop burning LLM calls each boot, so deleting it is a strict improvement; fleet-wide re-enrichment after a future provenance-version bump will need a deliberate mechanism (manual cmd tool or revert) if ever wanted.

**Tasks:**
- [x] Verify in prod (logs or schema, per Open Questions) that the conversation-ID migration, provisional_user rename, category backfill, impact backfill, cascade backfill, and share-link backfill are all no-ops on current instances. Record the evidence in the PR description.
- [x] Delete the six call sites from main.go (~L723, ~L730, ~L801, ~L1283, ~L1298, ~L1314) and their now-unused imports (`category`, per-job constructors).
- [x] Delete `server/storage/migrations.go` + `migrations_test.go` entirely (both public migrations plus private helpers `commonColumns`/`quoteJoinColumns`/`migrateReferenceColumn` have no other callers).
- [x] Delete `server/category/backfill.go` + its tests (the keyword categorizer in `category.go` stays — save flows and `cmd/recategorize` use it).
- [x] Delete `server/jobs/impact_backfill.go` (+ unit and integration tests), `community_cascade_backfill.go` (+ test), `share_link_backfill.go` (+ test), and the never-wired `community_conversation_backfill.go` (+ test). Keep `jobs/shutdown.go` — `workshop_generation.go` still calls `isShutdownError`/`logBackfillCancelled`.
- [x] Delete `storage.BackfillVideoThumbnails` from `server/storage/media.go` + its `media_test.go` cases.
- [x] Update `server/jobs/README.md` key-files list (it names `impact_backfill.go` and `community_cascade_backfill.go`).
- [x] Update `docs/client/media.md` § "Video Thumbnail Backfill" (future-enhancements list): it instructs wiring `storage.BackfillVideoThumbnails` into main.go, which never happened and the function is now deleted. Rewrite the entry to note thumbnails have generated at upload since 2026-03 and that a backfill, if ever needed, is a `git revert` away.
- [x] `npm run build`, `go test ./...`, and `go build -tags=integration ./...` (an integration test file is being deleted; confirm nothing else references the deleted symbols under the tag).
- [x] Close #2056 via the PR; comment on #2013 (backfill removed, AI categorizer remains) and #1637 (conversation backfill deleted, resurrect from history if Discuss ships).
**Validates:** The deletions are complete and self-contained — build and full test suite green with ~600 fewer lines, and prod evidence is on record that nothing still depended on startup migrations.

### Phase 2: Extract `server/config`; CORS + host middleware to `server/middleware`
**Goal:** All flag parsing, secret-file resolution, and startup validation behind one `config.Parse() (*Config, error)`; generic HTTP helpers in the middleware package.
**Tasks:**
- [x] Create `server/config` (with `doc.go` + `README.md` per repo conventions): the `serverConfig` struct (exported `Config`), `parseFlags`, the 13 `secretsflag.Resolve` pairings, `InjectPostgresPassword` wiring, `decodeBase64PEM`, `parseWeekday`, `parseCORSOrigins`, and the early validation currently at the top of `main()` (required flags, JWT secret length via `auth.ValidateSigningSecret`, map-provider enum, production CORS rules). `Parse` returns errors; only main calls `os.Exit`.
- [x] **Do not rename any flag or change any default** — deploy configs, `entrypoint.sh`, secret-fetch scripts, and the e2e harness pass flags by name.
- [x] Move `corsOriginValidator` to `server/middleware` (exported, doc comment per revive) and `hostMiddleware` to `server/middleware` as a named constructor.
- [x] Move the corresponding tests out of `server/main_test.go` (`TestSafeDBFields` stays with `safeDBFields` wherever Phase 4 puts it; CORS + PEM + parse tests move to the new packages; `TestHTTPTimeoutsAllowLongStreams` stays in `package main` with the timeout constants). Add tests for `config.Parse` validation paths (missing required flags, bad weekday, wildcard-CORS-in-prod).
- [x] Update `server/middleware/README.md` for the incoming CORS-origin validator and request-host middleware.
- [x] Refresh `docs/secret-rotation.md` — it cites `server/main.go:665-668` (JWT min-length check) and `server/main.go:334-340` (base64 PEM decode), both of which move into `server/config`.
- [x] `npm run build`, `go test ./...`; run the server e2e suite (`go test -run TestEndToEnd -timeout 10m ./server`) since it exercises the real flag surface.
**Validates:** The binary's externally observable startup contract (flags, validation errors, exit behavior) is unchanged while the parsing logic becomes independently testable.

### Phase 3: Periodic-job runner + statslog tickers
**Goal:** One helper owns the ticker/panic-recovery/error-streak loop; statslog owns its own goroutines; every background goroutine goes through `logging.GoSafe`.
**Tasks:**
- [x] Add `jobs.RunPeriodic(ctx, name string, interval time.Duration, fn func(context.Context) error)` (design to fit `errs.LogJobError` semantics: nil error resets the streak silently; `context.Canceled` exits quietly; runs once at startup then on each tick; wraps in `logging.GoSafe`). Unit-test with a fake clock/short intervals: startup run, tick run, panic containment, streak escalation, cancellation.
- [x] Convert the six loops: community purge reminder, community purge, scheduled-notif reconciler (10 min), scheduled-notif dispatcher (1 min), daily digest (hourly), weekly digest (hourly). **Keep each job's error-message strings identical** — they are alert-query surface.
- [x] Add `statslog.StartPoolStatsLogger(ctx, logger, statsFn)` (15 s) and `statslog.StartRuntimeStatsLogger(ctx, logger)` (30 s), preserving `PoolStatsMessage`/`RuntimeStatsMessage` and all attr fields byte-identical (Terraform `log_metrics.tf` filters on them; #1613). Update `server/statslog/doc.go` + `README.md`, which currently state the tickers live in main.go, and the matching sentence in `docs/server/observability.md` (§ log-based metrics).
- [x] Convert the remaining raw `go func(){ defer recover() }` blocks (pool stats — now gone into statslog, purge reminder — now RunPeriodic, HTTP-serve goroutine) to `logging.GoSafe`; `server/cmd/check-goroutines` continues to pass.
- [x] Fix the embeddings-backfill shutdown leak while touching this block: `logging.GoSafe(ctx, "backfill-embeddings", ...)` currently passes `context.Background()` into `BackfillEmbeddings`, so it ignores root-context cancellation on shutdown — the last offender called out by a prior performance review. Pass the root `ctx`.
- [x] Refresh `docs/server/scheduled_notifications.md` — it says the reconciler/dispatcher goroutines are "spawned from main.go" with a "search `scheduledReconcilers`" pointer; both claims shift when the loops adopt `RunPeriodic` (and again when Phase 5 moves the wiring into `jobs.go`).
**Validates:** ~200 lines of copy-pasted loop boilerplate collapse into one tested helper with provably identical log/alert surface — greppable by comparing dev-server log output before/after for the six job names and two stats messages.

### Phase 4: Move `initialize*` helpers and provider-chain assembly into owning packages
**Goal:** main.go composes; packages construct.
**Tasks:**
- [x] `initializeStorage` + `safeDBFields` (+ its test) → `server/storage`; `initializeBucketStorage` → `server/storage`. **Fix the DSN leak while moving:** the invalid-scheme error embeds the full connection string (`got: %s`) which can contain the DB password and flows into `logger.Error`; redact to scheme-only (or reuse `safeDBFields`).
- [x] `initializeNotificationProviders` — **deviation:** stays in `package main` (slimmed to take the provider name + shared Firebase app). It cannot live in `server/notifications` because `notifications/fcm` imports `notifications` for the `Provider` interface — the factory would create an import cycle; choosing between fcm/noop is composition-root logic anyway.
- [x] `initializeFirebaseApp` → `server/auth` or `server/firebase`; **construct one `riplsfirebase.NewApp` and share it** between phone auth and the FCM messaging client (today prod builds two).
- [x] `initializeFeedbackService` → `server/services/feedback` (it already gracefully degrades to a nil client).
- [x] Stock imagery/video chain (~95 lines: Unsplash→Pexels image fallback, Pexels→Pixabay video fallback, semantic-cache wrapping) → a `server/media` constructor taking the three API keys + storage handles, returning `(StockImageryProvider, StockVideoProvider)`. Unit-test the chain-assembly permutations (no keys / Unsplash only / Pexels only / all three).
- [x] `initializeAIProvider`/`initializeEmbedder` are thin `server/ai` wrappers — inline at the call site or move; no new abstraction.
**Validates:** Each provider assembly becomes unit-testable in its package (today none of it is testable without running main), and the single-Firebase-app change is verifiable in dev logs (one initialization line, not two).

### Phase 5: Table-driven handler registration + interceptor unification + composition-root split
**Goal:** main.go ≈ 200 lines: parse config → build deps → wire services → mount routes → start jobs → serve → shut down.
**Tasks:**
- [x] Replace the 17 `apiconnect.NewXServiceHandler` blocks with a registration table (path/handler pairs looped over `authMiddleware.Wrap`); keep the deliberate exceptions explicit (login/waitlist/health unauthenticated; media's `WithReadMaxBytes` option and `hostMiddleware`; dev-only admin + pprof). Preserve interceptor order (rate-limit outer, preferred-language inner — `commonOpts` appends two `WithInterceptors` calls today).
- [x] Apply `commonOpts` to ESM, Workshop, and Profile handlers (currently omitted). Zero behavior change today (no rate-limit rules match; preferred-language interceptor no-ops when Accept-Language is present) — state this analysis in the PR description as the review record.
- [x] Split the remaining composition root across focused `package main` files: `wiring.go` (service graph + event-bus subscriptions, preserving the construction-order comments), `routes.go` (mux + public routes + middleware chain, preserving the #1799 per-stream-chain and #1810 timeout rationale comments), `jobs.go` (background-job startup). Each file <500 lines per the architecture doc.
- [x] Keep the startup/shutdown log messages byte-identical (`starting server`, `server started`, `server failed to bind listener`, shutdown lines) — `docs/server/observability.md` documents the bind-failure line as an alert-relevant signal.
- [x] Doc-freshness sweep for the "wired in main.go" claims the split invalidates — update to the new file layout: `docs/server/architecture.md` (§ Domain Event Buses), `docs/server/observability.md` (health-registration snippet labeled `// server/main.go`, statslog ticker sentence if not already updated in Phase 3), `docs/server/llm.md` (provider init/flag parsing references), `docs/server/semantic_search.md` (embedder wiring pointer), `docs/weather.md` (shared provider wiring), `docs/realtime_updates.md`, `docs/client/chat.md` (subscriber wiring), `docs/stories.md` (subscriber wiring), and verify `docs/testing/update_system_tests.md` (notification-mode flag claim survives unchanged). Keep every claim true so the nightly doc-freshness ratchet has nothing to fix.
- [x] Full gate: `npm run build`, `go test ./...`, `go build -tags=integration ./...`, server e2e suite, and a manual dev-server boot (`npm run start:server`) confirming the startup log sequence and `/health` + `/readyz`.
**Validates:** The end state the issue asks for — a ~200-line main.go over focused files and libraries — with the one deliberate behavior change (interceptor unification) isolated, argued, and reviewable on its own.

## Audit Notes (ripls-audit, 2026-07-05)

Audited against: `docs/server/architecture.md`, `docs/server/conventions.md`, `docs/server/observability.md`, and the "Areas to Review" rubrics for code_quality, tech_debt, observability, error_handling, security, testing, and architecture; leaf docs checked for invalidated claims via a repo-wide sweep of durable docs referencing `main.go` (`scheduled_notifications.md`, `llm.md`, `semantic_search.md`, `weather.md`, `realtime_updates.md`, `client/chat.md`, `client/media.md`, `stories.md`, `secret-rotation.md`, `testing/update_system_tests.md`). Changes applied by the audit:

- **Security:** added the DSN redaction fix to Phase 4 — `initializeStorage`'s invalid-scheme error embeds the full connection string (password included) and flows to `logger.Error`.
- **Error handling / shutdown:** added the `BackfillEmbeddings` root-context fix to Phase 3 (currently passes `context.Background()`, ignoring shutdown cancellation — a known leftover from a prior performance review).
- **Documentation debt:** the plan named only two doc refresh targets; the sweep found ten durable docs whose main.go claims the refactor invalidates. Added per-phase refresh tasks (`client/media.md` in Phase 1, `secret-rotation.md` + `server/middleware/README.md` in Phase 2, `scheduled_notifications.md` in Phase 3) and a consolidated Phase 5 sweep.
- **Observability:** froze the startup/bind-failure log messages in Phase 5 (documented alert signal) alongside the already-frozen statslog and job-error messages.
- **Correctness detail:** noted interceptor-order preservation in the Phase 5 registration-table task, and pinned `TestHTTPTimeoutsAllowLongStreams` to `package main`.

## Related Issues
- #2056 — tracks the ShareLink backfill removal; **closed by Phase 1**.
- #2013 — category-categorization stopgap; Phase 1 removes the backfill call and comments there; the AI-categorizer half stays open.
- #1637 — community Discuss tab; the never-wired conversation backfill job (deleted in Phase 1) was written for it; note left for resurrection if needed.
- #2492 — phone-first pivot; source of the provisional_user rename migration deleted in Phase 1.
- #1613 — log-based SLI/SLO metrics; defines the statslog message/field contract Phase 3 must preserve.
- #1810 / #1799 — HTTP write-timeout and per-stream middleware-chain rationale; their comments are load-bearing and move intact in Phase 5.
- #1185 (closed) — the cascade backfill deleted in Phase 1 was its cleanup.
