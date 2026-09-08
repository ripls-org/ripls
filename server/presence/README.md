# presence

Shared building blocks for the profile pinned-sheet presence reads
(issue #2568, "the sheet holds the ask"). The person variant
(`ProfileService.GetProfilePresenceForViewer`,
`server/services/profile/presence.go`) and the community variant
(`CommunityService.GetCommunityPresenceForViewer`,
`server/services/community/presence.go`) share the same face-stack
resolution and ask-qualification rules; this package keeps those rules
identical so the two sheets never drift.

## Key files / types

- `presence.go` — `LoadFaces` (user ids → capped
  `ProfilePresenceFace` stack), `AskQualifies` (open + fresh gate for
  a `models.Request`), `TallyOffers` / `OfferTally` (active-offer
  count, offerer face ids, viewer-committed flag), `MaxFaces`.

## When to add code here vs. an adjacent directory

- Selection logic shared verbatim by both presence RPCs → here.
- Scope-specific selection (pair intersection for the person read,
  the community priority rule) → the owning service package.
- General "what's available" gathering → `server/available_now/`.
- The this-week needs detector feeding the Feed → `server/needs/`.
