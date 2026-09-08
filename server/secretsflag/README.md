# Secrets Flag Helper

This package resolves a credential value from either a `-<name>` value flag
or a paired `-<name>-file=<path>` file flag, so an operator can keep the
actual secret bytes out of the server's command line.

## Why this exists

Even with #1884 fetching secrets at startup from Google Cloud Secret Manager,
the post-fetch pattern was still:

```sh
exec ./server -openai-api-key="$OPENAI" -anthropic-api-key="$ANTHROPIC" ...
```

That puts every credential into the process's `argv`, where anything inside
the container that can read `/proc/<server-pid>/cmdline` sees the values
verbatim. The pattern web servers (apache, nginx) settled on is to take the
**path** to a file holding the secret, not the secret itself:

```nginx
ssl_certificate_key /etc/nginx/tls.key;
```

The path is visible in `ps`; the value is not.

This package is the minimal piece of plumbing that lets `server/main.go`
accept either form. The bytes live in a tmpfs file written by
`server/entrypoint.sh` (Cloud Run) or
`scripts/run_server_with_sm_secrets.sh` (local dev), and the server reads
the file at startup.

See issue #1885 and the plan at [`docs/issues/1885-secrets-file-flags.md`](../../docs/issues/1885-secrets-file-flags.md).

## Usage

In `server/main.go`, pair each existing value flag with a sibling `-file`
flag, then resolve both into a single string after `flag.Parse()`:

```go
import "go.ripls.org/ripls/server/secretsflag"

openAIAPIKey := flag.String("openai-api-key", "", "OpenAI API key")
openAIAPIKeyFile := flag.String("openai-api-key-file", "", "Path to a file containing the OpenAI API key (alternative to -openai-api-key)")
// ...
flag.Parse()

openAIKey, err := secretsflag.Resolve("openai-api-key", *openAIAPIKey, *openAIAPIKeyFile)
if err != nil {
    fmt.Fprintf(os.Stderr, "error: %v\n", err)
    os.Exit(1)
}
```

For the database URL, the password is embedded inside a connection string
rather than passed as a top-level flag. Use `InjectPostgresPassword` to swap
in a password read from a file:

```go
dbURL := flag.String("db", "", "Postgres connection URL")
dbPasswordFile := flag.String("db-password-file", "", "Path to a file containing the postgres password")
flag.Parse()

finalDBURL, err := secretsflag.InjectPostgresPassword(*dbURL, *dbPasswordFile)
```

## What the helper does

`Resolve(flagName, value, path)`:

- Returns `value` (trailing whitespace trimmed) when only the value flag is set.
- Reads `path` and returns its contents (trailing whitespace trimmed) when
  only the file flag is set.
- Returns an error when both flags are set — operators should pick one.
- Returns `("", nil)` when neither is set; the caller decides whether that
  is a required-flag failure.

Trimming policy: trailing whitespace — spaces, tabs, carriage returns, and one
or more newlines — is stripped from the **end** of the value (via
`strings.TrimRight`), for both the file and value paths. Secret Manager uploads
frequently pick up an unintended trailing newline or CRLF that silently
corrupts exact-match credentials (API keys, signing secrets, OAuth client IDs).
Leading whitespace and embedded newlines (multi-line PEM payloads, for example)
are preserved verbatim. Secrets must not contain leading whitespace.

## What it deliberately does not do

- **No logging.** The helper never logs the resolved secret. Errors carry
  the flag name and file path but never the file contents. Callers in
  `server/main.go` handle exit-on-error via `fmt.Fprintf(os.Stderr, ...)` +
  `os.Exit(1)`, matching the pattern the rest of `parseFlags` uses.
- **No file-mode enforcement.** The helper does not require `0400` perms on
  the file. The entrypoint scripts are responsible for writing the file
  with restrictive perms; failing closed inside the server would just
  delay the diagnostic.
- **No tmpdir lifecycle.** The helper reads the file once at startup and
  forgets the path. The operator (or wrapper script) owns deletion.

## When to add code here vs. elsewhere

- A new credential resolver pattern (env var fallback, multi-file
  concatenation, base64 decode wrapper)? **Here.**
- A startup-time check that a secret is non-empty or meets a length
  requirement? **Here** as a small helper, or **`server/main.go`** if it's
  one-off.
- A runtime fetch from Google Cloud Secret Manager? **No** — that
  reintroduces the SM-in-server-binary surface that #1884 deliberately
  removed.
