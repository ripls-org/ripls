# available_now

Shared library that builds the "Available Now" rail items for the
user profile and the workshop community screen. Two callers consume
the same vocabulary so the rails read with one voice:

| Caller | Mode | What it answers |
|--------|------|-----------------|
| User profile (`server/services/profile/`) | `ModePerUser` | What is **this user** offering inside the viewer ↔ target community intersection? |
| Workshop (`server/services/workshop/`) | `ModePerCommunity` | What is **currently available** inside the workshop's selected community scope (any owner)? |

## Key files

| File | Role |
|------|------|
| `available_now.go` | `Gather(ctx, storage, opts) ([]*api.Item, error)` entrypoint plus the mode-aware gear / request / experience loaders. |
| `available_now_test.go` | Smoke coverage for both modes, exclusion rules, ordering, and the empty-scope contract. |

## Surfaces

- **Gear** — gear in `GEAR_STATE_AVAILABLE`, mini-label `BORROW` (or `GIVEAWAY` when the listing's community-gear row has `AVAILABILITY_FOR_GIVEAWAY`).
- **Requests** — requests in `REQUEST_STATE_ACTIVE` or `OFFERS_RECEIVED`, mini-label `REQUEST`, ordered by their community-shared timestamp.
- **Experiences** — experiences in `EXPERIENCE_STATE_ACTIVE / JOINED / IN_PROCESS` with a future start time, mini-label `UPCOMING`, surfaced soonest-first within the bucket.

## Conventions

- Errors wrap via `fmt.Errorf("...: %w", err)`. RPC callers wrap the
  outermost error with `connecterr.Internal(ctx, op, err, kv...)` at
  the boundary.
- Logging via `logging.LoggerWithContext(ctx)` with
  `operation="GatherAvailableNow"`, `mode`,
  `community_id_count`, `items_returned`.
- Query budget: 6 reads per call (one pivot + one entity batch per
  kind in `ModePerCommunity`, or one owner-filter + one pivot per
  kind in `ModePerUser`). Enforce via
  `storage.AssertMaxQueries(t, ctx, 6, fn)` in any new test.
