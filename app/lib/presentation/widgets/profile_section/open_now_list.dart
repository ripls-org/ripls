import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/data/gen/ripls/api/item.pb.dart';
import 'package:ripls/l10n/app_localizations.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';

/// The vertical "OPEN RIGHT NOW" list on the person and community
/// profiles (issue #2568, `profile-final-hybrid-v2.html`): a kicker plus
/// up to [maxRows] open items — joinable events, borrowable gear,
/// giveaways, open asks — each with a title, an optional context
/// subline, and a trailing ghost verb button named for the action the
/// viewer can take.
///
/// Renders over the profile's full-bleed background photo, so all text
/// is light. `SizedBox.shrink()` when [items] is empty.
class OpenNowList extends StatelessWidget {
  const OpenNowList({
    super.key,
    required this.items,
    this.maxRows,
    this.kicker,
    this.onOpenItem,
    this.horizontalPadding = 24,
  });

  final List<Item> items;

  /// Horizontal inset for the kicker and rows. Hosts that already pad
  /// their content (the expanded profile sheet) pass a smaller value.
  final double horizontalPadding;

  /// Cap on rendered rows; null renders every item (the expanded-sheet
  /// variant).
  final int? maxRows;

  /// Uppercased header above the rows; null renders no header.
  final String? kicker;

  /// Tap routing for a row and its verb button. Wired by the host
  /// screen to the entity's detail surface, where the real commit
  /// action lives.
  final void Function(Item item)? onOpenItem;

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) return const SizedBox.shrink();
    final visible =
        maxRows == null ? items : items.take(maxRows!).toList(growable: false);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (kicker != null && kicker!.isNotEmpty)
          Padding(
            padding: EdgeInsets.fromLTRB(horizontalPadding, 16, horizontalPadding, 2),
            child: Text(
              kicker!.toUpperCase(),
              style: TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                letterSpacing: 1.3,
                color: Colors.white.withValues(alpha: 0.7),
              ),
            ),
          ),
        Padding(
          padding: EdgeInsets.symmetric(horizontal: horizontalPadding),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              for (var i = 0; i < visible.length; i++)
                _OpenRow(
                  item: visible[i],
                  isLast: i == visible.length - 1,
                  onTap: onOpenItem == null
                      ? null
                      : () => onOpenItem!(visible[i]),
                ),
            ],
          ),
        ),
      ],
    );
  }
}

/// The ghost verb for an item kind — the action the viewer can take on
/// the row (Join an event, Borrow gear, Claim a giveaway, commit to an
/// ask).
String openNowVerbFor(AppLocalizations l10n, ItemKind kind) {
  switch (kind) {
    case ItemKind.ITEM_KIND_EXPERIENCE:
      return l10n.openNowVerbJoin;
    case ItemKind.ITEM_KIND_GEAR:
      return l10n.openNowVerbBorrow;
    case ItemKind.ITEM_KIND_GIVEAWAY:
      return l10n.openNowVerbClaim;
    case ItemKind.ITEM_KIND_REQUEST:
      return l10n.openNowVerbImIn;
    default:
      return l10n.openNowVerbView;
  }
}

class _OpenRow extends StatelessWidget {
  const _OpenRow({required this.item, required this.isLast, this.onTap});

  final Item item;
  final bool isLast;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final subtitle = item.hasSubtitle() ? item.subtitle : '';
    final verb = openNowVerbFor(l10n, item.kind);
    return Container(
      decoration: BoxDecoration(
        border: isLast
            ? null
            : Border(
                bottom: BorderSide(
                  color: Colors.white.withValues(alpha: 0.08),
                ),
              ),
      ),
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        children: [
          Expanded(
            child: Tappable(
              semanticsLabel: l10n.a11yOpenNowRow(verb, item.title),
              onTap: onTap,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    item.title,
                    maxLines: 1,
                    overflow: TextOverflow.ellipsis,
                    style: const TextStyle(fontSize: 13, color: Colors.white),
                  ),
                  if (subtitle.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                      style: TextStyle(
                        fontSize: 11.5,
                        color: Colors.white.withValues(alpha: 0.7),
                      ),
                    ),
                  ],
                ],
              ),
            ),
          ),
          const SizedBox(width: 12),
          _GhostVerbButton(
            label: verb,
            semanticsLabel: l10n.a11yOpenNowRow(verb, item.title),
            onTap: onTap,
          ),
        ],
      ),
    );
  }
}

/// Pill-shaped ghost button. The pill is compact visually but sits
/// centered inside a ≥48px-tall tap area.
class _GhostVerbButton extends StatelessWidget {
  const _GhostVerbButton({
    required this.label,
    required this.semanticsLabel,
    this.onTap,
  });

  final String label;
  final String semanticsLabel;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        constraints: const BoxConstraints(minHeight: 48, minWidth: 48),
        alignment: Alignment.center,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
          decoration: BoxDecoration(
            color: Colors.white.withValues(alpha: 0.09),
            borderRadius: BorderRadius.circular(999),
            border: Border.all(color: Colors.white.withValues(alpha: 0.28)),
          ),
          child: Text(
            label,
            style: TextStyle(
              fontSize: 12,
              fontWeight: FontWeight.w600,
              color: Colors.white.withValues(alpha: 0.92),
            ),
          ),
        ),
      ),
    );
  }
}
