---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: The in-app feedback feature — FeedbackSheet/ViewModel/Repository/Service MVVM layers and the server pipeline that creates labeled GitHub issues via GitHub App auth, with device context and screenshots.
  globs: [server/services/feedback/**, server/github/**, app/lib/presentation/widgets/feedback/**, app/lib/data/repositories/feedback_repository.dart, app/lib/services/feedback_service.dart, app/lib/presentation/viewmodels/feedback_sheet_view_model.dart]
  triggers: [feedback, bug-report, feature-request, github-issue, github-app, feedback-sheet, screenshots]
  lens: [client, server, architecture]
  domain: feedback
freshness:
  verified_commit: "5c2d3e59c"
  verified_on: "2026-07-07"
---
# User Feedback System

## Overview

The Ripls User Feedback System enables users to submit bug reports and feature requests directly from the app, creating GitHub issues in a configured repository. The system collects user input, device context, and optional screenshots, then formats and submits everything as a properly-labeled GitHub issue using GitHub App authentication. It uses a three-layer architecture: FeedbackSheet (UI), FeedbackSheetNotifier (ViewModel), FeedbackRepository (data access), and FeedbackService (RPC), following Ripls' standard MVVM pattern.

## Architecture Principles

**Bottom Sheet Modal:** Feedback is presented as a modal bottom sheet (`FeedbackSheet`) accessible from the `PlusButtonModal` (the `+` action button). This keeps feedback discoverable without cluttering the navigation hierarchy.

**Server-Side GitHub Integration:** GitHub issue creation happens on the server using GitHub App authentication with auto-generated installation tokens, ensuring security and control over issue formatting and labeling without manual token rotation.

**Device Context Collection:** The app automatically collects app version, platform, OS version, device model, build number, and environment to help diagnose issues.

**ViewModel Pattern:** `FeedbackSheetNotifier` manages all form state using Riverpod's `NotifierProvider.autoDispose`, so state is automatically reset each time the sheet is opened and closed.

**Inline Error Display:** Submission errors appear as an inline banner in the form rather than a toast, ensuring visibility within the bottom sheet context.

## Feedback Submission Flow

### User Submits Bug Report

**Example: User reports app crash**

```
User taps + button → PlusButtonModal opens
    ↓
User taps "Something wrong? Share feedback"
    ↓
PlusButtonModal closes, FeedbackSheet opens
    ↓
User selects feedback type pill (Bug / Feature / Other)
    ↓
User fills in Title and Description
    ↓
User optionally adds screenshots (up to 3)
    ↓
User taps "Submit Feedback"
    ↓
FeedbackSheetNotifier.submitFeedback()
    ├─ canSubmit check (title + description non-empty, not already submitting)
    └─ Valid → Continue
        ↓
        Collect device context (app version, platform, OS, device model, build, environment)
        ↓
        Read screenshot bytes + detect MIME types
        ↓
        Repository → Service → Server RPC
        ↓
        Server validates, attaches screenshots, formats issue body, creates GitHub issue
        ↓
        Success → state.isSubmitted = true → success state shown
        Failure → state.errorMessage set → inline error banner shown
```

**Key Points:**
- No navigation required — everything happens in the bottom sheet
- Device context collected automatically on submit
- Screenshots uploaded as binary data with correct MIME type and extension
- GitHub issue formatted with markdown sections and labeled appropriately
- Success state replaces the form in-place with a thank-you message

---

## Four-Layer Architecture

### Layer 1: FeedbackSheet (UI)

**Purpose:** Bottom sheet modal presenting the feedback form

**File:** [app/lib/presentation/widgets/feedback/feedback_sheet.dart](../app/lib/presentation/widgets/feedback/feedback_sheet.dart)

**Entry Point:** `FeedbackSheet.show(context)` — static method to open the modal

**Form Fields:**
- Feedback Type: three pill-style buttons (🐛 Bug / ✨ Feature / 💬 Other)
- Title (required, 0-100 chars with live counter)
- Description (required, 0-5000 chars with live counter)
- Screenshots: thumbnail grid with remove buttons, "Add Screenshots" button (optional, up to 3)

**UI States:**
- **Form state:** type selector + text fields + screenshot section + submit button
- **Submitting:** submit button replaced with `CircularProgressIndicator`
- **Error:** inline `_buildErrorBanner` appears above submit button when `state.errorMessage != null`
- **Success:** entire form replaced with thank-you message and Done button

**Key Behaviors:**
- `isScrollControlled: true` so the sheet grows with the keyboard
- Pill selector uses `AppColors.transferCoralBackground` for selected state
- Screenshot thumbnails show 80×80 previews with red ×-badge remove buttons
- Submit button enabled only when `state.canSubmit` is true

### Layer 2: FeedbackSheetNotifier (ViewModel)

**Purpose:** Manages all feedback form state using Riverpod

**File:** [app/lib/presentation/viewmodels/feedback_sheet_view_model.dart](../app/lib/presentation/viewmodels/feedback_sheet_view_model.dart)

**State:** `FeedbackSheetState` (Freezed sealed class)

| Field | Type | Default |
|---|---|---|
| `feedbackType` | `FeedbackType` | `FEEDBACK_TYPE_BUG_REPORT` |
| `title` | `String` | `''` |
| `description` | `String` | `''` |
| `screenshots` | `List<XFile>` | `[]` |
| `isSubmitting` | `bool` | `false` |
| `isSubmitted` | `bool` | `false` |
| `errorMessage` | `String?` | `null` |

**Computed:** `canSubmit` — `true` when title and description are non-empty (trimmed) and not currently submitting.

**Key Methods:**
- `setFeedbackType(FeedbackType)` — updates selected type
- `setTitle(String)` — updates title, clamped to 100 chars
- `setDescription(String)` — updates description, clamped to 5000 chars
- `addScreenshot(XFile file)` — appends file, max 3 screenshots
- `removeScreenshot(int index)` — removes screenshot at index
- `submitFeedback()` — collects device context, reads screenshot bytes, calls repository
- `reset()` — resets all fields to defaults

**Provider:** `feedbackSheetProvider = NotifierProvider.autoDispose<FeedbackSheetNotifier, FeedbackSheetState>`

### Layer 3: FeedbackRepository (Data Access)

**Purpose:** Data access layer following repository pattern

**File:** [app/lib/data/repositories/feedback_repository.dart](../app/lib/data/repositories/feedback_repository.dart)

**Key Methods:**
- `submitFeedback(SubmitFeedbackRequest)` — passes request through to the RPC service

### Layer 4: FeedbackService (RPC Client)

**Purpose:** Thin wrapper around Connect RPC call for feedback submission

**File:** [app/lib/services/feedback_service.dart](../app/lib/services/feedback_service.dart)

**Key Methods:**
- `submitFeedback(SubmitFeedbackRequest)` — submits feedback to server via Connect RPC

---

## Server-Side Implementation

### GitHub Issue Creation Pipeline

**Entry Point:** `FeedbackService.SubmitFeedback()` in [server/services/feedback/service.go](../server/services/feedback/service.go)

**Steps:**

1. **Validation**

2. **Format Issue Title**

3. **Format Issue Body**

4. **Determine Labels**

5. **Create GitHub Issue**
   - Use custom `server/github` package with GitHub App authentication
   - Auto-generate short-lived installation tokens (no manual rotation needed)
   - Post to configured repository (owner/name)
   - Attach screenshots as GitHub issue comments or embedded links
   - Return issue URL and number

6. **Return Response**
   - `SubmitFeedbackResponse{issueUrl, issueNumber, message}`

**Implementation:** See [service.go](../server/services/feedback/service.go) for complete implementation.

### Screenshot URL Durability (#2549)

Screenshot URLs embedded in the issue body are GCS signed URLs valid for
`feedbackURLExpiry` (365 days). By default GCS signs via the runtime service
account's IAM `signBlob`, whose Google-managed key rotates every ~1-2 weeks —
so those URLs stop working long before the stated expiry. `SubmitFeedback`
calls `BucketStorage.GetDurableSignedURL`, which signs locally with a
dedicated, user-managed key (`--feedback-signer-key`/`-file`, optional) to
avoid that rotation dependency. If the signer isn't configured, it falls back
to the short-lived `signBlob` URL. [secrets.md](secrets.md) covers how the
credential reaches the server; its rotation cadence is a property of the
deployment, not of this code.

### Configuration

**Command-Line Flags:**

- `--github-app-id` - GitHub App ID for the feedback bot (required)
- `--github-installation-id` - GitHub App Installation ID for the target repository (required)
- `--github-app-private-key` - GitHub App private key, base64-encoded PEM (required)
- `--github-repo-owner` - Repository owner (required; no default)
- `--github-repo-name` - Repository name (required; no default)
- `--feedback-signer-key` (or `--feedback-signer-key-file`) - Service-account JSON key for durable screenshot URL signing (optional; see [Screenshot URL Durability](#screenshot-url-durability-2549) above)

**Why base64-encoded?**

PEM private keys are multi-line and contain characters that don't survive shell argument passing. Base64-encoding produces a single continuous line that can safely be passed as a flag value. The server decodes it on startup and hands the raw PEM bytes to the GitHub App client.

**Note:** GitHub Apps provide better security:
- Short-lived installation tokens (auto-generated, 1-hour validity)
- No manual token rotation needed
- Organization-owned (not tied to personal account)
- Higher rate limits (15,000 vs 5,000 requests/hour)
- Issues created as `ripls-bot[bot]` instead of personal account

**Fallback Behavior:**

If GitHub App credentials are not configured, the service initializes with a nil client and logs a warning. The server will return an error when users attempt to submit feedback. The app displays this error as an inline banner in the feedback sheet.

**Example (Production):**

```bash
# Encode the PEM (single continuous line, no wrapping)
B64=$(base64 -w0 /path/to/private-key.pem)

# Run server with flags
go run ./server \
  --github-app-id=$GITHUB_APP_ID \
  --github-installation-id=$GITHUB_INSTALLATION_ID \
  --github-app-private-key="$B64" \
  --github-repo-owner=$GITHUB_REPO_OWNER \
  --github-repo-name=$GITHUB_REPO_NAME
```

**Example (Development):**

For local development, you typically don't need GitHub credentials since feedback creation isn't required:

```bash
npm run start:server  # GitHub App not configured - will log warning
```

---

## Feedback Types

Every feedback issue gets the single label `["user-feedback"]` (`getIssueLabels`
in [service.go](../server/services/feedback/service.go)). Bug-vs-feature
classification is carried on the native GitHub **Issue Type** instead of a label:
`getIssueType` maps `FEEDBACK_TYPE_BUG_REPORT` → "Bug" and
`FEEDBACK_TYPE_FEATURE_REQUEST` → "Feature"; "Other" feedback is left untyped for
triage to classify later.

### Bug Report

**FeedbackType:** `FEEDBACK_TYPE_BUG_REPORT`

**Use Case:** App crashes, broken features, unexpected behavior

**GitHub Issue Type:** `Bug`


### Feature Request

**FeedbackType:** `FEEDBACK_TYPE_FEATURE_REQUEST`

**Use Case:** New features, improvements, enhancements

**GitHub Issue Type:** `Feature`


### Other Feedback

**FeedbackType:** `FEEDBACK_TYPE_UNSPECIFIED`

**Use Case:** General comments, questions, appreciation

**GitHub Issue Type:** (untyped — assigned during triage)

---

## Testing

### Server-Side Tests

**Feedback Service Tests** ([server/services/feedback/service_test.go](../server/services/feedback/service_test.go)):
- `TestValidateFeedbackRequest` - test cases covering all validation rules
- `TestFormatIssueTitle` - test cases covering title formatting and prefixes
- `TestGetIssueLabels` - test cases covering label assignment
- `TestFormatIssueBody` - test cases covering body formatting

### Client-Side Tests

**Repository Tests** ([app/test/data/repositories/feedback_repository_test.dart](../app/test/data/repositories/feedback_repository_test.dart)):
- Successful submission, exception propagation
- Uses Mockito mocks for the service layer

**ViewModel Tests** ([app/test/presentation/viewmodels/feedback_sheet_view_model_test.dart](../app/test/presentation/viewmodels/feedback_sheet_view_model_test.dart)):
- Initial state defaults
- `setFeedbackType` — all three types
- `setTitle` — clamping at 100 characters
- `setDescription` — clamping at 5000 characters
- `canSubmit` — empty fields, whitespace-only fields, submitting state
- `addScreenshot` / `removeScreenshot` — up to 3 screenshots, ignores 4th
- `submitFeedback` — success, failure (error message), isSubmitting during call, disposal safety
- `reset` — restores all fields to defaults

**Widget Tests** ([app/test/presentation/widgets/feedback/feedback_sheet_test.dart](../app/test/presentation/widgets/feedback/feedback_sheet_test.dart)):
- Header text, three type pills, two text fields
- Submit button disabled/enabled based on canSubmit
- Character counters (e.g., `5/100`, `16/5000`)
- Type selector pill tap updates state
- Loading indicator shown while submitting
- Success state (thank-you message, Done button, form hidden)
- Screenshot button label changes after adding screenshots
- Error banner visible when errorMessage is set

**Testing Pattern:** Use `@GenerateMocks` with Mockito, `PackageInfo.setMockInitialValues` for platform plugin mocking, run `flutter pub run build_runner build` to generate mocks.

---

# APPENDIX

## Resources

### Key Internal Files

**Server-Side:**
- [service.go](../server/services/feedback/service.go) - Feedback RPC service implementation
- [github_app.go](../server/services/feedback/github_app.go) - Constructs the service from GitHub App credentials (`NewFromGitHubApp`), degrading to a nil client when unconfigured
- [service_test.go](../server/services/feedback/service_test.go) - Feedback service tests

**Client-Side:**
- [feedback_service.dart](../app/lib/services/feedback_service.dart) - RPC client wrapper
- [feedback_repository.dart](../app/lib/data/repositories/feedback_repository.dart) - Repository layer
- [feedback_sheet.dart](../app/lib/presentation/widgets/feedback/feedback_sheet.dart) - Bottom sheet UI widget
- [feedback_sheet_view_model.dart](../app/lib/presentation/viewmodels/feedback_sheet_view_model.dart) - ViewModel (Notifier + State)
- [plus_button_modal.dart](../app/lib/presentation/widgets/plus_button_modal.dart) - Entry point (feedback link at bottom)

**Proto Definitions:**
- [feedback_service.proto](../proto/ripls/api/feedback_service.proto) - Service definition and message types (FeedbackType, DeviceContext, FeedbackScreenshot, SubmitFeedback RPC)

**Tests:**
- [feedback_repository_test.dart](../app/test/data/repositories/feedback_repository_test.dart) - Repository tests
- [feedback_sheet_view_model_test.dart](../app/test/presentation/viewmodels/feedback_sheet_view_model_test.dart) - ViewModel tests
- [feedback_sheet_test.dart](../app/test/presentation/widgets/feedback/feedback_sheet_test.dart) - Widget tests
- [plus_button_modal_test.dart](../app/test/presentation/widgets/plus_button_modal_test.dart) - Includes feedback link test
- [service_test.go](../server/services/feedback/service_test.go) - Server tests

**Documentation:**
- [environment_variables.md](../docs/environment_variables.md) - Configuration documentation

---

## Configuration Best Practices

### Development Environment

For local development, GitHub App credentials are typically not needed. The server will start with a warning that feedback submission is disabled. Submitting feedback in dev will show an inline error in the sheet.

If you need to test feedback creation locally:

1. Contact your team admin to obtain GitHub App credentials (App ID, Installation ID, Private Key)
2. Base64-encode the PEM and pass it as a flag to your local server command:
   ```bash
   B64=$(base64 -w0 /path/to/private-key.pem)
   go run ./server \
     --github-app-id=$GITHUB_APP_ID \
     --github-installation-id=$GITHUB_INSTALLATION_ID \
     --github-app-private-key="$B64" \
     --github-repo-owner=$GITHUB_REPO_OWNER \
     --github-repo-name=$GITHUB_REPO_NAME
   ```
3. Alternatively, use a test repository: `--github-repo-owner=myorg --github-repo-name=test-repo`

**Testing:**
- Use a test repository for development to avoid cluttering production issues
- Test all three feedback types (Bug, Feature, Other)
- Verify labels and markdown formatting on the created issue
- Check device context is included correctly
- Verify issues are created by the App's bot identity, not your personal account

### Production Environment

Production uses GitHub App authentication configured via GCP Secret Manager and environment variables:

1. **GitHub App Configuration** (one-time setup):
   - A GitHub App exists for this deployment; its App ID is deployment-specific
   - Installed on the target repository; the installation ID is deployment-specific
   - Private key stored in GCP Secret Manager as base64-encoded PEM: `github-app-private-key-base64`

2. **Environment Variables** (via Cloud Run):
   - `GITHUB_APP_ID` - The App ID for this deployment
   - `GITHUB_INSTALLATION_ID` - The installation ID for this deployment
   - `GITHUB_APP_PRIVATE_KEY` - Mounted from GCP Secret Manager (base64-encoded PEM); the Dockerfile passes it as `--github-app-private-key` and the server decodes it at startup

3. **Deployment:**
   - Terraform automatically configures these values
   - No manual token rotation required
   - Installation tokens auto-generate with 1-hour validity

**Security:**
- ✅ **No manual token rotation** - Installation tokens generated automatically
- ✅ **Organization-owned** - Not tied to any personal GitHub account
- ✅ **Short-lived tokens** - 1-hour validity (vs PAT: 30-90 days)
- ✅ **Minimal permissions** - Only Issues: Read and write on target repository
- ✅ **Private key stored securely** - GCP Secret Manager with IAM access control
- ✅ **No source control exposure** - Credentials never committed to code
- ✅ **Audit trail** - All actions logged as `ripls-bot[bot]`

**Key Rotation:**
- GitHub App private keys should be rotated annually (not monthly like PATs)
- Process: Generate new key → Update Secret Manager → Verify deployment
- Set calendar reminder for annual rotation
- Keep old key active for 24 hours during rotation

**Monitoring:**
- Track submission success rate
- Alert on GitHub API failures
- Monitor rate limits (GitHub App: 15,000 requests/hour)
- Review submitted issues periodically for quality
- Check Cloud Logging for GitHub App authentication errors

---

**Last Updated:** 2026-02-26
**Status:** Production-ready
