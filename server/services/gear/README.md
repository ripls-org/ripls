# services/gear

The `gear` service implements the GearService RPC interface: listing, saving, and deleting gear; AI-powered metadata generation from text, images, and web URLs; embodied carbon computation; stock imagery; and sharing-interest management.

## Key files

- `service.go` — service struct, constructor, and optional dependency setters.
- `gen_ai.go`, `gen_ai_streaming.go` — AI generation of gear metadata from text/image/webpage, including streaming variant.
- `gen_fanout.go` — asynchronous fan-out for post-save enrichment (product spec lookup, stock imagery).
- `detect_url.go` — URL-type detection (image vs. webpage) for routing to the right generation path.
- `stock_imagery.go`, `webpage_image.go` — stock image and web-scraped image helpers.
- `people.go` — helpers for resolving user identity in gear responses.
- `stats.go` — gear usage statistics.

## When to add code here vs. elsewhere

Gear RPC logic lives here. Transfer and loan lifecycle (starting/completing a loan or giveaway) belongs in `server/services/transfer`. Borrowing requests belong in `server/services/request`. Shared gear-fetching utilities used across services belong in `server/services` (the parent package).
