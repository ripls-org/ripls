# config

Command-line configuration for the main server binary. `config.Parse()` declares
every server flag, parses the command line, resolves file-backed secrets, and
validates startup invariants — returning a `*Config` whose fields the
composition root in `server/main.go` wires into services.

## Key files

- `config.go` — the `Config` struct (one exported field per flag), `Parse()`,
  and the `finalize` step that resolves paired `-<name>-file` secret flags via
  `server/secretsflag` and enforces invariants: `--db` and
  `--invite-link-hostname` required, JWT secret ≥ 32 bytes
  (`auth.ValidateSigningSecret`), `--map-provider` enum, and the CORS rules
  (dev defaults to wildcard; production requires an explicit allowlist and
  rejects `*`). Also exports `ParseWeekday` for the weekly digest flag.

## Invariants

- **The flag surface is a deployment contract.** Cloud Run args,
  `server/entrypoint.sh`, `scripts/run_server_with_sm_secrets.sh`, and the e2e
  harness all pass these flags by name — do not rename a flag or change a
  default without updating every caller (see `docs/secrets.md`).
- `Parse` must be called exactly once: it registers on the process-global
  `flag.CommandLine`. Validation lives in `finalize` so it stays unit-testable.

## When to add code here vs. elsewhere

A new server flag belongs here (declare it, thread it through `Config`, and
validate it in `finalize` if it has invariants). Logic that *consumes* a config
value belongs in the package that owns the behavior; main.go passes the value
through. Secret-file resolution mechanics live in `server/secretsflag`.
