import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/navigation_helpers.dart';
import 'package:ripls/data/gen/ripls/api/gear_service.pb.dart'
    show GetGearResponse;
import 'package:ripls/data/repositories/gear_repository.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/chat/cached_media_image.dart';
import 'package:ripls/services/providers.dart'
    show gearRepositoryProvider, mediaUrlProvider;

/// GearPill renders an inline chip representing a linked Gear item.
///
/// The pill resolves the gear by ID through [gearRepositoryProvider]. When
/// the gear cannot be resolved (deleted, missing, or not in the user's
/// access scope) the pill renders as a dim "removed item" tombstone with
/// an explicit semantics annotation so it remains discoverable to screen
/// readers without relying on color alone.
///
/// Used by the Needs/Contributions claim row and the read-only Archived
/// sheet to surface the gear a contributor linked to their claim.
class GearPill extends ConsumerWidget {
  /// Identifier of the Gear item to render.
  final String gearId;

  /// Optional community context for the gear lookup. Forwarded to
  /// [GearRepository.get] so community-scoped permissions resolve.
  final String? communityId;

  const GearPill({super.key, required this.gearId, this.communityId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final repo = ref.watch(gearRepositoryProvider);
    return FutureBuilder<GetGearResponse>(
      future: repo.get(gearId, communityId: communityId),
      builder: (context, snapshot) {
        if (snapshot.connectionState == ConnectionState.waiting) {
          return _PillShell(
            label: context.l10n.a11yGearPillLoading,
            child: const SizedBox(height: 22, width: 60),
          );
        }
        final gear = snapshot.data;
        if (gear == null || gear.deleted || gear.id.isEmpty) {
          return _TombstonePill(
            label: context.l10n.a11yGearPillRemoved,
            text: context.l10n.needsClaimGearRemovedTombstone,
          );
        }
        return _GearPillBody(
          gear: gear,
          semanticsLabel: context.l10n.a11yGearPill(gear.name),
          onOpen: () => NavigationHelpers.pushToItemScreen(
            context: context,
            itemId: gear.id,
            itemType: 'gear',
          ),
        );
      },
    );
  }
}

/// Inline underlined-text variant of [GearPill] used on the
/// contributors line of a need row. Reads "bringing YETI Tundra
/// Cooler" instead of carrying the chip's thumbnail + border —
/// preserves tap-to-open semantics but reads as continuous prose
/// with the surrounding name + count.
class GearLink extends ConsumerWidget {
  /// Identifier of the Gear item to render.
  final String gearId;

  /// Optional community context for the gear lookup. Forwarded to
  /// [GearRepository.get] so community-scoped permissions resolve.
  final String? communityId;

  /// Style applied to the gear name. Underline + weight overrides
  /// merge on top so the link reads like a hyperlink against the
  /// surrounding contributor text.
  final TextStyle? style;

  const GearLink({
    super.key,
    required this.gearId,
    this.communityId,
    this.style,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final repo = ref.watch(gearRepositoryProvider);
    final base = (style ?? const TextStyle()).copyWith(
      fontWeight: FontWeight.w600,
      decoration: TextDecoration.underline,
      decorationColor: AppColors.modalTextPrimary,
      decorationThickness: 1.5,
      color: AppColors.modalTextPrimary,
    );
    return FutureBuilder<GetGearResponse>(
      future: repo.get(gearId, communityId: communityId),
      builder: (context, snapshot) {
        if (snapshot.connectionState == ConnectionState.waiting) {
          return Text(
            context.l10n.a11yGearPillLoading,
            style: base.copyWith(
              decoration: TextDecoration.none,
              color: AppColors.modalTextMuted,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          );
        }
        final gear = snapshot.data;
        if (gear == null || gear.deleted || gear.id.isEmpty) {
          return Text(
            context.l10n.needsClaimGearRemovedTombstone,
            style: base.copyWith(
              decoration: TextDecoration.lineThrough,
              color: AppColors.modalTextMuted,
            ),
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          );
        }
        return Tappable(
          semanticsLabel: context.l10n.a11yGearPill(gear.name),
          onTap: () => NavigationHelpers.pushToItemScreen(
            context: context,
            itemId: gear.id,
            itemType: 'gear',
          ),
          child: Text(
            gear.name,
            style: base,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
          ),
        );
      },
    );
  }
}

class _GearPillBody extends ConsumerWidget {
  final GetGearResponse gear;
  final String semanticsLabel;
  final VoidCallback onOpen;

  const _GearPillBody({
    required this.gear,
    required this.semanticsLabel,
    required this.onOpen,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onOpen,
      inkBorderRadius: BorderRadius.circular(999),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          color: AppColors.cardBackground(context),
          borderRadius: BorderRadius.circular(999),
          border: Border.all(color: AppColors.border(context), width: 1),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _Thumb(gear: gear),
            const SizedBox(width: 6),
            Flexible(
              child: Text(
                gear.name,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w600,
                  color: AppColors.textPrimary(context),
                ),
                overflow: TextOverflow.ellipsis,
                maxLines: 1,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _Thumb extends ConsumerWidget {
  final GetGearResponse gear;

  const _Thumb({required this.gear});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    if (gear.mediaIds.isEmpty) {
      return Container(
        width: 18,
        height: 18,
        decoration: BoxDecoration(
          color: AppColors.surface(context),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Icon(
          Icons.inventory_2_outlined,
          size: 12,
          color: AppColors.textTertiary(context),
        ),
      );
    }
    final mediaId = gear.mediaIds.first;
    final urlAsync = ref.watch(mediaUrlProvider(mediaId));
    return urlAsync.when(
      loading: () => SizedBox(
        width: 18,
        height: 18,
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: AppColors.surface(context),
            borderRadius: BorderRadius.circular(4),
          ),
        ),
      ),
      error: (_, _) => SizedBox(
        width: 18,
        height: 18,
        child: DecoratedBox(
          decoration: BoxDecoration(
            color: AppColors.surface(context),
            borderRadius: BorderRadius.circular(4),
          ),
        ),
      ),
      data: (mediaUrl) => SizedBox(
        width: 18,
        height: 18,
        // semanticsLabel: null — the gear's name is already announced
        // by the enclosing Tappable's semantics label, so the
        // thumbnail is decorative here.
        child: CachedMediaImage(
          imageUrl: mediaUrl,
          cacheKey: 'gear-pill:$mediaId',
          fit: BoxFit.cover,
          borderRadius: BorderRadius.circular(4),
          semanticsLabel: null,
        ),
      ),
    );
  }
}

class _PillShell extends StatelessWidget {
  final String label;
  final Widget child;

  const _PillShell({required this.label, required this.child});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      child: ExcludeSemantics(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(
            color: AppColors.surface(context),
            borderRadius: BorderRadius.circular(999),
          ),
          child: child,
        ),
      ),
    );
  }
}

class _TombstonePill extends StatelessWidget {
  final String label;
  final String text;

  const _TombstonePill({required this.label, required this.text});

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      child: ExcludeSemantics(
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
          decoration: BoxDecoration(
            color: AppColors.surface(context).withValues(alpha: 0.55),
            borderRadius: BorderRadius.circular(999),
            border: Border.all(
              color: AppColors.border(context).withValues(alpha: 0.55),
              width: 1,
            ),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(
                Icons.help_outline,
                size: 14,
                color: AppColors.textTertiary(context),
              ),
              const SizedBox(width: 4),
              Text(
                text,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textTertiary(context),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
