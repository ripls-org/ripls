// waitlist.js — shared signup handler for the marketing pages (homescreen + crew).
//
// Both pages have a <form id="signupForm"> and a #formSuccess element. The form
// always has an email input (required) and may also have an optional name input
// and a "keeping_alive" textarea. Fields are read by name, so the same handler
// works for either page's markup; missing optional fields are simply omitted
// from the request.
//
// The confirmation copy is authored in #formSuccess (injected from copy.md) and
// used as the fallback shown when the server returns no message. Errors are
// surfaced to the user (never silently swallowed).
(function () {
  const WAITLIST_API_URL =
    'https://server.ripls.org/ripls.api.WaitlistService/JoinWaitlist';

  const form = document.getElementById('signupForm');
  const success = document.getElementById('formSuccess');
  if (!form || !success) return;

  // Capture the authored confirmation text, then clear it so the aria-live
  // region announces when we set it on submit.
  const defaultSuccess = success.textContent.trim();
  success.textContent = '';

  // Read a control's trimmed value by name. Uses querySelector rather than
  // form.<name> so a field named "name" doesn't collide with HTMLFormElement.name.
  const field = (name) => {
    const el = form.querySelector(`[name="${name}"]`);
    return el ? el.value.trim() : '';
  };

  form.addEventListener('submit', async (e) => {
    e.preventDefault();
    const button = form.querySelector('button[type="submit"]');
    const original = button.textContent;
    button.textContent = 'Joining…';
    button.disabled = true;
    success.classList.remove('visible');

    const payload = { email: field('email') };
    const name = field('name');
    if (name) payload.name = name;
    const note = field('keeping_alive');
    if (note) payload.intendedUse = note;

    try {
      const res = await fetch(WAITLIST_API_URL, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) {
        throw new Error(data.message || 'Something went wrong. Please try again.');
      }
      form.style.display = 'none';
      success.textContent =
        data.message ||
        (data.alreadyRegistered
          ? "You're already on the list — we've got you."
          : defaultSuccess);
      success.classList.add('visible');
    } catch (err) {
      button.textContent = original;
      button.disabled = false;
      success.textContent = err.message || 'Something went wrong. Please try again.';
      success.classList.add('visible');
    }
  });
})();
