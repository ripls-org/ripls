# rsvpstate

Shared RSVP-intention semantics for `models.ExperienceRSVP`.

## Key files

- `rsvpstate.go` — `IsGoing`, the single definition of which intentions
  count as planning to attend (YES and MAYBE).

## When to add code here vs. adjacent directories

Add code here only for RSVP semantics needed by more than one package
(the experience service, notifications, portfolio, impact metrics all
consume `IsGoing`). Conversion between `api` and `models` RSVP enums
belongs in the service packages that see both schemas
(`docs/proto_conventions.md`), not here. This package once held the
string vocabulary for the pre-#2829 stringly-typed fields; that era
ended when #2832 removed the string fields.
