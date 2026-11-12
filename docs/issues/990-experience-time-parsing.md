# Issue #990: Experience creation GenAI does not reliably parse time

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/990
**Filed:** 2026-04-01 | **Last activity:** 2026-04-14
**Labels:** none | **Assignees:** briangbrown
**Context docs:** none

## Problem Statement

The GenAI-powered experience creation gets dates and times wrong in two
distinct scenarios:

1. **Structured event links** (Eventbrite, Meetup, etc.) — pages with
   explicit, unambiguous dates like "January 30, 2025 at 7:00 PM MST"
   still produce wrong timestamps. This should be trivially correct.

2. **Free-form text with explicit times** — inputs like "hiking at 10am
   tomorrow" sometimes produce the wrong time, likely shifted by the
   user's UTC offset.

The root cause is a **timezone blindness** problem in `GenExperience`:

- The client sends `current_time_unix_sec` as a bare Unix timestamp
  (`experience_service.dart:414`).
- The server converts it to RFC3339 via `time.Unix(...).Format(time.RFC3339)`
  (`gen_ai.go:62`), which always produces a **UTC** time string (e.g.,
  `2026-04-14T19:00:00Z`).
- The LLM sees UTC as the "current time" and interprets all relative
  references ("tomorrow", "this Friday") relative to UTC.
- The LLM returns bare `date` (YYYY-MM-DD) and `time` (HH:MM) strings
  with no timezone context.
- The server parses these in UTC (`gen_ai.go:263`), but the user meant
  local time.

Meanwhile, `ConvertInformalTime` — the time-picker RPC — **already has
a `timezone` field** in its request proto and passes it to the LLM prompt.
`GenExperience` simply lacks this field.

A secondary problem: for structured event pages, the system relies
entirely on the LLM to extract dates from body text. Event platforms
embed machine-readable dates in JSON-LD / schema.org `Event` markup
which could be extracted with 100% reliability — the webfetch package
currently ignores this structured data.

## Context from Comments

No comments on this issue. The description calls out both structured
links and natural language as failure modes.

## Potential Solutions

### Option A: Fix timezone handling + improve prompts

- **Approach:** Add `timezone` field to `GenExperienceRequest` proto.
  Pass the user's IANA timezone into all experience prompts (text, image,
  webpage). Change the LLM output format from separate `date`/`time`
  strings to a single ISO 8601 datetime with timezone offset. Update
  server-side parsing to handle timezone-aware timestamps.
- **Pros:** Fixes the core bug for both structured and free-form inputs.
  Aligns `GenExperience` with `ConvertInformalTime` which already works
  correctly. Minimal server changes (one proto field, prompt updates,
  parsing update).
- **Cons:** Still relies on LLM for structured pages where regex would be
  100% reliable. Prompt changes need validation across all three
  providers.
- **Effort:** Medium.

### Option B: Hybrid extraction (structured metadata + LLM)

- **Approach:** In addition to Option A, extract JSON-LD / schema.org
  `Event` markup from webpages in `webfetch.Fetcher`. When structured
  event data is found (startDate, endDate, location), use it directly
  instead of asking the LLM to parse it from body text. Fall back to LLM
  for unstructured pages.
- **Pros:** 100% reliable date extraction for Eventbrite, Meetup, Google
  Calendar, and any site using schema.org markup. Reduces LLM's job to
  just title/description generation for structured pages.
- **Cons:** Additional complexity in webfetch. Need to handle edge cases
  (multiple events on one page, missing fields). Slightly more code.
- **Effort:** Medium-large.

### Option C: Two-pass LLM (extract then convert)

