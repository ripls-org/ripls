# Contributing

Thanks for looking. This is a Go server plus a Flutter app for community
sharing of goods and skills, and outside contributions are welcome.

## Sign your commits (DCO)

Every commit needs a `Signed-off-by` line. Git adds it for you:

```bash
git commit -s -m "Fix the thing"
```

That line is your statement, under the [Developer Certificate of
Origin](https://developercertificate.org/), that you wrote the patch or
otherwise have the right to contribute it under this project's license. It is
**not** a copyright assignment — you keep your copyright, and your contribution
is licensed under Apache-2.0, the same license the project ships under.

If you forget, CI will tell you. `git commit --amend -s` fixes the last commit;
`git rebase --signoff main` fixes a branch.

## Before you open a pull request

Generated code is **not** checked into source control, so generate it first:

```bash
npm install
npm run generate          # protobuf → Go and Dart
```

Then run the checks that cover what you touched:

```bash
npm run lint              # the full gate; slow but definitive
go test ./server/...      # server
cd app && flutter test    # client
```

Tests need Docker running — the Go suite starts PostgreSQL via testcontainers.
`npm run doctor:versions` will tell you if your local toolchain differs from the
one CI uses, which is the usual cause of "works on my machine".

## Conventions worth knowing up front

[`CLAUDE.md`](CLAUDE.md) is the authoritative style guide, and it is written for
both humans and coding agents. A few rules bite early:

- **Protos.** `proto/ripls/api/` (RPC types) and `proto/ripls/models/` (storage
  types) must never import each other, and no request/response message is shared
  between two RPCs. See [`docs/proto_conventions.md`](docs/proto_conventions.md).
- **TODOs need an issue.** `TODO(#1234):` or nothing — a CI gate enforces it.
  File the issue first; the TODO body should say enough that a reader can act
  without opening it.
- **Files stay under 1,000 lines.** Enforced for Go and Dart.
- **User-visible strings are localized.** The client renders in-app copy from
  ARB files; the server renders push and email copy from its own catalogs.

For architecture, start with [`docs/server/architecture.md`](docs/server/architecture.md)
or [`docs/client/architecture.md`](docs/client/architecture.md).
[`docs/llms.txt`](docs/llms.txt) is a generated index of every durable doc, tagged
with the code paths it governs — the fastest way to find the doc that covers the
area you are changing.

## Reporting a security issue

Please don't open a public issue. See [`SECURITY.md`](SECURITY.md).

## A note on the name

The code is Apache-2.0. The Ripls name, logo, and icons are not — see
[`TRADEMARK.md`](TRADEMARK.md). Fork freely; just rebrand before you ship to
users.
