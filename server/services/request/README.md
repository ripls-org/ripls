# services/request

The `request` service implements the RequestService RPC interface: creating and managing borrowing or help requests, AI-generated request descriptions, offer matching, fulfillment lifecycle, needs and contributions, undo support, and impact estimation at fulfillment.

## Key files

- `service.go` — service struct and optional dependency setters.
- `gen.go`, `gen_fanout.go`, `gen_streaming.go` — AI generation of request content (text and streaming variants) plus async post-save fan-out.
- `lifecycle.go` — `SubmitRequest`, `MarkRequestFulfilled`, `CancelRequest`.
- `offers.go` — offer-to-fulfill management (users offering to fulfill a request).
- `queries.go` — `ListRequests`, `GetRequest`, and other read operations.
- `needs.go` — needs associated with a request.
- `sharing.go` — cross-community sharing of requests.
- `stock_imagery.go` — stock imagery assignment for requests.
- `contributions.go`, `people.go` — ancillary read helpers.
- `undo.go` — undo support for request actions.
- `stats.go` — request statistics.

## When to add code here vs. elsewhere

Request RPCs belong here. Gear loans and giveaways belong in `server/services/transfer`. Community experiences and events belong in `server/services/experience`. Shared planning needs/contributions logic belongs in `server/planning`.
