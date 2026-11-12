# Staging website infrastructure

Per-PR staging deployments of the marketing site at
`https://<branch-slug>.staging.ripls.org/`. Each PR that touches `website/`
gets its own copy of `website/content/`, served read-only by a single nginx
container that routes by hostname.

## Files

- `docker-compose.yml` — defines `static_web_type2fun_staging`, an nginx
  container exposed on host port `18791`. Copied into the mimir repo as
  `stacks/type2fun_static_web_staging/`, next to the production stack
  `type2fun_static_web`, and deployed by pulling the mimir repo to `/opt/nas`.
  (It was missing from mimir after the Ubuntu migration, so every preview
  returned 502, until 2026-09-18.)
- `config/default.conf` — the only nginx config the container loads. Uses a
  regex `server_name` to extract the branch slug from the hostname and serve
  `/srv/sites/<slug>/`.

## Layout on mimir

```
/srv/sites/type2fun_static_web_staging/
├── config/                          # mounted read-only at /etc/nginx/conf.d
│   ├── default.conf                 # regex fallback + 404 catch-all
│   ├── locations.include            # production's routing (fallback only)
│   ├── site-web-staging.conf        # one server block per active PR branch
│   ├── locations-web-staging.include  #   + that branch's routing
│   ├── site-fix-1786.conf
│   └── locations-fix-1786.include
└── sites/                           # mounted read-only at /srv/sites
    ├── web-staging/                 # one dir per active PR branch
    │   ├── index.html
    │   └── ...
    └── fix-1786/
        └── ...
```

## Routing config

Routing rules live in `locations.include`, shared verbatim with production
(`website/config/locations.include`), so staging and prod can't drift — and a PR
that *changes* routing has to be exercisable on its own preview.

**Each preview gets its own copy.** Every PR deploy writes two files into
`config/`: `locations-<slug>.include` (that branch's snippet) and
`site-<slug>.conf` (a generated server block with an exact `server_name` that
includes it). nginx prefers an exact `server_name` over a regex, so each preview
is served by its own block and its own routing.

This matters because the alternative silently breaks. Until #2895 the snippet
was rsynced to **one global path** that every preview's server block included,
so the last website deploy to run set the routing for every branch at once — the
home-page PR's preview served main's `/` for five weeks while six later deploys
overwrote its snippet, and in the other direction that PR's own routing would
have 404'd the root of every other preview.

`config/default.conf` is the fallback for a preview with no server block of its
own (deployed before the split, or caught mid-deploy) plus a 404 catch-all for
undeployed hostnames. Its `locations.include` is production's, rsynced by
`publish_website.yaml` on merges to main.

Both kinds of server block send `Cache-Control: no-cache`. Without it nginx
sends only `ETag`/`Last-Modified` and browsers apply *heuristic* freshness —
they serve a preview from cache for a fraction of the file's age without asking
the server, so a reviewer sees a stale page after a deploy and can't tell it
apart from a deploy that didn't work. `no-cache` still allows a `304`, so it
costs a conditional request, not a re-download.

nginx is reloaded by the host-side watcher in `infra/web-nginx-reload/` (the
rsync deploy endpoint can't reload nginx itself). The watcher runs `nginx -t`
first, so a half-landed pair of files can only be skipped, never served — the
container keeps its last-good config and converges on the next poll.

## Workflows

- `.github/workflows/deploy_staging_website.yaml` — on every PR push that
  touches `website/`: rsyncs `website/content/` into `sites/<slug>/`, and writes
  `config/locations-<slug>.include` + `config/site-<slug>.conf`.
- `.github/workflows/cleanup_staging_website.yaml` — removes all three when the
  PR closes. Order matters: the server block goes first, since an `include`
  pointing at a deleted file fails `nginx -t` and would freeze the whole staging
  container on its last-good config.
- `.github/workflows/publish_website.yaml` — on merges to main, also refreshes
  the fallback `config/locations.include`.
