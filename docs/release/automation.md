---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Release automation via GitHub Actions + Fastlane — tag-triggered iOS/Android builds, version-from-pubspec plus git-commit-count build numbers, Match code signing, one-time secret setup, and the day-to-day release flow.
  globs: [.github/workflows/release_app.yaml, app/fastlane/**, app/pubspec.yaml]
  triggers: [release, fastlane, match, testflight, play-console, code-signing, git-tag, build-number]
  lens: [infra]
  skills: [issue, audit, triage]
  domain: release
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Release Automation for Ripls Flutter App

This document describes the release automation system for the Ripls Flutter app using GitHub Actions and Fastlane. The system automatically builds and uploads iOS and Android apps to their respective app stores when a version tag is pushed.

For instructions on how to actually use the automation to create a release, see cheat_sheet.md.

## Overview

### Architecture

```text
┌──────────────────────────────────────────────────────────────────────┐
│                         Release Flow                                 │
├──────────────────────────────────────────────────────────────────────┤
│                                                                      │
│   Developer              GitHub                 Stores               │
│   ─────────              ──────                 ──────               │
│                                                                      │
│   1. Merge PRs to main                                               │
│          │                                                           │
│          ▼                                                           │
│   2. git tag v1.2.0 ───► Tag Push ───► GitHub Actions                │
│      git push --tags                        │                        │
│                              ┌──────────────┴──────────────┐         │
│                              ▼                             ▼         │
│                         iOS Job                      Android Job     │
│                         (macOS)                      (Linux)         │
│                              │                             │         │
│                         Fastlane                     Fastlane        │
│                         ios release                  android release │
│                              │                             │         │
│                              ▼                             ▼         │
│                      TestFlight External           Open Testing      │
│                      (App Store Connect)           (Play Console)    │
│                                                                      │
│   3. Test on devices via TestFlight / Open Testing                   │
│                                                                      │
│   4. (Optional) Promote to Production manually via store consoles    │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

### Key Design Decisions

| Decision          | Choice              | Rationale                                                 |
| ----------------- | ------------------- | --------------------------------------------------------- |
| CI/CD Platform    | GitHub Actions      | Already using GitHub, native integration                  |
| Build/Deploy Tool | Fastlane            | Free, open-source, handles signing complexity             |
| Release Trigger   | Git tags (`v*.*.*`) | Clean audit trail, semantic versioning                    |
| Version Source    | `pubspec.yaml`      | Single source of truth, contains `major.minor.patch` only |
| Build Number      | Git commit count    | Automatic, monotonically increasing, reproducible         |
| Secrets Storage   | GitHub Secrets      | Secure, no external services required                     |

### Version Strategy

The version system separates semantic version from build number:

- **Semantic version** (`1.2.0`): Stored in `pubspec.yaml`, manually bumped for releases
- **Build number** (`347`): Automatically calculated from git commit count
- **Full version** (`1.2.0+347`): Assembled at build time

This means `pubspec.yaml` contains only the semantic version (no `+buildnumber` suffix). The build number is calculated in CI using `git rev-list --count HEAD`.

## Key Files

| File                                 | Purpose                                           |
| ------------------------------------ | ------------------------------------------------- |
| `.github/workflows/release_app.yaml` | GitHub Actions workflow triggered by version tags |
| `app/fastlane/Fastfile`              | Fastlane lanes for building and uploading         |
| `app/fastlane/Appfile`               | App identifiers and Apple team IDs                |
| `app/fastlane/Matchfile`             | iOS code signing configuration                    |
| `app/pubspec.yaml`                   | Version source (semantic version only)            |

## Technology Stack

### Fastlane

Fastlane is a free, open-source (MIT license) tool that automates mobile app deployment. It handles:

- iOS code signing complexity (certificates, provisioning profiles)
- Building IPAs and AABs
- Uploading to TestFlight and Play Store
- Certificate management via Match

| Aspect        | Details                      |
| ------------- | ---------------------------- |
| Cost          | Free                         |
| License       | MIT (open source)            |
| Repository    | github.com/fastlane/fastlane |
| Language      | Ruby                         |
| Maintained by | Google + community           |

### Match (iOS Code Signing)

Match is a Fastlane tool that stores iOS certificates and provisioning profiles in a private Git repository. This solves the "works on my machine" problem with iOS signing by giving CI access to the same certificates developers use.

## Web App Deployment

The Flutter Web bundle ships with the Go server container, not as a separate artifact. The pipeline is documented in **[`app/web/README.md`](../../app/web/README.md)** — see the "Pipeline" and "Deploy-time bundling (CI)" sections for the canonical reference.

In short: `.github/workflows/build_and_deploy_container.yaml` runs `npm run build:web:dev` or `:prod` against the matching `ripls-{env}` Secret Manager project before invoking `docker build`. The bundle is staged at `server/services/web/app_assets/`, the `Dockerfile`'s `COPY server ./server` picks it up, and the Go server embeds it via `//go:embed`. There is no separate web release flow — `dev.ripls.app` updates on every push to `main`, and `ripls.app` updates when a release-cut PR merges (via `release_finalize.yaml`).

---

## Part 1: Setup (One-Time)

This section describes the one-time setup required before the automation can work.

### 1.1 Prerequisites

Before starting, ensure you have:

- [ ] Apple Developer account ($99/year) with App Store Connect access
- [ ] Google Play Developer account ($25 one-time) with app created
- [ ] GitHub repository with admin access (for secrets)
- [ ] Ruby installed locally (for Fastlane setup)
- [ ] A private GitHub repository for Match certificates

### 1.2 Install Fastlane Locally

```bash
cd app

# Install bundler if not present
gem install bundler

# Install dependencies (uses existing Gemfile)
bundle install
```

### 1.3 Set Up iOS Code Signing with Match

Match stores your iOS certificates in a private Git repository.

#### 1.3.1 Create a Private Repository for Certificates

Create a new **private** GitHub repository for storing certificates (e.g., `your-org/ios-certificates`).

#### 1.3.2 Generate Certificates

Run this locally (requires Apple Developer account access):

```bash
cd app

# This will prompt for Apple ID credentials and create certificates
bundle exec fastlane match appstore

# You'll be prompted to set a passphrase - save this as MATCH_PASSWORD
```

This creates a distribution certificate and App Store provisioning profile, both encrypted and stored in your certificates repository.

#### 1.3.3 Create GitHub Personal Access Token for Match

1. Go to GitHub → Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Generate a new token with `repo` scope
3. Create the basic auth string: `echo -n "username:token" | base64`
4. Save this as `MATCH_GIT_BASIC_AUTH` secret

### 1.4 Set Up App Store Connect API Key

Create an API key for uploading to TestFlight:

1. Go to App Store Connect → Users and Access → Integrations → App Store Connect API
2. Click "Generate API Key"
3. Name: "GitHub Actions CI"
4. Access: "App Manager" role
5. Download the `.p8` file (you can only download it once!)
6. Note the Key ID and Issuer ID

Convert the key to base64:

```bash
base64 -i AuthKey_XXXXXXXXXX.p8 | tr -d '\n'
```

### 1.5 Set Up Google Play Service Account

Create a service account for uploading to Play Store:

1. Go to Google Play Console → Setup → API access
2. Click "Create new service account"
3. Follow the link to Google Cloud Console
4. Create service account with name like "github-actions-deploy"
5. Grant role: "Service Account User"
6. Create JSON key and download it
7. Back in Play Console, grant the service account "Release manager" permission

### 1.6 Configure Android Signing

#### 1.6.1 Create Release Keystore (if not already done)

```bash
keytool -genkey -v -keystore release.keystore -alias ripls \
  -keyalg RSA -keysize 2048 -validity 10000
```

Save this keystore securely - you'll need it for all future releases.

#### 1.6.2 Encode Keystore for CI

```bash
base64 -i release.keystore | tr -d '\n'
```

Save this as the `ANDROID_KEYSTORE_BASE64` secret.

### 1.7 Configure GitHub Secrets

Go to your GitHub repository → Settings → Secrets and variables → Actions → New repository secret

#### iOS Secrets

| Secret Name                     | Description                                | How to Get                                     |
| ------------------------------- | ------------------------------------------ | ---------------------------------------------- |
| `MATCH_PASSWORD`                | Encryption password for Match certificates | You created this during `fastlane match` setup |
| `MATCH_GIT_URL`                 | SSH URL of certificates repo               | `git@github.com:your-org/ios-certificates.git` |
| `MATCH_GIT_BASIC_AUTH`          | Base64-encoded `username:PAT`              | `echo -n "username:token" \| base64`           |
| `APP_STORE_CONNECT_KEY_ID`      | App Store Connect API Key ID               | From App Store Connect (e.g., `XXXXXXXXXX`)    |
| `APP_STORE_CONNECT_ISSUER_ID`   | App Store Connect Issuer ID                | From App Store Connect (UUID format)           |
| `APP_STORE_CONNECT_KEY_CONTENT` | Base64-encoded .p8 key content             | `base64 -i AuthKey_XXX.p8 \| tr -d '\n'`       |

#### Android Secrets

| Secret Name                       | Description                              | How to Get                                             |
| --------------------------------- | ---------------------------------------- | ------------------------------------------------------ |
| `PLAY_STORE_SERVICE_ACCOUNT_JSON` | Full JSON content of service account key | Copy entire contents of downloaded JSON file           |
| `ANDROID_KEYSTORE_BASE64`         | Base64-encoded release.keystore          | `base64 -i release.keystore \| tr -d '\n'`             |
| `ANDROID_KEYSTORE_PASSWORD`       | Keystore password                        | Password you set when creating keystore                |
| `ANDROID_KEY_ALIAS`               | Key alias in keystore                    | `ripls` (or whatever you used)                         |
| `ANDROID_KEY_PASSWORD`            | Key password                             | Password for the key (often same as keystore password) |

### 1.8 Configure TestFlight Public Link (iOS)

To allow anyone with a link to join TestFlight testing:

1. Go to App Store Connect → Your App → TestFlight
2. Click on your external testing group (e.g., "Ripls External Alpha")
3. Enable "Public Link" in the group settings
4. Copy the public link to share with testers

The release workflow automatically distributes builds to this group. Anyone with the public link can join without being manually added.

---

## Part 2: Day-to-Day Release Process

Once setup is complete, releasing is straightforward.

### 2.1 Normal Development Flow

```bash
# Regular development - no version changes needed
git checkout -b feature/my-new-feature
# ... make changes ...
git add -A
git commit -m "Add my new feature"
git push origin feature/my-new-feature
# Create PR, get review, merge to main
```

### 2.2 Cutting a Release

#### Step 1: Decide on Version Number

Follow semantic versioning:

- **Patch** (1.0.0 → 1.0.1): Bug fixes only
- **Minor** (1.0.0 → 1.1.0): New features, backward compatible
- **Major** (1.0.0 → 2.0.0): Breaking changes

#### Step 2: Update Version (if needed)

If bumping version, update `pubspec.yaml`:

```bash
git checkout main
git pull origin main

# Edit app/pubspec.yaml - change version field
# Example: version: 1.0.0 → version: 1.1.0

git add app/pubspec.yaml
git commit -m "Bump version to 1.1.0"
git push origin main
```

#### Step 3: Create and Push Tag

```bash
# Create annotated tag
git tag -a v1.1.0 -m "Release 1.1.0: Add gear categories feature"

# Push tag to trigger release
git push origin v1.1.0
```

#### Step 4: Monitor the Build

1. Go to GitHub → Actions → Release App workflow
2. Watch both iOS and Android jobs
3. Check for any failures

#### Step 5: Verify Uploads

- **iOS**: Check TestFlight in App Store Connect - build should appear in external testing group
- **Android**: Check Open Testing (beta) track in Play Console

### 2.3 Release Checklist

- [ ] All PRs merged to main
- [ ] Version bumped in pubspec.yaml (if needed)
- [ ] Changes committed and pushed
- [ ] Tag created: `git tag -a v1.2.3 -m "Release message"`
- [ ] Tag pushed: `git push origin v1.2.3`
- [ ] GitHub Actions workflow completed successfully
- [ ] Build appears in TestFlight external testing / Play Console Open Testing
