---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Google Play Store launch checklist and status log — organization account verification, Android signing/keystore setup, testing tracks (internal/closed/open), store listing, data safety, and production rollout.
  triggers: [play-store, google-play, android-release, open-testing, keystore, data-safety, store-listing]
  lens: [infra]
  skills: [issue, audit]
  domain: release
freshness:
  verified_commit: "ad3b0cdb0"
  verified_on: "2026-06-09"
---
# Google Play Store Release Guide / Status Log

## Overview

This guide outlines the complete process for launching an app on the Google Play Store as an organization, from initial account setup through testing to production release.

### About Google Play Testing Tracks

Google Play offers multiple testing tracks to distribute pre-release versions:

- **Internal testing:** Up to 100 testers, immediate access, no review required
- **Closed testing:** Invite-only testing with up to 2,000 testers per track
- **Open testing:** Public testing available to anyone, no invitation needed
- Testers access beta versions directly through Google Play Store
- Collect feedback and crash reports before production launch
- Promote builds from testing tracks to production when ready

---

## Release Checklist

### 1. Account Setup & Verification (Organization) ✅

**Obtain D-U-N-S Number:**

- [x] Obtain D-U-N-S number for your organization

**Google Play Console Registration:**

- [x] Create/log in to Google Account
- [x] Go to [play.google.com/console](https://play.google.com/console)
- [x] Register for Google Play Console
- [x] Pay one-time $25 registration fee
- [x] Select "Organization Account" type
- [x] Create/link Google Payments Profile

**Organization Verification:**

- [x] Submit D-U-N-S number and organization documents
- [x] Submit account owner's ID
- [x] Wait for Google verification review
- [x] Verify organization website via Google Search Console

---

### 2. App Development & Readiness ✅

**Complete App Development:**

- [x] Complete app development to production-ready state
- [x] Test thoroughly on multiple devices and Android versions
- [x] Fix critical bugs and crashes
- [x] Ensure compliance with Google Play policies

**Android Signing Setup:**

- [x] Generate upload keystore (`~/upload-keystore.jks`)
- [x] Create `app/android/key.properties` with signing credentials
- [x] Configure `app/android/app/build.gradle.kts` for release signing
- [x] Add keystore files to `.gitignore`
- [x] Backup keystore and passwords securely

**Build Configuration:**

- [x] Update npm scripts to build App Bundle (`.aab`) format
- [x] Build signed App Bundle: `npm run build:app:android:dev:release`
- [x] Output location: `app/build/app/outputs/bundle/release/app-release.aab`

**Prepare Store Assets:**

- [x] Create app logo/icon (512x512 pixels) - `app/assets/icon/icon_android_512.png`
- [x] Create feature graphic (1024x500 pixels) - `app/assets/feature_graphic.png`
- [x] Capture app screenshots:
  - Phone screenshots (minimum 2, up to 8)
  - 7-inch tablet screenshots (optional)
  - 10-inch tablet screenshots (optional)
- [ ] Create promo video (optional but recommended)
  - YouTube video showcasing your app

**Write Store Listing Content:**

- [x] Write short description (80 characters max)
- [x] Write full description (4000 characters max)
- [x] Research and select keywords for ASO
- [x] Create and host privacy policy publicly (required for most apps)
- [x] Prepare app title (30 characters max)

**Account Configuration:**

- [ ] Set up monetization/billing (if applicable)
- [ ] Complete tax information
- [ ] Set up banking information (if selling paid app or in-app purchases)

---

### 3. Testing & Pre-Launch

**Create App in Play Console:**

- [ ] Go to Google Play Console > All apps > Create app
- [ ] Enter app name
- [ ] Select default language
- [ ] Choose app or game
- [ ] Select free or paid
- [ ] Accept declarations
- [ ] Create app

**Upload First Build:**

- [x] Create app bundle (.aab) or APK
- [x] Go to Release > Testing > Internal testing
- [x] Create new release
- [x] Upload app bundle or APK
- [x] Add release notes
- [x] Save and review release

**Complete Store Listing:**

- [x] Go to Grow > Store presence > Main store listing
- [x] Upload app icon and feature graphic
- [x] Add screenshots for required device types
- [x] Enter short description
- [x] Enter full description
- [x] Add promotional graphics (optional)
- [x] Select app category
- [x] Provide contact details (email, phone, website)
- [x] Add privacy policy URL

**Content Rating:**

- [x] Go to Policy > App content > Content ratings
- [x] Complete content rating questionnaire
- [x] Submit for rating
- [x] Receive ratings for different regions

**Target Audience & Content:**

- [x] Go to Policy > App content > Target audience
- [x] Select target age groups
- [x] Complete store presence questionnaire
- [x] Declare ads (if app contains ads)
- [x] Complete privacy policy
- [x] Set app access settings
- [x] Complete data safety form:
  - [x] Created account deletion page at `website/content/account-deletion.html`
  - [x] Published at `https://ripls.org/account-deletion.html`
  - [x] Describe data collection practices
  - [x] Explain data sharing policies
  - [x] Detail security practices

**Internal Testing:**

- [x] Set up internal testing track
- [x] Add build to internal testing (`app-release.aab`)
- [x] Create internal testing release
- [ ] Add internal testers (up to 100)
- [ ] Internal testers receive access immediately
- [ ] Collect initial feedback

**Closed Testing (Optional):**

- [ ] Go to Release > Testing > Closed testing
- [ ] Create closed testing track
- [ ] Add closed testing group(s)
- [ ] Add testers via email list or Google Group
- [ ] Promote build from internal to closed testing
- [ ] Share opt-in URL with testers
- [ ] Collect and address feedback from testers

**Open Testing:**

- [x] Go to Release > Testing > Open testing
- [x] Set up open testing track
- [x] Set country availability for open testing
- [x] Promote build to open testing
- [x] Submit for review
- [ ] Wait for Google review approval (typically hours to a few days)
- [ ] Anyone can opt in and test once approved
- [ ] Collect feedback from broader audience

**Pre-Launch Activities:**

- [x] Perform final QA and bug fixes
- [x] Review all store listing information
- [x] Review all policy requirements
- [x] Ensure all required sections are complete

---

### 4. Production Release & Post-Launch

**Submit for Production:**

- [ ] Go to Release > Production
- [ ] Create new production release
- [ ] Select build to promote to production
- [ ] Choose release type:
  - **Managed release:** Control rollout percentage and timing
  - **Staged rollout:** Gradually increase user percentage
  - **Full rollout:** Release to all users immediately
- [ ] Add release notes for "What's new"
- [ ] Review release summary
- [ ] Submit for review

**Monitor Review Status:**

- [ ] Check review status in Play Console
- [ ] Review typically takes several hours to a few days
- [ ] Respond to any questions from Google review team
- [ ] Address rejection reasons if app is rejected:
  - Review policy violation details
  - Make necessary changes
  - Upload new build if needed
  - Resubmit for review

**Launch Day:**

- [ ] Receive approval notification
- [ ] App goes live on Google Play Store
- [ ] Verify app appears correctly in store
- [ ] Test app download and installation
- [ ] Share Play Store link with users

**Post-Launch Monitoring:**

- [ ] Monitor crash reports in Play Console
- [ ] Use Android vitals to track app performance:
  - Crash rate
  - ANR (App Not Responding) rate
  - Wake locks
  - Background processes
- [ ] Respond to user reviews (reply to feedback)
- [ ] Track app performance metrics:
  - Installations and uninstalls
  - User retention
  - User acquisition sources
  - Store listing performance
- [ ] Monitor customer support requests
- [ ] Address critical bugs with emergency updates

**Updates & Maintenance:**

- [ ] Plan regular updates (bug fixes, features, improvements)
- [ ] Upload new builds following same process
- [ ] Update "What's new" section for each version
- [ ] Use staged rollouts for major updates
- [ ] Monitor analytics to inform feature development
- [ ] Keep store listing fresh (update screenshots, description)
- [ ] Maintain compliance with evolving Play Store policies

---

## Future Automation Improvements

See `docs/release/automation.md` for the implemented build/upload automation.

**Build & Upload Automation:**

- [ ] Create Google Cloud service account for Play Console
- [ ] Link service account to Play Console with proper permissions
- [ ] Download and secure JSON key file (~/.google/)
- [ ] Set up fastlane or gradle-play-publisher for uploads
- [ ] Create automated upload scripts for internal/open testing
- [ ] Test automatic upload to internal testing track

**CI/CD Pipeline:**

- [ ] Set up GitHub Actions workflow for Android builds
- [ ] Configure automatic builds on git tags
- [ ] Auto-upload to Play Console on tag push
- [ ] Auto-promote builds between testing tracks

**Release Management:**

- [ ] Create version bump automation script
- [ ] Automatically generate release notes from commits
- [ ] Set up automatic track promotion (internal → closed → open → production)
- [ ] Configure staged rollout automation for production releases
