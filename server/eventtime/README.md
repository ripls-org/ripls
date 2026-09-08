# eventtime

Shared interpretation of stored `models.ExperienceTime` values for
server-rendered display. Server surfaces that print an event's date or time
without a viewing device — the SSR landing pages and `/go` link previews
(`server/services/web`) and the off-app SMS/email summaries
(`server/notifications`) — all resolve the moment through this package so
they agree on timezone handling.

## Key files / types

- `eventtime.go` — `Resolve(et)` maps an `ExperienceTime` (specific, range,
  or TBD) to a `Moment`: the localized `time.Time`, whether the timezone is
  actually known (`HasTimezone`), and the all-day flag.
  `Moment.ShowsTimeOfDay()` is the single rule for "may I print a
  wall-clock time?".

## Semantics worth knowing

- A stored timezone of `""` **or the literal `"UTC"`** counts as unknown:
  older creation paths persisted `"UTC"` as a fallback when the client's
  timezone was missing (#2621), so the literal is overwhelmingly that
  artifact rather than a real UTC-timezone venue. Unknown-timezone moments
  stay in UTC and renderers degrade to date-only labels.
- Invalid timezone names resolve as unknown rather than erroring — this is
  display-path code and must tolerate bad stored data.

## When to add code here vs. adjacent packages

Add code here when it interprets stored event-time data for rendering and
more than one server surface needs it. Formatting into surface-specific
label strings ("Sat, May 24", "6:00 PM") stays with each surface; timezone
*validation* of incoming API values lives in `server/timezone`.
