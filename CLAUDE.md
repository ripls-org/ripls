# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

This is a Go- and Flutter- based project for community building through sharing of goods.
The project uses Protocol Buffers for API definitions and type safety,
with the Buf toolchain for code generation. The server is written in Go and
the client is written with Flutter / Dart.

## Architecture

- **Protocol Definitions**: Type definitions are separated into two packages:
  - `proto/ripls/api/`: RPC API types exposed in service interfaces
  - `proto/ripls/models/`: Storage/persistence model types
- **Code Generation**: Go and Dart code is generated from protobuf definitions
  using Buf and stored in `server/gen/` and `app/lib/data/gen/` directories.
  **Generated code is not checked into source control** - it is generated
  automatically during builds and in CI/CD pipelines
- **Service Architecture**: Uses Connect-Go for gRPC-compatible services with
  HTTP/JSON support
- **Server Organization**: All Go server code is contained in `server/`
- **Client code**: Client code is in `app/`

### Key Components

**Protocol Buffers:**

- `proto/ripls/api/`: API types for RPC services (flattened request/response messages)
- `proto/ripls/models/`: Storage model types

**Server (Go):**

- `server/gen/ripls/api/`: Generated API Go code
- `server/gen/ripls/models/`: Generated storage model Go code
- `server/ai/`: AI provider integrations (Gemini, Vertex AI)
- `server/auth/`: Authentication library (JWT, OIDC)
- `server/email/`: Email service (Mailgun integration)
- `server/location/`: Location services (Mapbox, EXIF parsing)
- `server/notifications/`: Push notification providers (FCM, noop)
- `server/services/`: RPC service implementations
  - `community/`: Community management
  - `device/`: User device registration
  - `gear/`: Gear (items) management
  - `loan/`: Loan tracking
  - `location/`: Location/place management
  - `login/`: Authentication & user registration
  - `media/`: Media upload & storage
  - `search/`: Search across gear & communities
  - `user/`: User profile management
- `server/storage/`: Proto-SQL storage abstraction

**App (Flutter/Dart):**

- `app/lib/data/`: Data layer
  - `app/lib/data/gen/`: Generated Dart code from proto definitions
  - `app/lib/data/cache/`: Caching infrastructure (CacheManager, CachePolicy, HybridCacheManager)
  - `app/lib/data/repositories/`: Data access with transparent caching (MediaRepository, UserRepository, GearRepository, LoanRepository)
- `app/lib/core/`: Core utilities (config, theme, routing, utils)
- `app/lib/services/`: API clients & business logic
- `app/lib/presentation/`: UI layer (screens, widgets, viewmodels)

**IMPORTANT**: API and Models proto packages must never import each other. This
separation allows the API and storage schemas to evolve independently. The API uses
flattened request/response messages with inline fields, while storage uses rich domain
models. Service implementations handle conversion between these representations.

**IMPORTANT**: Never reuse RPC request or response message types across multiple RPCs.
Each RPC should have its own dedicated request and response types with inline fields.
This allows each RPC's contract to evolve independently without breaking other RPCs.

See [`docs/proto_conventions.md`](docs/proto_conventions.md) for the full
convention — API message shape, naming parity where a value genuinely
round-trips, where conversion lives, and round-trip test expectations.

## Finding Documentation Context

Durable guidance docs under `docs/` are indexed in **`docs/llms.txt`** — a
generated, hierarchical map of every architecture, subsystem, infrastructure,
workflow, convention, and product doc, each tagged with the code paths (`globs`)
it governs. **Before implementing, auditing, or planning any non-trivial change,
consult `docs/llms.txt` and read the docs relevant to the area you're touching**
— don't work from the code alone, and don't read every doc.

How to select what to read:

1. **Always read the anchors** for the side you're working on — the docs marked
   ⚓ in the map. For app work: `client/architecture.md` +
   `client/conventions.md` + `proto_conventions.md`. For server work:
   `server/architecture.md` + `server/conventions.md` + `proto_conventions.md`.
   The `conventions` docs hold the side-specific coding rules (Go / Flutter
   style, error handling, testing, etc.) that used to live inline here.
2. **Match by code path.** Each indexed doc lists `globs`. Read the docs whose
   globs match the files your change touches (e.g. editing `app/lib/**/chat/**` →
   read `client/chat.md` and `realtime_updates.md`).
3. **Match by topic.** For cross-cutting work without a clean code-path mapping
   (product, security, infra), match the doc `description`/`triggers` in the map
   to your task.
