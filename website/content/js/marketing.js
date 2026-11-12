/*
  marketing.js — shared behavior for the full-bleed marketing pages.

  Three things, all of which used to be copy-pasted inline on every page:

  1. Background loader — sections declare `data-bg="NN"` and the loader
     resolves `NN.{jpg,jpeg,png,webp}` against the page's background folder.
     The folder defaults to `background/` relative to the page; a page whose
     assets live somewhere other than its own subpath overrides it with an
     absolute base, e.g. `<body data-bg-base="/homescreen/background/">`
     (`homescreen/` kept its absolute paths from when it was served at `/`).
     If no file loads, the
     section keeps its dark fallback color and overlay, so the layout never
     collapses.

  2. Fade-in on scroll — sections get `.visible` as they enter the viewport;
     the hero is visible immediately.

  3. Phone video fallback — if a phone-frame video errors (asset missing,
     decode failure), it is replaced with its poster image so the frame is
     never empty.
*/

(function () {
  // ---------- Background image loader ----------
  const BG_BASE = document.body.dataset.bgBase || 'background/';
  const EXTENSIONS = ['jpg', 'jpeg', 'png', 'webp'];

  function loadBackground(section) {
    const num = section.dataset.bg;
    if (!num) return;
    let i = 0;
    (function tryNext() {
      if (i >= EXTENSIONS.length) return;
      const url = `${BG_BASE}${num}.${EXTENSIONS[i]}`;
      const img = new Image();
      img.onload = () => { section.style.backgroundImage = `url('${url}')`; };
      img.onerror = () => { i++; tryNext(); };
      img.src = url;
    })();
  }
  document.querySelectorAll('.section').forEach(loadBackground);

  // ---------- Fade-in on scroll ----------
  const io = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      if (entry.isIntersecting) {
        entry.target.classList.add('visible');
        io.unobserve(entry.target);
      }
    });
  }, { threshold: 0.18, rootMargin: '0px 0px -8% 0px' });

  document.querySelectorAll('.section').forEach(s => io.observe(s));

  const hero = document.querySelector('.hero');
  if (hero) hero.classList.add('visible');

  // ---------- Phone video fallback ----------
  document.querySelectorAll('.phone-frame video').forEach(v => {
    v.addEventListener('error', () => {
      const poster = v.getAttribute('poster');
      if (!poster) return;
      const img = document.createElement('img');
      img.src = poster;
      img.alt = v.getAttribute('aria-label') || '';
      v.replaceWith(img);
    });
  });
})();
