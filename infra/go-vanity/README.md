# Go module vanity host (`go.ripls.org`)

A standalone static site whose only job is to answer the Go toolchain's
`?go-get=1` probe with a `<meta name="go-import">` tag.

## Why it exists

The module path is `go.ripls.org/ripls`, not `github.com/ripls-org/ripls`.
That means the import path names the **project**, not the host the source
happens to sit on. Moving to a different forge — Codeberg, a self-hosted
Forgejo, anything — is an edit to one line of `content/index.html`. No import
statement anywhere changes, and no downstream user has to do anything.

The trade is that the obligation moves to us. `github.com/…` would have had
GitHub keep the path resolvable; now it is this DNS record and this container.
`proxy.golang.org` is an immutable mirror, so versions that have already been
published keep resolving even if this host is down — but a new version, a
`GOPRIVATE` consumer, and pkg.go.dev indexing all need it live.

It is deliberately **separate** from the marketing site: its own container, its
own config, its own reverse-proxy entry. A marketing deploy cannot break module
resolution, and vice versa.

## Files

- `docker-compose.yml` — the `ripls_go_vanity` nginx container, host port
  `18792`. Belongs at `/opt/nas/stacks/ripls_go_vanity/` on mimir, same as the
  other web containers, and is deployed by pulling the mimir repo there.
- `config/default.conf` — the whole nginx config. A catch-all: **every** path
  returns the same page.
- `content/index.html` — the page, carrying `go-import` and `go-source`.

### Why the catch-all matters

Resolving `go.ripls.org/ripls/server/storage`, the toolchain requests the
**full import path** with `?go-get=1`, not just the module root. Serving the
tag only at `/ripls` would leave deep imports depending on fallback behaviour.
This host serves nothing else, so answering everything is both simplest and
safest. A 404 to the go tool reads as "no such module".

## Layout on mimir

```
/mnt/user/media/sites/ripls_go_vanity/
├── config/
│   └── default.conf        # mounted read-only at /etc/nginx/conf.d
└── content/
    └── index.html          # mounted read-only at /srv/content
```

## First-time setup

Three manual steps; none is automated because none of them changes again.

1. **DNS.** Add `go.ripls.org` in Cloudflare, pointing at the same place the
   other mimir-hosted sites do. DNS records for this domain are managed by
   hand, not Terraform.
2. **Reverse proxy.** Add an entry mapping `go.ripls.org` to mimir port
   `18792`, alongside the existing website entries. TLS is terminated there,
   like every other site — the container itself is plain HTTP on the LAN.
3. **Bring it up.** Copy `config/` and `content/` to the path above, register the
   compose project, and start it.

## Deploying a change

The page changes about as often as the forge does. Copy `content/index.html` to
mimir and reload:

```bash
rsync -av infra/go-vanity/content/ root@mimir:/mnt/user/media/sites/ripls_go_vanity/content/
ssh root@mimir docker exec ripls_go_vanity nginx -s reload   # only if config changed
```

## Verifying

What the go tool actually does, and the two things worth checking after any
change:

```bash
# The tag is served, and served for a deep path as well as the root.
curl -s "https://go.ripls.org/ripls?go-get=1" | grep go-import
curl -s "https://go.ripls.org/ripls/server/storage?go-get=1" | grep go-import

# End to end, from outside any checkout.
cd "$(mktemp -d)" && go mod init scratch && go get go.ripls.org/ripls
```

`npm run lint:go-vanity` checks the narrower thing that actually drifts: that
the module path in the tag still matches the `module` line in `go.mod`.
