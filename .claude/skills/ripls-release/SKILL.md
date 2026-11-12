---
name: ripls-release
description: "Execute a ripls app release following the release cheat sheet. Takes a bump type ('major', 'minor', or 'fix') or an explicit version like '0.1.2'. Use when the user types /ripls-release or asks to cut a release, publish a release, or bump the app version."
args:
  - name: version
    description: "Bump type ('major', 'minor', 'fix') or explicit semantic version (e.g. '0.1.2')"
    required: true
---

# Ripls Release Skill

Follow the release cheat sheet at `docs/release/cheat_sheet.md` to release a new version of the ripls app.

The user has provided: `$version`

## Workflow

### Step 1 — Resolve and validate the version

1. Read `app/pubspec.yaml` to get the current version (e.g. `MAJOR.MINOR.PATCH`).
2. If `$version` is a bump type, compute the next version:
   - `major` → increment MAJOR, reset MINOR and PATCH to 0 (e.g. 0.12.1 → 1.0.0)
   - `minor` → increment MINOR, reset PATCH to 0 (e.g. 0.12.1 → 0.13.0)
   - `fix`   → increment PATCH (e.g. 0.12.1 → 0.12.2)
3. If `$version` is an explicit version string, use it directly.
4. Confirm that the resolved version is a valid semver bump from the current version (patch, minor, or major).
5. If the version doesn't make sense (e.g. going backwards, skipping numbers in a weird way), flag it and ask the user before proceeding.
6. Use the resolved version as `$version` for all remaining steps.

### Step 2 — Safety check: data model changes

1. Run `git log` to find the most recent release tag (e.g. `v0.1.1`).
2. Review changes to `proto/ripls/models/` since that tag using `git diff`.
3. If there are non-backwards-compatible changes (renamed protos, renumbered fields), warn the user that a database migration or reset may be needed. Ask if they want to proceed or handle the migration first.
4. If changes are backwards-compatible (new fields only) or there are no changes, note this and continue.

### Step 3 — Deploy server to prod (if needed)

Ask the user if they want to deploy the server to prod before the app release. If yes, remind them to trigger the deploy workflow from GitHub Actions targeting the 'prod' environment (this is a manual step in the GitHub UI — provide the link).

**This step is also the web app release.** The Flutter web build is compiled and copied into `server/services/web/app_assets/` (`npm run build:web:prod`), and `build_and_deploy_container.yaml` rebuilds that bundle and `//go:embed`s it into the server container. There is no separate web deploy — the web app ships with the server. So a prod server deploy releases the new web client; skipping the server deploy means the web app stays on the previous version even after the iOS/Android release.

After the prod server deploy finishes, verify the web client: load the prod server's web app in a browser, confirm it serves the real app (not `placeholder.html`) and that the build reflects this release.

### Step 4 — Create the release branch

```bash
git checkout main
git pull origin main
git checkout -b bump-version-$version
```

### Step 5 — Bump the version in pubspec.yaml

Edit `app/pubspec.yaml` to update the version from the current value to `$version`.

### Step 6 — Create release notes

Follow the canonical prompt at `docs/release/notes_prompt.md`. It covers
finding the previous tag, separating features from fixes, keeping
reporters' names out of the published notes, writing
`docs/release/notes/$version.md`, and running
`npm run generate:release-notes` to produce the website HTML.

Commit the release notes together with the version bump.

### Step 7 — Push and create PR

```bash
git push origin bump-version-$version
```

Create a PR using `gh pr create` with the version bump and release notes. The PR title should be something like "Bump version to $version".

### Step 8 — Post-merge steps (instructions only)

After the PR is reviewed and merged, tell the user what to do next (do NOT execute these automatically — the PR must be merged first):

1. Checkout main and pull:
   ```bash
   git checkout main
   git pull origin main
   ```

2. Create and push the release tag:
   ```bash
   git tag -a v$version -m "Release $version"
   git push origin v$version
   ```

3. Monitor the release:
   - Go to GitHub Actions > Release App and watch both iOS and Android jobs
   - Check [App Store Connect](https://appstoreconnect.apple.com/teams/c20df7de-4156-4347-8298-be402f561082/apps/6754605194/testflight/ios) for the iOS build
   - Check [Google Play Console](https://play.google.com/console/u/0/developers/6244056806996728059/app/4974837172956229507/releases/overview) for the Android build
   - **Web:** the Release App workflow builds iOS and Android only — it does not ship web. The web client was released by the Step 3 prod server deploy. If you skipped that deploy, the web app is still on the previous version; run it now to put web in sync with this release.