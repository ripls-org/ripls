# Trademark policy

The Apache-2.0 license in [`LICENSE`](LICENSE) grants you broad rights to the
**code**. It deliberately does not grant rights to the **name and the marks** —
section 6 of that license excludes trademarks explicitly. This page says what
that means in practice, because the honest answer is short and we would rather
state it than leave you guessing.

## What the marks are

- The name **Ripls**.
- The Ripls logo and wordmark, including `assets/ripls_logo*.svg` and the logo
  used in transactional email.
- The application icon set: `app/assets/icon/**`, the launcher and notification
  icons under `app/android/app/src/main/res/**`, the iOS and macOS app icons
  under `app/ios/Runner/Assets.xcassets/**` and `app/macos/Runner/Assets.xcassets/**`,
  and the web icons under `app/web/icons/**`.
- The `RiplsIcons` glyph fonts (`app/fonts/RiplsIcons.ttf`, `RiplsIcons1.ttf`).

## What you can do

- Use, modify, and redistribute the code under Apache-2.0, commercially or not.
- Run your own instance, for any purpose, including a competing one.
- Say truthfully that your work is *based on*, *derived from*, or *compatible
  with* Ripls. Nominative reference like that needs no permission.

## What you need to change before you ship

If you distribute a build to other people — an app store, a hosted service, a
downloadable binary — replace the marks first:

- **App icons and display names.** `app/android/app/build.gradle.kts` sets the
  application ID and `app_name`; the iOS and macOS display names live in
  `app/ios/Flutter/*.xcconfig` and `app/macos/Runner/Configs/AppInfo.xcconfig`.
- **Bundle and package identifiers.** Ship under your own, not `org.ripls.*`.
  Publishing under ours is not something we can permit even if we wanted to —
  the store accounts are tied to the identifiers.
- **The logo and the wordmark** anywhere they appear in your UI, your website,
  or your email templates.
- **The `RiplsIcons` fonts**, if you are re-branding rather than contributing.

The short version: don't ship something that looks like it came from us. A user
should never have to guess whether they are running the Ripls project's build or
yours — particularly for a product that handles who-knows-whom relationships in
someone's neighborhood.

## What we ask you not to do

- Use the name or marks as your product name, your organization's name, or in a
  domain name, in a way that suggests affiliation or endorsement.
- Modify the marks and keep calling them Ripls.
- Use the marks in a way that implies we vouch for your fork's security,
  privacy, or conduct.

## Questions, and permission

If you want to use the marks in a way this page does not cover — a conference
talk, a compatibility badge, a community meetup — just ask. The answer is
usually yes, and we would rather say yes explicitly than have you self-censor a
reasonable use. See [`SECURITY.md`](SECURITY.md) for how to reach us.

This policy is about avoiding confusion, not about restricting the code. If a
requirement here ever seems to be limiting what you can build rather than
protecting users from being misled, that is a bug in this page — tell us.
