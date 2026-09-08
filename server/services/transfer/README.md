# services/transfer

The `transfer` service implements the TransferService RPC interface: the full lifecycle of gear loans and giveaways — creation, interest expression, loan start, completion, cancellation, undo, and impact estimation. It enforces the transfer state machine and orchestrates notifications, chat messages, story creation, and impact recording.

## Key files

- `service.go` — service struct and optional dependency setters.
- `create.go` — `CreateTransfer`: builds a transfer record from a gear item and counterparty.
- `interest.go` — `ExpressInterest`: a potential borrower signals interest before a loan is confirmed.
- `lifecycle.go` — `StartLoan`, `CompleteTransfer`, `CancelTransfer`: state transitions with notification fan-out and impact recording.
- `state_machine.go` — the core state transition engine; validates the current state before advancing it.
- `context.go` — helpers for loading and assembling the "transfer context" (gear, users, community) needed by multiple operations.
- `queries.go` — `ListTransfers`, `GetTransfer`, and related read operations.
- `undo.go` — undo support for transfer actions.

## When to add code here vs. elsewhere

Loan and giveaway lifecycle RPCs belong here. Gear metadata management belongs in `server/services/gear`. Borrowing requests (where a borrower asks for something before a transfer exists) belong in `server/services/request`. Shared impact estimation belongs in `server/impact_metrics`.
