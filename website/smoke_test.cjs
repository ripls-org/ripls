/*
  smoke_test.cjs — self-contained Playwright smoke test for the static
  marketing site (the Ripls Foundation home page at `/`, plus the audience
  landing pages and their waitlist form).

  Why standalone (not in e2e/): the Flutter e2e harness boots a Postgres-backed
  Go server in its globalSetup. The marketing site is static HTML served by
  nginx with no backend, so this test serves content/ itself and mocks the
  waitlist endpoint — it must not depend on a database or the app server.

  It serves content/ with the same routing as website/config/locations.include
  (`/` -> /home/index.html, `/home/` -> 301 /), so it exercises the page
  exactly as it is served in production: at the site root, with root-absolute
  asset paths.

  The home-page block also asserts that every human-written string in
  home/content.md appears **verbatim** on the rendered page (#2895). That copy
  is the source of truth and must never be edited by an agent; this check turns
  any paraphrase or drift into a test failure.

  Run: `npm run test:website` (or `node website/smoke_test.cjs`).
  Requires the Playwright browser installed in e2e/ (CI installs it there).
*/

const http = require('node:http');
const fs = require('node:fs');
const path = require('node:path');

const CONTENT_DIR = path.join(__dirname, 'content');
const WAITLIST_URL_FRAGMENT = 'WaitlistService/JoinWaitlist';

/*
  Headings in home/content.md that name a slot rather than supplying a rendered
  string. "Top menu items" names the nav, whose items are rendered individually.
  Every other heading IS rendered, so keep this list to the genuinely structural
  ones — it and the "(omit ...)" rule below are the only ways a string in
  content.md can legitimately fail to appear on the page.
*/
const HOME_STRUCTURAL_HEADINGS = new Set(['Top menu items']);

/*
  A heading annotated "(omit ...)" — e.g. "## Tagline (omit for now)" — takes
  its whole section off the page. The author controls this from content.md, so
  reinstating a section never needs a test edit.
*/
const OMITTED_HEADING_RE = /\(omit\b[^)]*\)\s*$/i;

/*
  Bracketed directives that tell the injector to place a component, rather than
  copy to render. Anything else in brackets IS copy — the invite form's field
  labels, its button, and its confirmation are authored as [email],
  [request invite], and [on success: "..."].
*/
const HOME_DIRECTIVES = new Set(['[insert video from walkthrough here]']);

/*
  Reduce an authored line to the plain text the page should render: markdown
  links collapse to their label, _emphasis_ loses its underscores, a `- ` list
  marker is dropped, [on success: "x"] yields just x, and a lone [bracketed]
  field label loses its brackets.
*/
function renderedText(line) {
  const onSuccess = line.match(/^\[on success:\s*"(.*)"\]$/);
  if (onSuccess) return onSuccess[1];
  return line
    .replace(/^-\s+/, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/^\[([^\]]+)\]$/, '$1')
    .replace(/_([^_]+)_/g, '$1');
}

/*
  Every user-visible string in home/content.md: section headings, nav items,
  body paragraphs, and form field labels — skipping the file's own preamble,
  the component directives, and any section marked "(omit ...)".
*/
/* The `- ` items under "## Top menu items", in order. */
function homeMenuItems() {
  const md = fs.readFileSync(path.join(CONTENT_DIR, 'home', 'content.md'), 'utf8');
  const section = md.split(/^##\s+/m).find((s) => s.startsWith('Top menu items'));
  return section.split('\n').filter((l) => l.trim().startsWith('- ')).map((l) => l.trim().slice(2));
}

function homeCopyStrings() {
  const md = fs.readFileSync(path.join(CONTENT_DIR, 'home', 'content.md'), 'utf8');
  const strings = [];
  let omitting = false;
  for (const raw of md.split('\n')) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith('# ')) continue; // the file's own title
    const heading = line.match(/^#{2,3}\s+(.*)$/);
    if (heading) {
      // A `###` under an omitted `##` stays omitted; a new `##` clears it.
      if (line.startsWith('## ')) omitting = OMITTED_HEADING_RE.test(heading[1]);
      if (omitting) continue;
      if (!HOME_STRUCTURAL_HEADINGS.has(heading[1])) strings.push(heading[1]);
      continue;
    }
    if (omitting) continue;
    if (line.startsWith('This is the source') || line.startsWith('This content is')) continue;
    if (HOME_DIRECTIVES.has(line)) continue;
    strings.push(renderedText(line));
  }
  return strings;
}

