# profile_section

Surface-agnostic widgets that compose the "profile-shaped" section
stack shared by the person profile
([user_screen.dart](../../screens/users/user_screen.dart)) and the
community profile
([community_public_screen.dart](../../screens/communities/community_public_screen.dart)).
Both screens render the v11 synthesis chassis
(`docs/cowork/App UXR/group-profile-v11-synthesis.html` /
`user-profile-v11-synthesis.html`): the entity's photo fills the top
~46% under a scrim dissolving into the dark page — or the monogram
masthead when there is no photo (same body, zero layout shift) — then
one scrolling column: identity (kicker · serif name · tagline), the
single member/shared-groups row, clamped tag chips expanding in place,
the live-pulse pill, the inline circular action row, one editorial
hero stat (Time together + serif equivalence), and the quiet ledger
with plain-language subtexts. Rows with a destination carry a chevron
and expand in place via the morph-reveal panel.

The section structure originated with the v4 profile redesign
([docs/issues/1929-profile-v4-redesign.md](../../../../../docs/issues/1929-profile-v4-redesign.md));
the current hybrid layout is
[docs/issues/2568-directory-and-profiles.md](../../../../../docs/issues/2568-directory-and-profiles.md).

## Files

| File | Section | Notes |
|------|---------|-------|
| `profile_backdrop.dart` | Backdrop (photo / masthead) | `ProfileHeroBackdrop` — the photo filling the top ~46% under the dissolve scrim; `ProfileMastheadBackdrop` — the accent-tinted no-photo gradient; `ProfileMonogram` — the initials tile (squared for groups, circular for people). |
| `profile_hero.dart` | Identity block | Accent eyebrow, serif name, common-ground sentence (clamped to three lines with tap-to-expand when it overflows). No avatar — the backdrop photo/monogram is the identity. No relationship/member line — that's `profile_member_row.dart` now. |
| `profile_chips.dart` | Tag chips | Interests (group) / signature tags (person), clamped to five with "+N more" expanding **in place** and "show less" collapsing back. |
| `profile_pulse_pill.dart` | Live-pulse pill | Green-dot chip carrying the freshest sign of life (the next shared gathering); hosts hide it when nothing is live. |
| `profile_action_row.dart` | Inline action row | 46px circular icon buttons (Message primary · Plans · Library); each expands its destination in place via the morph-reveal panel. Disabled buttons render dimmed and inert. |
| `profile_hero_stat.dart` | Hero stat | The one editorial stat moment: kicker + big serif number + plain-language subtext + serif equivalence sentence; chevron + tap only when a drill-down is wired. |
| `profile_rename_sheet.dart` | Edit-name sheet | The overflow menu's rename flow: one prefilled field on the opaque `SolidSheet`, popping the trimmed new name; persistence stays with the calling screen. |
| `profile_member_row.dart` | Members/relationship row | Active-ask state only, between the hero and the metric rows: a bigger face stack (`ProfileFaceStack`, size 30) beside a bold name line + muted subtitle (shared interests for a person, "All members · N · specialties" for a group). Tappable only when `onTap` is given (group opens the Members tab; person has nowhere to route to). |
| `profile_metric_rows.dart` | Ledger rows | One hairline-separated row per metric: bold name over plain-language subtext left, serif value right, chevron + tap into the metric detail when wired. Consumes `MetricTileData`. |
| `open_now_list.dart` | Everything open | Vertical list of open `Item`s (events / gear / giveaways / asks) with per-row ghost verb buttons derived from `ItemKind`. `openNowVerbFor` maps kind → verb. Rendered inside the pinned sheet's expanded body (not the screen body). Empty list ⇒ collapsed. |
| `profile_face_stack.dart` | Face stack | Overlapping mini avatars + "+N" tail for social-pull rows ("Alfred +2 in"). |
| `metric_tile_grid.dart` | 2×2 metric tiles | The pre-hybrid grid; still exports `MetricTileData`, which the ledger rows consume. |
| `profile_members_panel.dart` | Members panel | The morph-reveal destination for the members row: the full member list for a group. |
| `profile_conversation_panel.dart` | Conversation panel | The morph-reveal destination for the Message action: the thread rendered in place. |
| `impact_row_narrative_tiers.dart` | Tier narrative helper | `timeGivenNarrative` composes a tier description from the community-impact health ladder. |

## When to add code here

- A new section that both surfaces will render → add a new file here.
- Touch-up to an existing section → edit the file in place.
- Surface-specific identity or routing → add it next to the surface's
  screen, not here.

## Conventions

- **Colors:** `AppColors.xxx(context)` from
  [app_colors.dart](../../../core/theme/app_colors.dart).
- **Typography:** `AppTheme.headingFont` (Libre Baskerville) for serif
  display copy; omit `fontFamily` for sans body copy.
- **Strings:** `context.l10n.userProfile*` for visible copy,
  `context.l10n.a11yProfile*` for screen-reader labels. The
  `userProfile` ARB prefix is retained (the strings predate the
  widget hoist) and is shared across surfaces — both surfaces resolve
  to the same locale-correct values.
- **Accessibility:** taps via `Tappable`; thumbnails via
  `CachedMediaImage(semanticsLabel: …)`.
- **Empty states:** widgets gate themselves with `SizedBox.shrink()`
  so the host screen does not need conditional rendering branches.
