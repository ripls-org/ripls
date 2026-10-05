---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How secrets reach the server and the app — the Secret Manager fetch-at-startup design, the entrypoint/fetch-secret wrappers, dart-define for client builds, and the gitleaks detection stack that keeps credentials out of the repo.
  globs: [server/entrypoint.sh, server/cmd/fetch-secrets/**, docker-compose.yaml, scripts/fetch_secret.sh, scripts/run_server_with_sm_secrets.sh, .gitleaks.toml, .gitleaksignore, .github/workflows/gitleaks-pr.yaml, .github/workflows/gitleaks-push.yaml]
  triggers: [secrets, api-keys, secret-manager, gitleaks, credentials, tokens, dart-define]
  lens: [infra, security]
  skills: [issue, audit, triage]
  domain: infra
freshness:
  verified_commit: "402d4d9ff"
  verified_on: "2026-08-22"
---
# How secrets work

This document is the **mechanism**: how a credential gets from wherever it is
stored into a running process, and how the repo keeps credentials from being
committed in the first place. It names no projects and no keys.

The per-credential inventory for a specific deployment — which secrets exist,
where to obtain each one, what breaks when one is missing — is instance data and
does not live in this repository. If you are running your own instance, that
inventory is yours to write; the flags below tell you what it needs to contain.

## The design in one paragraph

**The server binary is hermetic.** It has no cloud SDK, it never talks to a
secret store, and it holds no credential-fetching logic. Every credential
arrives as a command-line flag. An external wrapper is responsible for obtaining
the values and passing them in. This is the whole design, and it is what makes
the server runnable against any secret backend — or none, in local development,
where you pass the flags directly.

## The wrappers

| Caller | Wrapper | How it authenticates |
|---|---|---|
| Server container, on Cloud Run or any Docker host | `/app/entrypoint.sh` (`server/entrypoint.sh`), which runs `/app/fetch-secrets` (`server/cmd/fetch-secrets`) | Application Default Credentials: the metadata server's token on Cloud Run, a service-account key named by `GOOGLE_APPLICATION_CREDENTIALS` elsewhere — or nothing, with `SECRETS_FROM_DIR` |
| Local `npm run start:server` | `scripts/run_server_with_sm_secrets.sh` | The developer's `gcloud auth application-default login` |
| Flutter local run / dev build | `scripts/fetch_secret.sh`, once per `--dart-define` | Same as above |
| Flutter release build (CI) | `scripts/fetch_secret.sh` via Fastlane | A CI service-account credential |

The container entrypoint collects every credential into a private directory,
one file per credential, and passes the server a `-<name>-file` flag for each.
It takes them from one of two sources:

- **Secret Manager**, when `$GOOGLE_CLOUD_PROJECT` is set. `fetch-secrets` reads
  each secret by name in that project — nothing hardcodes a project. It is the
  only thing in the image that talks to a secret store.
- **A directory**, when `$SECRETS_FROM_DIR` is set. Each file in it is one
  credential, named after its flag: a file called `jwt-signing-secret` becomes
  `-jwt-signing-secret-file`. No secret store and no cloud account are needed;
  this is how the root `docker-compose.yaml` runs.

`fetch_secret.sh` takes the project as its first argument. All of these are
generic; pointing them at a different deployment is a matter of environment,
not code.

The database password may instead arrive as the `DB_PASSWORD` environment
variable — Cloud Run mounts it through its native `value_source.secret_key_ref`,
and a compose `env_file` works the same way. The entrypoint writes it to a file
like any other credential and removes it from the environment it hands the
server.

### Optional credentials degrade, they do not crash

`entrypoint.sh` fetches some secrets *optionally*. When they are absent the
corresponding feature stays unwired rather than failing at startup: no SMS
credentials means the SMS channel is never constructed; no email key means email
falls back to a mock. From a directory, every credential is optional in this
sense — a file that is not there is a feature that is not configured. This is
deliberate — a fresh instance should boot with an empty secret store and light
up features as they are configured.

An optional secret that *exists* but cannot be read because of a network or
server error still fails startup. Only "not there" degrades; "not reachable"
does not, or a transient error would quietly switch a feature off for the life
of the process.

### The client is different

Mobile binaries cannot reach a secret store at runtime, so the app embeds what it
needs at **build time** via `--dart-define`. Anything embedded this way ships
inside the binary and must be treated as public: use provider-side restrictions
(HTTP referrer, bundle ID, API scope) rather than secrecy to protect it. A
credential that cannot be restricted that way does not belong in the client.

## Keeping secrets out of the repo

Three layers, all blocking:

**1. Pre-commit hook.** Install once per clone:

```bash
brew install pre-commit
pre-commit install
```

`git commit` then runs gitleaks against staged content. `--no-verify` bypasses
it; the CI gates below do not care.

**2. PR gate.** `.github/workflows/gitleaks-pr.yaml` scans every pull request's
diff and blocks the merge.

**3. Push-time detection.** `.github/workflows/gitleaks-push.yaml` scans pushes
to *any* branch, so direct pushes and force-pushes are covered too.

Note what layer 3 does **not** do: it cannot prevent the secret from reaching
the remote. Actions runs after the push is accepted, so detection is a few
seconds behind exposure. If it fires, treat the credential as compromised and
rotate it — do not treat deleting the commit as remediation. On a public
repository, enable GitHub's push protection, which blocks at push time and does
not have this gap.

### Configuration

- `.gitleaks.toml` extends the default rules with project-specific patterns —
  notably credentials passed as CLI flag values, which is the shape this
  codebase would leak if it leaked. It also carries permanent allowlists for
  build artifacts, public-by-design client config, and test fixtures.
- `.gitleaksignore` holds fingerprints for known findings pending cleanup. The
  design intent is that it **shrinks**. An empty file means full blocking with
  no exceptions.

Allowlisting is the last resort, not the first: if you are reaching for it, ask
first whether the apparent leak is a real one.

### Running it by hand

```bash
# Working tree only (fast)
gitleaks detect --source . --config .gitleaks.toml --no-git --redact

# Full history (slow)
gitleaks detect --source . --config .gitleaks.toml --redact
```

`--redact` matters: without it, gitleaks prints the secret it found, and a
terminal scrollback or a CI log is one more place the value now exists.

## Practices worth keeping

- **Never commit a credential**, including into a comment or a doc. `.gitignore`
  blocks the common service-account-key filename patterns; that is a backstop,
  not a policy.
- **Separate credentials per environment.** Development and production should
  share nothing, so that a development leak is not a production incident.
- **Prefer workload identity over long-lived keys** where the platform offers
  it. The best-protected key is the one that does not exist.
- **Destroy superseded versions after a rotation settles.** Old versions are a
  standing liability and, in most secret stores, a standing charge.
- **Pass credentials by file path rather than by value** where a program
  supports it; anything on a command line is visible to every process that can
  read `/proc`.
