# Search widgets

Small shared affordances for search surfaces — the universal search
overlay, the Feed search pill, the Library (Discover) results header,
and the dock tabs while they're scoped to a search.

## Key files

- `close_search_button.dart` — circular ✕ used at the trailing edge of
  search pill surfaces, shared so the affordance reads the same
  everywhere.
- `search_scope_pill.dart` — the search-bar remnant a dock tab (Plans /
  Library / People) shows while filtered to a universal-search query;
  its ✕ cancels the scope and restores the full tab.
- `search_nearby_pill.dart` — the map's "search this area" pill.

## When to add code here

Add widgets that are shared across two or more search surfaces. A
widget owned by a single screen (e.g. the universal search overlay's
result cards) belongs next to that screen instead.
