---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: Apple App Store launch checklist and status log — organization account setup, TestFlight beta testing, store listing, age rating, and production submission for the iOS app.
  triggers: [app-store, testflight, apple, ios-release, app-review, store-listing]
  lens: [infra]
  skills: [issue, audit]
  domain: release
freshness:
  verified_commit: "9fdddc4d0"
  verified_on: "2026-06-09"
---
# Apple App Store Release Guide / Status Log

## Overview

This guide outlines the complete process for launching an app on the Apple App Store as an organization, from initial setup through TestFlight beta testing to production release.

### About TestFlight

Apple's official beta testing platform lets you distribute pre-release versions to testers before final App Store submission:

- Upload builds through App Store Connect (same place as final submissions)
- Distribute to up to **10,000 external testers** via email or public link
- Up to **100 internal testers** (your team members in App Store Connect)
- Testers download the TestFlight app, then access your beta app
- Each build is available for **90 days**
- Get crash reports and feedback from testers
- No need to collect device IDs
- Built-in feedback mechanism

---

## Release Checklist

### 1. Account Setup & Verification (Organization) ✅

**D-U-N-S Number:**
- [x] Go to [developer.apple.com/enroll/duns-lookup/](https://developer.apple.com/enroll/duns-lookup/)
- [x] Complete the D-U-N-S lookup/request form
- [x] Provide legal business name (exact match to registration)
- [x] Provide business headquarters address
- [x] Provide contact information and company details
- [x] Submit request
- [x] Wait for Dun & Bradstreet confirmation email
- [x] Answer verification call if D&B contacts you
- [x] Save D-U-N-S number when received

**Apple Developer Program Enrollment:**
- [x] Go to [developer.apple.com/programs/enroll/](https://developer.apple.com/programs/enroll/)
- [x] Sign in with Apple ID (or create one)
- [x] Select "Enroll as an Organization"
- [x] Enter D-U-N-S number
- [x] Enter legal entity information (must match D&B records exactly)
- [x] Enter organization details (address, phone, website)
- [x] Provide legal authority contact information
- [x] Review and agree to Apple Developer Program License Agreement
- [x] Pay $99 annual fee
- [x] Wait for Apple's verification process
- [x] Respond to any verification requests from Apple
- [x] Receive enrollment confirmation email

**App Store Connect Setup:**
- [x] Log into [appstoreconnect.apple.com](https://appstoreconnect.apple.com)
- [x] Complete organization profile
- [ ] Set up banking information (for paid apps - can do later)
- [ ] Set up tax forms (can do later)
- [x] Add team members to App Store Connect if needed

---

### 2. App Development & Readiness ✅

- [x] Complete app development to testable state
- [x] Set unique Bundle Identifier in Xcode
- [x] Set version and build numbers
- [x] Add app icon (all required sizes)
- [x] Test thoroughly on simulator and device
- [x] Fix critical bugs and crashes
- [x] Ensure compliance with Apple guidelines
- [x] Create App Store Connect app record
- [x] Archive and upload first build to App Store Connect

---

### 3. Testing & Pre-Launch (TestFlight)

**Complete Beta App Information:**
- [x] Go to TestFlight tab in App Store Connect
- [x] Fill out "Test Information":
- [x] Fill out "Test Details":

**Set Up Internal Testing (Optional):**
- [x] Go to TestFlight > Internal Testing
- [x] Create testing group or use default
- [x] Add internal testers (must be App Store Connect users)
- [x] Enable automatic distribution (optional)
- [x] Internal testers receive immediate access

**Set Up External Testing:**
- [x] Go to TestFlight > External Testing
- [x] Click "+" to create external testing group
- [x] Name the group
- [x] Add your build to the group
- [x] Choose testing method:
  - **Email invites:** Add tester emails
  - **Public link:** Enable and copy link
- [x] Submit for Beta App Review
- [ ] Wait for Beta App Review approval (24-48 hours)

**Distribute to Testers:**
- [ ] Once approved, send invitations or share public link
- [ ] Provide testers with instructions:
  - Download TestFlight app from App Store
  - Open invitation email or public link
  - Accept invitation
  - Install and test your app
  - Submit feedback through TestFlight

**Iterate and Update:**
- [ ] Monitor crash reports in App Store Connect
- [ ] Read tester feedback
- [ ] Fix bugs and add features
- [ ] Upload new builds through Xcode
- [ ] Add new builds to testing groups
- [ ] New builds auto-approve after first approval

---

### 4. Production Release Preparation

**Complete Store Listing:**
- [ ] Go to App Store Connect > My Apps > [Your App]
- [ ] Fill out App Information:
  - App Name (30 characters max)
  - Subtitle (30 characters max, optional)
  - Primary Language
  - Category (Primary and Secondary)
  - Content Rights (does it contain third-party content?)
- [ ] Add App Privacy details:
  - Complete privacy questionnaire
  - Link to Privacy Policy URL
  - Describe data collection practices
- [ ] Complete App Store listing for each language:
  - Promotional Text (170 characters, updateable without review)
  - Description (4000 characters)
  - Keywords (100 characters, comma-separated)
  - Support URL (required)
  - Marketing URL (optional)
- [ ] Upload App Store screenshots:
  - Required sizes for iPhone and iPad
  - 6.5" display (iPhone 14 Pro Max, etc.)
  - 5.5" display (iPhone 8 Plus, etc.)
  - 12.9" display (iPad Pro, etc.)
  - Up to 10 screenshots per device size
- [ ] Create App Preview videos (optional but recommended):
  - Up to 3 videos per device size
  - 15-30 seconds each
- [ ] Add app icon (1024x1024 pixels, no transparency)

**Set Up Pricing & Availability:**
- [ ] Choose price tier (or Free)
- [ ] Select available countries/regions
- [ ] Set pre-order availability (optional)
- [ ] Configure app distribution settings

**Complete Age Rating:**
- [ ] Fill out age rating questionnaire
- [ ] Review assigned age rating
- [ ] Adjust content if needed to achieve desired rating

**App Review Information:**
- [ ] Provide contact information for app review team
- [ ] Add demo account credentials (if app requires sign-in)
- [ ] Provide notes for reviewers (special instructions)
- [ ] Add attachments if needed (documentation, etc.)

---

### 5. Production Release & Post-Launch

**Submit for Review:**
- [ ] Select build for production release
- [ ] Review all information for accuracy
- [ ] Choose release method:
  - **Manual release:** You control when app goes live after approval
  - **Automatic release:** App goes live immediately after approval
  - **Scheduled release:** App goes live at specified date/time after approval
- [ ] Submit app for review
- [ ] Receive confirmation email

**Monitor Review Status:**
- [ ] Check status in App Store Connect ("Waiting for Review" → "In Review")
- [ ] Respond to any questions from App Review team
- [ ] Address rejection reasons if app is rejected:
  - Review rejection details
  - Make necessary changes
  - Upload new build if needed
  - Resubmit for review

**Launch Day:**
- [ ] Receive approval notification
- [ ] App goes live (automatically or manually release)
- [ ] Verify app appears correctly in App Store
- [ ] Test app download and installation
- [ ] Share App Store link with users

**Post-Launch Monitoring:**
- [ ] Monitor crash reports in App Store Connect
- [ ] Respond to user reviews (reply to feedback)
- [ ] Track app performance metrics:
  - Downloads and installations
  - User retention
  - Crash rates
  - App Store ranking
- [ ] Monitor customer support requests
- [ ] Address critical bugs with emergency updates

**Updates & Maintenance:**
- [ ] Plan regular updates (bug fixes, features, improvements)
- [ ] Upload new builds following same process
- [ ] Update What's New section for each version
- [ ] Submit updates for review
- [ ] New versions auto-approve faster after initial approval
- [ ] Monitor analytics to inform feature development

---

## Future Automation Improvements

See `docs/release/automation.md` for the implemented build/upload automation.

**Build & Upload Automation:**
- [ ] Set up App Store Connect API key (App Manager role)
- [ ] Store API credentials securely (~/.apple/)
- [ ] Create `build:app:ios:dev:upload` npm script
- [ ] Create `build:app:ios:prod:upload` npm script
- [ ] Test automatic upload with dev build

**CI/CD Pipeline:**
- [ ] Set up GitHub Actions workflow for iOS builds
- [ ] Configure automatic builds on git tags
- [ ] Auto-upload to TestFlight on tag push
- [ ] Auto-assign builds to testing groups via API

**Release Management:**
- [ ] Create version bump automation script
- [ ] Automatically generate release notes from commits
- [ ] Set up automatic TestFlight submission for beta review
