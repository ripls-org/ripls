# Portfolio Screens

The Home tab (portfolio inbox) and the Plans calendar.

## Purpose

These screens assemble the viewer's personal cross-community view: the Home
tab's "Needs you" decision queue and community pulse, and the Plans
calendar. The server-assembled Home view is documented in
[`docs/client/inbox.md`](../../../../../docs/client/inbox.md); the calendar in
[`docs/client/calendar.md`](../../../../../docs/client/calendar.md).

## Key Files

### Home tab (#2435 / #2634)
- **`home_tab_screen.dart`** — the Home root: greeting header, "Needs you"
  section, and the community-pulse feed.
- **`home_needs_you_see_all_screen.dart`** — the full "Needs you" decision
  queue, grouped by kind.
- **`home_decision_routing.dart`** — mixin shared by the root and see-all
  screen for opening an item and running a decision's action.
- **`home_calendar_screen.dart`** — the month × backdrop calendar, also
  instantiated as the top-level **Plans** dock destination.

## When to add here vs. elsewhere

Home/portfolio/calendar screens belong here. Home-tab row/card widgets live in
`widgets/home/`. The screens that explain how an impact metric was derived
are in `screens/impact_metrics/`.
