# GitHub

The `github` package wraps the GitHub API with App-based authentication. It exposes a minimal `Client` interface used by the feedback service to file issues programmatically.

## Key files

- `client.go` — defines the `Client` interface (`CreateIssue`) and `AppClient`, which authenticates as a GitHub App installation and generates short-lived tokens automatically.
- `forgejo.go` — `ForgejoClient`, the same `Client` interface against a Forgejo instance with an access token, for deployments whose feedback tracker lives there. It resolves label names to ids, creates missing labels, and applies the issue type as a label because Forgejo has no issue types. The package keeps its name: it is "the issue tracker client", whichever forge is behind it.
- `mock.go` — in-memory mock implementing `Client` for use in tests.

## When to add code here vs. elsewhere

Extend the `Client` interface here when additional GitHub API operations are needed server-wide (e.g. commenting on issues, labelling). Code that decides *when* to create an issue or *what content* to put in it belongs in the calling service (currently `server/services/feedback`).
