# Issue #1105: Don't do LLM generation of description text except for images

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1105
**Filed:** 2026-04-13 | **Last activity:** 2026-04-15
**Labels:** bug, user-feedback | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

When a user types a description for a gear item, experience, or request and then
triggers AI generation (e.g., by switching to image mode), the AI generates a
new description that overwrites what the user wrote. The user's text is treated
as a "prompt" to the LLM rather than as the actual description they want to
keep.

More broadly, even in text-only mode, the LLM rewrites the user's natural
language into AI-sounding prose. Users who write their own descriptions want
their words preserved — not replaced with generated text.

## Context from Comments

- **maintainer**: The behavior is surprising. There's no indication that
  typing into the text field is actually prompting an LLM. Manual creation
  should be either the default or a more obvious path. References #964 (undo /
  go back) as related.
- **tescobar**: For all item types (not just experiences), only generate the
  description from images — never from text input. Still generate title,
  metadata, location, time, etc. from text. Keeps descriptions authentic.
- **tescobar (caveat)**: Items with only a hyperlink and no descriptive text
  should still get a generated description, since the user hasn't written one.
- **maintainer**: Agreed, updated the issue to reflect broader scope.

## Recommendation

A single approach with three behavioral rules:

1. **Text mode**: User's text IS the description. LLM extracts title, metadata,
   location, time, value estimate — but does NOT generate a description. The
   server prompt is updated to omit description generation, and the response
   echoes the user's text verbatim as the description field.
2. **URL-only mode**: When the entire text input is a URL (no surrounding text),
   full generation including description continues as before (the user hasn't
   written anything to preserve).
3. **Image mode**: No change — full LLM generation including description.

Additionally, the UX needs clarity improvements: update hint text to make it
clear the user is writing a description (not a prompt), and make the text/image
mode toggle less ambiguous so users don't accidentally trigger image-based
regeneration after writing text.

## Implementation Plan

### Phase 1: Server — update LLM prompts to skip description on text input

**Goal:** When text mode is used, the LLM generates title, metadata, location,
time, and value estimate but NOT description. The response returns the user's
original text as the description.

**Tasks:**
- [ ] `server/ai/prompts.go` — `buildGearGenerationPrompt()`: Remove the
  DESCRIPTION section from the prompt instructions. Add a note to the prompt
  that the user's text will be used as the description verbatim and should not
  be generated. Remove `Description` from the output schema comment/examples.
- [ ] `server/ai/prompts.go` — `buildRequestGenerationPrompt()`: Same changes
  as gear.
- [ ] `server/ai/prompts.go` — `buildExperienceFromTextPrompt()`: Same changes
  as gear. Note: this prompt currently asks for description with specific tone
  guidance — all of that should be removed for text mode.
- [ ] `server/ai/provider_gemini.go` — `GenerateGearFromText()`: Remove
  `Description` from the `outputSchema` struct so the LLM doesn't waste tokens
  on it. Same for `GenerateExperienceFromText()` and
  `GenerateRequestFromText()`.
- [ ] Apply the same `outputSchema` change in all other providers
  (`provider_openai.go`, `provider_anthropic.go`, `provider_mock.go`).
- [ ] **Set description in the service layer, not providers.** In each service's
  gen_ai.go (`gear/gen_ai.go`, `experience/gen_ai.go`, `request/gen.go`), after
  the provider call returns, overwrite `generation.Description` with the
  original user prompt text. This avoids duplicating the passthrough logic
  across 4+ provider implementations and keeps the "user text = description"
  rule in one place per item type.
- [ ] Update or add tests confirming the response description matches the input
  text for text-mode generation.

**Validates:** Text-mode generation returns user's exact text as description
while still generating title, metadata, etc.

### Phase 2: Server — tighten URL-only detection for gear

**Goal:** Gear's URL auto-detection only triggers when the entire text input is
a URL, not when a URL appears within a longer description.

**Tasks:**
- [ ] `server/services/gear/gen_ai.go` — Change `extractURL()` (or the call
  site at line 77) to only match when the trimmed prompt is entirely a URL. A
  simple approach: `strings.TrimSpace(prompt)` matches the URL regex with no
  remaining non-whitespace text.
- [ ] Add test cases: "https://amazon.com/dp/123" should trigger URL mode,
  "Check out https://amazon.com/dp/123 - great drill" should NOT (text mode
  with URL in description).
- [ ] Experience already uses `oneof` (text/media_id/website_url) so no change
  needed there — the client explicitly chooses URL mode.
- [ ] Request has no URL mode, so no change needed.

**Validates:** URLs embedded in descriptive text are preserved in the
description, not used to trigger web scraping.

### Phase 3: Client — update button labels and mode-switch guard

**Goal:** Rename "Generate" buttons to "Create" (less AI-forward language), and
prevent accidental data loss when switching from text to image mode.

**Tasks:**
- [ ] Keep existing hint text in all creation modals (no changes to
  `creationGearHintText`, `creationExperienceHintText`,
  `creationRequestHintText`).
- [ ] Rename button labels across all three creation modals:
  - `create_gear_modal.dart`: "Generate Item" → "Create Item"
  - `experience_creation_modal.dart`: "Generate Experience" → "Create Experience"
  - `request_creation_modal.dart`: "Generate Request" → "Create Request"
- [ ] **i18n:** Add/update all changed button labels in `app_en.arb` and
  `app_es.arb`. If the current labels are hardcoded, move them to ARB keys
  following the `creation*` prefix convention (e.g.,
  `creationGearSubmitButton`). All user-visible strings must use
  `context.l10n.*` per i18n policy.
- [ ] Keep current "Text / Link" and "Image" toggle labels as-is.
- [ ] In all three creation modals (`create_gear_modal.dart`,
  `experience_creation_modal.dart`, `request_creation_modal.dart`): When the
  user has entered text in the "Text / Link" field and taps the "Image" toggle,
  show a confirmation dialog warning that their text will be discarded and
  they're switching to image-based creation mode. Only switch modes if the user
  confirms. If the text field is empty, switch immediately without a dialog.
- [ ] **i18n:** Add the confirmation dialog strings (title, body, confirm
  button, cancel button) to `app_en.arb` and `app_es.arb` using the
  `creation*` prefix (e.g., `creationSwitchToImageTitle`,
  `creationSwitchToImageBody`). The dialog implementation is a widget async
  exception per architecture policy — acceptable in the modal widget with a
  `mounted` guard after the `showDialog` await.

**Validates:** Users see "Create" (not "Generate"), and cannot accidentally lose
typed text by switching to image mode.

### Phase 4: Tappable links in descriptions

**Goal:** URLs embedded within text descriptions render as tappable links in the
app.

**Tasks:**
- [ ] Identify the widgets that render gear, experience, and request
  descriptions in the app.
- [ ] Replace plain `Text` widgets with a rich text widget that detects URLs and
  makes them tappable (e.g., using `url_launcher` + a regex-based link detector,
  or an existing package like `flutter_linkify`).
- [ ] Add tests for URL detection in description text.

**Validates:** A description like "Check out https://amazon.com/dp/123 - great
drill" renders with the URL as a tappable link.

## Related Issues
- #964 — Undo / go backward through conclusions. The ability to undo would
  mitigate the damage when AI generation overwrites user content, but this issue
  addresses the root cause (don't overwrite in the first place).
