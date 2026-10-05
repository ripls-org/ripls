# fetch-secrets

Writes Google Cloud Secret Manager secrets into a directory, one file per
secret, so the container entrypoint (`server/entrypoint.sh`) can hand each to
the server as a `-<name>-file` flag. It is baked into the server image at
`/app/fetch-secrets` and is the only thing in the image that talks to a secret
store; the server binary itself never does (see `docs/secrets.md`).

It authenticates with Application Default Credentials, which is what lets one
image run in two places:

- **Cloud Run** — the metadata server's token for the runtime service account.
- **Anywhere else** — a service-account key named by
  `GOOGLE_APPLICATION_CREDENTIALS`, e.g. a self-hosted Docker host.

## Key files

- `main.go` — flag parsing, the Secret Manager REST call, and the 0400 file
  writes. Uses `golang.org/x/oauth2/google` rather than the Secret Manager
  client library, which the one GET it makes does not justify.
- `main_test.go` — runs against a fake Secret Manager: required vs. optional
  failure handling, trimming, permissions, replacing files left by an earlier
  run, and that no value ever reaches the log.

## Behaviour worth knowing

- **Required vs. optional.** A required secret that is missing, unreadable, or
  empty fails the run. An optional one that is missing, unreadable, or has no
  enabled version becomes an empty file, which the server reads as "not
  configured". Any other error — network, quota, a 5xx — fails the run for
  either kind, so a transient hiccup cannot silently disable a feature.
- **No retries.** A failure exits non-zero. Cloud Run rolls the revision back;
  Docker's restart policy tries again.
- **Replaces, never refuses.** Files are written to a hidden temporary name and
  renamed into place, so a restarted container can overwrite the 0400 files
  from its previous start.

## When to change it vs. the entrypoint

Which secrets are fetched, and which are optional, is the entrypoint's list —
change it there. Change this command only for how a secret is read or written.