const { chromium } = require(path.join(__dirname, '..', 'e2e', 'node_modules', 'playwright'));

const MIME = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.png': 'image/png',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.webm': 'video/webm',
  '.mp4': 'video/mp4',
  '.svg': 'image/svg+xml',
};

// Static server mirroring website/config/default.conf for the paths we test.
function startServer() {
  const server = http.createServer((req, res) => {
    let urlPath = decodeURIComponent(req.url.split('?')[0]);
    if (urlPath === '/home/') {
      res.writeHead(301, { Location: '/' });
      return res.end();
    }
    if (urlPath === '/') urlPath = '/home/index.html'; // nginx: location = /
    let filePath = path.join(CONTENT_DIR, urlPath);
    if (filePath.endsWith('/')) filePath = path.join(filePath, 'index.html');
    if (!filePath.startsWith(CONTENT_DIR)) {
      res.writeHead(403);
      return res.end('forbidden');
    }
    fs.readFile(filePath, (err, data) => {
      if (err) {
        res.writeHead(404);
        return res.end('not found');
      }
      res.writeHead(200, { 'Content-Type': MIME[path.extname(filePath)] || 'application/octet-stream' });
      res.end(data);
    });
  });
  return new Promise((resolve) => server.listen(0, () => resolve(server)));
}

const checks = [];
function check(name, cond) {
  checks.push({ name, pass: !!cond });
  console.log(`${cond ? 'PASS' : 'FAIL'}  ${name}`);
}

