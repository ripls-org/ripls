---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Proto API/model conventions — flattened request/response messages, no cross-RPC reuse, API/models package separation, naming parity.
  globs: [proto/**]
  triggers: [proto, protobuf, rpc, api, message, schema]
  lens: [conventions]
  alwaysApply: true
  domain: conventions
freshness:
  verified_commit: "c071b8768"
  verified_on: "2026-07-31"
---
# Proto API and Models Conventions

This document codifies the conventions that govern the two protobuf packages in
this repository:

- `proto/ripls/api/` — the RPC contract with clients: request, response, and
  shared API types exposed over the wire.
- `proto/ripls/models/` — the storage/persistence types used by the Proto-SQL
  storage abstraction.

These are separate, independent contracts. The sections below describe how each
is shaped and the rules the service layer follows when it sits between them.

## Table of contents

1. [The separation rule](#the-separation-rule)
2. [API message shape](#api-message-shape)
3. [Field presence: use `optional`](#field-presence-use-optional)
4. [Field-naming parity across layers](#field-naming-parity-across-layers)
5. [Where conversion logic lives](#where-conversion-logic-lives)
6. [What belongs in the API](#what-belongs-in-the-api)
7. [Optional updatable scalars on save-style RPCs](#optional-updatable-scalars-on-save-style-rpcs)
8. [Round-trip tests](#round-trip-tests)
9. [Proto comments](#proto-comments)
10. [What enforces these rules](#what-enforces-these-rules)
11. [Deprecating and removing fields](#deprecating-and-removing-fields)

## The separation rule

The API is the contract with RPC clients. It stands on its own: its shape is
driven by what clients need, not by what the server happens to store. Whether a
given API field has a mirror in the storage model is an implementation detail,
and in most cases should be invisible to a reader of the `.proto` files.

Mechanically, the two packages never import each other. Run
`grep "ripls/api" proto/ripls/models/*.proto` and
`grep "ripls/models" proto/ripls/api/*.proto` — both return empty. **Only
service packages import both**; every other package imports one or the other.
Allowing a model proto to import an API proto (or vice versa) would couple the
two schemas: the wire format would be tied to the storage format, schema
evolution on one side would ripple into the other, and the "API is a separate
contract" claim would no longer hold.

**Client side:** The Flutter app imports *only* `proto/ripls/api` (via
generated Dart in `app/lib/data/gen/`). It never references
`proto/ripls/models`. Those generated types are a server/storage concern.

## API message shape

**Each RPC gets its own dedicated request and response message types.** No
`GearRequest` / `GearResponse` reused across `SaveGear`, `GetGear`,
`DeleteGear`. Instead: `SaveGearRequest` / `SaveGearResponse`,
`GetGearRequest` / `GetGearResponse`, etc. The aim is not to forbid type
reuse — we do factor shared API idioms into common protos (see
`api/common.proto`, `api/user.proto`) and that leads to less repetitive
client code when the same type appears in multiple places. The aim is to
avoid reuse that leads to unused fields in one direction, confusion about
which fields are populated when, or coupling that forces unrelated RPCs to
change together. When reuse doesn't exactly fit, prefer a flat or distinct
type that matches the RPC contract.

**Flattened fields are preferred where they make the contract clearer.**
`SaveLocationRequest` carries `double latitude_deg` inline rather than a
nested coordinate message. The rule is "shape the message around the RPC",
not "never nest" — shared API types are nested where that reads better for
the client.

## Field presence: use `optional`

- **Use `optional` for conditionally-present fields.** Any field that may not be
  set — timestamps that only exist after a state transition, strings that are
  empty when unset, message fields that are absent in some states — should use
  the `optional` keyword. This gives explicit presence tracking (`Has*()` in
  Dart, `!= nil` in Go) and eliminates ambiguity between "zero value" and
  "never set."
- **Go impact:** Optional scalars become pointers (`int64` → `*int64`,
  `string` → `*string`). Use nil checks for presence, dereference for access,
  and generated `Get*()` methods for nil-safe reads. Optional message fields
  have no Go impact (already pointers).
- **Dart impact:** Use `has*()` methods for presence checks instead of
  `> 0` / `.isEmpty` patterns.
- **Wire compatibility:** Adding `optional` to an existing proto3 field is
  wire-compatible — no migration needed for stored protos.

The [save-style RPC rule below](#optional-updatable-scalars-on-save-style-rpcs)
is a stricter specialization of this for updatable scalars, where absence
specifically means "leave unchanged."

## Field-naming parity across layers

We assume no relationship between API and model fields. Most of the time
they are independent: the API exposes only what clients need, and the model
stores whatever the server needs, with no guarantee that names, types, or
even the existence of a field line up.

When there *is* a direct relationship — a user-provided value that flows
client → storage → client unchanged — we keep the name the same at every
layer. That parity is a convention, not a derivation: the service layer
still does the conversion explicitly, but the identical name makes origin
trivially greppable and makes tooling easier.

| Layer | Example |
|-------|---------|
| Client (Dart) | `saveLocation(externalPlaceId: ...)` |
| API request | `api.SaveLocationRequest.external_place_id` |
| API response | `api.GeocodedLocation.external_place_id` |
| Storage | `models.Location.external_place_id` |

When a mapping is non-obvious (e.g. an API field that aggregates multiple
model fields, or vice versa) document it in a field comment so the next
reader doesn't have to reverse-engineer it.

## Where conversion logic lives

Conversion between `api` and `models` types lives in the **service layer**,
inline in the feature file that implements the relevant RPC. Helpers follow
the naming pattern `convertXToAPI` (storage → wire) and `convertXToStorage`
(wire → storage) and are unexported.

Concrete example: `server/services/gear/service.go` contains
`convertStorageGearMetadataToAPI`, `convertStorageValueEstimateToAPI`,
`convertAPIValueEstimateToStorage`, and siblings. They sit in the same file
as the RPC handlers that call them.

**Don't create a dedicated `convert.go` file for each service.** It's not
the current pattern (one exception exists — `experience/convert_informal_time.go`
— for a standalone conversion unrelated to the API/models boundary) and
splitting conversion into a separate file tends to hide coupling between
handlers and their conversions. Keep them adjacent.

**Don't put conversions in the proto packages or the storage package.**
Both are shared libraries. Conversion is a service-layer concern, and as
a corollary, **only service packages are allowed to import both `api` and
`models` protos**.

## What belongs in the API

Every model field is assumed private to the server implementation. The API
exposes the minimum set of fields required for client functionality —
never speculatively mirror model fields and never add API fields in
anticipation of future use.

A few categories of model field are obviously server-only and worth calling
out so reviewers don't ask each time:

- **Audit timestamps.** `created_at_unix_sec`, `updated_at_unix_sec` on
  every model. Only surfaced on the API if a client actually needs to
  display them.
- **Soft-delete metadata.** `models.X.deleted` (a `DeletedMetadata`
  message) is never exposed — deleted rows are filtered before the API
  layer ever sees them.
- **Search index fields.** Embedding vectors, tsvector columns,
  denormalized text used for indexing.
- **Internal-only counters.** Denormalized counts kept for query speed are
  re-derived for the client if needed, not returned directly.

When a model field is intentionally server-only it's fine to note that in
the field comment:

```proto
// Soft deletion metadata. Server-only; not exposed on the API.
optional DeletedMetadata deleted = 20;
```

But the default is "not on the API" — a model field without a comment is
still server-only, not a candidate for exposure.

## Optional updatable scalars on save-style RPCs

Every updatable scalar on a save-style RPC (e.g. `SaveGear`, `SaveUser`)
must be `optional`. A nil/absent field means "leave unchanged"; a non-nil
pointer — including one pointing to `""` — means "update to this value".

**Go:** optional scalars become `*string` — check `!= nil`, not `!= ""`:

```go
if req.Msg.Name != nil {
    resource.Name = *req.Msg.Name
}
```

**Dart:** drop `isNotEmpty` guards so `""` reaches the server:

```dart
if (name != null) request.name = name;
```

**Round-trip coverage:** every optional updatable scalar needs a set pass
(`AssertFieldRoundTrip`) and a clear pass (`AssertFieldClear`) in
`*/roundtrip_test.go` (see `server/services/roundtrip_helpers.go`).

## Round-trip tests

For any API field with round-trip semantics — the client sets it on a
save-style RPC and expects to read the same value back on a subsequent
read-style RPC — we assert that contract with a round-trip test. See
`server/services/roundtrip_helpers.go` for the `AssertFieldRoundTrip`
helper and the various `*/roundtrip_test.go` files for examples.

These tests live at the API boundary. They call the save RPC, call the
read RPC, and assert that the value came back equal. They don't know or
care whether there's a matching model field; they care that the API
contract round-trips. A round-trip test should exist for every API field
the client sets and reads back, regardless of how the server stores it
(or whether it stores it at all).

The tests are also the mechanical guard for the class of bug where a new
API field is wired into the save path but forgotten on one of the read
paths — adding the test forces you to exercise both paths.

## Proto comments

Proto comments are the API contract seen by every downstream client; this
section governs `.proto` file comments only — not READMEs or internal docs
(which **may** use vendor names — see
[READMEs and Internal Documentation in CLAUDE.md](../CLAUDE.md)).
Describe what a message, field, or RPC *means*; never describe how the server
produces it. Keep the following categories out of proto comments:

- **Vendor and dependency names.** Mapbox, Pexels, GCS, Unsplash, Anthropic,
  Gemini, OpenAI, etc. are implementation choices that can change without a
  wire-format change. Use the capability ("geocoding", "stock image",
  "AI provider"), not the vendor. The same rule applies to enum value names
  (`GEN_STREAM_ERROR_CODE_MAPBOX_FAILED` → `GEN_STREAM_ERROR_CODE_GEOCODING_FAILED`).
- **Transport plumbing.** Don't mention Connect, gRPC, HTTP status codes, or
  how errors propagate through a particular runtime.
- **Server-side mechanics.** Fan-out branches, tokenizers, goroutines, caches,
  job queues, and the moment a particular JSON field closes are invisible to
  clients. Describe *what* is emitted, not *when in the pipeline* it fires.
- **Internal sentinel values.** Don't reference placeholders like
  `USER_PRIMARY_LOCATION` that are server-prompt internals. Clients never see
  them; documenting them in the API leaks schema.
- **Project conventions.** Rules like "proto convention forbids
  reusing request types across RPCs" belong in this doc and in PR
  descriptions. A downstream client reading the proto does not care why the
  shape is what it is, only that it is what it is.

Write proto comments as you would for a public API doc: a reader outside the
project (or outside the server codebase) should understand the field without
knowing anything about how it's implemented.

## What enforces these rules

Mechanical:

- `npm run lint:proto` (buf lint, STANDARD rules) — catches proto hygiene
  (snake_case, required package, etc.) but does not enforce the API
  message-shape rules or the separation rule.
- `go test ./...` plus the round-trip helpers — catches missed read-path
  fields for any API that has round-trip coverage. See
  [Round-trip tests](#round-trip-tests).
- `npm run lint:proto:breaking` (buf breaking, `WIRE`) — rejects changes that
  would mis-decode data already written. Runs on every PR inside the `Go Tests`
  job, against `origin/main`. See
  [Deprecating and removing fields](#deprecating-and-removing-fields).

## Deprecating and removing fields

`buf.yaml` runs the `WIRE` category, which polices the binary wire format —
the only compatibility surface this repo has. Models persist to the
`binary_proto` column and the Flutter client's transport uses
`codec: const ProtoCodec()`; the single protojson caller is the CheckHealth
handler. `buf.yaml` records how the category was chosen and what it measured.

### What the gate rejects, permits, and misses

**Rejects** — each of these silently mis-decodes data already written:

- changing a field's type in place (`string` → `enum` is proto3 wire type 2 → 0)
- changing a field's cardinality, or moving it in or out of a `oneof`
- deleting a field without reserving its **number**
- changing an RPC's request or response type

**Permits** — none of these touch the binary encoding:

- adding a field, message, enum value, or RPC
- renaming a field (numbers are the wire identity, not names)
- reserving only the number; the name is optional under `WIRE`

**Misses.** `RPC_NO_DELETE` and `SERVICE_NO_DELETE` are `FILE`/`PACKAGE`-only,
so removing an RPC passes. A green gate is not evidence that dropping an RPC is
safe — check for callers yourself.

### Removing a field from `proto/ripls/models`

Storage protos have no version skew. The constraint is rows already in
Postgres, not app builds in the field, so nothing has to "age out" — a field can
go as soon as no code reads it.

1. Remove the reads.
2. Delete the field and **reserve its number**:

   ```proto
   message ScheduledNotification {
     reserved 8;
   }
   ```

3. Confirm the gate is clean: `npm run lint:proto:breaking`.

Existing rows keep the bytes; proto3 ignores them as unknown fields. The number
must never be reused, which is exactly what the `reserved` line buys — reusing
it would decode old bytes as whatever the new field claims to be.

### Changing a field's type in `proto/ripls/models`

Not possible in place. Add alongside, migrate, then remove:

1. **Add** a new field with a fresh number. Free — adding is never breaking.
2. **Dual-populate.** Route every write through one helper that sets both, so no
   caller can update one and forget the other.
3. **Read** the new field, falling back to the old one for rows written before
   the migration.
4. **Backfill.** Make the job idempotent and batched; re-running it is then the
   cheapest way to prove it finished.
5. **File the cleanup issue before** any `TODO(#NNNN)` referencing it lands
   (see CLAUDE.md), then remove the old field and reserve its number.

`models.ExperienceRSVP.intention` is the worked example: #2829 added
`intention_enum` alongside the string field and ran the migration, and #2832
(the cleanup issue step 5 pre-files) removed the string fields, reserved
numbers 5 and 6, and renamed the enum fields back to the clean names —
renames are free under `WIRE` because numbers are the wire identity. The
storage side followed with an idempotent column migration
(`server/storage/rsvp_column_rename.go`).

### Removing a field from `proto/ripls/api`

API protos *do* have version skew: released app builds still call the server and
still read the field. Removing it does not crash them — proto3 decodes an absent
field as its zero value — but they will render blanks.

**The wait is one shipped release plus about 24 hours.**

1. Mark it `[deprecated = true]` with a `TODO(#NNNN):` naming the issue that
   tracks removal, and keep populating it.
2. Migrate the client off it. Deleting a proto field is a **compile error** in
   Dart, so `npm run generate && flutter analyze` proves no client *code* reads
   it — but that says nothing about builds already in the field, which is what
   the wait is for.
3. Once a release carrying step 2 has shipped and been out roughly a day,
   delete the field and reserve its number.

**Magnitude does not matter — a fix release counts.** What the wait buys is
installed builds rolling forward, and a `v0.28.2` pushes users forward exactly
as well as a `v0.29.0` does. Counting in semver minors instead of shipped
releases made the interval an accident of how the version happened to be
bumped, which is why the rule used to read "two full release cycles" and
routinely stalled removals for weeks longer than the risk warranted.

Count from the release that shipped step 2, not from the deprecation marker.
For #2827: PRs #2836–#2839 removed the client's readers and all shipped in
v0.28.2, so those fields became removable a day later — #2835 retired them.

The 24 hours is a soak for the release itself, not for adoption: it is there so
a release that turns out to be broken can be pulled before the server drops the
fields its predecessor still needs. If a release is rolled back, the clock
restarts with the next one.

Batch removals into one issue rather than deprecating field-by-field: #2020
collected 19 fields across two protos and retired them together, which made the
wait a single decision instead of nineteen.

### Removing an RPC

The gate does not cover this — `RPC_NO_DELETE` and `SERVICE_NO_DELETE` are
`FILE`/`PACKAGE`-only — so a green `lint:proto:breaking` says nothing about
whether an RPC is safe to drop. Two things stand in for it:

1. **Prove nothing calls it.** Repository and service wrappers are not callers;
   check for a viewmodel or screen that actually invokes them. #2820 found four
   RPCs with full client plumbing on both sides and zero real callers, and #2831
   found three more.
2. **Wait the same one-release-plus-a-day** after the client stopped calling it.
   An old build hitting a removed RPC gets `UNIMPLEMENTED` rather than a blank
   field, so the failure is louder than a removed field — the screen breaks
   instead of rendering empty. The interval is the same, but prefer to let an
   RPC removal ride a release further than the minimum when the surface it
   breaks is one a stranded build would hit on launch.

Reserve nothing: RPC names and numbers are not reused the way field numbers are.
Delete the RPC's request/response messages in the same change — `MESSAGE_NO_DELETE`
is `FILE`-scoped too, so `WIRE` lets both ends go together.

#2526 is the worked example, and it shows why clause 1 says what it says. The
client had "stopped calling" `ShareGear` / `ShareExperience` / `ShareRequest` and
their `Unshare*` counterparts since v0.28.0, but a grep for those names still hit
dozens of Dart lines: the migration happened at the *service-wrapper* layer, so
`CommunityService.shareGear()` survives as a Dart method that builds a
`ShareItemRequest`. The wrappers are not callers. What mattered was that no
viewmodel or screen reached the deprecated wire surface, and four releases had
shipped since.

### When the wait is not enough

There is no force-upgrade or minimum-version gate in the codebase, so a build
that never updates keeps calling forever. One release plus a day is the working
rule, not a guarantee, and shortening the wait from two cycles did not change
that — neither interval ever bounded a build that simply never updates. If a
particular removal needs certainty rather than a convention —
a security fix, or a field whose absence would corrupt rather than blank a
screen — that needs version telemetry we do not currently ship, and the removal
should wait on building it.
