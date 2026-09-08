import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_chip.dart';
import 'package:ripls/presentation/widgets/needs/needs_actions.dart';

/// NeedClaimChip is the shared pill used to surface a need, contribution, or
/// suggestion across the experience surfaces (the RSVP composer's "Bringing"
/// row and the pitching-in roster). It renders as a [GlassChip] so every
/// surface looks identical, and tapping opens [NeedsActions.openClaimSheet] for
/// [title] — the "Claim a need" sheet when the viewer isn't bringing the item
/// yet, or the "Edit your contribution" variant when they already are.
class NeedClaimChip extends ConsumerWidget {
  /// The action handler carrying the [NeedsScope] for the surrounding surface.
  final NeedsActions actions;

  /// Item name shown on the chip and used as the claim-sheet title.
  final String title;

  /// Optional note carried into the claim sheet (a need's note or a
  /// contributor's comment).
  final String? note;

  /// Optional name of the person who proposed the item, shown in the claim
  /// sheet's status badge.
  final String? proposerName;

  /// Whether the chip renders in its selected (solid) state — used by the RSVP
  /// composer to show items the viewer is already bringing.
  final bool selected;

  /// When the item is an open need, its id + slot count so a fresh claim
  /// reserves a slot (rather than recording a free-form contribution).
  final String? linkedNeedId;

  /// Total slots on the linked need; ignored when [linkedNeedId] is null.
  final int slotsNeeded;

  /// Quantity shown as a "×N" badge when greater than 1 (e.g. how many of the
  /// item are being brought, or how many slots a need has).
  final int quantity;

  const NeedClaimChip({
    super.key,
    required this.actions,
    required this.title,
    this.note,
    this.proposerName,
    this.selected = false,
    this.linkedNeedId,
    this.slotsNeeded = 1,
    this.quantity = 1,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return GlassChip(
      primary: title,
      selected: selected,
      semanticsLabel: title,
      quantity: quantity,
      hasComment: note != null && note!.trim().isNotEmpty,
      onTap: () => actions.openClaimSheet(
        context,
        ref,
        name: title,
        note: note,
        proposerName: proposerName,
        linkedNeedId: linkedNeedId,
        slotsNeeded: slotsNeeded,
      ),
    );
  }
}
