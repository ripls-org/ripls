import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/add_option_card.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';

/// Foundation atoms for the v2 Volunteer / Archived sheets. Kept in
/// one file so the surface itself stays lean. Each atom is a small,
/// stateless presentation widget; behavior is owned by the parent.

/// ClaimCounter — small pill rendered under the volunteer sheet title.
/// "{N} OF {M} CLAIMED" with a sage dot when N > 0 and a
/// "· INCLUDING YOU" suffix when the current user has at least one
/// claim.
class NeedsClaimCounter extends StatelessWidget {
  final NeedsScopeKind scopeKind;
  final int claimed;
  final int total;
  final bool includingYou;

  const NeedsClaimCounter({
    super.key,
    required this.scopeKind,
    required this.claimed,
    required this.total,
    this.includingYou = false,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    final accent = AppColors.experienceSageGreen;
    final hasAny = claimed > 0;
    return Semantics(
      container: true,
      label: copy.volunteerCoverage(claimed, total),
      child: ExcludeSemantics(
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 6,
              height: 6,
              decoration: BoxDecoration(
                color: hasAny ? accent : AppColors.modalTextMuted,
                shape: BoxShape.circle,
              ),
            ),
            const SizedBox(width: 6),
            Text(
              '${claimed.toString()} OF ${total.toString()} CLAIMED',
              style: const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w700,
                letterSpacing: 0.8,
                color: AppColors.modalTextPrimary,
              ),
            ),
            if (includingYou) ...[
              const SizedBox(width: 6),
              Text(
                copy.volunteerCoverageIncludingYou,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.8,
                  color: accent,
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

/// SaveIndicator — quiet status pill rendered as the right half of
/// the bottom action row on NV1/NV2/NO1. Replaces the v1 disabled
/// "Close" CTA. Two states: idle ("Tap any item to claim — saves as
/// you go") and saved ("Saved · {N} of {M} covered").
class NeedsSaveIndicator extends StatelessWidget {
  final NeedsScopeKind scopeKind;
  final int claimedCount;
  final int totalCount;

  const NeedsSaveIndicator({
    super.key,
    required this.scopeKind,
    required this.claimedCount,
    required this.totalCount,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    final accent = AppColors.experienceSageGreen;
    final hasAny = claimedCount > 0;
    return Container(
      height: 52,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: BoxDecoration(
        color: hasAny
            ? accent.withValues(alpha: 0.10)
            : AppColors.modalInsetCardBg,
        border: Border.all(
          color: hasAny
              ? accent.withValues(alpha: 0.55)
              : AppColors.modalInsetCardBorder,
        ),
        borderRadius: BorderRadius.circular(16),
      ),
      alignment: Alignment.center,
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        mainAxisSize: MainAxisSize.min,
        children: [
          if (hasAny)
            Icon(Icons.check, size: 14, color: accent)
          else
            Container(
              width: 8,
              height: 8,
              decoration: const BoxDecoration(
                color: AppColors.modalTextMuted,
                shape: BoxShape.circle,
              ),
            ),
          const SizedBox(width: 8),
          Flexible(
            child: Text(
              hasAny
                  ? copy.volunteerSavedDone(claimedCount, totalCount)
                  : copy.volunteerSavedIdle,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: TextStyle(
                fontSize: 12.5,
                fontWeight: FontWeight.w600,
                color: hasAny ? accent : AppColors.modalTextMuted,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// AddOptionGhost — "Add something we're missing" affordance at the
/// bottom of every reply list. Opens the Picker for a single-item add.
/// Stays visible in edit mode too. Shares the compact [AddOptionCard]
/// shell with the location- and time-poll propose modals so all three
/// "add another option" affordances look the same.
class NeedsAddOptionGhost extends StatelessWidget {
  final NeedsScopeKind scopeKind;
  final VoidCallback? onTap;

  const NeedsAddOptionGhost({
    super.key,
    required this.scopeKind,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    return Padding(
      padding: const EdgeInsets.only(top: 4),
      child: AddOptionCard(
        label: copy.addOptionGhost,
        semanticsLabel: l10n.a11yNeedsAddOptionGhost,
        icon: Icons.add,
        accent: AppColors.experienceSageGreen,
        onTap: onTap,
      ),
    );
  }
}

/// EditDoneButton — 52px toggle. Inactive = "Edit" with pencil glyph;
/// active = sage-filled "Done".
class NeedsEditDoneButton extends StatelessWidget {
  final bool active;
  final VoidCallback onTap;

  const NeedsEditDoneButton({
    super.key,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: l10n.a11yNeedsEditToggle,
      onTap: onTap,
      child: Container(
        height: 52,
        padding: const EdgeInsets.symmetric(horizontal: 18),
        decoration: BoxDecoration(
          color: active ? accent : AppColors.modalInsetCardBg,
          border: active
              ? null
              : Border.all(color: AppColors.modalInsetCardBorder),
          borderRadius: BorderRadius.circular(16),
          boxShadow: active
              ? [
                  BoxShadow(
                    color: accent.withValues(alpha: 0.30),
                    offset: const Offset(0, 8),
                    blurRadius: 20,
                  ),
                ]
              : null,
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            if (!active) ...[
              const Icon(
                Icons.edit,
                size: 14,
                color: AppColors.modalTextPrimary,
              ),
              const SizedBox(width: 7),
            ],
            Text(
              active ? l10n.needsDoneButton : l10n.needsEditButton,
              style: TextStyle(
                fontSize: 14.5,
                fontWeight: FontWeight.w700,
                color: active
                    ? const Color(0xFF0F1A14)
                    : AppColors.modalTextPrimary,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// EditModeBanner — sage-tinted banner shown above the list while edit
/// mode is active. Two variants: organizer ("Editing — tap any item
/// to change it") and member ("You can edit things you added or
/// claimed. Others are read-only.").
class NeedsEditModeBanner extends StatelessWidget {
  final NeedsScopeKind scopeKind;
  final bool isOrganizer;

  const NeedsEditModeBanner({
    super.key,
    required this.scopeKind,
    required this.isOrganizer,
  });

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    final accent = AppColors.experienceSageGreen;
    return Container(
      margin: const EdgeInsets.only(top: 10, bottom: 4),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 9),
      decoration: BoxDecoration(
        color: accent.withValues(alpha: 0.12),
        border: Border.all(color: accent.withValues(alpha: 0.45)),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Icon(Icons.edit, size: 14, color: accent),
          const SizedBox(width: 9),
          Expanded(
            child: Text(
              isOrganizer
                  ? copy.editBannerOrganizer
                  : copy.editBannerMember,
              style: const TextStyle(
                fontSize: 12,
                color: AppColors.modalTextPrimary,
                height: 1.4,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Full-width "Manage" pill — the organizer-side counterpart to
/// [NeedsEditDoneButton]. Forwarded from v1; kept for parity with the
/// time-poll vote modal's Manage affordance.
class NeedsManagePillButton extends StatelessWidget {
  final VoidCallback onTap;
  final String label;
  final String semanticsLabel;

  const NeedsManagePillButton({
    super.key,
    required this.onTap,
    required this.label,
    required this.semanticsLabel,
  });

  @override
  Widget build(BuildContext context) {
    return Tappable(
      semanticsLabel: semanticsLabel,
      onTap: onTap,
      child: Container(
        height: 52,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        decoration: BoxDecoration(
          color: AppColors.modalInsetCardBg,
          border: Border.all(color: AppColors.modalInsetCardBorder),
          borderRadius: BorderRadius.circular(16),
        ),
        alignment: Alignment.center,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(
              Icons.more_horiz,
              size: 18,
              color: AppColors.modalTextPrimary,
            ),
            const SizedBox(width: 8),
            Text(
              label,
              style: const TextStyle(
                fontSize: 14.5,
                fontWeight: FontWeight.w700,
                color: AppColors.modalTextPrimary,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

