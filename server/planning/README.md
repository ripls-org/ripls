# Planning

The `planning` package is a shared library for managing needs and contributions — the lightweight coordination mechanism that lets participants signal what they will bring to or need from an experience or request. Both the experience service and the request service use this library.

## Key files

- `scope.go` — `Scope` type: identifies whether a set of needs/contributions belongs to an experience or a request; eliminates the need to duplicate field-name selection logic in each service.
- `list.go` — list and fetch helpers for needs and contributions.
- `slots.go` — slot-fill logic (marking a need as covered when a contribution is made).
- `chat.go` — helpers for posting system messages when a need is added or a contribution is made.
- `mock.go` — test helpers.

## When to add code here vs. elsewhere

Needs/contributions logic that is identical for experiences and requests belongs here. Logic specific to one entity type (e.g. "complete the experience when all needs are filled") belongs in the owning service.
