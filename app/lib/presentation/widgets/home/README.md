# Home tab widgets

Section widgets for the Home tab (#2435, #2634 v3) — the redesigned second
tab rendered by `screens/portfolio/home_tab_screen.dart` from a single
`GetHomeViewResponse`.

## Key files

- `community_pulse_section.dart` — the "In your communities" block: the
  feed rendered as an editorial pulse section.
- `needs_you_group_row.dart` — the editorial Needs-you row shown under
  each kind group: 48px thumbnail/avatar, two-line serif title, muted
  subtitle, and the row's action affordance.
- `home_editorial_row.dart` — the simplified inbox list row shared by
  Needs-you and Yours: serif title, muted subtitle, trailing affordance.
- `face_stack.dart` — row of overlapping circular avatars for the people
  behind an aggregated Home row (e.g. the helpers who pitched in on a
  request).
- `home_empty_states.dart` — the new-user hero and the all-caught-up calm
  card.
- `home_media_thumb.dart` — shared CachedMediaImage thumbnail loader.

## When to add code here vs. adjacent directories

Widgets here render `GetHomeViewResponse` data for the Home tab only.
Content-screen widgets live in `../content/`. If a widget is reused
outside the Home tab, promote it out of this directory.
