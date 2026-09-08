# services/portfolio

The `portfolio` service implements the PortfolioService RPC interface: a user's personal impact summary, goal tracking, inbox feed, weekly views, and watched community metrics. It aggregates data from multiple sources (storage, impact metrics) into the views that appear on a user's profile and portfolio screen.

## Key files

- `service.go` — service struct and constructor.
- `assembly.go` — the core assembly logic that gathers and joins data for the portfolio response.
- `fetch.go` — low-level data fetching helpers used by assembly.
- `goals.go` — `GetPortfolioGoals`, `SetPortfolioGoal`.
- `inbox_feed.go`, `inbox_view.go`, `inbox_week.go` — inbox and weekly impact summary.
- `metric_detail.go` — per-metric detail views linked from the portfolio.
- `metrics.go`, `portfolio_metrics.go` — metric aggregation helpers.
- `aggregates.go` — community-level aggregates for comparison.
- `community_ranking.go` — ranking a user's community contributions.
- `percentile.go` — percentile helpers.
- `watch.go` — `WatchCommunity`/`UnwatchCommunity`: community watching for impact tracking.
- `actions.go` — surfaces undoable actions surfaced in the portfolio context.

## When to add code here vs. elsewhere

Data that appears on a user's own portfolio screen belongs here. Community-wide impact leaderboards belong in `server/services/leaderboard`. User profile reads (display name, photo) belong in `server/services/user`. Raw impact calculations belong in `server/impact_metrics`.
