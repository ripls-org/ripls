#!/usr/bin/env node
/**
 * check_go_vanity.js — the vanity page's go-import tag must name the module
 * that go.mod declares.
 *
 * Those two strings live in different files, in different languages, deployed
 * by different mechanisms — go.mod ships with the code, the page is rsynced to
 * a static host. Nothing else compares them. If they drift, `go build` keeps
 * working for everyone who already has a checkout, and `go get` silently fails
 * for everyone who does not: the toolchain fetches the page, reads a prefix
 * that does not match the import path, and reports the module as unknown.
 * That is a failure nobody inside the project would ever hit.
 *
 * The page lives in infra/, which is private-bound and therefore absent from
 * the open-source export. When it is missing this check reports that and
 * passes, rather than failing a public checkout for the absence of a file it
 * is not supposed to have.
 *
 *   node scripts/check_go_vanity.js
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const GO_MOD = path.join(ROOT, 'go.mod');
const SITE_DIR = path.join(ROOT, 'infra', 'go-vanity');
const PAGE = path.join(SITE_DIR, 'content', 'index.html');

/** The module path declared by go.mod. */
function moduleFromGoMod(text) {
  const m = text.match(/^module\s+(\S+)\s*$/m);
  return m ? m[1] : null;
}

/**
 * The go-import tag's three fields: import prefix, VCS, repo URL.
 * Returns null when the tag is absent or malformed, which are different
 * failures from "the prefix is wrong" and are reported separately.
 */
function goImportFromPage(html) {
  const tag = html.match(/<meta\s+name="go-import"\s+content="([^"]+)"\s*\/?>/i);
  if (!tag) return null;
  const fields = tag[1].trim().split(/\s+/);
  if (fields.length !== 3) return { malformed: tag[1] };
  const [prefix, vcs, repo] = fields;
  return { prefix, vcs, repo };
}

function main() {
  // Skip on the DIRECTORY, not the page. Keying the skip on the page meant a
  // renamed or deleted page read as "public checkout" and passed silently —
  // which is what happened when site/ became content/. Absent directory is a
  // public checkout; present directory with no page is a broken one.
  if (!fs.existsSync(SITE_DIR)) {
    process.stdout.write(
      'check_go_vanity: infra/go-vanity/ not present (private-bound); skipping\n',
    );
    return 0;
  }
  if (!fs.existsSync(PAGE)) {
    process.stderr.write(
      `check_go_vanity: ${path.relative(ROOT, SITE_DIR)} exists but ` +
        `${path.relative(ROOT, PAGE)} is missing.\n` +
        '  The vanity host serves that file; without it `go get` gets a 500.\n',
    );
    return 1;
  }

  const modulePath = moduleFromGoMod(fs.readFileSync(GO_MOD, 'utf8'));
  if (!modulePath) {
    process.stderr.write('check_go_vanity: could not read the module line from go.mod\n');
    return 1;
  }

  const tag = goImportFromPage(fs.readFileSync(PAGE, 'utf8'));
  if (!tag) {
    process.stderr.write(
      `check_go_vanity: no <meta name="go-import"> tag in ${path.relative(ROOT, PAGE)}.\n` +
        '  Without it the page is just a page and `go get` cannot resolve the module.\n',
    );
    return 1;
  }
  if (tag.malformed !== undefined) {
    process.stderr.write(
      `check_go_vanity: go-import content must be "<prefix> <vcs> <repo-url>", got:\n` +
        `  ${tag.malformed}\n`,
    );
    return 1;
  }

  if (tag.prefix !== modulePath) {
    process.stderr.write(
      'check_go_vanity: the vanity page and go.mod disagree about the module path.\n\n' +
        `  go.mod declares:  ${modulePath}\n` +
        `  the page serves:  ${tag.prefix}\n\n` +
        '  `go get` resolves the path in the page, so this breaks fetching the\n' +
        '  module for anyone without a checkout — while every local build keeps\n' +
        `  working. Fix ${path.relative(ROOT, PAGE)}.\n`,
    );
    return 1;
  }

  if (tag.vcs !== 'git') {
    process.stderr.write(`check_go_vanity: expected vcs "git", got ${JSON.stringify(tag.vcs)}\n`);
    return 1;
  }
  if (!/^https:\/\/\S+$/.test(tag.repo)) {
    process.stderr.write(
      `check_go_vanity: repo URL must be an https URL, got ${JSON.stringify(tag.repo)}\n`,
    );
    return 1;
  }

  process.stdout.write(`check_go_vanity: ${modulePath} -> ${tag.repo}\n`);
  return 0;
}

if (require.main === module) {
  process.exitCode = main();
}

module.exports = { moduleFromGoMod, goImportFromPage };