async function main() {
  const server = await startServer();
  const base = `http://localhost:${server.address().port}`;
  const browser = await chromium.launch();

  try {
    // ---- The Ripls Foundation home page is served at the site root ----
    {
      const page = await browser.newPage();
      // 'load', not 'networkidle': the page embeds an autoplaying looping
      // walkthrough clip, so the network never goes idle and networkidle would
      // hang until the 30s timeout.
      await page.goto(`${base}/`, { waitUntil: 'load' });
      check('root serves the home page (title)', (await page.title()).includes('Ripls'));
      // The tagline is currently omitted in content.md, so the page must not
      // render one — the omission is real, not just untested.
      check('omitted tagline is absent', !(await page.$('.tagline')));
      check('mission emblem present', !!(await page.$('.emblem')));
      check('each principle carries an icon', (await page.$$('.principle .icon')).length === 3);
      check('brand wordmark present', (await page.$$('.brand')).length > 0);
      check('mission statement present', !!(await page.$('.mission-statement')));
      check('three principles', (await page.$$('.principle')).length === 3);

      // The nav mirrors content.md's list, and every item resolves to a real
      // section. Count comes from the copy, so adding an item needs no edit here.
      const wantMenu = homeMenuItems();
      const menu = await page.$$eval('.menu a', (els) =>
        els.map((a) => ({ href: a.getAttribute('href'), text: a.textContent.trim() })));
      check(`top menu has content.md's ${wantMenu.length} items, in order`,
        JSON.stringify(menu.map((m) => m.text)) === JSON.stringify(wantMenu));
      const targets = await Promise.all(menu.map((m) => page.$(m.href)));
      check('every menu item points at a real section', targets.every(Boolean));

      // tokens.gen.css + fonts.css applied: serif headings, sans chrome.
      const h2Font = await page.$eval('h2', (el) => getComputedStyle(el).fontFamily);
      check('section titles set in Libre Baskerville', /Libre Baskerville/.test(h2Font));
      const menuFont = await page.$eval('.menu a', (el) => getComputedStyle(el).fontFamily);
      check('menu set in Public Sans', /Public Sans/.test(menuFont));

      // Light only: the page must not carry a dark-mode block (#2895).
      check('no OS dark-mode switch on the home page', !(await page.evaluate(async () => {
        const css = await fetch('/css/home.css').then((r) => r.text());
        return /prefers-color-scheme\s*:\s*dark/.test(css);
      })));

      /*
        The copy invariant (#2895): home/content.md is 100% human-written and is
        injected verbatim. textContent, not innerText, so a CSS text-transform
        can never hide a mismatch behind re-casing.
      */
      const rendered = await page.evaluate(() => document.body.textContent);
      /*
        waitlist.js captures #formSuccess's authored text as its fallback and
        then CLEARS the node, so the confirmation line is real copy that is not
        in the DOM by the time we look. Union the served markup back in rather
        than exempting the string — that keeps the confirmation covered.
      */
      const served = await page.evaluate(() => fetch(location.href).then((r) => r.text()));
      const haystack = `${rendered}\n${served}`;
      const copy = homeCopyStrings();
      const missing = copy.filter((s) => !haystack.includes(s));
      check(`every content.md string rendered verbatim (${copy.length} strings)`, missing.length === 0);
      if (missing.length) missing.forEach((s) => console.error(`  not on page: ${s.slice(0, 80)}`));

      // The invite form carries the fields content.md asks for.
      check('invite form present', !!(await page.$('#signupForm')));
      check('invite form: email field', !!(await page.$('#signupForm [name="email"]')));
      check('invite form: optional name field', !!(await page.$('#signupForm [name="name"]')));
      check('invite form: intent textarea', !!(await page.$('#signupForm textarea[name="keeping_alive"]')));
      const labelled = await page.$$eval('#signupForm input, #signupForm textarea', (els) =>
        els.every((el) => !!document.querySelector(`label[for="${el.id}"]`)));
      check('invite form: every field has a real <label>', labelled);
      await page.close();
    }

    // ---- The homescreen audience page still renders at its own subpath ----
    {
      const page = await browser.newPage();
      await page.goto(`${base}/homescreen/`, { waitUntil: 'networkidle' });
      check('homescreen: hero headline present', !!(await page.$('.hero .headline')));
      // Background loads via the data-bg loader (absolute /homescreen/background path)
      const heroBg = await page.$eval('.hero', (el) => getComputedStyle(el).backgroundImage);
      check('homescreen: hero background loaded via data-bg', /\/homescreen\/background\/01\.jpg/.test(heroBg));
      // Shared marketing.css applied
      const font = await page.$eval('.headline', (el) => getComputedStyle(el).fontFamily);
      check('homescreen: marketing.css applied (Libre Baskerville headline)', /Libre Baskerville/.test(font));
      // Email input has an associated <label> (accessibility)
      const labelled = await page.$eval('#email', () => {
        const lbl = document.querySelector('label[for="email"]');
        return !!lbl && lbl.textContent.trim().length > 0;
      });
      check('homescreen: email input has a real <label>', labelled);
      // Fragment CTA keeps the URL on the same page (no <base> navigation)
      await page.click('.hero-cta');
      await page.waitForTimeout(200);
      check('homescreen: hero CTA fragment stays on page', new URL(page.url()).pathname === '/homescreen/');
      await page.close();
    }

    // ---- Waitlist submit: success ----
    {
      const page = await browser.newPage();
      let captured = null;
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, async (route) => {
        captured = JSON.parse(route.request().postData() || '{}');
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ message: "You're on the list!" }) });
      });
      await page.goto(`${base}/homescreen/`, { waitUntil: 'networkidle' });
      await page.fill('#email', 'tester@example.com');
      await page.fill('#name', 'Tester McTest');
      await page.fill('#keeping_alive', 'A Saturday ride');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('success: payload has email', captured && captured.email === 'tester@example.com');
      check('success: optional name in payload', captured && captured.name === 'Tester McTest');
      check('success: textarea mapped to intendedUse', captured && captured.intendedUse === 'A Saturday ride');
      check('success: message shown', (await page.textContent('#formSuccess')).includes('list'));
      check('success: form hidden', !(await page.isVisible('#signupForm')));
      await page.close();
    }

    // ---- Waitlist submit: no server message -> injected default confirmation ----
    {
      const page = await browser.newPage();
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, (route) =>
        route.fulfill({ status: 200, contentType: 'application/json', body: '{}' }));
      await page.goto(`${base}/homescreen/`, { waitUntil: 'networkidle' });
      await page.fill('#email', 'nodefault@example.com');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('default: falls back to copy.md confirmation', (await page.textContent('#formSuccess')).includes('hand-pick the first groups'));
      await page.close();
    }

    // ---- Waitlist submit: already registered ----
    {
      const page = await browser.newPage();
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, (route) =>
        route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ alreadyRegistered: true, message: 'Already on the list.' }) }));
      await page.goto(`${base}/homescreen/`, { waitUntil: 'networkidle' });
      await page.fill('#email', 'again@example.com');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('alreadyRegistered: message shown', (await page.textContent('#formSuccess')).toLowerCase().includes('already'));
      await page.close();
    }

    // ---- Waitlist submit: server error surfaces to the user ----
    {
      const page = await browser.newPage();
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, (route) =>
        route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ message: 'Server is down.' }) }));
      await page.goto(`${base}/homescreen/`, { waitUntil: 'networkidle' });
      await page.fill('#email', 'err@example.com');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('error: message surfaced (no silent fail)', (await page.textContent('#formSuccess')).includes('Server is down'));
      check('error: form still visible for retry', await page.isVisible('#signupForm'));
      await page.close();
    }

    // ---- Crew page waitlist works too (shared handler) ----
    {
      const page = await browser.newPage();
      let captured = null;
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, async (route) => {
        captured = JSON.parse(route.request().postData() || '{}');
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ message: "You're on the list!" }) });
      });
      await page.goto(`${base}/crew/`, { waitUntil: 'networkidle' });
      check('crew: form present', !!(await page.$('#signupForm')));
      await page.fill('#signupForm input[name="email"]', 'crew@example.com');
      await page.fill('#signupForm input[name="name"]', 'Crew Member');
      await page.fill('#signupForm [name="keeping_alive"]', 'A Saturday ride');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('crew: payload has email', captured && captured.email === 'crew@example.com');
      check('crew: optional name in payload', captured && captured.name === 'Crew Member');
      check('crew: textarea mapped to intendedUse', captured && captured.intendedUse === 'A Saturday ride');
      check('crew: success shown', (await page.textContent('#formSuccess')).length > 0);
      await page.close();
    }

    // ---- Moms page renders + waitlist works (shared CSS + handler) ----
    {
      const page = await browser.newPage();
      let captured = null;
      await page.route(`**/*${WAITLIST_URL_FRAGMENT}`, async (route) => {
        captured = JSON.parse(route.request().postData() || '{}');
        await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ message: "You're on the list!" }) });
      });
      await page.goto(`${base}/moms/`, { waitUntil: 'networkidle' });
      check('moms: marketing.css applied (Libre Baskerville headline)', /Libre Baskerville/.test(await page.$eval('.headline', el => getComputedStyle(el).fontFamily)));
      check('moms: hero background loaded via data-bg', /\/moms\/background\/01\.jpg|background\/01\.jpg/.test(await page.$eval('.hero', el => getComputedStyle(el).backgroundImage)));
      await page.fill('#signupForm input[name="email"]', 'mom@example.com');
      await page.fill('#signupForm input[name="name"]', 'A Mom');
      await page.fill('#signupForm [name="keeping_alive"]', 'A playgroup');
      await page.click('#signupForm button[type="submit"]');
      await page.waitForSelector('#formSuccess.visible', { timeout: 5000 });
      check('moms: payload has email', captured && captured.email === 'mom@example.com');
      check('moms: optional name in payload', captured && captured.name === 'A Mom');
      check('moms: textarea mapped to intendedUse', captured && captured.intendedUse === 'A playgroup');
      await page.close();
    }
  } finally {
    await browser.close();
    server.close();
  }

  const failed = checks.filter((c) => !c.pass);
  console.log(`\n${checks.length - failed.length}/${checks.length} checks passed`);
  if (failed.length) {
    console.error(`FAILED: ${failed.map((c) => c.name).join(', ')}`);
    process.exit(1);
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
