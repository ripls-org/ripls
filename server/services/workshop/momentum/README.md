# server/services/workshop/momentum — Workshop cascade detection + lever-copy guard

Pure-function detectors that pick which `StoredNudge` rows to generate on
Workshop surfaces, plus the `ValidateLeverCopy` validator that enforces
the brief's lever-copy writing rule (≤9 words, concrete object).

## Files

| File | Purpose |
|------|---------|
| `detector.go` | `Slot` enum, `Detection` struct, `Detector` interface, `DetectHighestPriority` orchestrator, `DefaultDetectors` chain |
| `active_quest_detector.go` | `ActiveQuestDetector` — priority 1: an upcoming instance of a rhythm the host already runs |
| `esm_repeat_signal_detector.go` | `ESMRepeatSignalDetector` — priority 2: a recently-completed event whose attendees voted to do it again |
| `calendar_gap_detector.go` | `CalendarGapDetector` — priority 3: a rhythm that has gone quiet |
| `seasonal_trigger_detector.go` | `SeasonalTriggerDetector` — priority 4: the anniversary of last year's tradition |
| `per_event.go` | `DetectForEvent` + `RecentlyCompletedEvents` — the per-event chain and its recap fallback |
| `bring_back.go` | `DetectBringBackItems` — love-revival items for the Bring-Back surface |
| `copy_generator.go` / `ai_copy.go` | `CopyGenerator` — the optional AI copy pass and the template fallback |
| `generator.go` | `SurfaceFor` + `MaterializeDetection` — turning a `Detection` into a `StoredNudge` |
| `lever_copy.go` | `ValidateLeverCopy(text)` — enforces the brief's rule on lever copy |

## When to add code here

- A new cascade detector. New detectors implement the `Detector` interface
  and are added to `DefaultDetectors()` at their priority slot.
- Refinements to the lever-copy rule (e.g. tightening the concrete-object
  whitelist). Update `ValidateLeverCopy` and the test cases in
  `lever_copy_test.go`.

**Every user-visible field a detector emits must be derivable from something
it read from storage.** Counts, names, and dates come from the query; social
proof ("people keep asking about X") requires a query that measured it, and a
citation requires a real source. The idle-offer detector that used to fill
priority 5 asserted both without measuring either, and was deleted for it
(#2892).

## When NOT to add code here

- AI-driven copy generation — that lives in the existing nudge generation
  pipeline (`server/services/feed/nudges.go`); momentum produces structural
  signal + placeholder copy, the pipeline upgrades it.
- RPC handlers — the WorkshopService at `server/services/workshop/`
  consumes `DefaultDetectors()` and `DetectHighestPriority` directly.
- Storage queries that aren't cascade-detection-shaped — those go in
  whichever package owns the entity.

## Cascade priority order

Per the brief's Quest hero priority cascade, in order:

1. `SlotActiveQuest` — upcoming instance of a recurring activity
2. `SlotESMRepeatSignal` — recently-completed event with ≥60% repeat signal
3. `SlotCalendarGap` — recurring rhythm with detectable gaps
4. `SlotSeasonalTrigger` — same date as last year's tradition
5. `SlotIdleOffer` — **unfilled.** The constant remains (slot names are
   logged, and the numbering is load-bearing) but no detector claims it. A
   grounded offer signal — `Transfer` state plus `latest_request_unix_sec`,
   with a CTA that resolves — would land back here.

The orchestrator returns the first non-nil hit, so higher-priority slots
preempt lower-priority ones. Each detector is independently fallible: a
panicking or erroring detector does not block the rest.

## Lever-copy rule

`ValidateLeverCopy(text)` returns nil when the lever passes the brief's
writing rule, an error otherwise. Use it from:

- The Workshop service tests (regression check on persisted `cta_label`).
- Future AI generation prompt validation steps (reject AI output that
  violates the rule before it ships to a host).

The rule:

- ≤ 9 words.
- Contains at least one of: digit, weekday name, proper noun (capitalized
  word that is not the start of the sentence), or a recognizable concrete
  noun from the lever vocabulary whitelist.