4. **Apply the engaged review dimensions.** For each engineering dimension your
   change touches (security, error handling, performance, accessibility, …), read
   its `docs/reviews/<dimension>_review_prompt.md` and follow that prompt's
   **"Areas to Review"** section as the checklist — skip the runner scaffolding.

When you add or substantially change a durable doc, add/update its `context:`
frontmatter block (schema in [`docs/context_map.md`](docs/context_map.md)). The
CI gate `npm run lint:context-map` (part of `npm run lint`) fails if a durable
doc is missing a valid block or if `docs/llms.txt` is stale — regenerate with
`npm run generate:context-map`. Excluded from the map: `docs/issues/`,
`docs/ai/`, `docs/release/notes/`, `docs/crashlytics/`, and eval/report outputs.

The `ripls-issue` and `ripls-audit` skills consume this same map
programmatically; this section is the same protocol for everyone else.

**Prose freshness.** Beyond routing, durable docs are kept *semantically* in sync
with the code they describe. Each carries a `freshness:` frontmatter stamp
(verified commit), and the `claude_refresh_docs` nightly ratchet (~2am US
Central) auto-refreshes the docs whose governed code changed in the day's merges
to main — verifying prose against code and opening a docs-only PR with the
fixes. The `ripls-doc-freshness` skill
(`.claude/skills/ripls-doc-freshness/`) is the per-doc verifier; the stamp schema
and the rest are in [`docs/context_map.md`](docs/context_map.md) → *Prose
freshness*. If you change code a doc describes, expect the nightly ratchet to
refresh that doc; if you change a doc, keep its claims true to the code.

## Development Commands

### Code Generation

Generated code is **not checked into source control**. It is automatically
generated in CI/CD pipelines and Docker builds. For local development, you must
generate the code before building or running tests:

```bash
# Generate code from protobuf definitions (Go and Dart)
npm run generate
```

**IMPORTANT**: Always use `npm run generate` to generate code from protobuf
definitions, not `buf generate` directly. The npm script ensures all required
code generators are in the PATH.

**Note**: You must generate code locally before:

- Running Go tests (`go test`)
- Running Flutter tests (`flutter test`)
- Building the server (`go build`)
- Running the server locally

### Setup Dependencies

```bash
# Install Go code generators (one-time setup)
go install github.com/bufbuild/buf/cmd/buf@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install connectrpc.com/connect/cmd/protoc-gen-connect-go@latest


# Install project dependencies
npm install

# Check your local toolchain matches CI/runners (versions.env). Catches a
# different local Go/Node/Flutter/JDK (and Xcode/CocoaPods/Ruby on a Mac)
# before it becomes a "works on my machine" CI failure.
npm run doctor:versions
```

### Go Module Management

```bash
# Initialize Go module (already done)
go mod init library

# Download dependencies
go mod tidy
```

### Running the local server

