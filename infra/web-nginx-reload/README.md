# Static-site nginx auto-reload (mimir)

`reload-on-change.sh` reloads the static-site nginx containers when their mounted
config or content changes.

## Why it exists

The website deploy pipeline rsyncs files into place but **cannot reload nginx**.
The deploy SSH endpoint (repo variables `WEB_DEPLOY_SSH` and `WEB_DEPLOY_PORT`) is a restricted `rsync-server`
container with no docker socket — it can only write files. nginx keeps serving
its in-memory config until something sends it a reload signal.

That gap caused a production outage: a deploy that removed the root `index.html`
(after the landing page moved under `/homescreen/`) shipped a new `default.conf`
with the rule to serve the page at `/`, but nginx was never reloaded — so it kept
running the old config against content that no longer had a root index, and `/`
started returning 403.

This watcher closes the gap. It polls each managed site's config + content dirs
and reloads the matching container within `INTERVAL` (default 15s) of any change.
It reloads **only after `nginx -t` passes**, so a broken config is never applied —
the container keeps serving its last-good config and the failure is logged.

## Managed containers

| Container | Watched host dirs |
|-----------|-------------------|
| `static_web_type2fun` (prod) | `…/type2fun_static_web/{config,content}` |
| `static_web_type2fun_staging` | `…/type2fun_static_web_staging/{config,sites}` |

## On mimir

It runs as the always-on `ripls-nginx-reload` systemd unit, which the **mimir
repo** owns (`deploy/systemd/`), alongside the other `ripls-*` units. The unit
runs `/opt/ripls/web-nginx-reload/reload-on-change.sh`; this repo owns the
script and `infra/sync.sh` ships it there. (It ran from `/boot/config/go` under
Unraid and was lost in the Ubuntu migration until #3059.)

After changing the script:

```sh
infra/sync.sh
ssh root@mimir systemctl restart ripls-nginx-reload
```

Logs go to the journal: `journalctl -u ripls-nginx-reload`.