- **Approach:** First pass extracts raw time mentions verbatim ("January
  30, 2025 at 7:00 PM MST"), second pass converts to structured format
  with timezone. This separates extraction from interpretation.
- **Pros:** More reliable separation of concerns. Good for academic
  correctness.
- **Cons:** Doubles latency and cost. Over-engineered — the single-pass
  approach works if given proper timezone context. No major accuracy gain
  over Option A with good prompts.
- **Effort:** Medium.

## Recommendation

**Option A first, Option B as a follow-up.** The timezone bug is the
primary issue and affects all input types. Once timezone handling is
correct, the LLM will reliably parse most inputs. Structured metadata
extraction (Option B) is a valuable follow-up for event platforms but is
not the root cause.

**Testing strategy is the prerequisite.** Before changing anything, we
need a comprehensive test suite that captures current behavior across all
failure modes. This establishes a baseline and prevents regressions.

The approach for testing:
1. Define a comprehensive set of time-parsing scenarios (structured
   links, free-form text, timezone edge cases, ambiguous inputs).
2. Run each scenario against a live LLM once to capture responses.
3. Check in the responses as golden data for a "smart mock" provider
   that returns scenario-specific responses.
4. Use the golden data in standard (non-integration) tests to verify the
   full server-side parsing pipeline.

**Cost is not a concern.** At ~$0.004 per GenExperience call (Claude
Haiku 4.5), even doubling the prompt size for better timezone context
is negligible.

## Open Questions

- **What IANA timezone does the Flutter client have access to?** —
  **Resolved:** Use the `timezone` package with `TZDateTime` for
  timezone-aware datetime operations, backed by the IANA Time Zone
  Database. Use `flutter_timezone` to get the device's local IANA
  timezone string (e.g., "America/Denver") from the OS. This replaces
  `DateTime.now().timeZoneName` which returns ambiguous abbreviations
  like "MST". The `ConvertInformalTime` client code also needs updating
  to use `flutter_timezone` instead of `DateTime.now().timeZoneName`.

- **Should the LLM return ISO 8601 with offset or a separate timezone
  field?** — Recommendation: Return ISO 8601 with offset (e.g.,
  `2026-01-30T19:00:00-07:00`). This is unambiguous, parseable by Go's
  `time.Parse(time.RFC3339, ...)`, and eliminates the separate
  date/time string concatenation that's currently error-prone. The LLM
  should also return the IANA timezone string separately for storage.

- **Should we extract JSON-LD in Phase 1 or defer?** — Recommendation:
  Defer to Phase 3. Timezone fixes alone will significantly improve
  structured page parsing. JSON-LD extraction is an optimization.

- **What about the `ExperienceGeneration` struct — should `Date` and
  `Time` fields be replaced with a single `DateTime` field?** —
  Recommendation: Yes. Add a `DateTime` field (ISO 8601 with offset)
  alongside the existing fields for backward compatibility during
  transition. Phase 2 removes the old fields.

## Implementation Plan

### Phase 1: Establish comprehensive time-parsing test suite

**Goal:** Create a baseline of current behavior across all time-parsing
scenarios. This suite will drive all subsequent fixes and prevent
regressions.

**Tasks:**
- [x] Define golden test scenarios in a JSON file
  (`server/test_data/time_parsing_golden.json`) covering:

  **Free-form text scenarios:**
  - Explicit time: "hiking at 10am tomorrow" (user in MST)
  - Explicit date+time: "dinner December 15 at 7pm"
  - Relative date: "this Saturday morning"
  - Fuzzy time: "coffee sometime next week"
  - Past tense: "we hiked yesterday at 8am"
  - No time: "want to go climbing"
  - Time with timezone: "meeting at 3pm EST"
  - Time with 24h format: "departing at 14:30"
  - Multi-day: "camping trip this weekend"
  - Recurring: "every Tuesday at 6pm"

  **Structured webpage scenarios:**
  - Eventbrite-style page with explicit datetime in body
  - Meetup-style page with "first Thursday" recurring pattern
  - Page with timezone abbreviation (MST, EST, PST, etc.)
  - Page with ISO 8601 datetime in body text
  - Page with date but no time
  - Page with multiple dates (multi-day festival)
  - Page with past date (recording/recap)

  **Timezone edge cases:**
  - User in UTC+0 (London)
  - User in UTC-7 (Denver) saying "tomorrow morning" at 11pm local
    (next day in UTC already)
  - User in UTC+12 (Auckland) — date rollover difference from UTC
  - User in UTC+5:30 (India) — half-hour offset
  - DST transition: user says "next Sunday" when clocks spring forward
  - Event page timezone differs from user timezone

  **Ambiguous/adversarial inputs:**
  - "noon" (12:00 vs 00:00)
  - "next Wednesday" on a Wednesday (this week or next?)
  - "12am" vs "12pm" (midnight vs noon)
  - "biweekly" (every two weeks vs twice a week)
  - Empty string / whitespace only
  - Very long input (1000+ characters)

- [x] Create an integration test
  (`server/ai/time_parsing_integration_test.go`, build tag
  `integration`) that runs every scenario against a live LLM
  (Claude Haiku 4.5) and records the raw AI responses to a checked-in
  golden response file
  (`server/test_data/time_parsing_responses.json`)

- [x] Create a `ScenarioMockProvider` (or extend `MockProvider`) that
  loads the golden response file and returns scenario-matched responses.
  Match by hashing the prompt text or using a scenario ID key.

- [x] Create unit tests
  (`server/services/experience/time_parsing_test.go`) that use the
  scenario mock to test the **full server-side pipeline**: prompt
  construction → (mocked) LLM response → response parsing →
  timestamp/timezone conversion → proto output. Assert on:
  - Correct Unix timestamp (within tolerance for inferred times)
  - Correct `TimeConfidence` enum
  - Correct timezone in `SpecificTime`
  - Correct date/time strings in `ExperienceGeneration`

- [x] Create unit tests for `ConvertInformalTime` pipeline
  (in `server/services/experience/time_parsing_test.go`) with
  the same scenario approach, covering the same timezone edge cases

- [x] Run the integration test suite once, commit the golden response
  file, and verify the unit tests pass against it. Document the
  baseline: which scenarios pass, which fail, and how they fail.
  **Baseline result:** All 36 scenarios pass against Claude Haiku 4.5.
  The LLM correctly parses dates and times in all cases when the
  current time in the prompt includes timezone offset (RFC3339).
  The timezone bug manifests at the server layer, not the LLM layer —
  `gen_ai.go` formats `currentTime` in UTC and parses responses in
  UTC, losing the user's timezone context.

**Validates:** We have a clear picture of current accuracy. We know
exactly which scenarios are broken (likely: all timezone-dependent ones)
and which work (likely: UTC-friendly inputs). This baseline drives
Phase 2 priorities.

### Phase 2: Fix timezone handling in GenExperience

**Goal:** Make GenExperience timezone-aware, matching the pattern
already used by ConvertInformalTime.

**Tasks:**
- [x] Add `string timezone = 8;` field to `GenExperienceRequest` in
  `experience_service.proto`
- [x] Run `npm run generate` to regenerate Go and Dart code
- [x] Add `flutter_timezone` package to `app/pubspec.yaml` (`timezone`
  already present)
- [x] Update the Flutter client (`experience_service.dart`) to send
  the user's IANA timezone in `GenExperienceRequest.timezone` using
  `FlutterTimezone.getLocalTimezone()` from the `flutter_timezone`
  package (returns IANA names like "America/Denver" on both platforms)
- [x] Update `ConvertInformalTime` call in `experience_service.dart`
  to also use `FlutterTimezone.getLocalTimezone()` instead of
  `DateTime.now().timeZoneName` (fixes the same ambiguity bug there)
- [x] Update `gen_ai.go` to format `currentTime` using the user's
  timezone instead of UTC: parse the IANA timezone, use
  `time.Unix(...).In(loc).Format(time.RFC3339)` so the prompt receives
  a timezone-aware current time string
- [ ] Update `ExperienceGeneration` struct to add a `DateTime` field
  (full ISO 8601 with offset) alongside existing `Date`/`Time` fields
  — **Deferred to Phase 4:** The current date/time fields work correctly
  now that the server parses them in the user's timezone. Adding a
  combined datetime field is a cleanup, not a fix.
- [x] Update all three experience prompts (text, image, webpage) in
  `prompts.go` to include timezone-aware instructions: the LLM is told
  that CURRENT TIME includes the user's timezone offset, and to return
  times in the user's local timezone. No separate USER TIMEZONE field
  needed since the offset is already in the CURRENT TIME string.
- [ ] ~~Update `gen_ai.go` timestamp parsing to prefer the new `DateTime`
  field~~ — **Not needed:** The server now correctly parses date/time
  in the user's timezone via `time.ParseInLocation(..., loc)`.
- [x] Update `SpecificTime.Timezone` to store the resolved IANA timezone
  instead of hardcoded "UTC"
- [ ] Re-run the integration test suite and update golden responses
- [x] Verify the unit test suite passes with timezone-aware parsing

**Validates:** All timezone-dependent scenarios pass. The golden response
file shows correct timestamps for users in non-UTC timezones.

### Phase 3: Structured metadata extraction from event pages

**Goal:** Extract machine-readable event data from webpages (JSON-LD,
schema.org) for 100% reliable date/time parsing on structured event
platforms.

**Tasks:**
- [x] Add `StructuredEventData` field to `webfetch.PageContent` with
  `Name`, `StartDate`, `EndDate` (pointers), and `Location` fields
- [x] In `webfetch.HTTPFetcher.FetchPageContent`, after HTML parsing,
  scan for `<script type="application/ld+json">` tags containing
  `@type: "Event"` (and subtypes like MusicEvent, SocialEvent, etc.).
  Parse JSON-LD and extract `startDate`, `endDate`, `location`, `name`.
  Handles: single objects, arrays, `@graph` wrappers, Place objects
  with PostalAddress, string locations, date-only formats.
- [x] In `gen_ai.go` `generateFromWebpage`, when structured event data
  has a valid `StartDate`: override AI-extracted date/time/confidence
  with the structured values. Set `TimeConfidence` to `EXPLICIT`.
  Still use LLM for title/description. Also fill `LocationQuery` from
  structured data if the LLM didn't extract one.
- [x] Add 15 unit tests for JSON-LD parsing: Eventbrite-style,
  Meetup/SocialEvent, array format, @graph wrapper, date-only, string
  location, PostalAddress, non-Event types, malformed JSON, empty
  script, no script, multiple scripts, missing startDate, MusicEvent
  subtype, ISO 8601 parsing (7 format variants)

**Validates:** Eventbrite and Meetup URLs produce 100% correct timestamps
from structured metadata. LLM-only fallback still works for pages without
JSON-LD.

### Pre-merge manual testing (PR #1132)

**Goal:** Verify timezone handling and JSON-LD extraction work correctly
on a real device before merging.

**Setup:** Run the server locally on branch `issue-990-time-parsing-tests`
with `npm run start:server`. Use iOS Simulator or Android Emulator.

- [x] **T1 — Text with explicit time:** Create experience from text:
  "dinner party Friday at 7pm". Verify date is Friday April 17 and time
  is 7:00 PM local.

- [x] **T2 — Text with relative time:** Create experience from text:
  "hiking tomorrow morning". Verify date is tomorrow and time is
  approximately 8:00 AM local.

- [x] **T3 — Eventbrite URL (JSON-LD extraction):** Create experience
  from URL:
  `https://www.eventbrite.com/e/1940s-ball-at-boulder-airport-with-the-world-famous-glenn-miller-orchestra-tickets-1661328019829`
  Verify: June 20, 2026 at 6:30 PM, location mentions "Boulder Airport".
  **Result:** Time correct (6:30 PM). Location wrong — resolved to
  "Boulder County Jail" instead of "Boulder Airport". Filed as separate
  issue.

- [x] **T4 — Meetup URL:** Create experience from URL:
  `https://www.meetup.com/boulder-social-hikers/events/314002058/`
  Verify: April 5, 2026 at 12:00 PM.

- [x] **T5 — URL without JSON-LD (LLM fallback):** Create experience
  from URL:
  `https://www.eventbrite.com/e/taste-of-pearl-2026-tickets-1981789132644`
  This page has no Event JSON-LD — verify the LLM still extracts
  date/time from body text. Expected: March 29, 2026.

- [ ] **T6 — Informal time picker:** ~~Edit any experience's time, type
  "tomorrow afternoon". Verify it resolves to tomorrow at ~2:00 PM local.~~
  **Skipped:** ConvertInformalTime has no UI entry point yet.

- [x] **T7 — No time at all:** Create experience from text: "want to go
  climbing". Verify time shows as TBD, not a random time.

### Phase 4: Clean up legacy date/time fields

**Goal:** Remove the transitional backward-compatibility fields once
Phase 2 has been validated in production.

**Tasks:**
- [ ] Remove `Date` and `Time` string fields from
  `ExperienceGeneration` struct, keeping only `DateTime`
- [ ] Update all three AI providers to stop populating the old fields
- [ ] Update prompts to only request the `datetime` field (remove
  separate `date`/`time` instructions)
- [ ] Update `gen_ai.go` to only parse the `DateTime` field
- [ ] Update golden test data to reflect new format
- [ ] Verify all tests pass

**Validates:** Clean, single-format datetime handling throughout the
pipeline. No more string concatenation for timestamp construction.

## Audit Notes

1. **Proto field addition:** Adding `timezone` to `GenExperienceRequest`
   is backward-compatible (new optional field). No migration needed.
   Follows proto field conventions in CLAUDE.md — should use `optional`
   if conditionally present, but since we want to default to UTC when
   absent, a plain `string` field is appropriate (empty = UTC fallback).

2. **Client-side timezone (resolved):** Use `flutter_timezone` package
   (`FlutterTimezone.getLocalTimezone()`) to get reliable IANA timezone
   names on both platforms. Use the `timezone` package with `TZDateTime`
   for timezone-aware datetime operations backed by the IANA Time Zone
   Database. `DateTime.now().timeZoneName` is unreliable — returns
   abbreviations like "MST" on Android which are ambiguous ("CST" =
   US Central or China Standard). Both `GenExperience` and
   `ConvertInformalTime` client code need updating.

3. **Server time formatting:** `time.Unix(sec, 0).Format(time.RFC3339)`
   in Go always produces UTC output (e.g., `2026-04-14T19:00:00Z`)
   because `time.Unix` returns a `time.Time` in UTC. To produce
   timezone-aware output, use `time.Unix(sec, 0).In(loc)` where `loc`
   is loaded via `time.LoadLocation(iana_timezone)`.

4. **Prompt format consistency:** The `ConvertInformalTime` prompt
   already includes `TIMEZONE: %s` and asks the LLM to use it for all
   calculations. The experience prompts should follow the same pattern
   for consistency.

5. **No server architecture changes needed.** The fix stays within the
   existing ai/experience service boundary. No new services, no new
   storage, no new cache patterns.

6. **Testing approach is sound.** The "capture once, mock forever"
   pattern for LLM responses avoids flaky CI tests while still
   validating the full parsing pipeline. The integration test (build
   tag gated) can be run manually to refresh golden data when prompts
   change.

7. **[AUDIT] Client architecture compliance:** Phase 2 updates
   `experience_service.dart` (service layer) to send the timezone
   field. This is correct — services own the RPC call construction.
   The ViewModel (`gen_experience_view_model.dart`) does not need
   changes since timezone is an implicit context field (like
   `current_time_unix_sec`), not user-provided input. No caching
   implications — `GenExperience` is a generative call, not cached.

8. **[AUDIT] Mock provider pattern:** The plan proposes a
   `ScenarioMockProvider` that matches responses by scenario key.
   This aligns with the existing `MockProvider` pattern in
   `server/ai/provider_mock.go` which already supports per-method
   function overrides. Rather than creating a new mock type, extend
   `MockProvider` with a helper that loads golden responses and wires
   them into `GenerateExperienceFromTextFunc` etc. based on prompt
   content. This keeps mock definitions centralized per server
   architecture guidelines (§ Mock Strategy: "Use a single, shared
   mock implementation where possible").

9. **[AUDIT] Server-side `time.LoadLocation` safety:** Go's
   `time.LoadLocation(iana)` can fail if the timezone database is not
   available (e.g., minimal Docker images). The server should validate
   the timezone string and fall back to UTC gracefully, not return an
   RPC error. This should be a logged warning, not a silent fallback
   — per CLAUDE.md error handling guidelines ("Fail Fast" but with
   documented fallback).

10. **[AUDIT] i18n — no new user-visible strings.** The plan adds a
    proto field and changes server prompts. No new Flutter UI strings
    are introduced, so no ARB file changes needed. Correct.

11. **[AUDIT] Integration test build tag:** The plan uses `integration`
    build tag for LLM round-trip tests. This matches the existing
    pattern (`gen_ai_integration_test.go` uses `integration`). The
    benchmark tests use a separate `benchmark` tag for longer-running
    multi-model comparisons — the time-parsing integration tests
    should use `integration` since they target a single model and
    are comparable in duration to existing integration tests.

12. **[AUDIT] Existing benchmark golden data overlap:** The plan
    creates `server/test_data/time_parsing_golden.json` for scenarios
    alongside the existing `server/test_data/benchmark_golden.json`
    which already has 3 experience-from-text and 2 experience-from-
    webpage cases with `expected_date`/`expected_time` fields. The new
    file should not duplicate these — either extend the existing
    benchmark golden data with timezone-focused cases, or keep them
    separate with a clear comment noting the relationship. Recommend
    keeping them separate since they serve different purposes
    (benchmark scoring vs. pipeline correctness).

## Related Issues
- None referenced in the issue.
