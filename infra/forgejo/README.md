# infra/forgejo

Things installed on the Forgejo instance itself, as opposed to workflows that
run on it (`.github/workflows/`) or its runners (`runners/forgejo/`).

- `pre-receive-gitleaks` — a git `pre-receive` hook that refuses a push which
  adds a secret. `docs/secrets.md` → *Push-time refusal* says where it sits
  among the other scans.
- `install_hook.sh` — installs, updates or removes that hook for one
  repository, along with the gitleaks binary it runs.
- `pre-receive-gitleaks.test.sh` — the hook's tests: real pushes into a
  throwaway bare repository. Needs `git` and a `gitleaks` binary.

Add a file here when it has to live on the instance to work. Server deploy
files belong in `infra/server/`.
