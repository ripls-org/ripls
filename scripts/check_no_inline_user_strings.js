#!/usr/bin/env node
/**
 * check_no_inline_user_strings.js — block regressions to inline English
 * copy in surfaces that should now go through the server localizer.
 *
 * Phases 2 and 3 of #1904 migrated push-notification copy and
 * user-facing email subjects to render via server/l10n. The migration
 * removed every `fmt.Sprintf("Capitalized English ...")` from those
 * files. This script guards against a regression: a contributor adding
 * a new notification type or email subject and reaching for the same
 * `fmt.Sprintf` they see elsewhere.
 *
 * Scope is narrow on purpose:
 *   - Only files that are 100% user-facing and have completed migration
 *     are scanned. Admin emails (activity_digest, waitlist_notification)
 *     stay English and are out of scope.
 *   - The pattern matches a `fmt.Sprintf("X...")` where X is a capital
 *     letter and the format string is at least 8 characters long. This
 *     filters out short format strings like `fmt.Sprintf("%d", n)` and
 *     identifier-style strings.
 *
 * The Dart-side equivalent (scripts/check_i18n.js, npm run lint:dart:i18n)
 * already exists; this is the Go-side analog called for in
 * docs/server/l10n.md Phase 6.
 *
 * Run via: npm run lint:go:no-inline-user-strings
 * Exit 0 = clean, Exit 1 = regression detected.
 *
 * Issue: #1904 Phase 6.
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');

// Files that are 100% user-facing and have completed l10n migration.
// Adding a new file here means: every user-visible string in that file
// must already go through server/l10n. Pre-flight: run this script
// before adding the path and confirm zero violations.
const SCANNED_FILES = [
  'server/notifications/community_subscriber/copy.go',
  'server/notifications/chat_subscriber/copy.go',
  'server/notifications/off_app_sms.go',
  'server/services/web/sms_webhook.go',
  // SSR landing handlers (#2090): every visitor-facing string renders
  // through server/l10n. These pass copy as bare string literals to render
  // helpers (not fmt.Sprintf), so PROSE_PATTERN below is what guards them.
  'server/services/web/service.go',
  'server/services/web/event_page.go',
  'server/services/web/gear_page.go',
  'server/services/web/request_page.go',
  // System-generated notifications (#2896): reminders, close prompts and
  // nudges render every string from notif.offapp.{kind}.* / the push title
  // key, so no English literal is authored here any more.
  'server/jobs/scheduled_notifications/experience_reminders.go',
  'server/jobs/scheduled_notifications/loan_return_reminders.go',
  'server/jobs/scheduled_notifications/request_followup_prompts.go',
  'server/services/experience/poll_manage.go',
  'server/services/experience/nudge_needs.go',
  'server/services/request/nudge_needs.go',
];

// Files that must contain NO user-facing copy at all (#2827): surfaces
// whose strings migrated to typed proto fields rendered by the client ARB
// catalog. These get the strict SHORT_COPY_PATTERN below, which also
// catches short labels ("Lend it", "Borrowing") that INLINE_PATTERN and
// PROSE_PATTERN both miss. A file is enrollable only once its last
// English literal is gone; #2835 removed the deprecated copy fields these
// surfaces were still feeding, which is what made them enrollable. The
// rare legitimate literal (a fixed format-layout string, say) carries
// `// user-string-allow: <why this is not user copy>` on its line.
const NO_COPY_FILES = [
  'server/services/experience/service.go',
  'server/services/experience/location_proposals.go',
  'server/services/portfolio/home_view_decisions.go',
  'server/services/portfolio/home_view_upnext.go',
  'server/services/portfolio/home_view_gear.go',
  'server/impact_metrics/metric_detail_calculator.go',
  'server/impact_metrics/metric_detail_factors.go',
  'server/impact_metrics/metric_detail_rankings.go',
  'server/impact_metrics/metric_detail_series.go',
  'server/impact_metrics/metric_detail_subsidiary.go',
];

// Matches fmt.Sprintf("Capitalized English ...") with a format string
// of at least 8 characters. Tolerates a leading whitespace/newline
// inside the format string. The capture is the inner text so error
// messages can point at it.
const INLINE_PATTERN = /fmt\.Sprintf\(\s*"([A-Z][^"]{7,})"/g;

// Strict pattern for NO_COPY_FILES: a double-quoted literal shaped like a
// capitalized English phrase — one capitalized word ("Borrowing", "Someone")
// optionally followed by more text after a space or punctuation ("Lend it",
// "Hosting · 2 going", "Mon, Jan 2"). That shape is what user copy looks
// like and what INLINE_PATTERN (needs fmt.Sprintf + 8 chars) and
// PROSE_PATTERN (needs trailing .!?) both miss.
//
// Two shapes deliberately pass, because flagging them would bury the copy
// findings under false positives:
//   - lowercase strings — Go log and error text is lowercase by convention
//     ("failed to query RSVPs"), as are dotted l10n keys;
//   - PascalCase identifiers — the capitalized word must not run straight
//     into another capital, so RPC names in structured-log fields
//     ("VoteOnLocation", "ComputeMetricDetail") and SCREAMING_SNAKE
//     constants are not copy candidates.
// The rare capitalized literal that is genuinely not copy (a time layout,
// say) carries the ALLOW_PRAGMA comment on its line.
const SHORT_COPY_PATTERN = /"([A-Z][a-z]+(?:[ ,.·:;!?—-][^"]*)?)"/g;

const ALLOW_PRAGMA = 'user-string-allow:';

// Matches a "prose" string literal: starts with a capital letter, contains
// a space, and ends with sentence punctuation (. ! ?). This catches inline
// user copy passed as bare string-literal args (the SSR pattern) that
// INLINE_PATTERN misses, while cleanly excluding Go log/error strings
// (lowercase, no trailing punctuation by convention), dotted l10n keys, and
// HTTP header names/values (no trailing punctuation). Comment lines are
// skipped so explanatory prose in comments doesn't trip the guard.
const PROSE_PATTERN = /"([A-Z][^"]*\s[^"]*[.!?])"/g;

function isCommentLine(line) {
  const t = line.trim();
  return t.startsWith('//') || t.startsWith('*') || t.startsWith('/*');
}

function checkFile(relPath, strict) {
  const fullPath = path.join(ROOT, relPath);
  if (!fs.existsSync(fullPath)) {
    console.error(
      `check_no_inline_user_strings: scanned file does not exist: ${relPath}`,
    );
    console.error('  Remove it from the scanned list or restore the file.');
    return [{ path: relPath, line: 0, snippet: '(file missing)' }];
  }
  const src = fs.readFileSync(fullPath, 'utf-8');
  const lines = src.split('\n');
  const violations = [];
  for (let i = 0; i < lines.length; i++) {
    let match;
    INLINE_PATTERN.lastIndex = 0;
    while ((match = INLINE_PATTERN.exec(lines[i])) !== null) {
      violations.push({
        path: relPath,
        line: i + 1,
        snippet: match[1].slice(0, 60),
      });
    }
    if (isCommentLine(lines[i])) {
      continue;
    }
    PROSE_PATTERN.lastIndex = 0;
    while ((match = PROSE_PATTERN.exec(lines[i])) !== null) {
      violations.push({
        path: relPath,
        line: i + 1,
        snippet: match[1].slice(0, 60),
      });
    }
    if (strict && !lines[i].includes(ALLOW_PRAGMA)) {
      SHORT_COPY_PATTERN.lastIndex = 0;
      while ((match = SHORT_COPY_PATTERN.exec(lines[i])) !== null) {
        violations.push({
          path: relPath,
          line: i + 1,
          snippet: match[1].slice(0, 60),
        });
      }
    }
  }
  return violations;
}

function main() {
  const violations = [
    ...SCANNED_FILES.flatMap((f) => checkFile(f, false)),
    ...NO_COPY_FILES.flatMap((f) => checkFile(f, true)),
  ];
  if (violations.length === 0) {
    console.log(
      `check_no_inline_user_strings passed: ${
        SCANNED_FILES.length + NO_COPY_FILES.length
      } file(s) clean.`,
    );
    return;
  }
  console.error(
    `check_no_inline_user_strings: ${violations.length} inline user-string regression(s) found:`,
  );
  for (const v of violations) {
    console.error(`  ${v.path}:${v.line}  "${v.snippet}..."`);
  }
  console.error('');
  console.error(
    'SCANNED_FILES are server-localized — use a server/l10n.Localizer.T call',
  );
  console.error(
    'with a key in server/l10n/source/en.toml (and the parallel key in es.toml),',
  );
  console.error(
    'not fmt.Sprintf with literal English. NO_COPY_FILES render in-app: emit a',
  );
  console.error(
    'typed proto field and add the copy to the client ARB catalog instead.',
  );
  console.error('See docs/server/l10n.md.');
  process.exit(1);
}

main();
