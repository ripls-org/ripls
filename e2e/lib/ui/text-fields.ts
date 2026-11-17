// e2e/lib/ui/text-fields.ts — locate the app's real text fields.
//
// Flutter Web keeps two kinds of <input> in the page:
//
//   - the field a person sees and types into, inside `<flt-semantics-host>`;
//   - since Flutter 3.50, zero-sized placeholder inputs in an autofill <form>
//     under `<flt-text-editing-host>`. The engine adds one for every field that
//     carries `autofillHints` (phone, email, password, one-time code, name) as
//     soon as a field in its group is focused, and leaves the form in the page
//     after the field loses focus so a password manager can still fill or save
//     the group.
//
// The placeholders have the textbox role and come FIRST in document order, so a
// page-wide `getByRole('textbox').first()`, `.nth(0)` or
// `locator('input[type="password"]').first()` resolves to an invisible
// placeholder once any such field has been touched: clicks wait for it to
// become visible until the test times out, and keystrokes sent to it never
// reach the app. Scope to the semantics host instead.

import { type Locator, type Page } from '@playwright/test';

/** The semantics tree: every element in it is one the app itself rendered. */
export function appSemantics(page: Page): Locator {
  return page.locator('flt-semantics-host');
}

/** The app's own text fields, in document order, without autofill placeholders. */
export function appTextFields(page: Page): Locator {
  return appSemantics(page).getByRole('textbox');
}
