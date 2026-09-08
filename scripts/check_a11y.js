#!/usr/bin/env node
/**
 * check_a11y.js — accessibility lint enforcement for Flutter presentation code.
 *
 * Implements Phase 2 of docs/ai/accessibility_plan.md:
 *
 *   - avoid_raw_gesture_detector              raw GestureDetector / InkWell with onTap
 *   - require_semantic_label_on_icon_button   IconButton without tooltip
 *   - require_l10n_for_semantic_labels        string literal on Semantics(label:) / tooltip:
 *   - require_semantic_label_on_cached_media  CachedMediaImage(...) without semanticsLabel:
 *   - prefer_accessible_modal                 raw showModalBottomSheet outside the helper
 *   - prefer_accessible_duration              Duration literal on AnimationController.duration:
 *   - avoid_color_only_status                 *Dot/*Indicator/*Badge widget with no Semantics
 *
 * Plus, from the #2162 e2e harness rollout:
 *
 *   - require_kebab_case_for_semantics_identifier
 *       Semantics(identifier: …) and the new semanticsIdentifier: parameter
 *       on Tappable/Toggle/IconAction/CachedMediaImage must use raw
 *       lowercase kebab strings (^[a-z0-9-]+$). No l10n keys.
 *
 * Behavior: walks app/lib/presentation/, applies each rule, compares the
 * resulting violations against scripts/a11y_allowlist.txt. New violations
 * (not present in the allowlist) cause exit 1; violations that have been
 * fixed but are still in the allowlist are reported as warnings so the
 * allowlist can be tightened. Adding to the allowlist on main requires
 * removing an existing entry — the ratchet is enforced by CI checking that
 * the allowlist never grows.
 *
 * Usage:
 *   node scripts/check_a11y.js                  # check all rules
 *   node scripts/check_a11y.js --update-baseline   # rewrite allowlist from current state
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const PRESENTATION_DIR = path.join(ROOT, 'app', 'lib', 'presentation');
const ALLOWLIST_PATH = path.join(__dirname, 'a11y_allowlist.txt');
const ACCESSIBILITY_DIR = path.join(
  PRESENTATION_DIR,
  'widgets',
  'accessibility',
);

// Files matching these patterns are exempt: they are the primitives
// themselves, generated bindings, and tests.
const EXEMPT_PREFIXES = [
  path.join(PRESENTATION_DIR, 'widgets', 'accessibility') + path.sep,
];
const EXEMPT_FILES = [
  // CachedMediaImage is the primitive — its own definition site is exempt
  // from the "callers must pass semanticsLabel" rule (Phase 1 added the
  // parameter; the class itself shouldn't lint against itself).
  path.join(PRESENTATION_DIR, 'widgets', 'chat', 'cached_media_image.dart'),
];
const EXEMPT_SUFFIXES = ['.g.dart', '.freezed.dart'];

function isExempt(filePath) {
  if (EXEMPT_PREFIXES.some((p) => filePath.startsWith(p))) return true;
  if (EXEMPT_FILES.includes(filePath)) return true;
  if (EXEMPT_SUFFIXES.some((s) => filePath.endsWith(s))) return true;
  return false;
}

function walkDart(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkDart(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.dart')) {
      out.push(full);
    }
  }
  return out;
}

// Strip // and /* */ comments and string contents (replace strings with
// "_") so regexes that match function calls aren't confused by literal
// occurrences inside strings or doc comments. We preserve line breaks so
// reported line numbers match the original file.
function stripCommentsAndStrings(src) {
  const out = [];
  let i = 0;
  let inLineComment = false;
  let inBlockComment = false;
  let inString = null; // null | "'" | '"'
  let stringIsTriple = false;
  while (i < src.length) {
    const c = src[i];
    const c2 = src[i + 1];
    if (inLineComment) {
      if (c === '\n') {
        inLineComment = false;
        out.push(c);
      }
      i++;
      continue;
    }
    if (inBlockComment) {
      if (c === '\n') out.push(c);
      if (c === '*' && c2 === '/') {
        inBlockComment = false;
        i += 2;
        continue;
      }
      i++;
      continue;
    }
    if (inString) {
      if (c === '\n' && !stringIsTriple) {
        // unterminated single-line string; emit closing quote and bail
        out.push(inString);
        out.push(c);
        inString = null;
        i++;
        continue;
      }
      if (c === '\n') {
        out.push(c);
        i++;
        continue;
      }
      if (c === '\\' && c2 != null) {
        // escape; skip next char in string body
        i += 2;
        continue;
      }
      if (stringIsTriple) {
        if (
          c === inString &&
          src[i + 1] === inString &&
          src[i + 2] === inString
        ) {
          out.push(inString);
          inString = null;
          stringIsTriple = false;
          i += 3;
          continue;
        }
      } else if (c === inString) {
        out.push(inString);
        inString = null;
        i++;
        continue;
      }
      // Replace string body chars (non-newline) with underscore.
      out.push('_');
      i++;
      continue;
    }
    if (c === '/' && c2 === '/') {
      inLineComment = true;
      i += 2;
      continue;
    }
    if (c === '/' && c2 === '*') {
      inBlockComment = true;
      i += 2;
      continue;
    }
    if ((c === "'" || c === '"')) {
      // detect triple
      if (src[i + 1] === c && src[i + 2] === c) {
        inString = c;
        stringIsTriple = true;
        out.push(c); // preserve opening quote so regexes can locate strings
        i += 3;
        continue;
      }
      inString = c;
      stringIsTriple = false;
      out.push(c);
      i++;
      continue;
    }
    out.push(c);
    i++;
  }
  return out.join('');
}

// --- Rules ----------------------------------------------------------------

// Each rule returns an array of { file, line, message }.

function ruleAvoidRawGestureDetector(filePath, src) {
  // Match GestureDetector( or InkWell( where onTap: appears at the same
  // paren-depth as the constructor's argument list. Walking the brace depth
  // (not a fixed 30-line window) avoids false positives where an outer
  // drag-only GestureDetector wraps an inner Tappable that has its own
  // onTap. Any new GestureDetector should use Tappable instead.
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  for (let i = 0; i < stripped.length; i++) {
    const m = stripped.slice(i).match(/^\b(GestureDetector|InkWell)\s*\(/);
    if (!m) continue;
    const startIdx = i;
    const argsStart = i + m[0].length;
    let depth = 1;
    let j = argsStart;
    let argBody = '';
    let topLevelArgs = '';
    while (j < stripped.length && depth > 0) {
      const c = stripped[j];
      if (c === '(') depth++;
      else if (c === ')') {
        depth--;
        if (depth === 0) break;
      }
      // Capture only chars at the immediate constructor depth (=== 1).
      if (depth === 1) topLevelArgs += c;
      argBody += c;
      j++;
    }
    if (
      /\bonTap\s*:/.test(topLevelArgs) ||
      /\bonLongPress\s*:/.test(topLevelArgs)
    ) {
      // Compute line number from startIdx.
      const line = stripped.slice(0, startIdx).split('\n').length;
      violations.push({
        file: filePath,
        line,
        rule: 'avoid_raw_gesture_detector',
        message:
          'Use Tappable / Toggle / IconAction from widgets/accessibility/ instead of raw GestureDetector / InkWell.',
      });
    }
    i = j;
  }
  return violations;
}

function ruleRequireTooltipOnIconButton(filePath, src) {
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const lines = stripped.split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (!/\bIconButton\s*\(/.test(lines[i])) continue;
    // window through the constructor: look for tooltip: in next 40 lines
    const window = lines.slice(i, i + 40).join('\n');
    if (!/\btooltip\s*:/.test(window)) {
      violations.push({
        file: filePath,
        line: i + 1,
        rule: 'require_semantic_label_on_icon_button',
        message:
          'IconButton requires tooltip: (use IconAction from widgets/accessibility/ to enforce this).',
      });
    }
  }
  return violations;
}

function ruleRequireL10nForSemanticLabels(filePath, src) {
  // Detects string literals passed to unambiguous accessibility-only
  // properties: tooltip:, semanticLabel:, and the label/value/hint
  // arguments inside a Semantics(...) constructor.
  //
  // We avoid flagging the bare `label:` argument outside Semantics(...)
  // because many unrelated Flutter widgets accept it (BottomNavigationBarItem,
  // InputDecoration, etc.). For Semantics(...), we use a windowed scan to
  // find string-literal arguments within the constructor body.
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const lines = stripped.split('\n');
  // Unambiguous accessibility-only argument names. After stripping, a string
  // literal looks like 'xxx' or "xxx" with underscores between the quotes.
  // semanticsLabel (with the trailing s) is the spelling used by the
  // codebase's own primitives (Tappable, Toggle, IconAction,
  // CachedMediaImage); semanticLabel (no s) is Flutter's icon-level argument.
  const literalArgRegex =
    /\b(tooltip|semanticLabel|semanticsLabel)\s*:\s*('[_]*'|"[_]*")/;
  for (let i = 0; i < lines.length; i++) {
    if (literalArgRegex.test(lines[i])) {
      violations.push({
        file: filePath,
        line: i + 1,
        rule: 'require_l10n_for_semantic_labels',
        message:
          'tooltip:, semanticLabel:, and semanticsLabel: must come from context.l10n.* (no string literals).',
      });
    }
  }
  // Semantics(...) constructor scan.
  for (let i = 0; i < lines.length; i++) {
    const m = lines[i].match(/\bSemantics\s*\(/);
    if (!m) continue;
    // Scan forward up to 30 lines for label/value/hint with string literal,
    // bailing early when the depth returns to 0 (constructor close).
    let depth = 0;
    let started = false;
    for (let j = i; j < Math.min(i + 30, lines.length); j++) {
      const l = lines[j];
      for (const ch of l) {
        if (ch === '(') {
          depth++;
          started = true;
        } else if (ch === ')') depth--;
      }
      if (
        /\b(label|value|hint|increasedValue|decreasedValue)\s*:\s*('[_]*'|"[_]*")/.test(
          l,
        )
      ) {
        violations.push({
          file: filePath,
          line: j + 1,
          rule: 'require_l10n_for_semantic_labels',
          message:
            'Semantics() label/value/hint must come from context.l10n.* (no string literals).',
        });
      }
      if (started && depth === 0) break;
    }
  }
  return violations;
}

function ruleRequireSemanticLabelOnCachedMediaImage(filePath, src) {
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const lines = stripped.split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (!/\bCachedMediaImage\s*\(/.test(lines[i])) continue;
    const window = lines.slice(i, i + 40).join('\n');
    if (!/\bsemanticsLabel\s*:/.test(window)) {
      violations.push({
        file: filePath,
        line: i + 1,
        rule: 'require_semantic_label_on_cached_media_image',
        message:
          'CachedMediaImage requires semanticsLabel: (pass null only if the image is purely decorative, with a comment explaining why).',
      });
    }
  }
  return violations;
}

function rulePreferAccessibleModal(filePath, src) {
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const lines = stripped.split('\n');
  for (let i = 0; i < lines.length; i++) {
    if (/\bshowModalBottomSheet\s*[<(]/.test(lines[i])) {
      violations.push({
        file: filePath,
        line: i + 1,
        rule: 'prefer_accessible_modal',
        message:
          'Use showAccessibleModal from widgets/accessibility/ instead of raw showModalBottomSheet (restores focus on dismiss).',
      });
    }
  }
  return violations;
}

function rulePreferAccessibleDuration(filePath, src) {
  // Catches duration: Duration(...) or duration: const Duration(...)
  // applied to AnimationController, AnimatedX, or top-level AnimationController(
  // construction. Excludes accessibleDuration(...) calls.
  //
  // Viewmodels live outside the widget tree and cannot call
  // accessibleDuration(context, ...) because they have no BuildContext.
  // Skip the rule for files under viewmodels/ — the system reduce-motion
  // setting still applies to any animations they kick off via widgets,
  // which DO go through accessibleDuration at the widget layer.
  if (/\/viewmodels\//.test(filePath)) return [];
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const lines = stripped.split('\n');
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    // Lines that pass a Duration constructor as a `duration:` argument.
    if (
      /\bduration\s*:\s*(const\s+)?Duration\s*\(/.test(line) &&
      !/accessibleDuration/.test(line)
    ) {
      violations.push({
        file: filePath,
        line: i + 1,
        rule: 'prefer_accessible_duration',
        message:
          'Wrap the Duration in accessibleDuration(context, ...) so the system "Reduce Motion" setting is honoured.',
      });
    }
  }
  return violations;
}

function ruleAvoidColorOnlyStatus(filePath, src) {
  // Heuristic: a class declared as `class FooDot/Indicator/Badge` that
  // contains no occurrence of Semantics( or Tooltip( in the file.
  const violations = [];
  const stripped = stripCommentsAndStrings(src);
  const classMatch = /class\s+([A-Z]\w*(?:Dot|Indicator|Badge))\s+/g;
  let m;
  while ((m = classMatch.exec(stripped)) !== null) {
    const className = m[1];
    if (
      !/\bSemantics\s*\(/.test(stripped) &&
      !/\bTooltip\s*\(/.test(stripped)
    ) {
      // Find approximate line number
      const before = stripped.slice(0, m.index);
      const line = before.split('\n').length;
      violations.push({
        file: filePath,
        line,
        rule: 'avoid_color_only_status',
        message:
          `${className} appears to convey state via color alone. Wrap its build output in Semantics(label: context.l10n.<status>) or add a Tooltip.`,
      });
    }
  }
  return violations;
}

function ruleRequireKebabCaseForSemanticsIdentifier(filePath, src) {
  // Operates on raw source (not stripped) so we can read the actual
  // identifier string value. Catches both:
  //   Semantics(identifier: 'event-hero-video-player', …)
  //   Tappable(semanticsIdentifier: 'event-hero-rsvp-yes', …)
  // and rejects anything that isn't /^[a-z0-9-]+$/. l10n keys, camelCase,
  // PascalCase, spaces, and snake_case all fail.
  //
  // Identifier strings are intentionally raw (NOT context.l10n.*) because
  // they're test contract surface, not user-facing copy. The kebab gate
  // keeps them tidy and stable across renames.
  const violations = [];
  // Strip only comments (not strings) so we can read string contents,
  // and so doc-comment examples don't false-positive.
  const noComments = src
    .replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
    .replace(/\/\/[^\n]*/g, '');
  const lines = noComments.split('\n');
  const argRegex =
    /\b(?:semanticsIdentifier|identifier)\s*:\s*(['"])([^'"]*)\1/g;
  const kebab = /^[a-z0-9-]+$/;
  for (let i = 0; i < lines.length; i++) {
    let m;
    argRegex.lastIndex = 0;
    while ((m = argRegex.exec(lines[i])) !== null) {
      const value = m[2];
      if (!kebab.test(value)) {
        violations.push({
          file: filePath,
          line: i + 1,
          rule: 'require_kebab_case_for_semantics_identifier',
          message:
            `Semantics identifier '${value}' must match ^[a-z0-9-]+$ ` +
            '(kebab-case, no spaces or camelCase). See docs/client/testing/semantics_identifiers.md.',
        });
      }
    }
  }
  return violations;
}

const RULES = [
  ruleAvoidRawGestureDetector,
  ruleRequireTooltipOnIconButton,
  ruleRequireL10nForSemanticLabels,
  ruleRequireSemanticLabelOnCachedMediaImage,
  rulePreferAccessibleModal,
  rulePreferAccessibleDuration,
  ruleAvoidColorOnlyStatus,
  ruleRequireKebabCaseForSemanticsIdentifier,
];

// --- Allowlist ------------------------------------------------------------

function fingerprint(violation) {
  // Path-relative to ROOT for stable identifiers across machines.
  const rel = path.relative(ROOT, violation.file);
  return `${violation.rule} ${rel}`;
}

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST_PATH)) return new Set();
  const lines = fs.readFileSync(ALLOWLIST_PATH, 'utf-8').split('\n');
  const set = new Set();
  for (const raw of lines) {
    const line = raw.replace(/#.*$/, '').trim();
    if (line) set.add(line);
  }
  return set;
}

function writeAllowlist(fingerprints) {
  const sorted = Array.from(fingerprints).sort();
  const header = [
    '# Accessibility lint allowlist (Phase 2 baseline).',
    '#',
    '# Each entry is "<rule_name> <relative_path_to_file>". Files in this',
    '# allowlist are exempted from the named rule. The allowlist can SHRINK',
    '# but never GROW on main — adding a new entry requires removing one.',
    '# CI enforces this ratchet.',
    '#',
    '# Regenerate from current state with:',
    '#   node scripts/check_a11y.js --update-baseline',
    '',
  ].join('\n');
  fs.writeFileSync(ALLOWLIST_PATH, header + sorted.join('\n') + '\n');
}

// --- Driver ---------------------------------------------------------------

function main() {
  const args = process.argv.slice(2);
  const updateBaseline = args.includes('--update-baseline');

  const files = walkDart(PRESENTATION_DIR).filter((f) => !isExempt(f));
  const allViolations = [];
  for (const file of files) {
    const src = fs.readFileSync(file, 'utf-8');
    for (const rule of RULES) {
      allViolations.push(...rule(file, src));
    }
  }

  const currentFingerprints = new Set(allViolations.map(fingerprint));

  if (updateBaseline) {
    writeAllowlist(currentFingerprints);
    console.log(
      `Wrote ${currentFingerprints.size} fingerprints to ${path.relative(ROOT, ALLOWLIST_PATH)}.`,
    );
    return;
  }

  const allowlist = loadAllowlist();

  const newViolations = allViolations.filter(
    (v) => !allowlist.has(fingerprint(v)),
  );
  const stale = Array.from(allowlist).filter((fp) => !currentFingerprints.has(fp));

  if (newViolations.length > 0) {
    console.error(
      `Accessibility lint failed: ${newViolations.length} new violation(s) not in allowlist.\n`,
    );
    for (const v of newViolations) {
      const rel = path.relative(ROOT, v.file);
      console.error(`  ${rel}:${v.line}  [${v.rule}]`);
      console.error(`    ${v.message}`);
    }
    console.error(
      `\nIf this is intentional and there is a tracked follow-up, remove an existing`,
    );
    console.error(
      `entry from ${path.relative(ROOT, ALLOWLIST_PATH)} and add the new one. The allowlist`,
    );
    console.error(`can SHRINK but never GROW on main.`);
    process.exit(1);
  }

  if (stale.length > 0) {
    console.warn(
      `${stale.length} stale allowlist entr${stale.length === 1 ? 'y' : 'ies'} (violation no longer present, allowlist can be tightened):`,
    );
    for (const fp of stale) console.warn(`  ${fp}`);
    console.warn(
      `\nRun: node scripts/check_a11y.js --update-baseline  to refresh.`,
    );
  }

  console.log(
    `Accessibility lint passed: ${allViolations.length} violation(s), all in allowlist.`,
  );
}

main();
