# walkthrough-export

Read-only extractor that turns one real event in a Ripls database into the
fixture inputs for a user journey walkthrough (#2684): a scrubbed screenplay
(`content.json`) and a media pull-list (`media.json`) that a `gsutil` step
downloads into `e2e/fixtures/walkthroughs/<pack>/`.

## Key files

- `main.go` — CLI + the object-graph walk: experience → RSVPs → users /
  provisional users → conversation messages → needs/contributions → media
  rows. Opens storage via `storage.OpenPostgreSQLDatabaseReadOnly` (no DDL),
  so it runs as a SELECT-only role (prod: `ripls_reader`).
- `content.go` — output types and the pure PII-scrub rules: first names only,
  email/phone redaction in free text, venue reduced to name + locality (never
  address lines/coordinates), collision-free fixture filenames.
- `content_test.go` — unit tests for the scrub rules; these are the
  regression floor for what may reach a committed fixture pack.

## Usage

See the full runbook (prod access over an SSH tunnel, gsutil pull, consent
fields, fixture review) in the extraction runbook, which is kept privately
because it covers this deployment's prod access. In short, with the tunnel up:

```bash
go run ./server/cmd/walkthrough-export \
  -db="postgres://ripls_reader:…@127.0.0.1:15432/ripls?sslmode=disable" \
  -name-like="picnic" -out=./picnic-export
```

## When to add code here vs. elsewhere

This command is only the DB→fixture-input shaping step. Media downloading
lives in the runbook (`gsutil` against `media.json`); reel seeding lives in
`e2e/lib/seed/`; scrub-rule changes go in `content.go` (bump
`scrubberVersion`) with tests.
