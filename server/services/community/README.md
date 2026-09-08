# services/community

The `community` service implements the CommunityService RPC interface: creating and managing communities, membership, invitation links, gear sharing settings, AI-generated community content, the per-user realtime event stream and its poll backstop, and story creation for community milestones.

## Key files

- `service.go` — service struct, constructors, stream lifetime constants.
- `lifecycle.go` — `CreateCommunity`, `GetCommunity`, `UpdateCommunity` (incl. promote-by-naming: an unnamed ad-hoc community gaining a name emits `COMMUNITY_NAMED`), delete/restore.
- `adhoc.go` — ad-hoc (nameless) per-item communities (#2492): `IsAdHoc`, `AdHocOrigin`, the shared `createCommunity` core, and `ProvisionAdHocCommunity` (called by item-creation flows). See `docs/ad_hoc_communities.md`.
- `membership.go` — join, leave, list members, provisional-user promotion.
- `invitations.go` — invitation link generation (short codes) and lookup.
- `gear_sharing.go` — community-level gear sharing settings.
- `gen.go` — AI-generated community name, description, and imagery (unary).
- `gen_streaming.go` — `StreamGenCommunity` (incremental title / description / media_ready / final / error). Routes on the request: `prompt` → text mode (Pexels stock-image fan-out); `media_id` → image mode (vision call, user's uploaded image becomes the community media; no Pexels). Exactly one of the two must be set.
- `events.go` — `ListCommunityEvents` (one community's activity), the storage→API event-type mapping, and `communityEventToItem`, the single-event denormalizer the realtime paths use.
- `user_events.go` — `ListUserEvents`, the portfolio-wide poll backstop, plus `denormalizeEvents`, the batching denormalizer both listings share.
- `user_streaming.go` — `StreamUserEvents`: the per-user stream registry, handler, catch-up replay and delivery loop. One connection per client covers every community they belong to (#2867); the per-community stream it replaced was deleted in #2869.
- `stream_subscriber.go` — the event-bus subscriber that resolves an event's community members and fans it out to their streams.
- `notifications.go` — push notifications for membership and gear-sharing events.
- `regions.go` — geographic region computation for a community based on member locations.
- `past_giveaways.go` — listing completed giveaways for a community.

## When to add code here vs. elsewhere

RPC logic for communities lives here. Cross-community membership utilities used by other services belong in `server/auth`. Geographic region computation for non-community purposes belongs in `server/location` or `server/community` (the shared library, not this service).