`npm run start:server` needs both `gcloud auth login` (CLI auth for Secret
Manager fetches) and `gcloud auth application-default login` (ADC for the
server's Vertex AI / Firebase calls). The startup script preflights both
and prints the exact reauth command on failure — hand that command to the
user; never run an interactive `gcloud auth ... login` from a tool.

## Configuration Files

- `buf.yaml`: Buf configuration for linting and validation (uses STANDARD
  linting rules)
- `buf.gen.yaml`: Code generation configuration for protoc-gen-go and
  protoc-gen-connect-go
- `go.mod`: Go module definition (Go 1.25.1)

## Style Guidelines

- Follow Buf style guide:
  https://buf.build/docs/best-practices/style-guide/#categories
- Generated code uses `paths=source_relative` option
- Every Go package must have a `// Package <name> ...` comment. Park it in
  a dedicated `doc.go` file (1–3 sentences). The `revive.package-comments`
  lint rule enforces presence and will fail the build if missing.
- Every multi-file directory under `server/` and `app/lib/` must have a
  `README.md` covering purpose, key files/types, and when to add code here
  vs. an adjacent directory. Style models: `server/ai/README.md` for larger
  packages, `app/lib/presentation/widgets/creation/README.md` for smaller
  ones. No vendor names in package-level docs — use the capability instead.

### READMEs and Internal Documentation

READMEs, `doc.go` package comments, and docs under `docs/` are written for
contributors, not API clients. **Use actual vendor names** where they help
orientation: "Mapbox" (not "geocoding provider"), "Mailgun" (not "email
service"), "Firebase Cloud Messaging / FCM" (not "push notification
platform"), "Pexels" (not "stock image provider"), "Vertex AI / Gemini" (not
"AI provider"). `docs/server/architecture.md` is the canonical style model.
The proto-comments vendor ban (see `docs/proto_conventions.md` →
"Proto comments") does **not** apply here.

### File Editing

-When making code changes, ALWAYS use the Edit tool (or Write for new files). NEVER use command-line text manipulation tools like `sed`, `awk`, or `perl` to edit code files. Use `sed`/`awk`/`perl` ONLY for bash scripting tasks where text transformation is the goal, NOT for editing source code files.

### TODO Comments

Applies to all languages (Go, Dart, TypeScript, shell, proto). A TODO
without a tracking issue is invisible to anyone but the author and rots
fast — the codebase already has multiple instances of "TODO: fix this"
that have been there for months with no one watching.

- **Every TODO must reference a GitHub issue.** Format: `TODO(#NNNN):`
  where `#NNNN` is an open issue tracking the cleanup. Existing examples
  in the codebase: the `TODO(#2020):` proto-deprecation markers in
  `proto/ripls/api/portfolio.proto` and `proto/ripls/api/item.proto`.
- **If no issue exists, file one before writing the TODO.** A two-minute
  `gh issue create` is part of the cost of leaving a TODO. The issue
  becomes the place where context lives — what triggered the TODO, why
  it was deferred, what needs to happen to remove it, who's affected.
  Inline TODOs are pointers, not specifications.
- **The TODO body itself must be enough for a future reader to act on
  without opening the issue.** State, in order: (1) what the code does
  today and why it's wrong/incomplete, (2) the concrete thing that would
  let the TODO be removed, (3) any constraint that blocked doing it now.
  Two to four lines is the right size. Issue-only TODOs ("TODO(#1234):
  fix this") are not allowed — if the inline context is "open the
  issue," delete the comment and let `git blame` lead the reader to the
  issue from the commit message.
- **Match the TODO description to the issue body, but don't duplicate
  it.** The issue captures every detail (reproduction, options, related
  work). The TODO captures the minimum a reader needs to decide whether
  this code is the right place to start.
- **Plain `TODO:` (no issue number) is reserved for in-PR scratch
  notes** that must be resolved or converted to a tracked TODO before
  the PR merges. Treat unresolved plain TODOs as PR-review bugs.
- **`FIXME` is not used.** Standardize on `TODO(#NNNN):`.
- **When closing a TODO, also close the issue** (or vice-versa: when
  closing the issue, grep the codebase and remove the TODO). Stale
  TODOs pointing at closed issues are worse than no TODO.

## Development Patterns and Preferences

### Architecture Patterns

- **Minimal Dependencies**: Only add the minimal required dependencies; avoid
  over-engineering

### File Organization

- **File Extensions**: Use `.yaml` extension for all YAML files (not `.yml`)
- **Git Operations**: Use `git mv` when renaming files to preserve history
- **npm Scripts**: Use npm as a universal task runner for cross-language build
  automation, with separate commands for different linting tools (e.g.,
  `lint:proto`, `lint:go`)
  **Files should be short (preferably < 1000 lines) and focused on related functionality.**

  **Dart files: enforced via CI.** `npm run lint:dart:size` (also wired into `.github/workflows/test_flutter.yaml`) fails when any non-generated Dart file under `app/lib/` exceeds **1,000 lines**. Excluded: `gen/`, `*.g.dart`, `*.freezed.dart`, `app/lib/l10n/app_localizations*.dart`. If a split is genuinely deferred, add a `// dart-line-count-allow` comment near the top of the file with a tracking-issue link explaining why; the gate skips files containing that comment. Do not add the hatch to silence the gate without an issue.

  **Go files: enforced via CI (#1424).** `npm run lint:go:size` (also wired into `.github/workflows/test_go.yaml`) fails when any non-generated Go file under `server/` exceeds **1,000 lines**. Excluded: `*.pb.go`, `*_connect.go`, and anything under a `gen/` directory. Split oversized files into sibling files in the same package following the naming convention in `docs/server/architecture.md` §3 (`service.go` keeps the struct/constructor/`Set*` wiring; feature files are named after functionality). If a split is genuinely deferred, add a `// go-line-count-allow` comment near the top of the file with a tracking-issue link explaining why; the gate skips files containing that comment. Do not add the hatch to silence the gate without an issue.

### Implementation Approach

- **Interface-First**: When refactoring to support multiple implementations,
  create the abstraction interface first to minimize code duplication
- **Provider Agnostic**: Keep infrastructure and containerization
  provider-agnostic; avoid cloud-specific dependencies in core application code
- **Single Responsibility**: Each function/module should have a clear, single
  purpose

### Prevent Claude Code Crashing

- don't put any claude branding or authorship in git change messages
- always add unit tests for each new library or significant library function
- Always use the full .yaml extension for YAML files