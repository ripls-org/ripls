# server/story/subscriber

Story-creation subscriber for the `community_event_bus`. Replaces the inline
`storyCreator.CreateStory` calls that used to live in
`services/{transfer,request,experience,community}` lifecycle paths
(#510 PR 4). Each service still records the same
[`CommunityEvent`](../../../proto/ripls/models/community_event.proto)
that triggered its prior inline story call; the subscriber listens for
those events and produces the same `CreateStoryRequest`.

## What it owns

- **Routing.** One `Subscriber.Handle` entry that dispatches by
  `event_type` to a per-type generator (`buildTransferStory`,
  `buildExperienceStory`, `buildRequestStory`, `buildInvitationStory`).
- **Story payload assembly.** Looks up owners, recipients, helpers,
  attendees, and provisional users; builds participant lists; pulls the
  persisted `ImpactEstimate` off the event's pre-fetched entity
  (`evt.Transfer`, `evt.Request`, `evt.Experience`) and converts it
  to the API shape `story.CreateStoryRequest` expects.
- **Soft-delete gate.** `community.IsActive` is checked before
  generating a story for a deleted community. Matches the implicit
  pre-#510 behavior (the upstream lifecycle paths reject deleted
  communities; this is belt-and-suspenders).
- **Timeout.** Each `CreateStory` call is bounded by 60 s through
  `ai.CallWithTimeout`, mirroring the prior service-side wrapper.

## What it does not own

- The community-event row insert (the publisher does that
  synchronously before fan-out).
- Pre-fetching the entities — `community_event_bus.PublishedEvent`
  carries `Transfer`, `Request`, `Experience`, `Gear`, `Actor` so
  the subscriber doesn't re-fetch them.

## Idempotency

`story.Generator.CreateStory` already de-duplicates by
`RelatedEntityID`. Re-handing the same event re-runs the lookup but
does NOT create a duplicate story. Documented as the contract this
subscriber requires from `story.Creator`. Do not change that
de-duplication without also adding an explicit dedup table or
upgrading the bus to a durable transport with at-least-once retry
semantics (see G6 in
[`docs/issues/510-community-event-pubsub-v2.md`](../../../docs/issues/510-community-event-pubsub-v2.md)).

## Soft-delete behavior

`community.IsActive(ctx, storage, community_id)` is checked at the top
of `Handle`. A soft-deleted community skips story generation. The
audit-trail `CommunityEvent` row is still recorded (the publisher does
that before this subscriber runs); only the side-effect (story) is
gated.

## Construction

The subscriber struct + constructor + `Handle` entry point live in
[`subscriber.go`](subscriber.go). Per-event generators live in
[`transfer.go`](transfer.go), [`experience.go`](experience.go),
[`request.go`](request.go), and [`invitation.go`](invitation.go).
Wire-up in `main.go`:

1. Construct `creator := story.NewGenerator(storyStorage, aiProvider)`.
2. Construct `sub := story_subscriber.New(storage, creator)`.
3. Register on the bus: `bus.Subscribe(sub)`.

The four emitter services (community, experience, transfer, request)
no longer accept a `story.Creator` parameter — story generation is
fully owned by this subscriber.

## Observability

Logs use `logging.LoggerWithContext(ctx)` so `request_id` and
`user_id` propagate via the dispatch ctx derived in
`pubsub.MemTopic.deriveDispatchCtx`. Each generator sets
`operation` to one of:

- `story_subscriber.handle` — entry-level dispatch logging.
- `story_subscriber.transfer` / `.experience` / `.request` /
  `.invitation` — per-type sub-logs.

The wider per-dispatch INFO/WARN line is emitted by
`pubsub.MemTopic` (subscriber name = `"story"`).
