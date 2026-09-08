import 'package:flutter/material.dart';
import 'package:ripls/core/extensions/l10n_extensions.dart';
import 'package:ripls/core/theme/app_colors.dart';
import 'package:ripls/core/utils/toast_helper.dart';
import 'package:ripls/presentation/viewmodels/needs_scope.dart';
import 'package:ripls/presentation/widgets/accessibility/show_accessible_modal.dart';
import 'package:ripls/presentation/widgets/accessibility/tappable.dart';
import 'package:ripls/presentation/widgets/modal/glass/glass_sheet.dart';
import 'package:ripls/presentation/widgets/needs/needs_claim_row.dart';
import 'package:ripls/presentation/widgets/needs/needs_copy.dart';
import 'package:ripls/presentation/widgets/needs/needs_volunteer_atoms.dart';
import 'package:ripls/presentation/widgets/needs/needs_volunteer_sheet.dart';

/// Archived sheet — NA in v2. Surfaced when the parent entity (an
/// Experience or a Request) is in its terminal state. Uses the same
/// row composition as the Volunteer sheet — same numbering, voter
/// stacks, count lines — but everything is read-only: no Edit chip,
/// no Manage button, no AddOptionGhost, no claim/unclaim taps.
/// Uncovered rows dim to 0.55 opacity so the read order favors what
/// actually showed up. The footer is a single "Save" CTA that the
/// caller hooks to whatever archive-persistence we eventually ship
/// (v2 ships it as a no-op toast — see deferred work in
/// `docs/client/needs.md`).
class NeedsArchivedSheet extends StatelessWidget {
  /// Scope flavour — picks Experience vs. Request copy via [NeedsCopy].
  final NeedsScopeKind scopeKind;

  final List<NeedsVolunteerEntry> entries;

  /// Optional "Event ended · {age}" suffix (e.g. "2 days ago"). When
  /// null only the eyebrow stem renders.
  final String? endedAgo;

  /// Called when the user taps the Save CTA. The caller is
  /// responsible for archiving the list to the user's records and
  /// dismissing the sheet. v2 ships this as a no-op + toast.
  final VoidCallback onSave;

  const NeedsArchivedSheet({
    super.key,
    required this.scopeKind,
    required this.entries,
    required this.onSave,
    this.endedAgo,
  });

  static Future<void> show(
    BuildContext context, {
    required NeedsScopeKind scopeKind,
    required List<NeedsVolunteerEntry> entries,
    required VoidCallback onSave,
    String? endedAgo,
  }) {
    return showAccessibleModal<void>(
      context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      barrierColor: AppColors.modalBackdrop,
      builder: (_) => NeedsArchivedSheet(
        scopeKind: scopeKind,
        entries: entries,
        endedAgo: endedAgo,
        onSave: onSave,
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = context.l10n;
    final copy = NeedsCopy(l10n, scopeKind);
    final accent = AppColors.experienceSageGreen;

    // Uncovered first (same sort as the Volunteer sheet).
    final sorted = [...entries]..sort((a, b) {
        if (a.isFullyCovered == b.isFullyCovered) return 0;
        return a.isFullyCovered ? 1 : -1;
      });

    var totalSlots = 0;
    var coveredSlots = 0;
    for (final e in entries) {
      if (e.isFreestanding) continue;
      totalSlots += e.slotsTotal;
      coveredSlots += e.slotsTotal - e.slotsRemaining;
    }

    final eyebrowText = endedAgo == null
        ? copy.archivedEyebrow
        : '${copy.archivedEyebrow} · $endedAgo';

    return GlassSheet(
      padding: EdgeInsets.zero,
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.of(context).size.height * 0.85,
          ),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 4, 20, 0),
                child: Text(
                  eyebrowText.toUpperCase(),
                  style: TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    letterSpacing: 1.6,
                    color: accent,
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 6, 20, 0),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Semantics(
                      header: true,
                      child: Text(
                        copy.archivedTitle,
                        style: const TextStyle(
                          fontSize: 28,
                          fontWeight: FontWeight.w800,
                          height: 1.1,
                          letterSpacing: -0.4,
                          color: AppColors.modalTextPrimary,
                        ),
                      ),
                    ),
                    if (totalSlots > 0) ...[
                      const SizedBox(height: 10),
                      NeedsClaimCounter(
                        scopeKind: scopeKind,
                        claimed: coveredSlots,
                        total: totalSlots,
                      ),
                    ],
                  ],
                ),
              ),
              Flexible(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.fromLTRB(20, 14, 20, 12),
                  child: entries.isEmpty
                      ? Padding(
                          padding: const EdgeInsets.symmetric(vertical: 32),
                          child: Text(
                            copy.volunteerEmpty,
                            textAlign: TextAlign.center,
                            style: const TextStyle(
                              fontSize: 13,
                              color: AppColors.modalTextMuted,
                            ),
                          ),
                        )
                      : Column(
                          crossAxisAlignment: CrossAxisAlignment.stretch,
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            for (final e in sorted)
                              _buildReadOnlyRow(context, e),
                          ],
                        ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 6, 20, 24),
                child: _ArchivedSaveButton(
                  label: copy.archivedSave,
                  onTap: onSave,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildReadOnlyRow(BuildContext context, NeedsVolunteerEntry e) {
    final l10n = context.l10n;
    final label = e.isFullyCovered
        ? l10n.a11yNeedsClaimRowCovered(e.name)
        : l10n.a11yNeedsClaimRow(e.name, e.slotsRemaining);
    final claimed = e.slotsTotal - e.slotsRemaining;
    return NeedsClaimRow(
      scopeKind: scopeKind,
      title: e.name,
      note: e.note,
      addedByName: e.addedByName,
      needed: e.slotsTotal,
      claimed: claimed,
      claimedByMe: false,
      semanticsLabel: label,
      contributors: e.contributors,
      readOnly: true,
      // Uncovered rows sink to 0.55 per the v2 mock.
      opacity: e.isFullyCovered ? 1.0 : 0.55,
    );
  }
}

/// Single-CTA save button at the bottom of the Archived sheet.
class _ArchivedSaveButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;

  const _ArchivedSaveButton({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final accent = AppColors.experienceSageGreen;
    return Tappable(
      semanticsLabel: label,
      onTap: onTap,
      child: Container(
        height: 52,
        decoration: BoxDecoration(
          color: accent,
          borderRadius: BorderRadius.circular(16),
          boxShadow: [
            BoxShadow(
              color: accent.withValues(alpha: 0.30),
              offset: const Offset(0, 8),
              blurRadius: 20,
            ),
          ],
        ),
        alignment: Alignment.center,
        child: Text(
          label,
          style: const TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w800,
            color: Color(0xFF0F1A14),
          ),
        ),
      ),
    );
  }
}

/// Helper exposed to [NeedsActions]: surfaces the standard "Saved to
/// your records." toast after the Save CTA fires. Centralised here so
/// the copy stays consistent with the sheet.
void showNeedsArchivedSavedToast(BuildContext context) {
  ToastHelper.showSuccess(context, context.l10n.needsArchivedSavedToast);
}
