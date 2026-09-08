# services/user

The `user` service implements the UserService RPC interface: reading user profiles, updating display names and photos, and surfacing a user's story feed. It delegates identity management to `server/auth` and reads story data from `server/storage`.

## Key files

- `service.go` — service struct, constructor, and `GetUser` / `UpdateUser` RPCs.
- `user_stories.go` — `ListUserStories`: retrieves the activity stories associated with a user.
- `stats.go` — user-level statistics (loan counts, etc.) used in profile views.

## When to add code here vs. elsewhere

User profile RPCs belong here. Authentication and identity proofs belong in `server/auth`. Impact portfolio data belongs in `server/services/portfolio`. Device and notification registration belongs in `server/services/device`.
